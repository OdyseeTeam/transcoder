package tower

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/lbryio/transcoder/library"
	ldb "github.com/lbryio/transcoder/library/db"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/pkg/resolve"
	"github.com/lbryio/transcoder/storage"

	"github.com/draganm/miniotest"
	"github.com/stretchr/testify/suite"
)

type towerSuite struct {
	suite.Suite
	TowerTestHelper
	library.LibraryTestHelper

	tower     *towerRPC
	s3addr    string
	s3cleanup func() error
	s3drv     *storage.S3Driver
}

func TestTowerSuite(t *testing.T) {
	suite.Run(t, new(towerSuite))
}

func (s *towerSuite) SetupTest() {
	var err error
	s.Require().NoError(s.SetupTowerDB())
	s.Require().NoError(s.SetupLibraryDB())

	s.tower = NewTestTowerRPC(s.T(), s.TowerDB)
	s.Require().NoError(s.tower.declareQueues())

	s.s3addr, s.s3cleanup, err = miniotest.StartEmbedded()
	s.Require().NoError(err)

	s.s3drv, err = storage.InitS3Driver(
		storage.S3Configure().
			Name("tower-test").
			Endpoint(s.s3addr).
			Region("us-east-1").
			Credentials("minioadmin", "minioadmin").
			Bucket("storage-s3-test").
			DisableSSL(),
	)
	s.Require().NoError(err)
}

func (s *towerSuite) TearDownTest() {
	s.NoError(s.tower.deleteQueues())
	s.tower.publisher.Close()
	s.tower.consumer.Close()

	s.Require().NoError(s.TearDownTowerDB())
	s.Require().NoError(s.TearDownLibraryDB())
}

func (s *towerSuite) TestSuccess() {
	if testing.Short() {
		s.T().Skip("skipping TestPipelineSuccess")
	}

	srvWorkDir := s.T().TempDir()
	cltWorkDir := s.T().TempDir()
	streamURL := "lbry://@specialoperationstest#3/fear-of-death-inspirational#a"
	trReq, err := resolve.ResolveStream(streamURL)
	s.Require().NoError(err)

	lib := library.New(library.Config{
		DB:  s.DB,
		Log: zapadapter.NewKV(nil),
	})
	_, err = lib.AddChannel("@specialoperationstest#3", ldb.ChannelPriorityNormal)
	s.Require().NoError(err)
	mgr := manager.NewManager(lib, 0)

	srv, err := NewServer(
		DefaultServerConfig().
			Logger(zapadapter.NewKV(nil)).
			HttpServer(":18080", "http://localhost:18080").
			VideoManager(mgr).
			DB(s.TowerDB).
			WorkDir(srvWorkDir).
			DevMode(),
	)
	s.Require().NoError(err)

	err = srv.StartAll()
	s.Require().NoError(err)

	wrk, err := NewWorker(DefaultWorkerConfig().
		S3Driver(s.s3drv).
		PoolSize(3).
		WorkDir(cltWorkDir).
		Logger(zapadapter.NewKV(nil)),
	)
	s.Require().NoError(err)
	wrk.StartWorkers()

	httpURL := fmt.Sprintf("%v/api/v2/video/%v", srv.HttpServerURL, url.PathEscape(streamURL))
	resp, err := http.Get(httpURL)
	s.Require().NoError(err)
	s.Require().EqualValues(http.StatusAccepted, resp.StatusCode, httpURL)

	var v ldb.Video
	for v.ID == 0 {
		v, err = srv.videoManager.Library().GetVideo(trReq.SDHash)
		if err != nil {
			s.Require().ErrorIs(err, sql.ErrNoRows)
		}
		time.Sleep(500 * time.Millisecond)
	}
	s.Equal(trReq.SDHash, v.SDHash)
	s.Equal(s.s3drv.Name(), v.Storage)
	_, err = s.s3drv.GetFragment(v.TID, library.MasterPlaylistName)
	s.NoError(err, "remote path does not exist: %s/%s", v.TID, library.MasterPlaylistName)
}
