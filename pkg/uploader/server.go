package uploader

import (
	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
)

func NewServer(uploadPath string, checkAuth func(*fasthttp.RequestCtx) bool) *fasthttp.Server {
	r := router.New()
	h := FileHandler{
		uploadPath: uploadPath,
		checkAuth:  checkAuth,
	}
	r.POST(`/{sd_hash:[a-z0-9]{96}}`, h.Handle)
	return &fasthttp.Server{
		Handler: r.Handler,
	}
}
