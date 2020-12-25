package client

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"time"

	"github.com/karlseguin/ccache/v2"
	"go.uber.org/zap"
)

const hlsURLTemplate = "http://%v/api/v1/video/hls/%v"

var logger = zap.NewExample().Sugar().Named("transcoder_client")

type HTTPRequester interface {
	Do(req *http.Request) (res *http.Response, err error)
}

type Client struct {
	*Configuration
	cache *ccache.Cache
}

type Configuration struct {
	cacheSize  int64
	server     string
	videoPath  string
	httpClient HTTPRequester
}

func Configure() *Configuration {
	return &Configuration{
		cacheSize: int64(math.Pow(1024, 3)),
		httpClient: &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: 1200 * time.Second,
			Transport: &http.Transport{
				Dial: (&net.Dialer{
					Timeout:   15 * time.Second,
					KeepAlive: 120 * time.Second,
				}).Dial,
				TLSHandshakeTimeout:   30 * time.Second,
				ResponseHeaderTimeout: 15 * time.Second,
			},
		},
	}
}

// CacheSize defines local disk cache size for downloaded transcoded videos.
func (c *Configuration) CacheSize(size int64) *Configuration {
	c.cacheSize = size
	return c
}

// Server sets transcoder server API address.
func (c *Configuration) Server(server string) *Configuration {
	c.server = server
	return c
}

// Server sets transcoder server API address.
func (c *Configuration) VideoPath(videoPath string) *Configuration {
	c.videoPath = videoPath
	return c
}

func (c *Configuration) HTTPClient(httpClient HTTPRequester) *Configuration {
	c.httpClient = httpClient
	return c
}

func New(cfg *Configuration) Client {
	return Client{
		Configuration: cfg,
		cache: ccache.New(ccache.
			Configure().
			MaxSize(cfg.cacheSize).
			ItemsToPrune(20).
			OnDelete(deleteCachedVideo),
		),
	}
}

func hlsCacheKey(lbryURL, sdHash string) string {
	return fmt.Sprintf("hls::%v::%v", lbryURL, sdHash)
}

func (c Client) Get(kind, lbryURL, sdHash string) (*CachedVideo, Downloadable) {
	logger.Debugw("getting video from cache", "url", lbryURL, "key", hlsCacheKey(lbryURL, sdHash))
	item := c.cache.Get(hlsCacheKey(lbryURL, sdHash))
	if item != nil {
		return item.Value().(*CachedVideo), nil
	}
	logger.Debugw("cache miss", "url", lbryURL, "key", hlsCacheKey(lbryURL, sdHash))

	stream := newHLSStream(lbryURL, sdHash, &c)
	return nil, stream
}
