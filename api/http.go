package api

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/lbryio/transcoder/internal/metrics"
	"github.com/lbryio/transcoder/pkg/claim"
	"github.com/lbryio/transcoder/pkg/timer"
	"github.com/lbryio/transcoder/video"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

var httpVideoPath = "/streams"

// APIServer ties HTTP API together and allows to start/shutdown the web server.
type APIServer struct {
	*Configuration
	httpServer *fasthttp.Server
	stopChan   chan os.Signal
	stopWait   time.Duration
}

type Configuration struct {
	debug        bool
	videoPath    string
	addr         string
	videoManager *VideoManager
}

func Configure() *Configuration {
	return &Configuration{
		addr:      ":8080",
		videoPath: path.Join(os.TempDir(), "transcoder"),
	}
}

func (c *Configuration) Debug(debug bool) *Configuration {
	c.debug = debug
	return c
}

func (c *Configuration) Addr(addr string) *Configuration {
	c.addr = addr
	return c
}

func (c *Configuration) VideoPath(videoPath string) *Configuration {
	c.videoPath = videoPath
	return c
}

func (c *Configuration) VideoManager(videoManager *VideoManager) *Configuration {
	c.videoManager = videoManager
	return c
}

func (h *APIServer) handleVideo(ctx *fasthttp.RequestCtx) {
	urlQ := ctx.UserValue("url").(string)
	kind := ctx.UserValue("kind").(string)
	path := string(ctx.Path())

	url, err := url.PathUnescape(urlQ)
	if err != nil {
		logger.Errorw("url parsing error", "url", urlQ, "error", err)
		ctx.SetStatusCode(http.StatusBadRequest)
		fmt.Fprint(ctx, err.Error())
		return
	}

	ll := logger.Named("http").With(
		"url", url,
		"path", path,
	)

	v, err := h.videoManager.GetVideoOrCreateTask(url, kind)

	if err == video.ErrChannelNotEnabled || err == video.ErrNoSigningChannel {
		ctx.SetStatusCode(http.StatusForbidden)
		ll.Infow("transcoding disabled")
		return
	} else if err == video.ErrTranscodingUnderway {
		ctx.SetStatusCode(http.StatusAccepted)
		ll.Infow("trancoding pending")
		return
	} else if err == claim.ErrStreamNotFound {
		ctx.SetStatusCode(http.StatusNotFound)
		ll.Infow("stream not found")
		return
	} else if err != nil {
		ctx.SetStatusCode(http.StatusInternalServerError)
		ll.Infow("internal error", "error", err)
		fmt.Fprint(ctx, err.Error())
		return
	}

	ctx.Response.StatusCode()
	location, remote := v.GetLocation()
	if !remote {
		metrics.StreamsRequestedCount.WithLabelValues(metrics.StorageLocal).Inc()
		location = fmt.Sprintf("%v/%v", httpVideoPath, location)
	} else {
		metrics.StreamsRequestedCount.WithLabelValues(metrics.StorageRemote).Inc()
	}
	ll.Infow("redirecting to video", "location", location)
	ctx.Redirect(location, http.StatusSeeOther)
}

func handlePanic(ctx *fasthttp.RequestCtx, p interface{}) {
	ctx.SetStatusCode(http.StatusInternalServerError)
	logger.Errorw("panicked", "url", ctx.Request.URI(), "panic", p)
}

func corsMiddleware(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
		h(ctx)
	}
}

func metricsMiddleware(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		t := timer.Start()
		h(ctx)
		metrics.HTTPAPIRequests.WithLabelValues(fmt.Sprintf("%v", ctx.Response.StatusCode())).Observe(t.Duration())
	}
}

func NewServer(cfg *Configuration) *APIServer {
	r := router.New()

	s := &APIServer{
		Configuration: cfg,
		httpServer: &fasthttp.Server{
			Handler: metricsMiddleware(corsMiddleware(r.Handler)),
		},
	}

	// r.GET("/api/v1/video/{kind:hls}/{url}/{sdHash:^[a-z0-9]{96}$}", h.handleVideo)
	r.GET("/api/v1/video/{kind:hls}/{url}", s.handleVideo)
	r.ServeFiles(path.Join(httpVideoPath, "{filepath:*}"), s.videoPath)
	r.GET("/metrics", fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler()))

	if !s.debug {
		r.PanicHandler = handlePanic
	}
	return s
}

func (s APIServer) Addr() string {
	return s.addr
}

func (s APIServer) URL() string {
	return "http://" + s.addr
}

func (s APIServer) Start() error {
	logger.Infow("listening", "bind", s.addr, "video_path", s.videoPath, "debug", s.debug)
	return s.httpServer.ListenAndServe(s.addr)
}

func (s APIServer) Shutdown() error {
	logger.Info("shutting down...")
	return s.httpServer.Shutdown()
}
