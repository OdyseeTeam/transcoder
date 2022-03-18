package tower

import (
	"io/ioutil"
	"testing"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/library"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/storage"

	"github.com/draganm/miniotest"
	"github.com/stretchr/testify/suite"
)

type pipelineSuite struct {
	suite.Suite
	workDir, uploadDir string
	s3addr             string
	s3cleanup          func() error
	s3drv              *storage.S3Driver
}

func TestPipelineSuite(t *testing.T) {
	suite.Run(t, new(pipelineSuite))
}

func (s *pipelineSuite) SetupSuite() {
	var err error
	s.s3addr, s.s3cleanup, err = miniotest.StartEmbedded()
	s.Require().NoError(err)

	s.s3drv, err = storage.InitS3Driver(
		storage.S3Configure().
			Endpoint(s.s3addr).
			Region("us-east-1").
			Credentials("minioadmin", "minioadmin").
			Bucket("storage-s3-test").
			DisableSSL(),
	)
	s.Require().NoError(err)
}

func (s *pipelineSuite) SetupTest() {
	workDir, err := ioutil.TempDir("", "transcoder_pipeline_suite")
	s.Require().NoError(err)
	uploadDir, err := ioutil.TempDir(workDir, "uploads")
	s.Require().NoError(err)
	s.workDir = workDir
	s.uploadDir = uploadDir
}

func (s *pipelineSuite) TestProcessSuccess() {
	url := "lbry://@specialoperationstest#3/fear-of-death-inspirational#a"
	sdh := "f12fb044f5805334a473bf9a81363d89bd1cb54c4065ac05be71a599a6c51efc6c6afb257208326af304324094105774"
	enc, err := encoder.NewEncoder(encoder.Configure())
	s.Require().NoError(err)
	c, err := newPipeline(s.workDir, "", s.s3drv, enc, zapadapter.NewKV(nil))
	s.Require().NoError(err)

	wt := createWorkerTask(MsgTranscodingTask{URL: url, SDHash: sdh})

	c.Process(wt)
	var r taskResult
loop:
	for {
		select {
		case p := <-wt.progress:
			s.Require().NotEmpty(p.Stage)
		case r = <-wt.result:
			s.Require().NotNil(r.remoteStream)
			break loop
		case err := <-wt.errors:
			s.FailNow("unexpected error", err)
			break loop
		}
	}

	sf, err := s.s3drv.GetFragment(r.remoteStream.TID(), library.MasterPlaylistName)
	s.Require().NoError(err)
	s.Require().NotNil(sf)
}

func (s *pipelineSuite) TestProcessFailure() {
	url := "lbry://@specialoperationstest#3/nonexisting#a"
	sdh := "f12fb044f5805334a473bf9a81363d89bd1cb54c4065ac05be71a599a6c51efc6c6afb257208326af304324094105774"
	enc, err := encoder.NewEncoder(encoder.Configure())
	s.Require().NoError(err)
	c, err := newPipeline(s.workDir, "", s.s3drv, enc, zapadapter.NewKV(nil))
	s.Require().NoError(err)

	wt := createWorkerTask(MsgTranscodingTask{URL: url, SDHash: sdh})
	c.Process(wt)

loop:
	for {
		select {
		case p := <-wt.progress:
			s.Require().NotEmpty(p.Stage)
		case <-wt.result:
			s.FailNow("did not expect this to succeed")
			break loop
		case e := <-wt.errors:
			s.Require().EqualError(e.err, "could not resolve stream URI")
			break loop
		}
	}
}
