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
	"go.uber.org/zap"
)

const (
	Downloading = iota
	DownloadDone
	Failed
)

// ErrAlreadyDownloading when returned means that video retrieval is already underway
// and nothing needs to be done at this time.
var ErrAlreadyDownloading = errors.New("video is already downloading")

type CachedVideo struct {
	dirName string
	size    int64
}

type Downloadable interface {
	Init() error
	Download() error
	Progress() <-chan Progress
	Done() bool
}

type Progress struct {
	Error       error
	BytesLoaded int64
	Stage       int
}

type HLSStream struct {
	URL             string
	size            int64
	SDHash          string
	client          *Client
	progress        chan Progress
	done            bool
	filesFetched    int
	bytesDownloaded int64
	playlistURL     string
	logger          *zap.SugaredLogger
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
	s := &HLSStream{
		URL:      url,
		progress: make(chan Progress, 1),
		client:   client,
		SDHash:   sdHash,
		logger:   client.logger.With("url", url, "sd_hash", sdHash),
	}
	return s
}

func (s HLSStream) Progress() <-chan Progress {
	return s.progress
}

func (s HLSStream) Done() bool {
	return s.done
}

func (s *HLSStream) Init() error {
	if s.client.isDownloading(s.SDHash) {
		TranscodedResult.WithLabelValues(resultDownloading).Inc()
		s.logger.Debugw("already downloading")
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
		s.logger.Debugw("stream encoding underway")
		return errors.New("encoding underway")
	case http.StatusSeeOther:
		TranscodedResult.WithLabelValues(resultFound).Inc()
		loc, err := res.Location()
		if err != nil {
			return err
		}
		s.logger.Debugw("got playlist URL", "location", loc)
		s.playlistURL = loc.String()
		return nil
	default:
		s.logger.Warnw("unknown http status", "status_code", res.StatusCode)
		return fmt.Errorf("unknown http status: %v", res.StatusCode)
	}
}

func (s *HLSStream) Download() error {
	if s.playlistURL == "" {
		return errors.New("playlistURL is empty")
	}
	defer close(s.progress)

	if !s.client.canStartDownload(s.SDHash) {
		return errors.New("download already in progress")
	}
	defer s.client.releaseDownload(s.SDHash)

	rootPath := strings.Replace(s.playlistURL, "/"+MasterPlaylistName, "", 1)

	if err := os.MkdirAll(s.LocalPath(), os.ModePerm); err != nil {
		return err
	}

	streamSize, err := HLSPlaylistDive(rootPath, s.retrieveFile, s.saveFile)
	if err != nil {
		rmErr := os.RemoveAll(s.LocalPath())
		if rmErr != nil {
			s.logger.Warnw("download cleanup failed", "err", rmErr)
		}
		return fmt.Errorf("download start failed: %v", err)
	}

	// Download complete
	s.logger.Debugw("got all files, saving video to cache",
		"size", streamSize,
	)
	s.progress <- Progress{Stage: DownloadDone}
	s.client.CacheVideo(s.DirName(), streamSize)
	s.done = true

	return nil
}

func (s HLSStream) fetch(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	s.logger.Debugw("fetching", "url", url)
	return s.client.httpClient.Do(req)
}

func (s *HLSStream) makeProgress(bl int64) {
	s.filesFetched++
	s.bytesDownloaded += bl
	p := Progress{Stage: Downloading, BytesLoaded: s.bytesDownloaded}
	s.logger.Debugf("download progress: %+v", p)
	// s.progress <- p
}

func (s HLSStream) retrieveFile(rootPath ...string) ([]byte, error) {
	rawurl := strings.Join(rootPath, "/")
	ll := s.logger.With("remote_url", rawurl)

	ll.Debugw("fetching stream part")
	_, err := url.Parse(rawurl)
	if err != nil {
		return nil, err
	}

	res, err := s.fetch(rawurl)
	if err != nil {
		ll.Warnw("stream fragment fetch failed", "err", err)
		return nil, err
	}

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		ll.Warnw("reading stream response body failed", "err", err)
		return nil, err
	}

	return data, nil
}

func (s HLSStream) saveFile(data []byte, name string) error {
	s.logger.Debugw("writing stream fragment")
	err := ioutil.WriteFile(path.Join(s.LocalPath(), name), data, os.ModePerm)
	if err != nil {
		s.logger.Warnw("writing stream fragment failed", "err", err)
		return err
	}
	s.makeProgress(int64(len(data)))

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
