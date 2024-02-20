package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/odyseeteam/transcoder/library/db"
	"github.com/odyseeteam/transcoder/pkg/logging"
	"github.com/odyseeteam/transcoder/pkg/resolve"

	"github.com/c2h5oh/datasize"
	"github.com/panjf2000/ants/v2"
	"github.com/pkg/errors"
	"github.com/tabbed/pqtype"
)

const (
	SchemeRemote = "remote"
)

var ErrStreamNotFound = errors.New("stream not found")
var storageURLs = map[string]string{
	"wasabi": "https://s3.wasabisys.com/t-na2.odycdn.com",
	"legacy": "https://na-storage-1.transcoder.odysee.com/t-na",
}

type Storage interface {
	Name() string
	GetURL(tid string) string
	Put(stream *Stream, _ bool) error
	Delete(streamTID string) error
	// GetFragment(sdHash, name string) (storage.StreamFragment, error)
}

type Library struct {
	db      *db.Queries
	storage Storage
	log     logging.KVLogger
}

type Config struct {
	Storage Storage
	DB      db.DBTX
	Log     logging.KVLogger
}

func New(config Config) *Library {
	return &Library{
		db:      db.New(config.DB),
		log:     config.Log,
		storage: config.Storage,
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

func (lib *Library) Retire(v db.Video) error {
	ll := lib.log.With("tid", v.TID, "sd_hash", v.SDHash)

	err := lib.storage.Delete(v.TID)
	if err != nil {
		ll.Warn("failed to delete remote video", "err", err)
		return err
	}

	err = lib.db.DeleteVideo(context.Background(), v.TID)
	if err != nil {
		ll.Warn("failed to delete video record", "err", err)
		return err
	}

	ll.Info("video retired", "url", v.URL, "size", v.Size, "age", v.CreatedAt, "accessed", v.AccessedAt)
	return nil
}

// RetireVideos deletes older videos from S3, keeping total size of remote videos at maxSize.
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
		url := fmt.Sprintf("%s/%s", storageURLs[v.Storage], v.Path)
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
