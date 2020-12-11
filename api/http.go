package api

import (
	"fmt"
	"log"
	"net/http"
	"path"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/video"

	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

var httpVideoPath = "/streams"
var logger = zap.NewExample().Sugar().Named("http")

func handleVideo(ctx *fasthttp.RequestCtx) {
	kind := ctx.UserValue("kind").(string)
	url := ctx.UserValue("url").(string)
	sdHash := ctx.UserValue("sdHash").(string)
	logger.Infow("GET", "url", url)
	pl, err := GetVideoOrCreateTask(url, sdHash, kind)
	if err == video.ErrChannelNotEnabled {
		ctx.SetStatusCode(http.StatusForbidden)
		return
	} else if err == video.ErrTranscodingUnderway {
		ctx.SetStatusCode(http.StatusAccepted)
		return
	} else if err != nil {
		ctx.SetStatusCode(http.StatusInternalServerError)
		fmt.Fprint(ctx, err.Error())
	}
	ctx.Redirect(fmt.Sprintf("%v/%v/%v", httpVideoPath, pl.Path, encoder.MasterPlaylist), http.StatusPermanentRedirect)
}

func StartHTTP(bind, videoPath string) {
	r := router.New()
	r.GET("/api/v1/video/{kind:^(dash)|(hls)|(range)$}/{url}/{sdHash:^[a-z0-9]{97}$}", handleVideo)
	r.ServeFiles(path.Join(videoPath, "{filepath:*}"), httpVideoPath)
	log.Fatal(fasthttp.ListenAndServe(bind, r.Handler))
}
