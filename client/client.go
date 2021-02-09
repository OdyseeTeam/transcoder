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
	cmap "github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"
)

const (
	hlsURLTemplate = "%v/api/v1/video/hls/%v"
	dlStarted      = iota
)

type HTTPRequester interface {
	Do(req *http.Request) (res *http.Response, err error)
}

type Client struct {
	*Configuration
	cache     *ccache.Cache
	downloads cmap.ConcurrentMap
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
	c := Client{
		Configuration: cfg,
		downloads:     cmap.New(),
	}
	c.cache = ccache.New(ccache.
		Configure().
		MaxSize(cfg.cacheSize).
		ItemsToPrune(20).
		OnDelete(c.deleteCachedVideo),
	)

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
		logger.Errorw(
			"unable to delete cached video",
			"path", path, "err", err,
		)
	}
}

// Get returns either a cached video or downloadable instance for further processing.
func (c Client) Get(kind, lbryURL, sdHash string) (*CachedVideo, Downloadable) {
	logger.Debugw("getting video from cache", "url", lbryURL, "key", hlsCacheKey(sdHash))
	cv := c.GetCachedVideo(sdHash)
	if cv != nil {
		return cv, nil
	}
	logger.Debugw("cache miss", "url", lbryURL, "key", hlsCacheKey(sdHash))

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
	c.cache.Set(hlsCacheKey(path), cv, 24*30*12*time.Hour)
}

func (c Client) RestoreCache() (int64, error) {
	var streamsRestored int64
	cvs, err := godirwalk.ReadDirnames(c.videoPath, nil)

	if err != nil {
		return streamsRestored, errors.Wrap(err, "cannot restore cache")
	}

	// Verify that all stream files are present
	for _, cvPath := range cvs {
		// Skip non-sdHashes
		if len(cvPath) != 96 {
			continue
		}
		cvFullPath := path.Join(c.videoPath, cvPath)

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
			logger.Infow("removing broken playlist", "path", cvPath, "err", err)
			continue
		}
		c.CacheVideo(cvPath, cvSize)
		streamsRestored++
	}

	return streamsRestored, nil
}
