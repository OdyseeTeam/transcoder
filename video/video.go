package video

import (
	"context"
	"errors"
	"time"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/storage"
)

type RemoteDriver interface {
	Put(stream *storage.LocalStream) (*storage.RemoteStream, error)
	Delete(sdHash string) error
	GetFragment(sdHash, name string) (storage.StreamFragment, error)
}

type LocalDriver interface {
	Delete(sdHash string) error
	Path() string
}

type Config struct {
	db     *db.DB
	local  LocalDriver
	remote RemoteDriver

	maxLocalSize  uint64
	maxRemoteSize uint64
}

func Configure() *Config {
	return &Config{}
}

// DB is SQL DB instance which is used for storing videos.
func (c *Config) DB(db *db.DB) *Config {
	c.db = db
	return c
}

// LocalStorage is a local storage driver for accessing videos on disk.
func (c *Config) LocalStorage(s LocalDriver) *Config {
	c.local = s
	return c
}

// LocalStorage is a remote (S3) storage driver for accessing remote videos.
func (c *Config) RemoteStorage(s RemoteDriver) *Config {
	c.remote = s
	return c
}

func (c *Config) MaxLocalSize(s string) *Config {
	c.maxLocalSize = StringToSize(s)
	return c
}

func (c *Config) MaxRemoteSize(s string) *Config {
	c.maxRemoteSize = StringToSize(s)
	return c
}

// Library contains methods for accessing videos database.
type Library struct {
	*Config
	queries Queries
}

func NewLibrary(cfg *Config) *Library {
	l := &Library{
		Config:  cfg,
		queries: Queries{cfg.db},
	}
	return l
}

// Add records data about video into database.
func (q Library) Add(params AddParams) (*Video, error) {
	return q.queries.Add(context.Background(), params)
}

// AddLocalStream moves the stream folder resiging elsewhere into videos folder.
// and saves it into database.
func (q Library) AddLocalStream(url, channel string, ls storage.LocalStream) (*Video, error) {
	if err := ls.Move(q.local.Path()); err != nil {
		return nil, err
	}

	p := AddParams{
		URL:      url,
		SDHash:   ls.SDHash(),
		Type:     formats.TypeHLS,
		Channel:  channel,
		Path:     ls.BasePath(),
		Size:     ls.Size(),
		Checksum: ls.Checksum(),
	}
	return q.queries.Add(context.Background(), p)
}

// AddRemoteStream writes remote stream into database.
// and saves it into database.
func (q Library) AddRemoteStream(rs storage.RemoteStream) (*Video, error) {
	if rs.Manifest == nil {
		return nil, errors.New("cannot add remote stream, manifest is missing")
	}
	m := rs.Manifest
	p := AddParams{
		URL:        m.URL,
		SDHash:     m.SDHash,
		Channel:    m.ChannelURL,
		RemotePath: rs.URL,
		Size:       rs.Size(),
		Checksum:   rs.Checksum(),
		Type:       formats.TypeHLS,
	}
	return q.queries.Add(context.Background(), p)
}

func (q Library) Get(sdHash string) (*Video, error) {
	return q.queries.Get(context.Background(), sdHash)
}

func (q Library) Path() string {
	return q.local.Path()
}

func (q Library) Furlough(v *Video) error {
	ll := logger.With("sd_hash", v.SDHash)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := q.local.Delete(v.SDHash)
	if err != nil {
		ll.Warnw("failed to delete local video", "err", err)
		return err
	}

	err = q.queries.UpdatePath(ctx, v.SDHash, "")
	if err != nil {
		ll.Warnw("failed to mark video as deleted locally", "err", err)
		return err
	}

	ll.Infow("video furloughed", "url", v.URL, "size", v.GetSize(), "age", v.CreatedAt, "last_accessed", v.LastAccessed)
	return nil
}

func (q Library) Retire(v *Video) error {
	ll := logger.With("sd_hash", v.SDHash)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := q.remote.Delete(v.SDHash)
	if err != nil {
		ll.Warnw("failed to delete remote video", "err", err)
		return err
	}

	err = q.queries.Delete(ctx, v.SDHash)
	if err != nil {
		ll.Warnw("failed to delete video record", "err", err)
		return err
	}

	ll.Infow("video retired", "url", v.URL, "size", v.GetSize(), "age", v.CreatedAt, "last_accessed", v.LastAccessed)
	return nil
}

func (q Library) ListLocalOnly() ([]*Video, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return q.queries.ListLocalOnly(ctx)
}

func (q Library) ListAll() ([]*Video, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return q.queries.ListAll(ctx)
}

func (q Library) ListLocal() ([]*Video, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return q.queries.ListLocal(ctx)
}

func (q Library) ListRemoteOnly() ([]*Video, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return q.queries.ListRemoteOnly(ctx)
}

func (q Library) UpdateRemotePath(sdHash, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return q.queries.UpdateRemotePath(ctx, sdHash, url)
}
