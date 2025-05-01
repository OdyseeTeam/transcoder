package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OdyseeTeam/transcoder/library"
	"github.com/OdyseeTeam/transcoder/pkg/resolve"

	"github.com/Pallinder/go-randomdata"
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
	s3addr        string
	httpServerURL string
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
	{"v0_s000000.ts", 2_000_000},
	{"v1_s000000.ts", 760_000},
	{"v2_s000000.ts", 300_000},
	{"v3_s000000.ts", 120_000},
}

func TestClientSuite(t *testing.T) {
	suite.Run(t, new(clientSuite))
}

func (s *clientSuite) SetupSuite() {
	s.s3addr = "http://localhost:9000/transcoded"
	s.httpServerURL = "http://localhost:8080"
}

func (s *clientSuite) TestPlayFragment() {
	c := New(
		Configure().
			VideoPath(path.Join(s.T().TempDir(), "TestPlayFragment")).
			Server(s.httpServerURL).
			LogLevel(Dev).
			RemoteServer(s.s3addr),
	)

	// Request stream and wait until it's available.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	wait := time.NewTicker(1000 * time.Millisecond)
Waiting:
	for {
		select {
		case <-ctx.Done():
			s.FailNow("transcoding is taking too long")
		case <-wait.C:
			rr := httptest.NewRecorder()
			_, err := c.PlayFragment(streamURL, streamSDHash, "master.m3u8", rr, httptest.NewRequest(http.MethodGet, "/video", nil))
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
			sz, err := c.PlayFragment(streamURL, streamSDHash, tc.name, rr, httptest.NewRequest(http.MethodGet, "/", nil))
			s.Require().NoError(err)
			s.Require().Equal(http.StatusOK, rr.Result().StatusCode)
			rbody, err := io.ReadAll(rr.Result().Body)
			s.Require().NoError(err)
			if tc.size > 0 {
				// Different transcoding runs produce slightly different files.
				s.InDelta(tc.size, len(rbody), float64(tc.size)*0.2)
				s.EqualValues(sz, len(rbody))
			} else {
				absPath, err := filepath.Abs(filepath.Join("./testdata", "known-stream", tc.name))
				s.Require().NoError(err)
				tbody, err := os.ReadFile(absPath)
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

			switch {
			case strings.HasSuffix(tc.name, ".m3u8"):
				s.Equal("application/x-mpegurl", rr.Result().Header.Get("content-type"))
			case strings.HasSuffix(tc.name, ".ts"):
				s.Equal("video/mp2t", rr.Result().Header.Get("content-type"))
			case strings.HasSuffix(tc.name, ".png"):
				s.Equal("image/png", rr.Result().Header.Get("content-type"))
			}
		})
	}
	for _, tc := range streamFragmentCases {
		s.Run(tc.name, func() {
			rr := httptest.NewRecorder()
			_, err := c.PlayFragment(streamURL, streamSDHash, tc.name, rr, httptest.NewRequest(http.MethodGet, "/", nil))
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
			Server(s.httpServerURL).
			LogLevel(Dev).
			RemoteServer(s.s3addr),
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
			_, err := c.PlayFragment(streamURL, streamSDHash, "master.m3u8", rr, httptest.NewRequest(http.MethodGet, "/video", nil))
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
			sz, err := c.PlayFragment(streamURL, streamSDHash, tc.name, rr, httptest.NewRequest(http.MethodGet, "/", nil))
			s.Require().NoError(err)
			s.NotZero(sz)
		})
	}

	c.remoteServer = "http://localhost:13131"
	c.cache.Clear()

	for _, tc := range streamFragmentCases {
		s.Run(tc.name, func() {
			rr := httptest.NewRecorder()
			sz, err := c.PlayFragment(streamURL, streamSDHash, tc.name, rr, httptest.NewRequest(http.MethodGet, "/", nil))
			s.Error(err)
			s.Zero(sz)
		})
	}
}

func (s *clientSuite) TestRestoreCache() {
	dstPath := s.T().TempDir()

	c := New(Configure().VideoPath(dstPath))

	cvDirs := []string{}
	for range [10]int{} {
		sdHash := randomdata.Alphanumeric(96)
		library.PopulateHLSPlaylist(s.T(), dstPath, sdHash)
		cvDirs = append(cvDirs, sdHash)
	}

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
