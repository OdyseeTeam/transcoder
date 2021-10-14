package uploader

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/Pallinder/go-randomdata"
	"github.com/fasthttp/router"
	"github.com/karrick/godirwalk"
	"github.com/stretchr/testify/suite"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
)

type uploaderSuite struct {
	suite.Suite

	path, sdHash, inPath, tarPath string
	csum                          []byte

	router *router.Router
}

func serve(handler fasthttp.RequestHandler, req *http.Request) (*http.Response, error) {
	ln := fasthttputil.NewInmemoryListener()
	defer ln.Close()

	go func() {
		err := fasthttp.Serve(ln, handler)
		if err != nil {
			panic(fmt.Errorf("failed to serve: %v", err))
		}
	}()

	client := http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return ln.Dial()
			},
		},
	}

	return client.Do(req)
}

func verifyPathChecksum(path string, csum []byte) (int64, error) {
	var size int64
	hash := sha256.New()
	err := godirwalk.Walk(path, &godirwalk.Options{
		Callback: func(fullPath string, de *godirwalk.Dirent) error {
			if de.IsDir() && fullPath == path {
				return nil
			}
			fs, err := os.Stat(fullPath)
			if err != nil {
				return err
			}
			size += fs.Size()

			f, err := os.Open(fullPath)
			if err != nil {
				return err
			}
			_, err = io.Copy(hash, f)
			if err != nil {
				return err
			}

			return nil
		}})
	if err != nil {
		return 0, err
	}
	sum := hash.Sum(nil)
	if !bytes.Equal(sum, csum) {
		return 0, fmt.Errorf("checksum verification failed: %v != %v", sum, csum)
	}
	return size, nil
}

func TestManagerSuite(t *testing.T) {
	suite.Run(t, new(uploaderSuite))
}

func (s *uploaderSuite) SetupSuite() {
	p, err := os.MkdirTemp("", "")
	s.Require().NoError(err)
	r := router.New()
	h := FileHandler{
		uploadPath: path.Join(p, "incoming"),
		checkAuth:  func(_ *fasthttp.RequestCtx) bool { return true },
	}
	r.POST(`/{sd_hash:[a-z0-9]{96}}`, h.Handle)

	sdHash := strings.ToLower(randomdata.Alphanumeric(96))
	tarPath := path.Join(p, fmt.Sprintf("%v.tar", sdHash))

	populateHLSPlaylist(s.T(), p, sdHash)
	csum, err := packStream(path.Join(p, sdHash), tarPath)
	s.Require().NoError(err)

	s.router = r
	s.path = p
	s.sdHash = sdHash
	s.inPath = h.uploadPath
	s.tarPath = tarPath
	s.csum = csum
}

func (s *uploaderSuite) serve(req *http.Request) (*http.Response, error) {
	return serve(s.router.Handler, req)
}

func (s *uploaderSuite) TearDownSuite() {
	os.RemoveAll(s.path)
}

func (s *uploaderSuite) TestFileHandling() {
	req, err := buildUploadRequest(s.tarPath, "http://inmemory/"+s.sdHash, s.csum)
	s.Require().NoError(err)
	r, err := s.serve(req)
	s.Require().NoError(err)

	b, _ := ioutil.ReadAll(r.Body)
	s.Equal(http.StatusAccepted, r.StatusCode, string(b))

	_, err = verifyPathChecksum(path.Join(s.inPath, s.sdHash), s.csum)
	s.Require().NoError(err)
}

func (s *uploaderSuite) TestFileHandling_EmptyFile() {
	f, err := ioutil.TempFile("", "")
	s.Require().NoError(err)
	f.Close()

	req, err := buildUploadRequest(f.Name(), "http://inmemory/"+s.sdHash, s.csum)
	s.Require().NoError(err)
	r, err := s.serve(req)
	s.Require().NoError(err)

	s.Equal(http.StatusBadRequest, r.StatusCode)
	b, _ := ioutil.ReadAll(r.Body)
	s.Contains(string(b), "doesn't match calculated checksum")

	_, err = os.Stat(path.Join(s.inPath, s.sdHash))
	s.True(os.IsNotExist(err), "uploaded artifacts were not cleaned up")
}
