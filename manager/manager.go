package manager

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/OdyseeTeam/transcoder/library"
	db "github.com/OdyseeTeam/transcoder/library/db"
	"github.com/OdyseeTeam/transcoder/pkg/conductor/metrics"
	"github.com/OdyseeTeam/transcoder/pkg/logging/zapadapter"
	"github.com/OdyseeTeam/transcoder/pkg/mfr"
	"github.com/OdyseeTeam/transcoder/pkg/resolve"

	"github.com/fasthttp/router"
	"github.com/karlseguin/ccache/v2"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/pprofhandler"
)

const (
	videoPlaylistPath      = "."
	channelURIPrefix       = "lbry://"
	level5SupportThreshold = 1000
)

var (
	cacheSize = int64(math.Pow(1024, 4))
)

type TranscodingRequest struct {
	resolve.ResolvedStream
	queue *mfr.Queue
}

type HttpServerConfig struct {
	ManagerToken string
	Bind         string
}

type VideoLibrary interface {
	GetURL(sdHash string) (string, error)
	GetAllChannels() ([]db.Channel, error)
	// Add(params video.AddParams) (*video.Video, error)
	// AddLocalStream(url, channel string, ls storage.LocalStream) (*video.Video, error)
	// AddRemoteStream(storage.RemoteStream) (*video.Video, error)
}

type VideoManager struct {
	lib      *library.Library
	pool     *Pool
	cache    *ccache.Cache
	channels *channelList
}

// NewManager creates a video library manager with a pool for future transcoding requests.
func NewManager(lib *library.Library, minHits uint) *VideoManager {
	m := &VideoManager{
		lib:      lib,
		pool:     NewPool(),
		channels: newChannelList(),
		cache: ccache.New(ccache.
			Configure().
			MaxSize(cacheSize)),
	}

	channels, err := lib.GetAllChannels()
	if err != nil {
		logger.Error("error loading channels", "err", err)
	}
	m.channels.Load(channels)
	logger.Infow("loaded channels", "count", len(channels))
	go m.channels.StartLoadingChannels(lib)

	m.pool.AddQueue("priority", 0, func(key string, value interface{}, queue *mfr.Queue) bool {
		r := value.(*TranscodingRequest)
		if m.channels.GetPriority(r) != db.ChannelPriorityHigh {
			return false
		}
		logger.Infow("accepted for 'priority' queue", "uri", r.URI)
		r.queue = queue
		queue.Hit(key, r)
		return true
	})

	m.pool.AddQueue("enabled", 0, func(key string, value interface{}, queue *mfr.Queue) bool {
		r := value.(*TranscodingRequest)
		if m.channels.GetPriority(r) != db.ChannelPriorityNormal {
			return false
		}
		logger.Infow("accepted for 'priority' queue", "uri", r.URI)
		r.queue = queue
		queue.Hit(key, r)
		return true
	})

	m.pool.AddQueue("level5", 0, func(key string, value interface{}, queue *mfr.Queue) bool {
		r := value.(*TranscodingRequest)
		s := r.ChannelSupportAmount
		if level5SupportThreshold > s {
			return false
		}
		r.ChannelSupportAmount = 0
		logger.Debugw("accepted for 'level5' queue", "uri", r.URI, "support_amount", r.ChannelSupportAmount)
		r.queue = queue
		queue.Hit(key, r)
		return true
	})

	m.pool.AddQueue("common", uint(minHits), func(key string, value interface{}, queue *mfr.Queue) bool {
		r := value.(*TranscodingRequest)
		if m.channels.GetPriority(r) == db.ChannelPriorityDisabled {
			return false
		}
		logger.Debugw("accepted for 'common' queue", "uri", r.URI, "support_amount", r.ChannelSupportAmount)
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

func (m *VideoManager) RequestStatus(sdHash string) int {
	for _, l := range m.pool.levels {
		if _, status := l.queue.Get(sdHash); status != mfr.StatusNone {
			return status
		}
	}
	return mfr.StatusNone
}

func (m *VideoManager) Library() *library.Library {
	return m.lib
}

// Video checks if video exists in the library or waiting in one of the queues.
// If neither, it adds claim to the pool for later processing.
func (m *VideoManager) Video(uri string) (string, error) {
	uri = strings.TrimPrefix(uri, "lbry://")
	tr, err := m.ResolveStream(uri)
	if err != nil {
		return "", err
	}

	if m.channels.GetPriority(tr) == db.ChannelPriorityDisabled {
		return "", resolve.ErrTranscodingForbidden
	}

	vloc, err := m.lib.GetVideoURL(tr.SDHash)
	if err != nil {
		return "", m.pool.Admit(tr.SDHash, tr)
	}

	return vloc, nil
}

// Requests returns next transcoding request to be processed. It polls all queues in the pool evenly.
func (m *VideoManager) Requests() <-chan *TranscodingRequest {
	out := make(chan *TranscodingRequest)
	go func() {
		for next := range m.pool.Out() {
			if next == nil {
				continue
			}

			r := next.Value.(*TranscodingRequest)
			logger.Infow("task dispatcher: sending next request", "uri", r.URI, "hits", next.Hits())
			out <- r
			logger.Infow("task dispatcher: next request sent", "uri", r.URI, "hits", next.Hits())
		}
	}()
	return out
}

func (m *VideoManager) StartHttpServer(config HttpServerConfig) (chan struct{}, error) {
	stopChan := make(chan struct{})
	router := router.New()

	metrics.RegisterConductorMetrics()

	CreateRoutes(router, m, zapadapter.NewKV(logger.Desugar()), func(ctx *fasthttp.RequestCtx) bool {
		return ctx.UserValue(TokenCtxField).(string) == config.ManagerToken
	})

	router.GET("/debug/pprof/{profile:*}", pprofhandler.PprofHandler)

	logger.Infow("starting tower http server", "addr", config.Bind)
	server := &fasthttp.Server{
		Handler:          MetricsMiddleware(CORSMiddleware(router.Handler)),
		Name:             "tower",
		DisableKeepalive: true,
	}
	// s.upAddr = l.Addr().String()

	go func() {
		err := server.ListenAndServe(config.Bind)
		if err != nil {
			logger.Error("http server error", "err", err)
		}
	}()
	go func() {
		<-stopChan
		logger.Infow("shutting down tower http server", "addr", config.Bind)
		server.Shutdown()
	}()

	return stopChan, nil
}

func (m *VideoManager) ResolveStream(uri string) (*TranscodingRequest, error) {
	item, err := m.cache.Fetch(fmt.Sprintf("claim:%v", uri), 300*time.Second, func() (interface{}, error) {
		return resolve.ResolveStream(uri)
	})
	if err != nil {
		return nil, err
	}
	rs := item.Value().(*resolve.ResolvedStream)
	return &TranscodingRequest{ResolvedStream: *rs}, nil
}

func (r *TranscodingRequest) Release() {
	if r.queue == nil {
		return
	}
	logger.Infow("transcoding request released", "lbry_url", r.URI)
	r.queue.Release(r.URI)
}

func (r *TranscodingRequest) Reject() {
	if r.queue == nil {
		return
	}
	logger.Infow("transcoding request rejected", "lbry_url", r.URI)
	r.queue.Done(r.URI)
}

func (r *TranscodingRequest) Complete() {
	if r.queue == nil {
		return
	}
	logger.Infow("transcoding request completed", "lbry_url", r.URI)
	r.queue.Done(r.URI)
}
