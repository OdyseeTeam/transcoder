package manager

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/lbryio/transcoder/pkg/mfr"
	"github.com/lbryio/transcoder/video"

	"github.com/karlseguin/ccache/v2"
)

const (
	videoPlaylistPath      = "."
	uriPrefix              = "lbry://"
	level5SupportThreshold = 1000
)

var (
	enabledChannels = []string{}
	cacheSize       = int64(math.Pow(1024, 4))
)

func LoadEnabledChannels(channels []string) {
	enabledChannels = apply(channels, func(e string) string {
		return uriPrefix + strings.Replace(strings.ToLower(e), "#", ":", 1)
	})
	logger.Infof("%v channels enabled", len(enabledChannels))
}

type VideoManager struct {
	library video.VideoLibrary
	pool    *Pool
	cache   *ccache.Cache
}

// NewManager creates a video library manager with a pool for future transcoding requests.
func NewManager(l video.VideoLibrary, minHits int) *VideoManager {
	m := &VideoManager{
		library: l,
		pool:    NewPool(),
		cache: ccache.New(ccache.
			Configure().
			MaxSize(cacheSize)),
	}

	// Check if channel should be added to enabled queue
	m.pool.AddQueue("enabled", 0, func(key string, value interface{}, queue *mfr.Queue) bool {
		r := value.(*TranscodingRequest)
		for _, e := range enabledChannels {
			if e == r.ChannelURI {
				logger.Debugw("accepted for 'enabled' queue", "uri", r.URI)
				r.queue = queue
				queue.Hit(key, r)
				return true
			}
		}
		return false
	})

	m.pool.AddQueue("level5", 0, func(key string, value interface{}, queue *mfr.Queue) bool {
		r := value.(*TranscodingRequest)
		if r.ChannelSupportAmount >= level5SupportThreshold {
			logger.Debugw("accepted for 'level5' queue", "uri", r.URI, "support_amount", r.ChannelSupportAmount)
			r.queue = queue
			queue.Hit(key, r)
			return true
		}
		return false
	})

	m.pool.AddQueue("common", uint(minHits), func(key string, value interface{}, queue *mfr.Queue) bool {
		r := value.(*TranscodingRequest)
		r.queue = queue
		queue.Hit(key, r)
		return true
	})

	go m.pool.Start()

	return m
}

func (m *VideoManager) Pool() *Pool {
	return m.pool
}

func (m *VideoManager) RequestStatus(uri string) int {
	for _, l := range m.pool.levels {
		if _, status := l.queue.Get(uri); status != mfr.StatusNone {
			return status
		}
	}
	return mfr.StatusNone
}

func (m *VideoManager) Library() video.VideoLibrary {
	return m.library
}

// Video checks if video exists in the library or waiting in one of the queues.
// If neither, it adds claim to the pool for later processing.
func (m *VideoManager) Video(uri string) (*video.Video, error) {
	tr, err := m.resolveRequest(uri)
	if err != nil {
		return nil, err
	}

	v, err := m.getVideo(tr.SDHash)
	if v == nil || err == sql.ErrNoRows {
		return nil, m.pool.Admit(uri, tr)
	}

	return v, nil
}

// Requests returns next transcoding request to be processed.
func (m *VideoManager) Requests() <-chan *TranscodingRequest {
	out := make(chan *TranscodingRequest)
	go func() {
		for next := range m.pool.Out() {
			if next == nil {
				continue
			}

			r := next.Value.(*TranscodingRequest)
			logger.Infow("pulling out next request", "uri", r.URI, "hits", next.Hits())
			out <- r
		}
	}()
	return out
}

// getVideo helps to avoid hitting video SQLite database too hard.
func (m *VideoManager) getVideo(h string) (*video.Video, error) {
	var v *video.Video
	item, err := m.cache.Fetch(fmt.Sprintf("video:%v", h), 30*time.Second, func() (interface{}, error) {
		return m.library.Get(h)
	})
	if item != nil {
		v = item.Value().(*video.Video)
	}
	return v, err
}

func (m *VideoManager) resolveRequest(uri string) (*TranscodingRequest, error) {
	item, err := m.cache.Fetch(fmt.Sprintf("claim:%v", uri), 120*time.Second, func() (interface{}, error) {
		return ResolveRequest(uri)
	})
	if err != nil {
		return nil, err
	}
	return item.Value().(*TranscodingRequest), nil
}

func apply(s []string, f func(e string) string) []string {
	r := []string{}
	for _, e := range s {
		r = append(r, f(e))
	}
	return r
}
