package manager

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/lbryio/transcoder/internal/metrics"
	"github.com/lbryio/transcoder/library/db"
	"github.com/lbryio/transcoder/pkg/dispatcher"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/resolve"
	"github.com/lbryio/transcoder/pkg/timer"

	"github.com/fasthttp/router"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

const (
	TokenCtxField     = "token"
	AuthHeader        = "Authorization"
	AdminChannelField = "channel"
)

type AuthCallback func(*fasthttp.RequestCtx) bool

type httpVideoHandler struct {
	manager      *VideoManager
	log          logging.KVLogger
	authCallback AuthCallback
}

// CreateRoutes creates a set of HTTP entrypoints that will route requests into video library.
func CreateRoutes(r *router.Router, manager *VideoManager, log logging.KVLogger, cb AuthCallback) {
	h := httpVideoHandler{
		log:          log,
		manager:      manager,
		authCallback: cb,
	}

	r.GET("/api/v1/video/{kind:hls}/{url}", h.handleVideo)
	r.GET("/api/v2/video/{url}", h.handleVideo)
	r.GET("/api/v3/video", h.handleVideo) // accepts URL as a query param

	r.POST("/api/v1/channel", h.handleChannel)

	metrics.RegisterMetrics()
	dispatcher.RegisterMetrics()
	RegisterMetrics()
	r.GET("/metrics", fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler()))
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

	location, err := h.manager.Video(videoURL)

	if err != nil {
		var (
			statusCode    int
			statusMessage string
		)
		switch err {
		case resolve.ErrTranscodingForbidden:
			statusCode = http.StatusForbidden
		case resolve.ErrChannelNotEnabled:
			statusCode = http.StatusForbidden
		case resolve.ErrNoSigningChannel:
			statusCode = http.StatusForbidden
		case resolve.ErrTranscodingQueued:
			statusCode = http.StatusAccepted
		case resolve.ErrTranscodingUnderway:
			statusCode = http.StatusAccepted
		case resolve.ErrClaimNotFound:
			statusCode = http.StatusNotFound
			ll.Info("claim not found")
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

	if err == resolve.ErrNoSigningChannel || err == resolve.ErrChannelNotEnabled {
		ctx.SetStatusCode(http.StatusForbidden)
		ll.Debug("transcoding disabled")
		return
	} else if err == resolve.ErrTranscodingQueued {
		ctx.SetStatusCode(http.StatusAccepted)
		ll.Debug("trancoding queued")
		return
	} else if err == resolve.ErrTranscodingForbidden {
		ctx.SetStatusCode(http.StatusForbidden)
		ctx.Response.SetBodyString(err.Error())
		ll.Debug(err.Error())
		return
	} else if err == resolve.ErrTranscodingUnderway {
		ctx.SetStatusCode(http.StatusAccepted)
		ll.Debug("trancoding underway")
		return
	} else if err == resolve.ErrClaimNotFound {
		ctx.SetStatusCode(http.StatusNotFound)
		ll.Info("claim not found")
		return
	} else if err != nil {
		ctx.SetStatusCode(http.StatusInternalServerError)
		ll.Errorw("internal error", "error", err)
		fmt.Fprint(ctx, err.Error())
		return
	}

	ctx.Response.StatusCode()
	metrics.StreamsRequestedCount.WithLabelValues(metrics.StorageRemote).Inc()
	ll.Debugw("stream found", "location", location)
	ctx.Redirect(location, http.StatusSeeOther)
}

func (h httpVideoHandler) handleChannel(ctx *fasthttp.RequestCtx) {
	if h.authCallback == nil {
		h.log.Error("management endpoint called but authenticator function not set")
		ctx.SetStatusCode(http.StatusForbidden)
		ctx.SetBodyString("authorization failed")
		return
	}
	token := strings.Replace(string(ctx.Request.Header.Peek(AuthHeader)), "Bearer ", "", 1)
	ctx.SetUserValue(TokenCtxField, token)

	if !h.authCallback(ctx) {
		h.log.Info("authorization failed")
		ctx.SetStatusCode(http.StatusForbidden)
		ctx.SetBodyString("authorization failed")
		return
	}

	channel := string(ctx.FormValue(AdminChannelField))
	if channel == "" {
		ctx.SetStatusCode(http.StatusBadRequest)
		fmt.Fprint(ctx, "channel missing")
		return
	}
	var priority db.ChannelPriority
	priority.Scan(ctx.FormValue("priority"))
	c, err := h.manager.lib.AddChannel(channel, priority)
	if err != nil {
		ctx.SetStatusCode(http.StatusBadRequest)
		fmt.Fprint(ctx, err.Error())
		return
	}
	ctx.SetStatusCode(http.StatusCreated)
	fmt.Fprintf(ctx, "channel %s (%s) added with priority %s", c.URL, c.ClaimID, c.Priority)
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
