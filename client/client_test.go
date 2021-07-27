package client

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karrick/godirwalk"
	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/video"
	"github.com/lbryio/transcoder/workers"
	"github.com/stretchr/testify/suite"
)

var streamURL = "@specialoperationstest#3/fear-of-death-inspirational#a"
var streamSDHash = "f12fb044f5805334a473bf9a81363d89bd1cb54c4065ac05be71a599a6c51efc6c6afb257208326af304324094105774"

type dummyRedirectClient string

func (c dummyRedirectClient) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusSeeOther,
		Header: http.Header{
			"Location": {string(c)},
		},
	}, nil
}

type clientSuite struct {
	suite.Suite
	assetsPath string
	httpAPI    *manager.HttpAPI
}

func TestClientSuite(t *testing.T) {
	suite.Run(t, new(clientSuite))
}

func (s *clientSuite) SetupTest() {
	p, err := ioutil.TempDir("", "")
	s.Require().NoError(err)
	s.assetsPath = p
	vPath := path.Join(s.assetsPath, "videos")
	s.Require().NoError(os.MkdirAll(vPath, os.ModePerm))

	vdb := db.OpenTestDB()
	s.Require().NoError(vdb.MigrateUp(video.InitialMigration))

	libCfg := video.Configure().
		LocalStorage(storage.Local(vPath)).
		DB(vdb)
	lib := video.NewLibrary(libCfg)

	vdb = db.OpenTestDB()
	s.Require().NoError(vdb.MigrateUp(video.InitialMigration))

	mgr := manager.NewManager(lib, 0)

	workers.SpawnEncoderWorkers(1, mgr)
	s.httpAPI = manager.NewHttpAPI(
		manager.ConfigureHttpAPI().
			Debug(true).
			Addr("127.0.0.1:50808").
			VideoPath(vPath).
			VideoManager(mgr),
	)
	go func() {
		err := s.httpAPI.Start()
		if err != nil {
			s.FailNow(err.Error())
		}
	}()

	manager.LoadConfiguredChannels(
		[]string{
			"@specialoperationstest#3",
		},
		[]string{},
	)
}

func (s *clientSuite) TearDownTest() {
	go s.httpAPI.Shutdown()
	s.Require().NoError(os.RemoveAll(s.assetsPath))
}

func (s *clientSuite) TestPlayFragment() {
	c := New(Configure().VideoPath(path.Join(s.assetsPath, "TestPlayFragment")).Server("http://" + s.httpAPI.Addr()).LogLevel(Dev))

	// Request stream and wait until it's available.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	wait := time.NewTicker(500 * time.Millisecond)
Waiting:
	for {
		select {
		case <-ctx.Done():
			s.FailNow("transcoding is taking too long")
		case <-wait.C:
			rr := httptest.NewRecorder()
			err := c.PlayFragment(streamURL, streamSDHash, "master.m3u8", rr, httptest.NewRequest(http.MethodGet, "/video", nil))
			if err != nil {
				s.Require().ErrorIs(err, manager.ErrTranscodingUnderway)
			} else {
				break Waiting
			}
		}
	}
	cancel()

	// Compare stream playlists and fragments against known content or size.
	cases := []struct {
		name string
		size int64
	}{
		{"master.m3u8", 0},
		{"stream_0.m3u8", 0},
		{"stream_1.m3u8", 0},
		{"stream_2.m3u8", 0},
		{"seg_0_000000.ts", 3007812},
		{"seg_1_000000.ts", 1882632},
		{"seg_2_000000.ts", 678492},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			rr := httptest.NewRecorder()
			err := c.PlayFragment(streamURL, streamSDHash, tc.name, rr, httptest.NewRequest(http.MethodGet, "/", nil))
			s.Require().NoError(err)
			s.Require().Equal(http.StatusOK, rr.Result().StatusCode)
			rbody, err := ioutil.ReadAll(rr.Result().Body)
			s.Require().NoError(err)
			if tc.size > 0 {
				// Different transcoding runs produce slightly different files.
				s.InDelta(tc.size, len(rbody), 6000)
			} else {
				absPath, err := filepath.Abs(filepath.Join("./testdata", "known-stream", tc.name))
				s.Require().NoError(err)
				tbody, err := ioutil.ReadFile(absPath)
				s.Require().NoError(err)
				s.Equal(strings.TrimRight(string(tbody), "\n"), strings.TrimRight(string(rbody), "\n"))
			}
			if tc.name == MasterPlaylistName {
				s.Equal(cacheHeaderHit, rr.Result().Header.Get(cacheHeader))
			} else {
				s.Equal(cacheHeaderMiss, rr.Result().Header.Get(cacheHeader))
			}
			s.Equal("public, max-age=21239", rr.Result().Header.Get(cacheControlHeader))
			s.Equal("GET, OPTIONS", rr.Result().Header.Get("Access-Control-Allow-Methods"))
			s.Equal("*", rr.Result().Header.Get("Access-Control-Allow-Origin"))
			s.Equal("video/MP2T", rr.Result().Header.Get("content-type"))
		})
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			rr := httptest.NewRecorder()
			err := c.PlayFragment(streamURL, streamSDHash, tc.name, rr, httptest.NewRequest(http.MethodGet, "/", nil))
			s.Require().NoError(err)
			s.Equal(cacheHeaderHit, rr.Result().Header.Get(cacheHeader))
			s.Equal("public, max-age=21239", rr.Result().Header.Get(cacheControlHeader))
		})
	}
}

func (s *clientSuite) TestRestoreCache() {
	dstPath := path.Join(s.assetsPath, "TestRestoreCache")

	c := New(Configure().VideoPath(dstPath))

	cvDirs := []string{}
	for range [10]int{} {
		sdHash := randomString(96)
		s.populateHLSPlaylist(dstPath, sdHash)
		cvDirs = append(cvDirs, sdHash)
	}

	c = New(Configure().VideoPath(dstPath))
	n, err := c.RestoreCache()
	s.Require().NoError(err)
	s.EqualValues((78*4+5)*10, n)

	for _, sdHash := range cvDirs {
		fragments, err := godirwalk.ReadDirnames(path.Join(dstPath, sdHash), nil)
		s.Require().NoError(err)
		for _, fname := range fragments {
			fg, hit, err := c.getCachedFragment("zzz", sdHash, fname)
			s.Require().NoError(err)
			s.Require().NotNil(fg)
			s.Require().True(hit)

			fi, err := os.Stat(c.fullFragmentPath(fg))
			s.Require().NoError(err)
			s.EqualValues(fi.Size(), fg.Size())
		}
	}
}

func (s *clientSuite) Test_sdHashRe() {
	m := sdHashRe.FindStringSubmatch("http://t0.lbry.tv:18081/streams/85e8ad21f40550ebf0f30f7a0f6f092e8c62c7c697138e977087ac7b7f29554f8e0270447922493ff564457b60f45b18/master.m3u8")
	s.Equal("85e8ad21f40550ebf0f30f7a0f6f092e8c62c7c697138e977087ac7b7f29554f8e0270447922493ff564457b60f45b18", m[1])
}

func (s *clientSuite) TestRemoteURL() {
	sdhash := "bec50ab288153ed03b0eb8dafd814daf19a187e07f8da4ad91cf778f5c39ac74d9d92ad6e3ebf2ddb6b7acea3cb8893a"
	cl := dummyRedirectClient(fmt.Sprintf("remote://%v/master.m3u8", sdhash))
	c := New(Configure().HTTPClient(cl))
	u, err := c.fragmentURL("morgan", sdhash, "master.m3u8")
	s.Require().NoError(err)
	s.Equal(
		fmt.Sprintf("%v/%v/%v", defaultRemoteServer, sdhash, "master.m3u8"),
		u,
	)
}

func (s *clientSuite) TestLocalURL() {
	sdhash := "bec50ab288153ed03b0eb8dafd814daf19a187e07f8da4ad91cf778f5c39ac74d9d92ad6e3ebf2ddb6b7acea3cb8893a"
	cl := dummyRedirectClient(fmt.Sprintf("http://transcoder.com/streams/%v/master.m3u8", sdhash))
	c := New(Configure().HTTPClient(cl))
	u, err := c.fragmentURL("morgan", sdhash, "master.m3u8")
	s.Require().NoError(err)
	s.Equal(
		fmt.Sprintf("http://transcoder.com/streams/%v/%v", sdhash, "master.m3u8"),
		u,
	)
}

func (s *clientSuite) Test_fragmentURL() {
	cl := dummyRedirectClient("http://t0.lbry.tv:18081/streams/bec50ab288153ed03b0eb8dafd814daf19a187e07f8da4ad91cf778f5c39ac74d9d92ad6e3ebf2ddb6b7acea3cb8893a/master.m3u8")
	dstPath := path.Join(s.assetsPath, "Test_fragmentURL")
	c := New(Configure().HTTPClient(cl).VideoPath(dstPath).LogLevel(Dev))

	u, err := c.fragmentURL("morgan", "0b8dfc049b2165fad5829aca24f2ddfae3acef8d73bc5e04ff8b932fce9fc463dc6cf3e638413f04536638d2e7218427", "master.m3u8")
	s.Require().Error(err)
	s.Regexp("remote sd hash mismatch", err.Error())
	s.Equal("", u)

	u, err = c.fragmentURL("morgan", "azazaz", "master.m3u8")
	s.Require().Error(err)
	s.Regexp("remote sd hash mismatch", err.Error())
	s.Equal("", u)

	u, err = c.fragmentURL("vanquish-trailer-(2021)-morgan-freeman,#b7b150d1bbca4650ad4ab921dd8d424bf77c1141", "azazaz", "master.m3u8")
	s.Require().Error(err)
	s.Regexp("remote sd hash mismatch", err.Error())
	s.Equal("", u)

	u, err = c.fragmentURL(
		"vanquish-trailer-(2021)-morgan-freeman,#b7b150d1bbca4650ad4ab921dd8d424bf77c1141",
		"bec50ab288153ed03b0eb8dafd814daf19a187e07f8da4ad91cf778f5c39ac74d9d92ad6e3ebf2ddb6b7acea3cb8893a",
		"master.m3u8")
	s.Require().NoError(err)
	s.Equal("http://t0.lbry.tv:18081/streams/bec50ab288153ed03b0eb8dafd814daf19a187e07f8da4ad91cf778f5c39ac74d9d92ad6e3ebf2ddb6b7acea3cb8893a/master.m3u8", u)
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
func (s *clientSuite) populateHLSPlaylist(dstPath, sdHash string) {
	err := os.MkdirAll(path.Join(dstPath, sdHash), os.ModePerm)
	s.Require().NoError(err)

	srcPath, _ := filepath.Abs("./testdata")
	storage := storage.Local(srcPath)
	ls, err := storage.Open("dummystream")
	s.Require().NoError(err)
	err = ls.Dive(
		func(rootPath ...string) ([]byte, error) {
			if path.Ext(rootPath[len(rootPath)-1]) == ".m3u8" {
				return ioutil.ReadFile(path.Join(rootPath...))
			}
			return make([]byte, 10000), nil
		},
		func(data []byte, name string) error {
			return ioutil.WriteFile(path.Join(dstPath, sdHash, name), data, os.ModePerm)
		},
	)
	s.Require().NoError(err)
}
