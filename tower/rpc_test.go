package tower

import (
	"math/rand"
	"path"
	"testing"
	"time"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/video"

	"github.com/Pallinder/go-randomdata"
	"github.com/stretchr/testify/suite"
)

type towerSuite struct {
	suite.Suite
}

func TestTowerSuite(t *testing.T) {
	suite.Run(t, new(towerSuite))
}

// func (s *towerSuite) SetupTest() {
// 	srv, err := NewUploadServer()
// 	s.Require().NoError(err)

// 	err = srv.startConsumingInbox(func(_ rabbitmq.Delivery) bool { fmt.Println("discarding"); return true })
// 	s.Require().NoError(err)
// 	srv.Stop()
// }

func (s *towerSuite) TestHeartbeats() {
	// messagesToSend := 50
	workersNum := 10

	poolSizes := make([]int, workersNum)
	poolTotal := 0
	workers := make([]*Worker, workersNum)

	srv, err := NewServer(DefaultServerConfig().
		Logger(zapadapter.NewKV(logging.Create("rpc-test.server", logging.Dev).Desugar())).
		Timings(Timings{
			TWorkerStatus: 50 * time.Millisecond,
			TWorkerWait:   100 * time.Millisecond,
			TRequestPick:  100 * time.Millisecond,
		}).
		DevMode(),
	)
	s.Require().NoError(err)

	s.Require().NoError(err)

	go srv.startWatchingWorkerStatus()
	err = srv.startConsumingWorkerStatus()
	s.Require().NoError(err)

	for i := 0; i < workersNum; i++ {
		poolSizes[i] = rand.Intn(99) + 1
		poolTotal += poolSizes[i]
		w, err := NewWorker(DefaultWorkerConfig().
			PoolSize(poolSizes[i]).
			Timings(Timings{TWorkerStatus: 200 * time.Millisecond}),
		)
		s.Require().NoError(err)
		w.log = zapadapter.NewKV(logging.Create("rpc-test.worker", logging.Dev).Desugar())
		go w.StartSendingStatus()
		workers[i] = w
	}

	time.Sleep(5 * time.Second)
	s.Equal(poolTotal, srv.registry.capacity)
	s.Equal(poolTotal, srv.registry.available)

	for _, w := range workers {
		w.Stop()
	}

	time.Sleep(5 * time.Second)
	s.Equal(0, srv.registry.capacity)
	s.Equal(0, srv.registry.available)
}

func (s *towerSuite) TestQueueEagerness() {
	requestsToSend := 200
	workersNum := 3
	poolSize := 5

	srv, err := NewServer(
		DefaultServerConfig().
			Logger(zapadapter.NewKV(logging.Create("rpc-test.server", logging.Dev).Desugar())).
			Timings(Timings{
				// A combination of very fast workers state update and slightly longer (but not too long)
				// request pick interval is needed here, much shorter than in actual production use.
				TWorkerStatus: 50 * time.Millisecond,
				TRequestPick:  250 * time.Millisecond,
			}).
			DevMode(),
	)
	s.Require().NoError(err)

	go srv.startWatchingWorkerStatus()
	err = srv.startConsumingWorkerStatus()
	s.Require().NoError(err)

	requestsChan := make(chan *manager.TranscodingRequest, 1000)
	for i := 0; i < requestsToSend; i++ {
		requestsChan <- &manager.TranscodingRequest{URI: "lbry://" + randomdata.SillyName(), SDHash: randomdata.Alphanumeric(96)}
	}

	for i := 0; i < workersNum; i++ {
		wrk, err := NewWorker(DefaultWorkerConfig().
			PoolSize(poolSize).
			Timings(Timings{TWorkerStatus: 100 * time.Millisecond}).
			Logger(zapadapter.NewKV(logging.Create("rpc-test.worker", logging.Dev).Desugar())),
		)
		s.Require().NoError(err)
		go wrk.StartSendingStatus()
		wrk.requestHandler = func(msg MsgRequest) {
			wrk.log.Info("started working", "msg", msg)
			time.Sleep(20 * time.Minute)
		}
		wrk.StartWorkers()
	}

	go srv.startRequestsPicking(requestsChan)

	time.Sleep(5 * time.Second)

	s.Equal(15, srv.registry.capacity)
	s.Equal(0, srv.registry.available)
	s.GreaterOrEqual(len(requestsChan), requestsToSend-workersNum*poolSize-10)
}

func (s *towerSuite) TestPipelineSuccess() {
	if testing.Short() {
		s.T().Skip("skipping TestPipelineSuccess")
	}

	libPath := s.T().TempDir()
	srvWorkDir := s.T().TempDir()
	cltWorkDir := s.T().TempDir()
	url := "lbry://@specialoperationstest#3/fear-of-death-inspirational#a"
	trReq, err := manager.ResolveRequest(url)
	s.Require().NoError(err)

	vdb := db.OpenTestDB()
	s.Require().NoError(vdb.MigrateUp(video.InitialMigration))

	libCfg := video.Configure().
		LocalStorage(storage.Local(libPath)).
		DB(vdb)

	mgr := manager.NewManager(video.NewLibrary(libCfg), 0)
	srv, err := NewServer(
		DefaultServerConfig().
			Logger(zapadapter.NewKV(logging.Create("rpc-test.server", logging.Dev).Desugar())).
			HttpServer(":18080", "http://localhost:18080/").
			Timings(Timings{
				TWorkerStatus: 50 * time.Millisecond,
				TRequestPick:  250 * time.Millisecond,
			}).
			VideoManager(mgr).
			WorkDir(srvWorkDir).
			DevMode(),
	)
	s.Require().NoError(err)

	go srv.startRequestSweep()
	go srv.startWatchingWorkerStatus()
	err = srv.startConsumingWorkerStatus()
	s.Require().NoError(err)
	err = srv.startConsumingResponses()
	s.Require().NoError(err)
	err = srv.startHttpServer()
	s.Require().NoError(err)

	requestsChan := make(chan *manager.TranscodingRequest, 10)
	go srv.startRequestsPicking(requestsChan)
	requestsChan <- trReq

	wrk, err := NewWorker(DefaultWorkerConfig().
		PoolSize(3).
		Timings(Timings{TWorkerStatus: 100 * time.Millisecond}).
		WorkDir(cltWorkDir).
		Logger(zapadapter.NewKV(logging.Create("rpc-test.worker", logging.Dev).Desugar())),
	)
	s.Require().NoError(err)

	go wrk.StartSendingStatus()
	wrk.StartWorkers()

	var v *video.Video
	for v == nil {
		v, _ = srv.videoManager.Library().Get(trReq.SDHash)
		time.Sleep(500 * time.Millisecond)
	}
	s.Equal(trReq.SDHash, v.SDHash)
	s.DirExists(path.Join(libPath, v.Path))
	s.Greater(v.Size, int64(0))
	s.NotEmpty(v.Channel)
	s.NotEmpty(v.Checksum)

	s.NoDirExists(path.Join(wrk.workDir, dirStreams, trReq.SDHash))
	s.NoDirExists(path.Join(wrk.workDir, dirTranscoded, trReq.SDHash))
}

func (s *towerSuite) TestPipelineFailure() {
}

func (s *towerSuite) TestPipelineWorkerTimeout() {
	if testing.Short() {
		s.T().Skip("skipping TestPipelineWorkerTimeout")
	}

	libPath := s.T().TempDir()
	srvWorkDir := s.T().TempDir()
	cltWorkDir := s.T().TempDir()
	url := "lbry://@specialoperationstest#3/fear-of-death-inspirational#a"
	// url := "lbry://@passionforfood#3/how-to-make-the-perfect-pita-pocket#a"
	trReq, err := manager.ResolveRequest(url)
	s.Require().NoError(err)

	vdb := db.OpenTestDB()
	s.Require().NoError(vdb.MigrateUp(video.InitialMigration))

	libCfg := video.Configure().
		LocalStorage(storage.Local(libPath)).
		DB(vdb)

	mgr := manager.NewManager(video.NewLibrary(libCfg), 0)
	srv, err := NewServer(
		DefaultServerConfig().
			Logger(zapadapter.NewKV(logging.Create("rpc-test.server", logging.Dev).Desugar())).
			HttpServer(":18080", "http://localhost:18080/").
			Timings(Timings{
				TWorkerStatus:       50 * time.Millisecond,
				TRequestPick:        250 * time.Millisecond,
				TRequestHeartbeat:   10 * time.Millisecond,
				TRequestSweep:       300 * time.Millisecond,
				TRequestTimeoutBase: 10 * time.Millisecond,
			}).
			VideoManager(mgr).
			WorkDir(srvWorkDir).
			DevMode(),
	)
	s.Require().NoError(err)
	go func() {
		err = srv.StartAll()
		s.Require().NoError(err)
	}()

	requestsChan := make(chan *manager.TranscodingRequest, 10)
	go srv.startRequestsPicking(requestsChan)
	requestsChan <- trReq

	wrk, err := NewWorker(DefaultWorkerConfig().
		PoolSize(3).
		Timings(Timings{
			TRequestHeartbeat: 30 * time.Second,
		}).
		WorkDir(cltWorkDir).
		Logger(zapadapter.NewKV(logging.Create("rpc-test.worker", logging.Dev).Desugar())),
	)
	s.Require().NoError(err)

	go wrk.StartSendingStatus()

	// baseHandler := wrk.requestHandler
	// firstCall := true
	// wrk.requestHandler = func(msg MsgRequest) {

	// 	baseHandler(msg)
	// }
	wrk.StartWorkers()

	time.Sleep(30 * time.Second)
	s.Fail("fail")
}

func (s *towerSuite) TestWorkerRejectsDuplicateRefs() {
	wrk, err := NewWorker(DefaultWorkerConfig().
		PoolSize(3).
		Timings(Timings{
			TRequestHeartbeat: 30 * time.Second,
		}).
		Logger(zapadapter.NewKV(logging.Create("rpc-test.worker", logging.Dev).Desugar())),
	)
	s.Require().NoError(err)
	wrk.requestHandler = func(msg MsgRequest) {
		wrk.log.Info("started working", "msg", msg)
		time.Sleep(20 * time.Minute)
	}
	go wrk.StartSendingStatus()
	wrk.StartWorkers()

	srv, err := NewServer(
		DefaultServerConfig().
			Logger(zapadapter.NewKV(logging.Create("rpc-test.server", logging.Dev).Desugar())).
			Timings(Timings{
				TWorkerStatus: 50 * time.Millisecond,
				TRequestPick:  250 * time.Millisecond,
			}).
			DevMode(),
	)
	s.Require().NoError(err)

	go srv.startWatchingWorkerStatus()
	err = srv.startConsumingWorkerStatus()
	s.Require().NoError(err)

	requestsChan := make(chan *manager.TranscodingRequest, 1000)
	go srv.startRequestsPicking(requestsChan)
	req := &manager.TranscodingRequest{URI: "lbry://" + randomdata.SillyName(), SDHash: randomdata.Alphanumeric(96)}
	requestsChan <- req
	delete(srv.state.Requests, req.SDHash)
	requestsChan <- req

	time.Sleep(4 * time.Second)

	s.Len(wrk.workers, 2)
}
