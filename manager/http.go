package manager

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"

	"github.com/lbryio/transcoder/internal/metrics"
	"github.com/lbryio/transcoder/pkg/dispatcher"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/pkg/timer"

	"github.com/fasthttp/router"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"go.uber.org/zap"
)

var httpVideoPath = "/streams"

// HttpAPI ties HTTP API together and allows to start/shutdown the web server.
type HttpAPI struct {
	*HttpAPIConfiguration
	logger     *zap.SugaredLogger
	httpServer *fasthttp.Server
}

type httpVideoHandler struct {
	manager *VideoManager
	log     logging.KVLogger
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

func NewHttpAPI(cfg *HttpAPIConfiguration) *HttpAPI {
	r := router.New()

	AttachVideoHandler(r, "", cfg.videoPath, cfg.mgr, zapadapter.NewKV(logger.Named("http").Desugar()))

	s := &HttpAPI{
		HttpAPIConfiguration: cfg,
		httpServer: &fasthttp.Server{
			Handler: MetricsMiddleware(CORSMiddleware(r.Handler)),
		},
		logger: logger.Named("http"),
	}

	return s
}

// NewRouter creates a set of HTTP entrypoints that will route requests into video library
// and video transcoding queue.
func AttachVideoHandler(r *router.Router, prefix, videoPath string, manager *VideoManager, log logging.KVLogger) {
	h := httpVideoHandler{
		log:     log,
		manager: manager,
	}
	g := r.Group(prefix)

	// r.GET("/api/v1/video/{kind:hls}/{url}/{sdHash:[a-z0-9]{96}}", h.handleVideo)
	g.GET("/api/v1/video/{kind:hls}/{url}", h.handleVideo)
	g.GET("/api/v2/video/{url}", h.handleVideo)
	g.GET("/api/v3/video", h.handleVideo) // accepts URL as a query param
	g.GET(httpVideoPath+"/{filepath:*}", func(ctx *fasthttp.RequestCtx) {
		p, _ := ctx.UserValue("filepath").(string)
		ctx.Redirect("remote://"+p, http.StatusSeeOther)
	})

	metrics.RegisterMetrics()
	dispatcher.RegisterMetrics()
	RegisterMetrics()
	g.GET("/metrics", fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler()))
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

func (h httpVideoHandler) handleVideo(ctx *fasthttp.RequestCtx) {
	var path, videoURL string
	var err error
	urlQ, _ := ctx.UserValue("url").(string)

	if urlQ != "" {
		path = string(ctx.Path())

		videoURL, err = url.PathUnescape(urlQ)
		if err != nil {
			logger.Errorw("url parsing error", "url", urlQ, "error", err)
			ctx.SetStatusCode(http.StatusBadRequest)
			fmt.Fprint(ctx, err.Error())
			return
		}
	} else {
		videoURL = string(ctx.FormValue("url"))
	}

	if videoURL == "" {
		logger.Info("no url supplied")
		ctx.SetStatusCode(http.StatusBadRequest)
		fmt.Fprint(ctx, "no url supplied")
		return
	}

	ll := logger.With(
		"url", videoURL,
		"path", path,
	)

	v, err := h.manager.Video(videoURL)

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

	if err == ErrNoSigningChannel || err == ErrChannelNotEnabled {
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

func CORSMiddleware(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
		h(ctx)
	}
}

func MetricsMiddleware(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		t := timer.Start()
		h(ctx)
		metrics.HTTPAPIRequests.WithLabelValues(fmt.Sprintf("%v", ctx.Response.StatusCode())).Observe(t.Duration())
	}
}
