package client

import (
	"io/ioutil"
	"math/rand"
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

	poller := q.StartPoller(1)
	go video.SpawnProcessing(path.Join(s.assetsPath, "videos"), q, lib, poller)
	s.apiServer = api.NewServer(
		api.Configure().
			Debug(true).
			Addr("127.0.0.1:50808").
			VideoPath(path.Join(s.assetsPath, "videos")).
			VideoManager(api.NewManager(q, lib)),
	)
	go s.apiServer.Start()

	video.LoadEnabledChannels(
		[]string{
			"@specialoperationstest#3",
		})
}

func (s *ClientSuite) TearDownSuite() {
	s.Require().NoError(os.RemoveAll(s.assetsPath))
}

func (s *ClientSuite) TestCache() {
	vPath := path.Join(s.assetsPath, "TestCache")

	c := New(Configure().VideoPath(vPath))

	cvDirs := []string{}
	for range [10]int{} {
		dir := randomString(96)
		cvDirs = append(cvDirs, dir)
		os.MkdirAll(path.Join(vPath, dir), os.ModePerm)
		ioutil.WriteFile(path.Join(vPath, dir, "master.m3u8"), []byte("12345"), os.ModePerm)
		c.CacheVideo(dir, 5)

		cv := c.GetCachedVideo(dir)
		s.Require().NotNil(cv)
	}

	c = New(Configure().VideoPath(vPath))
	n, err := c.RestoreCache()
	s.Require().NoError(err)
	s.EqualValues(10, n)

	for _, dir := range cvDirs {
		cv := c.GetCachedVideo(dir)
		s.Require().NotNil(cv)
		s.EqualValues(5, cv.Size())
	}
}

func (s *ClientSuite) TestGet() {
	vPath := path.Join(s.assetsPath, "Test_restoreCache")
	c := New(Configure().VideoPath(vPath).Server(s.apiServer.URL()))
	s.Require().NotNil(c.httpClient)

	cv, dl := c.Get("hls", streamURL, streamSDHash)
	s.Require().Nil(cv)

	time.Sleep(100 * time.Millisecond)
	err := dl.Download()
	s.Require().EqualError(err, "encoding underway")

	s.T().Log("waiting for transcoder to ready up the stream")

	for {
		cv, dl = c.Get("hls", streamURL, streamSDHash)
		s.Require().Nil(cv)
		if dl != nil {
			err := dl.Download()
			if err == nil {
				break
			}
		}
		time.Sleep(1000 * time.Millisecond)
	}

	s.T().Log("transcoder is ready, HLSStream.startDownload is working")
	time.Sleep(1000 * time.Millisecond)

	cv, dl2 := c.Get("hls", streamURL, streamSDHash)
	s.Nil(cv)
	err = dl2.Download()
	s.Nil(err)

	p := <-dl2.Progress()
	s.T().Logf("got dl2 progress: %+v", p)
	s.EqualError(p.Error, "download already in progress")

	for p := range dl.Progress() {
		s.T().Logf("got download progress: %+v", p)
		s.Require().NoError(p.Error)
		if p.Done {
			break
		}
	}

	cv, dl = c.Get("hls", streamURL, streamSDHash)
	s.Nil(dl)

	f, err := os.Open(path.Join(vPath, cv.DirName(), encoder.MasterPlaylist))
	s.NoError(err)
	rawpl, _, err := m3u8.DecodeFrom(f, true)
	s.NoError(err)
	f.Close()

	masterpl := rawpl.(*m3u8.MasterPlaylist)
	for _, plv := range masterpl.Variants {
		f, err := os.Open(path.Join(vPath, cv.DirName(), plv.URI))
		s.NoError(err)
		p, _, err := m3u8.DecodeFrom(f, true)
		s.NoError(err)
		f.Close()
		mediapl := p.(*m3u8.MediaPlaylist)
		for _, seg := range mediapl.Segments {
			if seg == nil {
				continue
			}
			s.FileExists(path.Join(vPath, cv.DirName(), seg.URI))
		}
	}
}

func (s *ClientSuite) TestGetCachedVideo() {
	vPath := path.Join(s.assetsPath, "TestGetCachedVideo")
	c := New(Configure().VideoPath(vPath))
	hash := randomString(96)
	cachedPath := path.Join(vPath, hash)
	os.MkdirAll(cachedPath, os.ModePerm)
	c.CacheVideo(hash, 6000)

	cv := c.GetCachedVideo(hash)
	s.Equal(cv.dirName, hash)
	s.Equal(cv.size, int64(6000))

	s.Require().NoError(os.Remove(cachedPath))
	s.Nil(c.GetCachedVideo(hash))
}

func randomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	b := make([]rune, n)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}
