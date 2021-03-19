package client

import (
	"io/ioutil"
	"math"
	"net"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/karlseguin/ccache/v2"
	"github.com/karrick/godirwalk"
	"github.com/lbryio/transcoder/pkg/logging"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	hlsURLTemplate = "%v/api/v1/video/hls/%v"
	dlStarted      = iota
	Dev            = iota
	Prod
)

type HTTPRequester interface {
	Do(req *http.Request) (res *http.Response, err error)
}

type Client struct {
	*Configuration
	cache     *ccache.Cache
	downloads cmap.ConcurrentMap
	logger    *zap.SugaredLogger
}

type Configuration struct {
	cacheSize    int64
	itemsToPrune uint32
	server       string
	videoPath    string
	httpClient   HTTPRequester
	logLevel     int
}

func Configure() *Configuration {
	return &Configuration{
		cacheSize:    int64(math.Pow(1024, 3)),
		itemsToPrune: 100,
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
		logLevel: Prod,
	}
}

// CacheSize defines local disk cache size for downloaded transcoded videos.
func (c *Configuration) CacheSize(size int64) *Configuration {
	c.cacheSize = size
	return c
}

// ItemsToPrune defines how many items to prune during cache cleanup
func (c *Configuration) ItemsToPrune(i uint32) *Configuration {
	c.itemsToPrune = i
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

// LogLevel sets verbosity of logging. `Dev` outputs a lot of debugging info, `Prod` is more restrained.
func (c *Configuration) LogLevel(l int) *Configuration {
	c.logLevel = l
	return c
}

func New(cfg *Configuration) Client {
	c := Client{
		Configuration: cfg,
		downloads:     cmap.New(),
	}
	if c.logLevel == Dev {
		c.logger = logging.Create("client", logging.Dev)
	} else {
		c.logger = logging.Create("client", logging.Prod)
	}
	c.cache = ccache.New(ccache.
		Configure().
		MaxSize(c.cacheSize).
		ItemsToPrune(c.itemsToPrune).
		OnDelete(c.deleteCachedVideo),
	)

	c.logger.Infow("transcoder client configured", "cache_size", c.cacheSize, "server", c.server, "video_path", c.videoPath)
	c.startSweeper()
	return c
}

func hlsCacheKey(sdHash string) string {
	return "hls::" + sdHash
}

func (c Client) deleteCachedVideo(i *ccache.Item) {
	cv := i.Value().(*CachedVideo)
	path := path.Join(c.videoPath, cv.DirName())
	err := os.RemoveAll(path)
	if err != nil {
		c.logger.Errorw(
			"unable to delete cached video",
			"path", path, "err", err,
		)
	} else {
		TranscodedCacheSizeBytes.Sub(float64(cv.Size()))
		TranscodedCacheItemsCount.Dec()
		c.logger.Infow("purged cache item", "name", cv.DirName(), "size", cv.Size())
	}
}

// Get returns either a cached video or downloadable instance for further processing.
func (c Client) Get(kind, lbryURL, sdHash string) (*CachedVideo, Downloadable) {
	c.logger.Debugw("getting video from cache", "url", lbryURL, "key", hlsCacheKey(sdHash))
	cv := c.GetCachedVideo(sdHash)
	if cv != nil {
		TranscodedResult.WithLabelValues(resultLocalCache).Inc()
		return cv, nil
	}
	c.logger.Debugw("cache miss", "url", lbryURL, "key", hlsCacheKey(sdHash))

	stream := newHLSStream(lbryURL, sdHash, &c)
	return nil, stream
}

// func (c Client) downloadExists(sdHash string) bool {
// 	return c.downloads.Has(sdHash)
// }

func (c Client) isDownloading(key string) bool {
	return c.downloads.Has(key)
}

func (c Client) canStartDownload(key string) bool {
	ok := c.downloads.SetIfAbsent(key, dlStarted)
	return ok
}

func (c Client) releaseDownload(key string) {
	c.downloads.Remove(key)
}

func (c Client) GetCachedVideo(sdHash string) *CachedVideo {
	item := c.cache.Get(hlsCacheKey(sdHash))
	if item == nil {
		return nil
	}
	cv, _ := item.Value().(*CachedVideo)
	if cv == nil {
		return nil
	}

	_, err := os.Stat(path.Join(c.videoPath, cv.dirName))
	if err != nil {
		c.cache.Delete(sdHash)
		return nil
	}
	return cv
}

func (c Client) CacheVideo(path string, size int64) {
	cv := &CachedVideo{size: size, dirName: path}
	TranscodedCacheSizeBytes.Add(float64(cv.Size()))
	TranscodedCacheItemsCount.Inc()
	c.logger.Infow("cached item", "name", cv.DirName(), "size", cv.Size())
	c.cache.Set(hlsCacheKey(path), cv, 24*30*12*time.Hour)
}

// SweepCache goes through cache directory and removes broken streams, optionally restoring healthy ones in the cache.
func (c Client) SweepCache(restore bool) (int64, error) {
	var swept int64
	cvs, err := godirwalk.ReadDirnames(c.videoPath, nil)

	if err != nil {
		return 0, errors.Wrap(err, "cannot sweep cache")
	}

	// Verify that all stream files are present
	for _, sdHash := range cvs {
		// Skip non-sdHashes
		if len(sdHash) != 96 {
			continue
		}
		if c.isDownloading(sdHash) {
			continue
		}
		cvFullPath := path.Join(c.videoPath, sdHash)

		cvSize, err := HLSPlaylistDive(
			cvFullPath,
			func(rootPath ...string) ([]byte, error) {
				f, err := os.Open(path.Join(rootPath...))
				defer f.Close()
				if err != nil {
					return nil, err
				}
				if path.Ext(rootPath[len(rootPath)-1]) != ".m3u8" {
					s, err := os.Stat(path.Join(rootPath...))
					if err != nil {
						return nil, err
					}
					return make([]byte, s.Size()), nil
				}
				return ioutil.ReadAll(f)
			},
			func(data []byte, name string) error {
				return nil
			},
		)

		if err != nil {
			os.RemoveAll(cvFullPath)
			c.logger.Infow("removed broken stream", "path", sdHash, "err", err)
			continue
		}
		if restore {
			c.CacheVideo(sdHash, cvSize)
		}
		swept++
	}

	c.logger.Infow("cache swept", "count", swept)
	return swept, nil
}

func (c Client) startSweeper() {
	sweepTicker := time.NewTicker(5 * time.Minute)
	go func() {
		for range sweepTicker.C {
			_, err := c.SweepCache(false)
			if err != nil {
				c.logger.Warnw("periodic sweep failed", "err", err)
			}
		}
	}()
}
