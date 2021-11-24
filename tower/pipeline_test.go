package tower

import (
	"fmt"
	"io/ioutil"
	"net"
	"path"
	"testing"
	"time"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/uploader"
	"github.com/lbryio/transcoder/storage"
	"github.com/stretchr/testify/suite"
	"github.com/valyala/fasthttp"
)

type pipelineSuite struct {
	suite.Suite
	workDir, uploadDir string
	upServer           *fasthttp.Server
	upAddr             string
}

func TestChainSuite(t *testing.T) {
	suite.Run(t, new(pipelineSuite))
}

func (s *pipelineSuite) SetupTest() {
	workDir, err := ioutil.TempDir("", "transcoder_pipeline_suite")
	s.Require().NoError(err)
	uploadDir, err := ioutil.TempDir(workDir, "uploads")
	s.Require().NoError(err)
	s.workDir = workDir
	s.uploadDir = uploadDir

	upServer := uploader.NewUploadServer(
		uploadDir,
		func(_ *fasthttp.RequestCtx) bool { return true },
		func(_ storage.LightLocalStream) {},
	)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	s.Require().NoError(err)
	s.upAddr = l.Addr().String()
	go func() {
		upServer.Serve(l)
	}()
	s.upServer = upServer
	s.T().Logf("listening on %v", s.upAddr)
}

func (s *pipelineSuite) TestProcessSuccess() {
	url := "lbry://@specialoperationstest#3/fear-of-death-inspirational#a"
	sdh := "f12fb044f5805334a473bf9a81363d89bd1cb54c4065ac05be71a599a6c51efc6c6afb257208326af304324094105774"
	enc, err := encoder.NewEncoder(encoder.Configure())
	s.Require().NoError(err)
	c, err := newPipeline(s.workDir, enc, 1*time.Second, logging.NoopKVLogger{})
	s.Require().NoError(err)
	stop := make(chan struct{})

	t := &task{url: url, callbackURL: fmt.Sprintf("http://%v/%v", s.upAddr, sdh)}
	defer t.cleanup()

	tc := c.Process(stop, t)
	p, err := s.watchTask(tc)
	s.Require().NoError(err)
	s.Require().Equal(StageDone, p.Stage)
	s.FileExists(path.Join(s.uploadDir, sdh, "master.m3u8"))
}
func (s *pipelineSuite) TestProcessFinalFailure() {
	url := "lbry://@specialoperationstest#3/fear-of-death-inspirational#a"
	sdh := "f12fb044f5805334a473bf9a81363d89bd1cb54c4065ac05be71a599a6c51efc6c6afb257208326af304324094105774"
	enc, err := encoder.NewEncoder(encoder.Configure())
	s.Require().NoError(err)
	c, err := newPipeline(s.workDir, enc, 1*time.Second, logging.NoopKVLogger{})
	s.Require().NoError(err)
	stop := make(chan struct{})

	s.upServer.MaxRequestBodySize = 1

	t := &task{url: url, callbackURL: fmt.Sprintf("http://%v/%v", s.upAddr, sdh)}
	defer t.cleanup()

	tc := c.Process(stop, t)
	p, err := s.watchTask(tc)
	s.Require().Nil(p)
	s.Require().Error(err)
}

func (s *pipelineSuite) watchTask(tc taskControl) (*pipelineProgress, error) {
	for {
		select {
		case err := <-tc.Errc:
			return nil, err
		case p := <-tc.Progress:
			s.Require().NotEmpty(p.Stage)
			if p.Stage == StageDone {
				return &p, nil
			}
		case tc = <-tc.Next:
			s.T().Log("moving on to the next stage")
		case <-tc.TaskDone:
			err := <-tc.Errc
			if err != nil {
				return nil, err
			}
		}
	}
}
