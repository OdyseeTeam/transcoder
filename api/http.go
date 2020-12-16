package api

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/video"

	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

var httpVideoPath = "/streams"
var logger = zap.NewExample().Sugar().Named("http")

type handler struct {
	manager *VideoManager
}

func initDebugLogger() {
	l, _ := zap.NewDevelopment()
	l = l.Named("http")
	logger = l.Sugar()
}

func (h *handler) handleVideo(ctx *fasthttp.RequestCtx) {
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

	v, err := h.manager.GetVideoOrCreateTask(url, kind)

	if err == video.ErrChannelNotEnabled || err == video.ErrNoSigningChannel {
		ctx.SetStatusCode(http.StatusForbidden)
		ll.Infow("forbidden")
		return
	} else if err == video.ErrTranscodingUnderway {
		ctx.SetStatusCode(http.StatusAccepted)
		ll.Infow("accepted")
		return
	} else if err != nil {
		ctx.SetStatusCode(http.StatusInternalServerError)
		ll.Errorw("internal error", "error", err)
		fmt.Fprint(ctx, err.Error())
		return
	}

	path := fmt.Sprintf("%v/%v/%v", httpVideoPath, v.GetPath(), encoder.MasterPlaylist)
	ll.Infow("found", "path", path)
	ctx.Redirect(path, http.StatusPermanentRedirect)
}

func handlePanic(ctx *fasthttp.RequestCtx, p interface{}) {
	ctx.SetStatusCode(http.StatusInternalServerError)
	logger.Errorw("panicked", "url", ctx.Request.URI(), "panic", p)
}

func InitHTTP(bind, videoDir string, debug bool, m *VideoManager) {
	r := router.New()
	h := handler{manager: m}
	// r.GET("/api/v1/video/{kind:hls}/{url}/{sdHash:^[a-z0-9]{96}$}", h.handleVideo)
	r.GET("/api/v1/video/{kind:hls}/{url}", h.handleVideo)
	r.ServeFiles(path.Join(httpVideoPath, "{filepath:*}"), videoDir)

	if debug {
		initDebugLogger()
	} else {
		r.PanicHandler = handlePanic
	}

	logger.Infow("started listening", "bind", bind, "video_dir", videoDir, "debug", debug)
	log.Fatal(fasthttp.ListenAndServe(bind, r.Handler))
}
