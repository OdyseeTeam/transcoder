package manager

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"

	"github.com/lbryio/transcoder/internal/metrics"
	"github.com/lbryio/transcoder/pkg/timer"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

var httpVideoPath = "/streams"

// HttpAPI ties HTTP API together and allows to start/shutdown the web server.
type HttpAPI struct {
	*HttpAPIConfiguration
	logger     *zap.SugaredLogger
	httpServer *fasthttp.Server
}

type HttpAPIConfiguration struct {
	debug     bool
	videoPath string
	addr      string
	mgr       *VideoManager
}

func ConfigureHttpAPI() *HttpAPIConfiguration {
	return &HttpAPIConfiguration{
		addr:      ":8080",
		videoPath: path.Join(os.TempDir(), "transcoder"),
	}
}

func (c *HttpAPIConfiguration) Debug(debug bool) *HttpAPIConfiguration {
	c.debug = debug
	return c
}

func (c *HttpAPIConfiguration) Addr(addr string) *HttpAPIConfiguration {
	c.addr = addr
	return c
}

func (c *HttpAPIConfiguration) VideoPath(videoPath string) *HttpAPIConfiguration {
	c.videoPath = videoPath
	return c
}

func (c *HttpAPIConfiguration) VideoManager(mgr *VideoManager) *HttpAPIConfiguration {
	c.mgr = mgr
	return c
}

func NewHttpAPI(cfg *HttpAPIConfiguration) *HttpAPI {
	r := router.New()

	s := &HttpAPI{
		HttpAPIConfiguration: cfg,
		httpServer: &fasthttp.Server{
			Handler: metricsMiddleware(corsMiddleware(r.Handler)),
		},
		logger: logger.Named("http"),
	}

	// r.GET("/api/v1/video/{kind:hls}/{url}/{sdHash:^[a-z0-9]{96}$}", h.handleVideo)
	r.GET("/api/v1/video/{kind:hls}/{url}", s.handleVideo)
	r.GET("/api/v2/video/{url}", s.handleVideo)
	r.ServeFiles(path.Join(httpVideoPath, "{filepath:*}"), s.videoPath)
	r.GET("/metrics", fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler()))

	if !s.debug {
		r.PanicHandler = handlePanic
	}
	return s
}

func (h *HttpAPI) handleVideo(ctx *fasthttp.RequestCtx) {
	urlQ := ctx.UserValue("url").(string)
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

	v, err := h.mgr.Video(url)

	if err != nil {
		var (
			statusCode    int
			statusMessage string
		)
		switch err {
		case ErrTranscodingForbidden:
			statusCode = http.StatusForbidden
		case ErrChannelNotEnabled:
			statusCode = http.StatusForbidden
		case ErrNoSigningChannel:
			statusCode = http.StatusForbidden
		case ErrTranscodingQueued:
			statusCode = http.StatusAccepted
		case ErrTranscodingUnderway:
			statusCode = http.StatusAccepted
		case ErrStreamNotFound:
			statusCode = http.StatusNotFound
		default:
			statusCode = http.StatusInternalServerError
			ll.Errorw("internal error", "err", err)
		}

		ll.Debug(err.Error())
		ctx.SetStatusCode(statusCode)
		if statusMessage == "" {
			statusMessage = err.Error()
		}
		ctx.SetBodyString(statusMessage)
		return
	}

	if err == ErrChannelNotEnabled || err == ErrNoSigningChannel {
		ctx.SetStatusCode(http.StatusForbidden)
		ll.Debug("transcoding disabled")
		return
	} else if err == ErrTranscodingQueued {
		ctx.SetStatusCode(http.StatusAccepted)
		ll.Debug("trancoding queued")
		return
	} else if err == ErrTranscodingForbidden {
		ctx.SetStatusCode(http.StatusForbidden)
		ctx.Response.SetBodyString(err.Error())
		ll.Debug(err.Error())
		return
	} else if err == ErrTranscodingUnderway {
		ctx.SetStatusCode(http.StatusAccepted)
		ll.Debug("trancoding underway")
		return
	} else if err == ErrStreamNotFound {
		ctx.SetStatusCode(http.StatusNotFound)
		ll.Debug("stream not found")
		return
	} else if err != nil {
		ctx.SetStatusCode(http.StatusInternalServerError)
		ll.Errorw("internal error", "error", err)
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
	ll.Infow("stream found", "location", location)
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

func (s HttpAPI) Addr() string {
	return s.addr
}

func (s HttpAPI) URL() string {
	return "http://" + s.addr
}

func (s HttpAPI) Start() error {
	s.logger.Infow("listening", "bind", s.addr, "video_path", s.videoPath, "debug", s.debug)
	return s.httpServer.ListenAndServe(s.addr)
}

func (s HttpAPI) Shutdown() error {
	s.logger.Info("shutting down...")
	return s.httpServer.Shutdown()
}
