package client

import (
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/video"

	"github.com/karlseguin/ccache/v2"
	"github.com/karrick/godirwalk"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	hlsURLTemplate        = "/api/v1/video/hls/%v"
	fragmentURLTemplate   = "/streams/%v"
	fragmentCacheDuration = time.Hour * 24 * 30
	dlStarted             = iota
	Dev                   = iota
	Prod
)

var ErrNotOK = errors.New("http response not OK")

type HTTPRequester interface {
	Do(req *http.Request) (res *http.Response, err error)
}

type Client struct {
	*Configuration
	logger *zap.SugaredLogger

	cache      *ccache.Cache
	streamURLs *sync.Map
}

type Configuration struct {
	cacheSize    int64
	itemsToPrune uint32
	server       string
	videoPath    string
	httpClient   HTTPRequester
	logLevel     int
}

type Fragment struct {
	path string
	size int64
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

func (f Fragment) Size() int64 {
	return f.size
}

func New(cfg *Configuration) Client {
	c := Client{
		Configuration: cfg,
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
		OnDelete(c.deleteCachedFragment),
	)
	c.logger.Infow("transcoder client configured", "cache_size", c.cacheSize, "server", c.server, "video_path", c.videoPath)
	return c
}

// PlayFragment ...
func (c Client) PlayFragment(lurl, sdHash, fragment string, w http.ResponseWriter, r *http.Request) error {
	path, err := c.getCachedFragment(lurl, sdHash, fragment)
	if err != nil {
		return err
	}

	http.ServeFile(w, r, path)
	return nil
}

func cacheFragmentKey(sdHash, name string) string {
	return fmt.Sprintf("hlsf::%v/%v", sdHash, name)
}

func (c Client) deleteCachedFragment(i *ccache.Item) {
	fg := i.Value().(*Fragment)
	path := path.Join(c.videoPath, fg.path)
	err := os.RemoveAll(path)
	if err != nil {
		c.logger.Errorw(
			"unable to delete cached fragment",
			"path", path, "err", err,
		)
	} else {
		TranscodedCacheSizeBytes.Sub(float64(fg.Size()))
		TranscodedCacheItemsCount.Dec()
		// c.logger.Infow("purged cache item", "name", cv.DirName(), "size", cv.Size())
	}
}

func (c Client) fragmentURL(url, sdHash, name string) (string, error) {
	var furl string
	if d, ok := c.streamURLs.Load(sdHash); !ok {
		res, err := c.fetch(c.server + fmt.Sprintf(hlsURLTemplate, url))
		if err != nil {
			return furl, err
		}

		switch res.StatusCode {
		case http.StatusForbidden:
			TranscodedResult.WithLabelValues(resultForbidden).Inc()
			return furl, video.ErrChannelNotEnabled
		case http.StatusNotFound:
			TranscodedResult.WithLabelValues(resultNotFound).Inc()
			return furl, errors.New("stream not found")
		case http.StatusAccepted:
			TranscodedResult.WithLabelValues(resultUnderway).Inc()
			c.logger.Debugw("stream encoding underway")
			return furl, errors.New("encoding underway")
		case http.StatusSeeOther:
			TranscodedResult.WithLabelValues(resultFound).Inc()
			loc, err := res.Location()
			if err != nil {
				return furl, err
			}
			c.logger.Debugw("got playlist URL", "location", loc)
			furl = strings.TrimSuffix(loc.String(), "/"+MasterPlaylistName)
			c.streamURLs.Store(sdHash, furl)
			return furl, nil
		default:
			c.logger.Warnw("unknown http status", "status_code", res.StatusCode)
			return furl, fmt.Errorf("unknown http status: %v", res.StatusCode)
		}
	} else {
		furl, _ := d.(string)
		return furl, nil
	}
}

func (c Client) fetch(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, c.server+url, nil)
	if err != nil {
		return nil, err
	}
	c.logger.Debugw("fetching", "url", url)
	r, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if r.StatusCode != http.StatusOK {
		return r, ErrNotOK
	}
	return r, nil
}

func (c Client) getCachedFragment(lurl, sdHash, name string) (string, error) {
	key := cacheFragmentKey(sdHash, name)
	item, err := c.cache.Fetch(key, fragmentCacheDuration, func() (interface{}, error) {
		path := path.Join(c.videoPath, sdHash, name)

		url, err := c.fragmentURL(lurl, sdHash, name)
		if err != nil {
			return nil, err
		}
		r, err := c.fetch(url)
		if err != nil {
			return nil, err
		}
		defer r.Body.Close()

		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		size, err := io.Copy(f, r.Body)
		if err != nil {
			return nil, err
		}

		TranscodedCacheSizeBytes.Add(float64(size))
		TranscodedCacheItemsCount.Inc()

		return &Fragment{
			path: path,
			size: size,
		}, nil
	})

	if err != nil {
		return "", err
	}
	fg, _ := item.Value().(*Fragment)
	if fg == nil {
		return "", err
	}

	_, err = os.Stat(path.Join(c.videoPath, fg.path))
	if err != nil {
		c.cache.Delete(key)
		return "", fmt.Errorf("missing cached fragment: %v", path.Join(c.videoPath, fg.path))
	}
	return fg.path, nil
}

// RestoreCache ...
func (c Client) RestoreCache(restore bool) (int64, error) {
	var swept int64
	sdirs, err := godirwalk.ReadDirnames(c.videoPath, nil)

	if err != nil {
		return 0, errors.Wrap(err, "cannot sweep cache")
	}

	// Verify that all stream files are present
	for _, sdHash := range sdirs {
		// Skip non-sdHashes
		if len(sdHash) != 96 {
			continue
		}

		spath := path.Join(c.videoPath, sdHash)
		fragments, err := godirwalk.ReadDirnames(spath, nil)
		if err != nil {
			return 0, err
		}
		for _, name := range fragments {
			s, err := os.Stat(path.Join(spath, name))
			if err != nil {
				// warn?
				continue
			}
			c.cache.Set(
				cacheFragmentKey(sdHash, name),
				&Fragment{path: path.Join(sdHash, name), size: s.Size()},
				fragmentCacheDuration,
			)
		}
		swept++
	}

	c.logger.Infow("cache swept", "count", swept)
	return swept, nil
}
