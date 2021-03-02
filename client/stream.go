package client

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/lbryio/transcoder/video"
)

// ErrAlreadyDownloading when returned means that video retrieval is already underway
// and nothing needs to be done at this time.
var ErrAlreadyDownloading = errors.New("video is already downloading")

type CachedVideo struct {
	dirName string
	size    int64
}

type Downloadable interface {
	Download() error
	Progress() <-chan Progress
}

type Progress struct {
	Error       error
	Stage       int
	Done        bool
	BytesLoaded int64
}

type HLSStream struct {
	URL          string
	size         int64
	SDHash       string
	client       *Client
	progress     chan Progress
	filesFetched int
}

func (v *CachedVideo) Size() int64 {
	return v.size
}

func (v *CachedVideo) DirName() string {
	return v.dirName
}

func (v CachedVideo) delete() error {
	return os.RemoveAll(v.DirName())
}

func newHLSStream(url, sdHash string, client *Client) *HLSStream {
	s := &HLSStream{URL: url, progress: make(chan Progress, 1), client: client, SDHash: sdHash}
	return s
}

func (s HLSStream) fetch(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return s.client.httpClient.Do(req)
}

func (s HLSStream) retrieveFile(rootPath ...string) ([]byte, error) {
	rawurl := strings.Join(rootPath, "/")
	ll := logger.With("remote_url", rawurl, "url", s.rootURL())

	ll.Debugw("fetching stream part")
	_, err := url.Parse(rawurl)
	if err != nil {
		return nil, err
	}

	res, err := s.fetch(rawurl)
	if err != nil {
		ll.Warnw("stream part fetch failed", "err", err)
		return nil, err
	}

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	s.makeProgress(int64(len(data)))
	return data, nil
}

func (s HLSStream) saveFile(data []byte, name string) error {
	ll := logger.With("path", path.Join(s.LocalPath(), name), "url", s.rootURL())
	ll.Debugw("saving stream part")
	err := ioutil.WriteFile(path.Join(s.LocalPath(), name), data, os.ModePerm)
	if err != nil {
		ll.Warnw("saving stream part failed", "err", err)
		return err
	}

	return nil
}

func (s HLSStream) Download() error {
	ll := logger.With("url", s.rootURL(), "sd_hash", s.SDHash)
	if s.client.isDownloading(s.SDHash) {
		TranscodedResult.WithLabelValues(resultDownloading).Inc()
		ll.Debugw("already downloading")
		return ErrAlreadyDownloading
	}
	res, err := s.fetch(s.rootURL())
	if err != nil {
		return err
	}

	switch res.StatusCode {
	case http.StatusForbidden:
		TranscodedResult.WithLabelValues(resultForbidden).Inc()
		return video.ErrChannelNotEnabled
	case http.StatusNotFound:
		TranscodedResult.WithLabelValues(resultNotFound).Inc()
		return errors.New("stream not found")
	case http.StatusAccepted:
		TranscodedResult.WithLabelValues(resultUnderway).Inc()
		ll.Debugw("stream encoding underway")
		return errors.New("encoding underway")
	case http.StatusSeeOther:
		TranscodedResult.WithLabelValues(resultFound).Inc()
		loc, err := res.Location()
		if err != nil {
			return err
		}
		ll.Debugw("starting stream download", "location", loc)
		go func() {
			err := s.startDownload(loc.String())
			if err != nil {
				s.progress <- Progress{Error: err}
			}
		}()
		return nil
	default:
		ll.Infow("unknown http status", "status_code", res.StatusCode)
		return fmt.Errorf("unknown http status: %v", res.StatusCode)
	}
}

func (s HLSStream) Progress() <-chan Progress {
	return s.progress
}

func (s *HLSStream) makeProgress(bl int64) {
	s.filesFetched++
	s.progress <- Progress{Stage: s.filesFetched, BytesLoaded: bl}
}

func (s *HLSStream) startDownload(playlistURL string) error {
	if !s.client.canStartDownload(s.SDHash) {
		return errors.New("download already in progress")
	}

	rootPath := strings.Replace(playlistURL, "/"+MasterPlaylistName, "", 1)

	if err := os.MkdirAll(s.LocalPath(), os.ModePerm); err != nil {
		return err
	}

	streamSize, err := HLSPlaylistDive(rootPath, s.retrieveFile, s.saveFile)
	if err != nil {
		return err
	}

	s.progress <- Progress{Stage: 999999, BytesLoaded: streamSize}

	// Download complete
	logger.Debugw("got all files, saving to cache",
		"url", s.URL,
		"size", streamSize,
		"path", s.LocalPath(),
		"key", hlsCacheKey(s.SDHash),
	)
	s.client.CacheVideo(s.DirName(), streamSize)

	s.client.releaseDownload(s.rootURL())

	s.progress <- Progress{Done: true}
	close(s.progress)
	return nil
}

func (s HLSStream) rootURL() string {
	return fmt.Sprintf(hlsURLTemplate, s.client.server, s.SafeURL())
}

func (s HLSStream) SafeURL() string {
	return url.PathEscape(s.URL)
}

func (s HLSStream) LocalPath() string {
	return path.Join(s.client.videoPath, s.DirName())
}

func (s HLSStream) DirName() string {
	return s.SDHash
}
