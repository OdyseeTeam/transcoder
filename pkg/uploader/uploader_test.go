package uploader

import (
	"bytes"
	"context"
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
	"github.com/karrick/godirwalk"
	"github.com/lbryio/transcoder/storage"
	"github.com/stretchr/testify/suite"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
)

type uploaderSuite struct {
	suite.Suite

	path, sdHash, inPath, tarPath string
	localStream                   storage.LightLocalStream

	server   *fasthttp.Server
	uploaded map[string]string
}

const secretToken = "abcabc"

func TestManagerSuite(t *testing.T) {
	suite.Run(t, new(uploaderSuite))
}

func (s *uploaderSuite) SetupTest() {
	p := s.T().TempDir()

	server := NewUploadServer(
		path.Join(p, "incoming"),
		func(ctx *fasthttp.RequestCtx) bool {
			return ctx.UserValue("token").(string) == secretToken
		},
		func(ls storage.LightLocalStream) {
			s.uploaded[ls.SDHash] = ls.Path
		},
	)

	sdHash := strings.ToLower(randomdata.Alphanumeric(96))
	tarPath := path.Join(p, fmt.Sprintf("%v.tar", sdHash))

	populateHLSPlaylist(s.T(), p, sdHash)
	ls, err := storage.OpenLocalStream(path.Join(p, sdHash))
	s.Require().NoError(err)

	csum, err := packStream(ls, tarPath)
	ls.Checksum = csum
	s.Require().NoError(err)

	s.server = server
	s.path = p
	s.sdHash = sdHash
	s.inPath = path.Join(p, "incoming")
	s.tarPath = tarPath
	s.localStream = *ls
	s.uploaded = map[string]string{}
}

func (s *uploaderSuite) TestUpload_Success() {
	expectedStreamPath := path.Join(s.inPath, s.sdHash)

	req, err := buildUploadRequest(s.tarPath, "http://inmemory/"+s.sdHash, secretToken, s.localStream.Checksum)
	s.Require().NoError(err)
	r, err := serve(s.server, req)
	s.Require().NoError(err)

	b, _ := ioutil.ReadAll(r.Body)
	s.Require().Equal(http.StatusAccepted, r.StatusCode, string(b))

	_, err = verifyPathChecksum(expectedStreamPath, s.localStream.Checksum)
	s.Require().NoError(err)

	s.Equal(expectedStreamPath, s.uploaded[s.sdHash])
}

func (s *uploaderSuite) TestUpload_InvalidToken() {
	req, err := buildUploadRequest(s.tarPath, "http://inmemory/"+s.sdHash, "wrongtoken", s.localStream.Checksum)
	s.Require().NoError(err)
	r, err := serve(s.server, req)
	s.Require().NoError(err)

	b, _ := ioutil.ReadAll(r.Body)
	s.Require().Equal(http.StatusForbidden, r.StatusCode, string(b))
}

func (s *uploaderSuite) TestUpload_InvalidChecksum() {
	req, err := buildUploadRequest(s.tarPath, "http://inmemory/"+s.sdHash, secretToken, []byte("abc"))
	s.Require().NoError(err)
	r, err := serve(s.server, req)
	s.Require().NoError(err)

	b, _ := ioutil.ReadAll(r.Body)
	s.Require().Equal(http.StatusBadRequest, r.StatusCode, string(b))
}

func (s *uploaderSuite) TestUpload_EmptyFile() {
	expectedStreamPath := path.Join(s.inPath, s.sdHash)

	f, err := ioutil.TempFile("", "")
	s.Require().NoError(err)
	f.Close()

	req, err := buildUploadRequest(f.Name(), "http://inmemory/"+s.sdHash, secretToken, s.localStream.Checksum)
	s.Require().NoError(err)
	r, err := serve(s.server, req)
	s.Require().NoError(err)

	s.Equal(http.StatusBadRequest, r.StatusCode)
	b, _ := ioutil.ReadAll(r.Body)
	s.Contains(string(b), "doesn't match calculated checksum")

	_, err = os.Stat(expectedStreamPath)
	s.True(os.IsNotExist(err), "uploaded artifacts were not cleaned up")

	s.Empty(s.uploaded[s.sdHash])
}

func serve(server *fasthttp.Server, req *http.Request) (*http.Response, error) {
	ln := fasthttputil.NewInmemoryListener()
	defer ln.Close()

	go func() {
		err := server.Serve(ln)
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
	hash := storage.GetHash()
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
