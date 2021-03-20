package client

import (
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grafov/m3u8"
	"github.com/karrick/godirwalk"
	"github.com/lbryio/transcoder/api"
	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/queue"
	"github.com/lbryio/transcoder/storage"
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

func (s *ClientSuite) SetupTest() {
	s.assetsPath = path.Join(os.TempDir(), "transcoder_test")
	os.RemoveAll(s.assetsPath)
	s.Require().NoError(os.MkdirAll(path.Join(s.assetsPath, "sqlite"), os.ModePerm))
	s.Require().NoError(os.MkdirAll(path.Join(s.assetsPath, "videos"), os.ModePerm))
	s.Require().NoError(os.MkdirAll(path.Join(s.assetsPath, "client"), os.ModePerm))

	vdb := db.OpenDB(path.Join(s.assetsPath, "sqlite", "video.sqlite"))
	vdb.MigrateUp(video.InitialMigration)
	qdb := db.OpenDB(path.Join(s.assetsPath, "sqlite", "queue.sqlite"))
	qdb.MigrateUp(queue.InitialMigration)

	lib := video.NewLibrary(
		video.Configure().
			LocalStorage(storage.Local(path.Join(s.assetsPath, "videos"))).
			DB(vdb),
	)
	q := queue.NewQueue(qdb)

	poller := q.StartPoller(1)
	go video.SpawnProcessing(q, lib, poller)
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

func (s *ClientSuite) TearDownTest() {
	go s.apiServer.Shutdown()
	s.Require().NoError(os.RemoveAll(s.assetsPath))
}

func (s *ClientSuite) TestSweepCache() {
	vPath := path.Join(s.assetsPath, "TestSweepCache")

	c := New(Configure().VideoPath(vPath))

	cvDirs := map[string]int64{}
	for range [10]int{} {
		dir := randomString(96)
		size, err := populateHLSPlaylist(path.Join(vPath, dir))
		s.Require().NoError(err)
		cvDirs[dir] = size

		c.CacheVideo(dir, size)
		cv := c.GetCachedVideo(dir)
		s.Require().NotNil(cv)
		s.Require().Equal(size, cv.Size())
	}

	c = New(Configure().VideoPath(vPath))
	n, err := c.SweepCache(true)
	s.Require().NoError(err)
	s.EqualValues(10, n)

	for dir, size := range cvDirs {
		cv := c.GetCachedVideo(dir)
		s.Require().NotNil(cv)
		s.EqualValues(size, cv.Size())
	}

	// Obliterate half the playlist files to simulate partially downloaded transcoded stream
	// and make sure the playlist cache entry doesn't get restored.
	var cvToNuke string
	for dir := range cvDirs {
		cvToNuke = dir
		break
	}
	entries, err := godirwalk.ReadDirnames(path.Join(c.videoPath, cvToNuke), nil)
	s.Require().NoError(err)
	for _, e := range entries[len(entries)/2:] {
		if strings.HasSuffix(e, ".m3u8") {
			continue
		}
		s.Require().NoError(os.Remove(path.Join(vPath, cvToNuke, e)))
	}

	c = New(Configure().VideoPath(vPath))
	_, err = c.SweepCache(true)
	s.Require().NoError(err)
	s.Require().Nil(c.GetCachedVideo(cvToNuke))
}

func (s *ClientSuite) TestGet() {
	s.T().SkipNow()

	vPath := path.Join(s.assetsPath, "TestGet")
	c := New(Configure().VideoPath(vPath).Server(s.apiServer.URL()).LogLevel(Dev))
	s.Require().NotNil(c.httpClient)

	cv, dl := c.Get("hls", streamURL, streamSDHash)
	s.Require().Nil(cv)

	time.Sleep(100 * time.Millisecond)
	err := dl.Init()
	s.Require().EqualError(err, "encoding underway")

	s.T().Log("waiting for transcoder to ready up the stream")

	for {
		cv, dl = c.Get("hls", streamURL, streamSDHash)
		s.Require().Nil(cv)
		if dl != nil {
			err := dl.Init()
			if err == nil {
				break
			}
		}
	}

	go func() {
		err := dl.Download()
		s.Require().NoError(err)
	}()

	<-dl.Progress()
	cv, dl2 := c.Get("hls", streamURL, streamSDHash)
	s.Require().Nil(cv)
	err = dl2.Init()
	s.EqualError(err, "video is already downloading")

	for p := range dl.Progress() {
		s.T().Logf("got download progress: %+v", p)
		s.Require().NoError(p.Error)

		if p.Stage == DownloadDone {
			break
		}
	}

	err = dl2.Init()
	s.NoError(err)

	cv, dl = c.Get("hls", streamURL, streamSDHash)
	s.Nil(dl)

	f, err := os.Open(path.Join(vPath, cv.DirName(), MasterPlaylistName))
	s.NoError(err)
	rawpl, _, err := m3u8.DecodeFrom(f, true)
	s.NoError(err)
	f.Close()

	pool.Stop()

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

func (s *ClientSuite) TestPoolDownload() {
	vPath := path.Join(s.assetsPath, "TestPoolDownload")
	c := New(Configure().VideoPath(vPath).Server(s.apiServer.URL()))
	s.Require().NotNil(c.httpClient)

	cv, dl := c.Get("hls", streamURL, streamSDHash)
	s.Require().Nil(cv)

	time.Sleep(100 * time.Millisecond)
	err := dl.Init()
	s.Require().EqualError(err, "encoding underway")

	s.T().Log("waiting for transcoder to ready up the stream")

	for {
		req, _ := http.NewRequest(
			http.MethodGet,
			"http://127.0.0.1:50808/api/v1/video/hls/lbry:%2F%2F@specialoperationstest%233%2Ffear-of-death-inspirational%23a", nil)
		r, err := c.httpClient.Do(req)
		s.Require().NoError(err)
		if r.StatusCode == http.StatusSeeOther {
			break
		}
		time.Sleep(1000 * time.Millisecond)
	}

	s.T().Log("transcoder is ready, starting HLSStream download")
	cv, dl = c.Get("hls", streamURL, streamSDHash)
	s.Require().Nil(cv)

	PoolDownload(dl)
	// result := PoolDownload(dl)
	// time.Sleep(30 * time.Millisecond)

	// cv, dl2 := c.Get("hls", streamURL, streamSDHash)
	// s.Nil(cv)
	// err = dl2.Download()
	// s.EqualError(err, "video is already downloading")

	<-dl.Progress()
	cv, dl2 := c.Get("hls", streamURL, streamSDHash)
	s.Require().Nil(cv)
	err = dl2.Init()
	s.EqualError(err, "video is already downloading")

	for p := range dl.Progress() {
		s.T().Logf("got download progress: %+v", p)
		s.Require().NoError(p.Error)

		if p.Stage == DownloadDone {
			break
		}
	}

	// for {
	// 	if result.Done() {
	// 		break
	// 	} else if result.Failed() {
	// 		s.FailNow("download task failed", err)
	// 	}
	// }

	cv, dl = c.Get("hls", streamURL, streamSDHash)
	s.Nil(dl)
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

func (s *ClientSuite) TestCacheSize() {
	vPath := path.Join(s.assetsPath, "TestCacheSize")

	cSize := int64(3131915 * 25)

	c := New(Configure().VideoPath(vPath).CacheSize(cSize))

	cvDirs := map[string]int64{}
	for range [50]int{} {
		dir := randomString(96)
		size, err := populateHLSPlaylist(path.Join(vPath, dir))
		s.Require().NoError(err)
		cvDirs[dir] = size

		c.CacheVideo(dir, size)
		cv := c.GetCachedVideo(dir)
		s.Require().NotNil(cv)
		s.Require().Equal(size, cv.Size())
	}

	var storedSize int64
	for dir := range cvDirs {
		cv := c.GetCachedVideo(dir)
		if cv != nil {
			storedSize += cv.Size()
		}
	}
	s.LessOrEqual(storedSize, cSize)
}

func randomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	b := make([]rune, n)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}

// populateHLSPlaylist generates a stream of 3131915 bytes in size, segments binary data will all be zeroes.
func populateHLSPlaylist(vPath string) (int64, error) {
	err := os.MkdirAll(vPath, os.ModePerm)
	if err != nil {
		return 0, err
	}

	plPath, _ := filepath.Abs("./testdata")
	size, err := HLSPlaylistDive(
		plPath,
		func(rootPath ...string) ([]byte, error) {
			if path.Ext(rootPath[len(rootPath)-1]) == ".m3u8" {
				return ioutil.ReadFile(path.Join(rootPath...))
			}
			return make([]byte, 10000), nil
		},
		func(data []byte, name string) error {
			return ioutil.WriteFile(path.Join(vPath, name), data, os.ModePerm)
		},
	)
	if err != nil {
		return 0, err
	}
	return size, nil
}
