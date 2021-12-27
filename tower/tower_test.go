package tower

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/draganm/miniotest"
	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/tower/queue"
	"github.com/lbryio/transcoder/video"

	"github.com/stretchr/testify/suite"
)

type towerSuite struct {
	suite.Suite
	tower     *towerRPC
	s3addr    string
	s3cleanup func() error
	s3drv     *storage.S3Driver
	db        *sql.DB
	dbCleanup func() error
}

func TestTowerSuite(t *testing.T) {
	suite.Run(t, new(towerSuite))
}

func (s *towerSuite) SetupTest() {
	db, dbCleanup, err := queue.CreateTestDB()
	s.Require().NoError(err)
	s.db = db
	s.dbCleanup = dbCleanup
	s.Require().NoError(err)
	s.tower = CreateTestTowerRPC(s.T(), db)

	s.Require().NoError(s.tower.deleteQueues())
	s.Require().NoError(s.tower.declareQueues())

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

func (s *towerSuite) TearDownTest() {
	s.NoError(s.tower.deleteQueues())
	s.tower.publisher.StopPublishing()
	s.tower.consumer.StopConsuming("", true)
}

func (s *towerSuite) TestSuccess() {
	if testing.Short() {
		s.T().Skip("skipping TestPipelineSuccess")
	}

	libPath := s.T().TempDir()
	srvWorkDir := s.T().TempDir()
	cltWorkDir := s.T().TempDir()
	streamURL := "lbry://@specialoperationstest#3/fear-of-death-inspirational#a"
	trReq, err := manager.ResolveRequest(streamURL)
	s.Require().NoError(err)

	vdb := db.OpenTestDB()
	s.Require().NoError(vdb.MigrateUp(video.InitialMigration))

	libCfg := video.Configure().
		LocalStorage(storage.Local(libPath)).
		RemoteStorage(s.s3drv).
		DB(vdb)

	manager.LoadConfiguredChannels([]string{"@specialoperationstest#3"}, []string{}, []string{})
	mgr := manager.NewManager(video.NewLibrary(libCfg), 0)

	srv, err := NewServer(
		DefaultServerConfig().
			Logger(zapadapter.NewKV(nil)).
			HttpServer(":18080", "http://localhost:18080/").
			VideoManager(mgr).
			DB(s.db).
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

	httpURL := fmt.Sprintf("%vapi/v2/video/%v", srv.httpServerURL, url.PathEscape(streamURL))
	fmt.Println(httpURL)
	resp, err := http.Get(httpURL)
	s.Require().NoError(err)
	s.Require().EqualValues(http.StatusAccepted, resp.StatusCode)

	var v *video.Video
	for v == nil {
		v, _ = srv.videoManager.Library().Get(trReq.SDHash)
		time.Sleep(500 * time.Millisecond)
	}
	s.Equal(trReq.SDHash, v.SDHash)
	_, err = s.s3drv.GetFragment(v.RemotePath, storage.MasterPlaylistName)
	s.NoError(err, "remote path does not exist: %s/%s", v.RemotePath, storage.MasterPlaylistName)
}
