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

	"github.com/draganm/miniotest"
	"github.com/lbryio/transcoder/library"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/pkg/resolve"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/tower"

	"github.com/karrick/godirwalk"
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
	library.LibraryTestHelper
	tower   *tower.ServerLite
	cleanup func() error
	s3addr  string
}

var streamFragmentCases = []struct {
	name string
	size int64
}{
	{"master.m3u8", 0},
	{"v0.m3u8", 0},
	{"v1.m3u8", 0},
	{"v2.m3u8", 0},
	{"v3.m3u8", 0},
	{"v0_s000000.ts", 1800000},
	{"v1_s000000.ts", 300000},
	{"v2_s000000.ts", 300000},
	{"v3_s000000.ts", 170000},
}

func TestClientSuite(t *testing.T) {
	suite.Run(t, new(clientSuite))
}

func (s *clientSuite) SetupSuite() {
	s3addr, s3cleanup, err := miniotest.StartEmbedded()
	s.Require().NoError(err)
	s3drv, err := storage.InitS3Driver(
		storage.S3Configure().
			Name("tower-test").
			Endpoint("http://"+s3addr).
			Region("us-east-1").
			Credentials("minioadmin", "minioadmin").
			Bucket("storage-s3-test").
			DisableSSL(),
	)
	s.Require().NoError(err)

	s.Require().NoError(s.SetupLibraryDB())
	lib := library.New(library.Config{
		DB:      s.DB,
		Storage: s3drv,
		Log:     zapadapter.NewKV(nil),
	})
	_, err = lib.AddChannel("@specialoperationstest#3", "")
	s.Require().NoError(err)
	mgr := manager.NewManager(lib, 0)

	tower, err := tower.NewTestTowerLite(s.T(), s3drv, mgr)
	s.Require().NoError(err)
	err = tower.StartAll()
	s.Require().NoError(err)

	s.tower = tower
	s.cleanup = s3cleanup
	s.s3addr = s3addr
}

func (s *clientSuite) TearDownSuite() {
	s.Require().NoError(s.cleanup())
	s.Require().NoError(s.TearDownLibraryDB())
}

func (s *clientSuite) TestPlayFragment() {
	c := New(
		Configure().
			VideoPath(path.Join(s.T().TempDir(), "TestPlayFragment")).
			Server(s.tower.HttpServerURL).
			LogLevel(Dev).
			RemoteServer("http://" + s.s3addr + "/storage-s3-test"),
	)

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
				s.Require().ErrorIs(err, resolve.ErrTranscodingUnderway)
			} else {
				break Waiting
			}
		}
	}
	cancel()

	// Compare stream playlists and fragments against known content or size.

	for _, tc := range streamFragmentCases {
		s.Run(tc.name, func() {
			rr := httptest.NewRecorder()
			err := c.PlayFragment(streamURL, streamSDHash, tc.name, rr, httptest.NewRequest(http.MethodGet, "/", nil))
			s.Require().NoError(err)
			s.Require().Equal(http.StatusOK, rr.Result().StatusCode)
			rbody, err := ioutil.ReadAll(rr.Result().Body)
			s.Require().NoError(err)
			if tc.size > 0 {
				// Different transcoding runs produce slightly different files.
				s.InDelta(tc.size, len(rbody), float64(tc.size)*0.1)
			} else {
				absPath, err := filepath.Abs(filepath.Join("./testdata", "known-stream", tc.name))
				s.Require().NoError(err)
				tbody, err := ioutil.ReadFile(absPath)
				s.Require().NoError(err)
				s.Equal(strings.TrimRight(string(tbody), "\n"), strings.TrimRight(string(rbody), "\n"))
			}
			if tc.name == MasterPlaylistName {
				s.Equal(cacheHeaderHit, rr.Result().Header.Get(cacheHeaderName))
			} else {
				s.Equal(cacheHeaderMiss, rr.Result().Header.Get(cacheHeaderName))
			}
			s.Equal("public, max-age=21239", rr.Result().Header.Get(cacheControlHeaderName))
			s.Equal("GET, OPTIONS", rr.Result().Header.Get("Access-Control-Allow-Methods"))
			s.Equal("*", rr.Result().Header.Get("Access-Control-Allow-Origin"))

			if strings.HasSuffix(tc.name, ".m3u8") {
				s.Equal("application/x-mpegurl", rr.Result().Header.Get("content-type"))
			} else if strings.HasSuffix(tc.name, ".ts") {
				s.Equal("video/mp2t", rr.Result().Header.Get("content-type"))
			} else if strings.HasSuffix(tc.name, ".png") {
				s.Equal("image/png", rr.Result().Header.Get("content-type"))
			}
		})
	}
	for _, tc := range streamFragmentCases {
		s.Run(tc.name, func() {
			rr := httptest.NewRecorder()
			err := c.PlayFragment(streamURL, streamSDHash, tc.name, rr, httptest.NewRequest(http.MethodGet, "/", nil))
			s.Require().NoError(err)
			s.Equal(cacheHeaderHit, rr.Result().Header.Get(cacheHeaderName))
			s.Equal("public, max-age=21239", rr.Result().Header.Get(cacheControlHeaderName))
		})
	}
}

func (s *clientSuite) TestPlayFragmentStorageDown() {
	c := New(
		Configure().
			VideoPath(path.Join(s.T().TempDir(), "TestPlayFragment")).
			Server(s.tower.HttpServerURL).
			LogLevel(Dev).
			RemoteServer("http://" + s.s3addr + "/storage-s3-test"),
	)

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
				s.Require().ErrorIs(err, resolve.ErrTranscodingUnderway)
			} else {
				break Waiting
			}
		}
	}
	cancel()

	for _, tc := range streamFragmentCases {
		s.Run(tc.name, func() {
			rr := httptest.NewRecorder()
			err := c.PlayFragment(streamURL, streamSDHash, tc.name, rr, httptest.NewRequest(http.MethodGet, "/", nil))
			s.Require().NoError(err)
		})
	}

	c.remoteServer = "http://localhost:63333"
	c.cache.Clear()

	for _, tc := range streamFragmentCases {
		s.Run(tc.name, func() {
			rr := httptest.NewRecorder()
			err := c.PlayFragment(streamURL, streamSDHash, tc.name, rr, httptest.NewRequest(http.MethodGet, "/", nil))
			s.Error(err)
		})
	}
}

func (s *clientSuite) TestRestoreCache() {
	dstPath := s.T().TempDir()

	c := New(Configure().VideoPath(dstPath))

	cvDirs := []string{}
	for range [10]int{} {
		sdHash := randomString(96)
		library.PopulateHLSPlaylist(s.T(), dstPath, sdHash)
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
	cl := dummyRedirectClient(fmt.Sprintf("remote://storage1/%v/", sdhash))
	c := New(Configure().HTTPClient(cl))
	u, err := c.getFragmentURL("morgan", sdhash, "master.m3u8")
	s.Require().NoError(err)
	s.Equal(
		fmt.Sprintf("%v/%v/%v", defaultRemoteServer, sdhash, "master.m3u8?origin=storage1"),
		u,
	)
}

func (s *clientSuite) TestGetPlaybackPath() {
	url := "morgan"
	sdhash := "bec50ab288153ed03b0eb8dafd814daf19a187e07f8da4ad91cf778f5c39ac74d9d92ad6e3ebf2ddb6b7acea3cb8893a"
	cl := dummyRedirectClient(fmt.Sprintf("remote://storage1/%v/master.m3u8", sdhash))
	c := New(Configure().HTTPClient(cl))
	u := c.GetPlaybackPath("morgan", sdhash)
	s.Equal(
		fmt.Sprintf("%s/%s/%s", url, sdhash, "master.m3u8"),
		u,
	)
}

func (s *clientSuite) Test_getFragmentURL() {
	cl := dummyRedirectClient("remote://storage1/sdhash/")
	dstPath := s.T().TempDir()
	c := New(Configure().HTTPClient(cl).VideoPath(dstPath).LogLevel(Dev))

	u, err := c.getFragmentURL(
		"vanquish-trailer-(2021)-morgan-freeman,#b7b150d1bbca4650ad4ab921dd8d424bf77c1141",
		"sdhash",
		"master.m3u8")
	s.Require().NoError(err)
	s.Equal("https://cache-us.transcoder.odysee.com/sdhash/master.m3u8?origin=storage1", u)
}

func randomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	b := make([]rune, n)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}
