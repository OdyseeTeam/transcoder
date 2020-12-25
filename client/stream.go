package client

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/grafov/m3u8"
	"github.com/karlseguin/ccache/v2"
	"github.com/lbryio/transcoder/video"
)

type CachedVideo struct {
	URL       string
	localPath string
	size      int64
}

type Downloadable interface {
	Download() error
	Progress() <-chan Progress
}

type Progress struct {
	err   error
	stage int
	Done  bool
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

func (v *CachedVideo) LocalPath() string {
	return v.localPath
}

func (v CachedVideo) delete() error {
	return os.RemoveAll(v.localPath)
}

func deleteCachedVideo(i *ccache.Item) {
	cv := i.Value().(*CachedVideo)
	err := cv.delete()
	if err != nil {
		// log
	}
}

func newHLSStream(url, sdHash string, client *Client) *HLSStream {
	return &HLSStream{URL: url, progress: make(chan Progress, 20000), client: client, SDHash: sdHash}
}

func (s HLSStream) fetch(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// s.makeProgress()
	return s.client.httpClient.Do(req)
}

func (s HLSStream) retrieveFile(rawurl string) (io.ReadCloser, int64, error) {
	var bytesRead int64

	parsedurl, err := url.Parse(rawurl)
	if err != nil {
		return nil, 0, err
	}

	res, err := s.fetch(rawurl)
	if err != nil {
		return nil, 0, err
	}

	out, err := os.Create(path.Join(s.LocalPath(), path.Base(parsedurl.Path)))
	if err != nil {
		return nil, 0, err
	}
	if bytesRead, err = io.Copy(out, bufio.NewReader(res.Body)); err != nil {
		return nil, 0, err
	}
	_, err = out.Seek(0, io.SeekStart)
	if err != nil {
		return nil, bytesRead, err
	}
	return out, bytesRead, nil
}

func (s HLSStream) Download() error {
	logger.Debugw("stream download", "url", s.rootURL())
	res, err := s.fetch(s.rootURL())
	if err != nil {
		return err
	}

	logger.Debugw("transcoder response", "status", res.StatusCode)
	switch res.StatusCode {
	case http.StatusForbidden:
		return video.ErrChannelNotEnabled
	case http.StatusNotFound:
		return errors.New("stream not found")
	case http.StatusAccepted:
		return errors.New("encoding underway")
	case http.StatusSeeOther:
		loc, err := res.Location()
		if err != nil {
			return err
		}
		go func() {
			err := s.startDownload(loc.String())
			if err != nil {
				s.progress <- Progress{err: err}
			}
		}()
		return nil
	default:
		return fmt.Errorf("unknown http status: %v", res.StatusCode)
	}
}

func (s HLSStream) Progress() <-chan Progress {
	return s.progress
}

func (s HLSStream) makeProgress() {
	s.filesFetched++
	s.progress <- Progress{stage: s.filesFetched}
}

func (s HLSStream) storeInCache(key, rootPath string, size int64) {
	cv := &CachedVideo{URL: s.URL, size: size, localPath: s.SDHash}
	s.client.cache.Set(hlsCacheKey(s.URL, s.SDHash), cv, 24*30*12*time.Hour)
}

func (s HLSStream) startDownload(playlistURL string) error {
	var streamSize int64

	basePath := strings.Replace(playlistURL, "/master.m3u8", "", 1)

	if err := os.MkdirAll(s.LocalPath(), os.ModePerm); err != nil {
		return err
	}

	logger.Debugw("downloading master playlist", "url", playlistURL)
	res, br, err := s.retrieveFile(playlistURL)
	streamSize += br
	if err != nil {
		return err
	}

	p, _, err := m3u8.DecodeFrom(bufio.NewReader(res), true)
	if err != nil {
		return err
	}
	res.Close()

	masterpl := p.(*m3u8.MasterPlaylist)
	for _, plv := range masterpl.Variants {
		url := fmt.Sprintf("%v/%v", basePath, plv.URI)
		logger.Debugw("downloading variant playlist", "url", url)
		res, br, err := s.retrieveFile(url)
		streamSize += br
		if err != nil {
			return err
		}

		p, _, err := m3u8.DecodeFrom(bufio.NewReader(res), true)
		if err != nil {
			return err
		}
		res.Close()
		mediapl := p.(*m3u8.MediaPlaylist)

		for _, seg := range mediapl.Segments {
			if seg == nil {
				continue
			}
			url := fmt.Sprintf("%v/%v", basePath, seg.URI)
			logger.Debugw("downloading media", "url", url)
			res, br, err := s.retrieveFile(url)
			streamSize += br
			if err != nil {
				return err
			}
			res.Close()
		}
	}

	logger.Debugw("got all files, saving to cache",
		"url", s.URL,
		"size", streamSize,
		"path", s.LocalPath(),
		"key", hlsCacheKey(s.URL, s.SDHash),
	)
	s.storeInCache(hlsCacheKey(s.URL, s.SDHash), s.LocalPath(), streamSize)

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
	return path.Join(s.client.videoPath, s.SDHash)
}
