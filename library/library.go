package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/OdyseeTeam/transcoder/library/db"
	"github.com/OdyseeTeam/transcoder/pkg/logging"
	"github.com/OdyseeTeam/transcoder/pkg/resolve"

	"github.com/c2h5oh/datasize"
	"github.com/panjf2000/ants/v2"
	"github.com/pkg/errors"
	"github.com/tabbed/pqtype"
)

const (
	SchemeRemote = "remote"
)

var ErrStreamNotFound = errors.New("stream not found")

type Storage interface {
	Name() string
	GetURL(item string) string
	Put(stream *Stream, _ bool) error
	Delete(tid string) error
	DeleteFragments(tid string, fragments []string) error
	// GetFragment(sdHash, name string) (storage.StreamFragment, error)
}

type Library struct {
	db       *db.Queries
	storages map[string]Storage
	log      logging.KVLogger
}

type Config struct {
	Storages map[string]Storage
	DB       db.DBTX
	Log      logging.KVLogger
}

func New(config Config) *Library {
	return &Library{
		db:       db.New(config.DB),
		log:      config.Log,
		storages: config.Storages,
	}
}

func (lib *Library) GetVideo(sdHash string) (db.Video, error) {
	return lib.db.GetVideo(context.Background(), sdHash)
}

func (lib *Library) GetVideoURL(sdHash string) (string, error) {
	var url string
	v, err := lib.db.GetVideo(context.Background(), sdHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrStreamNotFound
		}
		return "", err
	}
	err = lib.db.RecordVideoAccess(context.Background(), v.SDHash)
	if err != nil {
		return "", err
	}
	url = fmt.Sprintf("%s://%s/%s/", SchemeRemote, v.Storage, v.Path)
	return url, nil
}

func (lib *Library) AddRemoteStream(stream Stream) error {
	if stream.Manifest == nil {
		return errors.New("cannot add remote stream, manifest is missing")
	}
	m := stream.Manifest
	bm, err := json.Marshal(m)
	if err != nil {
		return errors.Wrap(err, "cannot marshal stream manifest")
	}
	p := db.AddVideoParams{
		TID:      m.TID,
		URL:      m.URL,
		SDHash:   m.SDHash,
		Channel:  m.ChannelURL,
		Storage:  stream.RemoteStorage,
		Path:     m.TID,
		Size:     stream.Size(),
		Checksum: sql.NullString{String: stream.Checksum(), Valid: true},
		Manifest: pqtype.NullRawMessage{RawMessage: bm, Valid: true},
	}
	_, err = lib.db.AddVideo(context.Background(), p)
	return err
}

func (lib *Library) AddChannel(uri string, priority db.ChannelPriority) (db.Channel, error) {
	var c db.Channel
	claim, err := resolve.Resolve(uri)
	if err != nil {
		return c, err
	}
	if claim.ClaimID == "" {
		return c, errors.New("channel not found")
	}

	if priority == "" {
		priority = db.ChannelPriorityNormal
	}
	return lib.db.AddChannel(context.Background(), db.AddChannelParams{
		URL:      claim.CanonicalURL,
		ClaimID:  claim.ClaimID,
		Priority: priority,
	})
}

func (lib *Library) GetAllChannels() ([]db.Channel, error) {
	return lib.db.GetAllChannels(context.Background())
}

// RetireVideos deletes older videos from S3, keeping total size of remote videos at maxSize.
func (lib *Library) RetireVideos(storageName string, maxSize uint64) (uint64, uint64, error) {
	items, err := lib.db.GetAllVideosForStorage(context.Background(), storageName)
	if err != nil {
		return 0, 0, err
	}
	return tailVideos(items, maxSize, lib.Retire)
}

func (lib *Library) Retire(video db.Video) error {
	var deleted bool

	ll := lib.log.With("tid", video.TID, "sd_hash", video.SDHash, "storage", video.Storage)

	storage, ok := lib.storages[video.Storage]
	if !ok {
		return fmt.Errorf("storage %s not found", video.Storage)
	}

	if video.Manifest.Valid {
		manifest := &Manifest{}
		err := json.Unmarshal(video.Manifest.RawMessage, manifest)
		if err != nil { // nolint:gocritic
			ll.Warn("failed to parse video manifest", "err", err)
		} else if len(manifest.Files) == 0 {
			ll.Warn("empty video manifest", "err", err)
		} else {
			err = storage.DeleteFragments(video.TID, manifest.Files)
			if err != nil {
				ll.Warn("failed to delete fragments for remote video", "err", err)
			} else {
				deleted = true
			}
		}
	}
	if !deleted {
		err := storage.Delete(video.TID)
		if err != nil {
			ll.Warn("failed to delete remote video", "err", err)
			return err
		}
	}

	err := lib.db.DeleteVideo(context.Background(), video.TID)
	if err != nil {
		ll.Warn("failed to delete video record", "err", err)
		return err
	}

	ll.Info("video retired", "url", video.URL, "size", video.Size, "age", video.CreatedAt, "accessed", video.AccessedAt)
	return nil
}

// ValidateStreams removes broken streams from the database.
func (lib *Library) ValidateStreams(storageName string, offset, limit int32, remove bool) ([]string, []string, error) {
	broken := []string{}
	valid := []string{}
	tids := map[string]string{}
	items, err := lib.db.GetAllVideosForStorageLimit(
		context.Background(),
		db.GetAllVideosForStorageLimitParams{
			Storage: storageName,
			Offset:  offset,
			Limit:   limit,
		},
	)
	if err != nil {
		return nil, nil, err
	}
	wg := sync.WaitGroup{}
	results := make(chan *ValidationResult)

	go func() {
		for vr := range results {
			if len(vr.Missing) > 0 {
				broken = append(broken, vr.URL)
				lib.log.Info("broken stream", "url", vr.URL, "missing", vr.Missing)
			} else {
				valid = append(valid, vr.URL)
			}
		}
	}()
	pipe := func(i interface{}) error {
		url := i.(string)
		vr, _ := ValidateStream(url, true, true)
		results <- vr
		return nil
	}

	p, _ := ants.NewPoolWithFunc(10, func(i interface{}) {
		err := pipe(i)
		if err != nil {
			lib.log.Error("error submitting a url", "err", err)
		}
		wg.Done()
	})
	defer p.Release()

	for _, v := range items {
		url := lib.storages[v.Storage].GetURL(v.Path)
		tids[url] = v.TID
		wg.Add(1)
		_ = p.Invoke(url)
	}
	wg.Wait()

	if remove {
		for _, u := range broken {
			err := lib.db.DeleteVideo(context.Background(), tids[u])
			if err != nil {
				lib.log.Info("video removal failed", "tid", tids[u], "err", err)
			} else {
				lib.log.Info("video removed", "tid", tids[u], "url", u)
			}
		}
	}
	return valid, broken, nil
}

func StringToSize(s string) uint64 {
	var size datasize.ByteSize
	err := size.UnmarshalText([]byte(s))
	if err != nil {
		logger.Warn(err)
	}
	return size.Bytes()
}
