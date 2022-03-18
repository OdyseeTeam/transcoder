package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/c2h5oh/datasize"
	"github.com/lbryio/transcoder/library/db"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/resolve"
)

const (
	SchemeRemote = "remote"
)

var ErrStreamNotFound = errors.New("stream not found")
var storageURLs = map[string]string{
	"wasabi": "https://s3.wasabisys.com/t-na",
	"legacy": "https://cache.transcoder.odysee.com/t-na",
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

func (lib *Library) GetStorageURL(name string) (string, error) {
	url, ok := storageURLs[name]
	if !ok {
		return "", fmt.Errorf("no url configured for storage %v", name)
	}
	return url, nil
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
	lib.db.RecordVideoAccess(context.Background(), v.SDHash)
	url = fmt.Sprintf("%s://%s/%s/", SchemeRemote, v.Storage, v.Path)
	return url, nil
}

func (lib *Library) AddRemoteStream(stream Stream) error {
	if stream.Manifest == nil {
		return errors.New("cannot add remote stream, manifest is missing")
	}
	m := stream.Manifest
	p := db.AddVideoParams{
		TID:      m.TID,
		URL:      m.URL,
		SDHash:   m.SDHash,
		Channel:  m.ChannelURL,
		Storage:  stream.RemoteStorage,
		Path:     m.TID,
		Size:     stream.Size(),
		Checksum: sql.NullString{String: stream.Checksum(), Valid: true},
	}
	_, err := lib.db.AddVideo(context.Background(), p)
	return err
}

func (lib *Library) AddChannel(uri string, priority db.ChannelPriority) (db.Channel, error) {
	var c db.Channel
	claim, err := resolve.Resolve(uri)
	if err != nil {
		return c, err
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

func StringToSize(s string) uint64 {
	var size datasize.ByteSize
	err := size.UnmarshalText([]byte(s))
	if err != nil {
		logger.Warn(err)
	}
	return size.Bytes()
}
