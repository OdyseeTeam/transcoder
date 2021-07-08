package client

import (
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/timer"

	"github.com/brk0v/directio"
	"github.com/karlseguin/ccache/v2"
	"github.com/karrick/godirwalk"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	MasterPlaylistName = "master.m3u8"

	SchemaRemote = "remote://"

	cacheHeader         = "x-cache"
	cacheHeaderHit      = "HIT"
	cacheHeaderMiss     = "MISS"
	cacheControlHeader  = "cache-control"
	clientCacheDuration = 21239

	defaultRemoteServer = "https://cache.transcoder.odysee.com/t-na"

	fragmentCacheDuration = time.Hour * 24 * 30
	hlsURLTemplate        = "/api/v1/video/hls/%v"
	fragmentURLTemplate   = "/streams/%v"
	dlStarted             = iota
	Dev                   = iota + 1
	Prod
)

var (
	ErrNotOK = errors.New("http response not OK")

	errRefetch = errors.New("need to refetch")
	sdHashRe   = regexp.MustCompile(`/([A-Za-z0-9]{96})/?`)
)

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
	remoteServer string
	httpClient   HTTPRequester
	logLevel     int
}

type Fragment struct {
	path string
	size int64
}

func Configure() *Configuration {
	return &Configuration{
		remoteServer: defaultRemoteServer,
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

// VideoPath is where transcoded videos will be downloaded and stored.
func (c *Configuration) VideoPath(videoPath string) *Configuration {
	c.videoPath = videoPath
	return c
}

// RemoteServer is full URL of remote transcoded videos storage server (sans forward slash at the end).
func (c *Configuration) RemoteServer(s string) *Configuration {
	c.remoteServer = s
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

func (f Fragment) Path() string {
	return f.path
}

func New(cfg *Configuration) Client {
	c := Client{
		Configuration: cfg,
		streamURLs:    &sync.Map{},
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

	RegisterMetrics()

	c.logger.Infow("transcoder client configured", "cache_size", c.cacheSize, "server", c.server, "video_path", c.videoPath)
	return c
}

func newFragment(sdHash, name string, size int64) *Fragment {
	return &Fragment{
		path: path.Join(sdHash, name),
		size: size,
	}
}

// PlayFragment ...
func (c Client) PlayFragment(lbryURL, sdHash, fragment string, w http.ResponseWriter, r *http.Request) error {
	var ch string
	ll := c.logger.With("lbryURL", lbryURL, "sd_hash", sdHash, "fragment", fragment)
	fg, hit, err := c.getCachedFragment(lbryURL, sdHash, fragment)
	if err != nil {
		ll.Warnf("failed to serve fragment: %v", err)
		return err
	}

	c.logger.Infow("serving fragment", "path", c.fullFragmentPath(fg), "cache_hit", hit)
	if hit {
		ch = cacheHeaderHit
	} else {
		ch = cacheHeaderMiss
	}
	w.Header().Set(cacheHeader, ch)
	w.Header().Set(cacheControlHeader, fmt.Sprintf("public, max-age=%v", clientCacheDuration))
	http.ServeFile(w, r, c.fullFragmentPath(fg))
	return nil
}

func (c Client) fullFragmentPath(fg *Fragment) string {
	return path.Join(c.videoPath, fg.path)
}

func (c Client) buildUrl(url string) string {
	url = strings.TrimSuffix(url, MasterPlaylistName)
	if !strings.HasPrefix(url, SchemaRemote) {
		return url
	}
	return fmt.Sprintf("%v/%v", c.remoteServer, strings.TrimPrefix(url, SchemaRemote))
}

func (c Client) GetPlaybackPath(lbryURL, sdHash string) string {
	if _, err := c.fragmentURL(lbryURL, sdHash, MasterPlaylistName); err != nil {
		c.logger.Debugw("playback path not found", "lbryURL", lbryURL, "sd_hash", sdHash, "err", err)
		return ""
	}
	c.logger.Debugw("playback path found", "lbryURL", lbryURL, "sd_hash", sdHash)
	return fmt.Sprintf("%v/%v/%v", strings.Replace(lbryURL, "#", "/", 1), sdHash, MasterPlaylistName)
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
	}
}

func (c Client) discardFragmentURL(sdHash string) {
	c.streamURLs.Delete(sdHash)
}

func (c Client) fragmentURL(lbryURL, sdHash, name string) (string, error) {
	if d, ok := c.streamURLs.Load(sdHash); !ok {
		// Getting root playlist location from transcoder.
		res, err := c.fetch(c.server + fmt.Sprintf(hlsURLTemplate, url.PathEscape(lbryURL)))
		if err != nil {
			return "", err
		}

		switch res.StatusCode {
		case http.StatusForbidden:
			TranscodedResult.WithLabelValues(resultForbidden).Inc()
			return "", manager.ErrChannelNotEnabled
		case http.StatusNotFound:
			TranscodedResult.WithLabelValues(resultNotFound).Inc()
			return "", errors.New("stream not found")
		case http.StatusAccepted:
			TranscodedResult.WithLabelValues(resultUnderway).Inc()
			c.logger.Debugw("stream encoding underway")
			return "", errors.New("encoding underway")
		case http.StatusSeeOther:
			TranscodedResult.WithLabelValues(resultFound).Inc()
			loc, err := res.Location()
			if err != nil {
				return "", err
			}
			streamURL := c.buildUrl(loc.String())
			c.logger.Debugw("got stream URL", "stream_url", streamURL)
			rsdHash := sdHashRe.FindStringSubmatch(streamURL)
			if len(rsdHash) != 2 {
				return "", fmt.Errorf("malformed remote sd hash in URL: %v", streamURL)
			} else if rsdHash[1] != sdHash {
				return "", fmt.Errorf("remote sd hash mismatch: %v != %v", sdHash, rsdHash[1])
			}
			c.streamURLs.Store(sdHash, streamURL)
			return streamURL + name, nil
		default:
			c.logger.Warnw("unknown http status", "status_code", res.StatusCode)
			return "", fmt.Errorf("unknown http status: %v", res.StatusCode)
		}
	} else {
		streamURL, _ := d.(string)
		return streamURL + name, nil
	}
}

func (c Client) fetch(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.logger.Debugw("fetching", "url", url)
	r, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (c Client) getCachedFragment(lbryURL, sdHash, name string) (*Fragment, bool, error) {
	var (
		item *ccache.Item
		err  error
		miss bool
	)

	key := cacheFragmentKey(sdHash, name)
	TranscodedCacheQueryCount.Inc()
	for i := 0; i <= 3; i++ {
		item, err = c.cache.Fetch(key, fragmentCacheDuration, func() (interface{}, error) {
			var src string

			miss = true

			c.logger.Debugw("cache miss", "key", key)
			TranscodedCacheMiss.Inc()

			fpath := path.Join(c.videoPath, sdHash, name)

			if err := os.MkdirAll(path.Join(c.videoPath, sdHash), os.ModePerm); err != nil {
				return nil, err
			}

			url, err := c.fragmentURL(lbryURL, sdHash, name)
			if err != nil {
				return nil, err
			}

			if strings.HasPrefix(url, SchemaRemote) {
				src = fetchSourceRemote
			} else {
				src = fetchSourceLocal
			}

			t := timer.Start()
			r, err := c.fetch(url)
			FetchCount.WithLabelValues(src).Inc()
			if err != nil {
				FetchFailureCount.WithLabelValues(src, "unknown").Inc()
				return nil, err
			}

			defer r.Body.Close()

			if r.StatusCode != http.StatusOK {
				FetchFailureCount.WithLabelValues(src, fmt.Sprintf("%v", r.StatusCode)).Inc()
				if r.StatusCode == http.StatusNotFound {
					c.discardFragmentURL(sdHash)
					return nil, errRefetch
				}
				return r, ErrNotOK
			}

			FetchDurationSeconds.WithLabelValues(src).Add(t.Stop())

			//TODO: like in reflector this should be written to a tmp path and then moved to avoid data corruption on crashes
			f, err := openFile(fpath)
			if err != nil {
				return nil, err
			}
			defer f.Close()

			// Use directio writer
			dio, err := directio.New(f)
			if err != nil {
				return nil, err
			}
			defer dio.Flush()
			// Write the body to file
			size, err := io.Copy(dio, r.Body)

			if err != nil {
				return nil, err
			}

			FetchSizeBytes.WithLabelValues(src).Add(float64(size))
			c.logger.Debugw("saved fragment", "url", url, "size", size, "path", fpath)

			TranscodedCacheSizeBytes.Add(float64(size))
			TranscodedCacheItemsCount.Inc()

			return newFragment(sdHash, name, size), nil
		})
		// TODO: Messy loop logic can be improved.
		if err != nil {
			if err == errRefetch {
				continue
			}
			return nil, false, err
		}
		break
	}

	// TODO: Handle better when we're giving up on re-fetch.
	if item == nil {
		return nil, false, errors.New("cache could not retrieve item")
	}

	fg, _ := item.Value().(*Fragment)
	if fg == nil {
		return nil, false, errors.New("cached item does not contain fragment")
	}

	_, err = os.Stat(c.fullFragmentPath(fg))
	if err != nil {
		c.cache.Delete(key)
		return nil, false, fmt.Errorf("fragment in cache but not on disk: %v", c.fullFragmentPath(fg))
	}

	return fg, !miss, nil
}

func (c Client) cacheFragment(sdHash, name string, fg *Fragment) {
	TranscodedCacheSizeBytes.Add(float64(fg.Size()))
	TranscodedCacheItemsCount.Inc()
	c.cache.Set(cacheFragmentKey(sdHash, name), fg, fragmentCacheDuration)
}

// RestoreCache ...
func (c Client) RestoreCache() (int64, error) {
	var fnum, size int64
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
				c.logger.Warnw("unable to stat cache fragment", "err", err)
				continue
			}
			c.cacheFragment(sdHash, name, newFragment(sdHash, name, s.Size()))
			size += s.Size()
			fnum++
		}
	}

	c.logger.Infow("cache restored", "fragments_number", fnum, "size", size)
	return fnum, nil
}
