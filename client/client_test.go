package client

import (
	"os"
	"path"
	"testing"
	"time"

	"github.com/grafov/m3u8"
	"github.com/lbryio/transcoder/api"
	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/queue"
	"github.com/lbryio/transcoder/video"
	"github.com/stretchr/testify/suite"
)

var streamURL = "lbry://@specialoperationstest#3/fear-of-death-inspirational#a"
var streamSDHash = "f12fb044f5805334a473bf9a81363d89bd1cb54c4065ac05be71a599a6c51efc6c6afb257208326af304324094105774"

type ClientSuite struct {
	suite.Suite
	assetsPath string
	apiServer  *api.APIServer
}

func TestClientSuite(t *testing.T) {
	suite.Run(t, new(ClientSuite))
}

func (s *ClientSuite) SetupSuite() {
	s.assetsPath = path.Join(os.TempDir(), "transcoder_test")
	os.RemoveAll(s.assetsPath)
	s.Require().NoError(os.MkdirAll(path.Join(s.assetsPath, "sqlite"), os.ModePerm))
	s.Require().NoError(os.MkdirAll(path.Join(s.assetsPath, "videos"), os.ModePerm))
	s.Require().NoError(os.MkdirAll(path.Join(s.assetsPath, "client"), os.ModePerm))

	vdb := db.OpenDB(path.Join(s.assetsPath, "sqlite", "video.sqlite"))
	vdb.MigrateUp(video.InitialMigration)
	qdb := db.OpenDB(path.Join(s.assetsPath, "sqlite", "queue.sqlite"))
	qdb.MigrateUp(queue.InitialMigration)

	lib := video.NewLibrary(vdb)
	q := queue.NewQueue(qdb)

	go video.SpawnProcessing(path.Join(s.assetsPath, "videos"), q, lib)
	s.apiServer = api.NewServer(
		api.Configure().
			Debug(true).
			Addr("127.0.0.1:50808").
			VideoPath(path.Join(s.assetsPath, "videos")).
			VideoManager(api.NewManager(q, lib)),
	)
	go s.apiServer.Start()
}

func (s *ClientSuite) TearDownSuite() {
	s.Require().NoError(os.RemoveAll(s.assetsPath))
}

func (s *ClientSuite) TestGet() {
	c := New(Configure().VideoPath(path.Join(s.assetsPath, "client")).Server(s.apiServer.Addr()))
	s.Require().NotNil(c.httpClient)

	cv, dl := c.Get("hls", streamURL, streamSDHash)
	s.Require().Nil(cv)

	time.Sleep(100 * time.Millisecond)
	err := dl.Download()
	s.T().Log("getting")
	s.Require().EqualError(err, "encoding underway")

	s.T().Log("waiting for transcoder to ready up the stream")
	for {
		cv, dl = c.Get("hls", streamURL, streamSDHash)
		s.Require().Nil(cv)
		if dl != nil {
			err := dl.Download()
			s.T().Log("error is", err)
			if err == nil {
				break
			}
		}
		time.Sleep(1000 * time.Millisecond)
	}
	s.T().Log("transcoder is ready")
	s.T().Log("stream download started")
	for p := range dl.Progress() {
		s.Require().NoError(p.err)
		if p.Done {
			break
		}
		s.T().Log("got download progress:", p.stage)
	}

	cv, dl = c.Get("hls", streamURL, streamSDHash)
	s.Nil(dl)
	s.FileExists(path.Join(cv.rootPath, "master.m3u8"))

	f, err := os.Open(path.Join(cv.rootPath, encoder.MasterPlaylist))
	s.NoError(err)
	p, _, err := m3u8.DecodeFrom(f, true)
	s.NoError(err)
	f.Close()

	masterpl := p.(*m3u8.MasterPlaylist)
	for _, plv := range masterpl.Variants {
		f, err := os.Open(path.Join(cv.rootPath, plv.URI))
		s.NoError(err)
		p, _, err := m3u8.DecodeFrom(f, true)
		s.NoError(err)
		f.Close()
		mediapl := p.(*m3u8.MediaPlaylist)
		for _, seg := range mediapl.Segments {
			if seg == nil {
				continue
			}
			s.FileExists(path.Join(cv.rootPath, seg.URI))
		}
	}
}
