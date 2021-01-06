package api

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/pkg/claim"
	"github.com/lbryio/transcoder/video"

	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
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

	url, err := url.PathUnescape(urlQ)
	if err != nil {
		logger.Errorw("url parsing error", "url", urlQ, "error", err)
		ctx.SetStatusCode(http.StatusBadRequest)
		fmt.Fprint(ctx, err.Error())
		return
	}

	ll := logger.With("url", url)

	v, err := h.videoManager.GetVideoOrCreateTask(url, kind)

	if err == video.ErrChannelNotEnabled || err == video.ErrNoSigningChannel {
		ctx.SetStatusCode(http.StatusForbidden)
		ll.Debugw("forbidden")
		return
	} else if err == video.ErrTranscodingUnderway {
		ctx.SetStatusCode(http.StatusAccepted)
		ll.Debugw("trancoding underway")
		return
	} else if err == claim.ErrStreamNotFound {
		ctx.SetStatusCode(http.StatusNotFound)
		ll.Debugw("stream not found")
		return
	} else if err != nil {
		ctx.SetStatusCode(http.StatusInternalServerError)
		ll.Errorw("internal error", "error", err)
		fmt.Fprint(ctx, err.Error())
		return
	}

	path := fmt.Sprintf("%v/%v/%v", httpVideoPath, v.GetPath(), encoder.MasterPlaylist)
	ll.Debugw("found", "path", path)
	ctx.Redirect(path, http.StatusSeeOther)
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

func loggingMiddleware(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		logger.Debugw("http request", "path", fmt.Sprintf("%v", ctx.Path()))
		h(ctx)
	}
}

func NewServer(cfg *Configuration) *APIServer {
	r := router.New()

	s := &APIServer{
		Configuration: cfg,
		httpServer: &fasthttp.Server{
			Handler: loggingMiddleware(corsMiddleware(r.Handler)),
		},
	}

	// r.GET("/api/v1/video/{kind:hls}/{url}/{sdHash:^[a-z0-9]{96}$}", h.handleVideo)
	r.GET("/api/v1/video/{kind:hls}/{url}", s.handleVideo)
	r.ServeFiles(path.Join(httpVideoPath, "{filepath:*}"), s.videoPath)

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

func (s APIServer) Stop() error {
	logger.Info("shutting down...")
	return s.httpServer.Shutdown()
}
