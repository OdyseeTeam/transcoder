package tower

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/router"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/tower/metrics"
	"github.com/prometheus/client_golang/prometheus"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/valyala/fasthttp"
)

const (
	TWorkerWait          = "worker_wait"
	TRequestPick         = "request_pick"
	TRequestSweep        = "request_sweep"
	TRequestHeartbeat    = "request_heartbeat"
	TWorkerStatus        = "worker_status"
	TWorkerStatusTimeout = "worker_status_timeout"
	TRequestTimeoutBase  = "request_timeout_base"
)

type ServerConfig struct {
	rmqAddr                 string
	dsn                     string
	workDir, workDirUploads string
	httpServerBind          string
	httpServerURL           string
	log                     logging.KVLogger
	videoManager            *manager.VideoManager
	timings                 map[string]time.Duration
	state                   *State
	devMode                 bool
}

type Server struct {
	*ServerConfig
	rpc      *towerRPC
	registry *workerRegistry
	stopChan chan struct{}

	httpServer *fasthttp.Server

	backCh *amqp.Channel
}

type worker struct {
	id        string
	capacity  int
	available int
	lastSeen  time.Time
}

type workerRegistry struct {
	sync.RWMutex
	workers   map[string]*worker
	capacity  int
	available int
}

type Timings map[string]time.Duration

func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		rmqAddr:        "amqp://guest:guest@localhost/",
		workDir:        ".",
		httpServerBind: ":18080",
		log:            logging.NoopKVLogger{},
		state:          &State{lock: sync.RWMutex{}, Requests: map[string]*RunningRequest{}},
		timings:        defaultTimings(),
	}
}

func (c *ServerConfig) Logger(logger logging.KVLogger) *ServerConfig {
	c.log = logger
	return c
}

func (c *ServerConfig) Timings(t Timings) *ServerConfig {
	for k, v := range t {
		c.timings[k] = v
	}
	return c
}

func (c *ServerConfig) HttpServer(bind, url string) *ServerConfig {
	c.httpServerBind = bind
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	c.httpServerURL = url
	return c
}

func (c *ServerConfig) VideoManager(manager *manager.VideoManager) *ServerConfig {
	c.videoManager = manager
	return c
}

func (c *ServerConfig) WorkDir(workDir string) *ServerConfig {
	c.workDir = workDir
	return c
}

func (c *ServerConfig) State(state *State) *ServerConfig {
	c.state = state
	return c
}

func (c *ServerConfig) RMQAddr(addr string) *ServerConfig {
	c.rmqAddr = addr
	return c
}

func (c *ServerConfig) DSN(addr string) *ServerConfig {
	c.dsn = addr
	return c
}

func (c *ServerConfig) DevMode() *ServerConfig {
	c.devMode = true
	return c
}

func NewServer(config *ServerConfig) (*Server, error) {
	var err error
	s := Server{
		ServerConfig: config,
		registry:     &workerRegistry{workers: map[string]*worker{}},
		stopChan:     make(chan struct{}),
	}

	s.workDirUploads = path.Join(s.workDir, "uploads")
	s.rpc, err = newTowerRPC(s.rmqAddr, s.dsn, s.log)
	if err != nil {
		return nil, err
	}
	s.state.StartDump()

	return &s, nil
}

func (s *Server) StartAll() error {
	if s.videoManager == nil {
		return errors.New("VideoManager is not configured")
	}

	s.rpc.declareQueues()

	// go s.startWatchingWorkerStatus()
	if err := s.startForwardingRequests(s.videoManager.Requests()); err != nil {
		return err
	}
	if err := s.startHttpServer(); err != nil {
		return err
	}
	return nil
}

func (s *Server) StopAll() {
	close(s.stopChan)
	s.rpc.consumer.StopConsuming("", false)
	s.rpc.consumer.Disconnect()
	s.rpc.publisher.StopPublishing()
	s.state.StopDump()
}

func (s *Server) startForwardingRequests(requests <-chan *manager.TranscodingRequest) error {
	activeTasks, err := s.rpc.startConsumingWorkRequests()
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case at := <-activeTasks:
				go func() {
					tr := <-requests
					mtt := MsgTranscodingTask{
						Ref:    tr.SDHash,
						URL:    tr.URI,
						SDHash: tr.SDHash,
					}
					// Sending actual transcoding task to worker
					at.SendPayload(mtt)
					// Timing out a task means it will be shipped back to the queue again
					ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
					s.manageTask(ctx, at, tr)
					defer cancel()
				}()
			case <-s.stopChan:
				return
			}
		}
	}()

	return nil
}

func (s *Server) manageTask(ctx context.Context, at *activeTask, tr *manager.TranscodingRequest) {
	for {
		select {
		case p := <-at.progress:
			s.log.Info("progress received", "progress", p.Percent, "stage", p.Stage)
		case <-ctx.Done():
			s.log.Error("active task timed out")
			if tr != nil {
				tr.Release()
			}
			return
		case d := <-at.done:
			m := d.RemoteStream.Manifest
			if m == nil {
				s.log.Error("remote stream missing manifest", "task", fmt.Sprintf("%+v", at))
				metrics.TranscodingRequestsErrors.With(prometheus.Labels{
					metrics.LabelWorkerName: at.workerID,
					metrics.LabelStage:      "add",
				}).Inc()
				return
			}
			metrics.TranscodingRequestsRunning.With(prometheus.Labels{metrics.LabelWorkerName: at.workerID}).Dec()
			if _, err := s.videoManager.Library().AddRemoteStream(*d.RemoteStream); err != nil {
				s.log.Error("error adding remote stream", "err", err)
				metrics.TranscodingRequestsErrors.With(prometheus.Labels{
					metrics.LabelWorkerName: at.workerID,
					metrics.LabelStage:      "add",
				}).Inc()
				if tr != nil {
					tr.Reject()
				}
				return
			}
			if tr != nil {
				tr.Complete()
			}
			metrics.TranscodingRequestsDone.With(prometheus.Labels{metrics.LabelWorkerName: at.workerID}).Inc()
			return
		case <-s.stopChan:
			return
		}

	}
}

func (s *Server) startHttpServer() error {
	router := router.New()

	metrics.RegisterMetrics()
	manager.AttachVideoHandler(router, "", s.videoManager.Library().Path(), s.videoManager, s.log)

	s.log.Info("starting tower http server", "addr", s.httpServerBind, "url", s.httpServerURL)
	l, err := net.Listen("tcp", s.httpServerBind)
	if err != nil {
		return err
	}

	// TODO: Cleanup middleware attachment.
	httpServer := &fasthttp.Server{
		Handler: manager.MetricsMiddleware(manager.CORSMiddleware(router.Handler)),
		Name:    "tower",
	}
	// s.upAddr = l.Addr().String()

	s.httpServer = httpServer
	go func() {
		go httpServer.Serve(l)
		<-s.stopChan
		s.log.Info("shutting down tower http server", "addr", s.httpServerBind, "url", s.httpServerURL)
		httpServer.Shutdown()
	}()

	return nil
}

func defaultTimings() Timings {
	return Timings{
		TWorkerWait:          1000 * time.Millisecond,
		TRequestPick:         500 * time.Millisecond,
		TRequestSweep:        10 * time.Second,
		TWorkerStatusTimeout: 10 * time.Second,
		TRequestTimeoutBase:  1 * time.Minute,
		// Below are used by both server and worker
		TRequestHeartbeat: 10 * time.Second,
		TWorkerStatus:     300 * time.Millisecond,
	}
}
