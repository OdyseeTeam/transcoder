package tower

import (
	"fmt"
	"io/ioutil"
	"net"
	"path"
	"testing"

	"github.com/lbryio/transcoder/pkg/uploader"
	"github.com/stretchr/testify/suite"
	"github.com/valyala/fasthttp"
)

type chainSuite struct {
	suite.Suite
	workDir, uploadDir string
	upServer           *fasthttp.Server
	upAddr             string
}

func TestChainSuite(t *testing.T) {
	suite.Run(t, new(chainSuite))
}

func (s *chainSuite) SetupSuite() {
	workDir, err := ioutil.TempDir("", "transcoder_chain_suite")
	s.Require().NoError(err)
	uploadDir, err := ioutil.TempDir(workDir, "uploads")
	s.Require().NoError(err)
	s.workDir = workDir
	s.uploadDir = uploadDir

	upServer := uploader.NewServer(uploadDir, func(_ *fasthttp.RequestCtx) bool { return true })
	l, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	s.upAddr = l.Addr().String()
	go func() {
		upServer.Serve(l)
	}()
	s.upServer = upServer
	s.T().Logf("listening on %v", s.upAddr)
}

func (s *chainSuite) TestEnter() {
	url := "lbry://@specialoperationstest#3/fear-of-death-inspirational#a"
	sdh := "f12fb044f5805334a473bf9a81363d89bd1cb54c4065ac05be71a599a6c51efc6c6afb257208326af304324094105774"
	c, err := newChain(s.workDir, 10)
	s.Require().NoError(err)

	pChan := c.Enter(url, fmt.Sprintf("http://%v/%v", s.upAddr, sdh))
	var p chainProgress
	for p = range pChan {
		s.Require().NoError(p.Error)
	}
	s.Require().Equal("done", p.Stage)
	s.FileExists(path.Join(s.uploadDir, sdh, "master.m3u8"))
}
