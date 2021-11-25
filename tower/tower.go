package tower

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/router"
	"github.com/lbryio/lbry.go/extras/crypto"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/uploader"
	"github.com/lbryio/transcoder/storage"
	"github.com/streadway/amqp"

	"github.com/rabbitmq/amqp091-go"
	"github.com/valyala/fasthttp"
	"github.com/wagslane/go-rabbitmq"
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
	publisher rabbitmq.Publisher
	consumer  rabbitmq.Consumer
	registry  *workerRegistry
	stopChan  chan interface{}

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

func (c *ServerConfig) DevMode() *ServerConfig {
	c.devMode = true
	return c
}

func NewServer(config *ServerConfig) (*Server, error) {
	server := Server{
		ServerConfig: config,
		registry:     &workerRegistry{workers: map[string]*worker{}},
		stopChan:     make(chan interface{}),
	}

	server.workDirUploads = path.Join(server.workDir, "uploads")

	publisher, err := rabbitmq.NewPublisher(server.rmqAddr, amqp091.Config{})
	if err != nil {
		return nil, err
	}
	server.publisher = publisher

	consumer, err := rabbitmq.NewConsumer(server.rmqAddr, amqp091.Config{})
	if err != nil {
		return nil, err
	}
	server.consumer = consumer

	amqpConn, err := amqp.DialConfig(server.rmqAddr, amqp.Config{})
	if err != nil {
		return nil, err
	}
	ch, err := amqpConn.Channel()
	if err != nil {
		return nil, err
	}
	server.backCh = ch

	returns := publisher.NotifyReturn()
	go func() {
		for r := range returns {
			server.log.Warn(fmt.Sprintf("message returned from server: %+v\n", r))
		}
	}()
	server.state.StartDump()

	return &server, nil
}

func (s *Server) StartAll() error {
	if s.videoManager == nil {
		return errors.New("VideoManager is not configured")
	}

	if s.devMode {
		s.deleteQueues()
	}
	s.declareQueues()

	go s.startRequestSweep()
	go s.startWatchingWorkerStatus()
	if err := s.startConsumingWorkerStatus(); err != nil {
		return err
	}
	if err := s.startConsumingResponses(); err != nil {
		return err
	}
	if err := s.startHttpServer(); err != nil {
		return err
	}
	s.startRequestsPicking(s.videoManager.Requests())
	return nil
}

func (s *Server) StopAll() {
	close(s.stopChan)
	s.consumer.StopConsuming(responsesConsumerName, false)
	s.consumer.Disconnect()
	s.state.StopDump()
}

func (s *Server) InitiateRequest(request *manager.TranscodingRequest) error {
	rr := &RunningRequest{
		Ref:                request.SDHash,
		URL:                request.URI,
		SDHash:             request.SDHash,
		Channel:            request.ChannelURI,
		TsCreated:          time.Now(),
		transcodingRequest: request,
		Stage:              StagePending,
		CallbackToken:      crypto.RandString(32),
	}
	s.state.lock.Lock()
	defer s.state.lock.Unlock()
	if _, ok := s.state.Requests[rr.Ref]; ok {
		return ErrRequestExists
	}
	s.state.Requests[rr.Ref] = rr

	logging.AddLogRef(s.log, rr.SDHash).Info("initiating request", "url", rr.URL)
	return s.publishRequest(rr)
}

func (s *Server) declareQueues() error {
	if _, err := s.backCh.QueueDeclare(requestsQueueName, true, false, false, false, amqp.Table{}); err != nil {
		return err
	}
	if _, err := s.backCh.QueueDeclare(responsesQueueName, true, false, false, false, amqp.Table{}); err != nil {
		return err
	}
	if _, err := s.backCh.QueueDeclare(workerStatusQueueName, true, false, false, false, amqp.Table{}); err != nil {
		return err
	}
	return nil
}

func (s *Server) deleteQueues() error {
	if _, err := s.backCh.QueueDelete(requestsQueueName, false, false, false); err != nil {
		return err
	}
	if _, err := s.backCh.QueueDelete(responsesQueueName, false, false, false); err != nil {
		return err
	}
	if _, err := s.backCh.QueueDelete(workerStatusQueueName, false, false, false); err != nil {
		return err
	}
	return nil
}

func (s *Server) publishRequest(rr *RunningRequest) error {
	msg := MsgRequest{
		Ref:         rr.SDHash,
		URL:         rr.URL,
		SDHash:      rr.SDHash,
		CallbackURL: s.httpServerURL + "callback/" + rr.Ref,
		Key:         rr.CallbackToken,
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	logging.AddLogRef(s.log, rr.SDHash).Debug("publishing request", "request", msg)
	return s.publisher.Publish(
		body,
		[]string{requestsQueueName},
		rabbitmq.WithPublishOptionsHeaders(rabbitmq.Table{"type": "request.new"}),
		rabbitmq.WithPublishOptionsContentType("application/json"),
		rabbitmq.WithPublishOptionsExchange("transcoder"),
		rabbitmq.WithPublishOptionsMandatory,
		rabbitmq.WithPublishOptionsPersistentDelivery,
	)
}

func (s *Server) startRequestsPicking(requests <-chan *manager.TranscodingRequest) {
	for {
		for s.registry.available == 0 {
			s.log.Debug("no workers available, waiting")
			time.Sleep(s.timings[TWorkerWait])
		}
		s.log.Info("workers available, starting", "available", s.registry.available)
		select {
		case <-s.stopChan:
			s.log.Info("stopped picking up requests")
			return
		case r := <-requests:
			if r != nil {
				log := logging.AddLogRef(s.log, r.SDHash).With("url", r.URI)
				log.Info("picked up transcoding request")
				if err := s.InitiateRequest(r); err != nil {
					if errors.Is(err, ErrRequestExists) {
						r.Reject()
					} else {
						r.Release()
					}
					log.Info("failed to initiate request", "err", err)
				}
			}
		}
	}
}

func (s *Server) startHttpServer() error {
	router := router.New()
	uploader.AttachFileHandler(router, "/callback", s.workDir,
		func(ctx *fasthttp.RequestCtx) bool {
			ref := ctx.UserValue("ref").(string)
			token := ctx.UserValue("token").(string)
			if rr, ok := s.state.Requests[ref]; ok {
				return rr.CallbackToken == token
			}
			return false
		},
		func(ls storage.LightLocalStream) {
			log := logging.AddLogRef(s.log, ls.SDHash)
			s.state.lock.RLock()
			rr, ok := s.state.Requests[ls.SDHash]
			s.state.lock.RUnlock()
			if !ok {
				log.Error("uploader callback received but no corresponding request found")
				return
			}
			rr.Uploaded = true

			if _, err := s.videoManager.Library().AddLightLocalStream(rr.URL, rr.Channel, ls); err != nil {
				rr.Stage = StageFailedFatally
				rr.Error = err.Error()
				log.Error("failed to add stream", "url", rr.URL, "err", err)
				return
			}
			rr.Stage = StageCompleted
			if rr.transcodingRequest != nil {
				rr.transcodingRequest.Complete()
			}
			log.Info("upload received", "url", rr.URL)
		},
	)
	manager.AttachVideoHandler(router, "", s.videoManager.Library().Path(), s.videoManager, s.log)

	s.log.Info("starting tower http server", "addr", s.httpServerBind, "url", s.httpServerURL)
	l, err := net.Listen("tcp", s.httpServerBind)
	if err != nil {
		return err
	}

	// TODO: Cleanup middleware attachment.
	httpServer := &fasthttp.Server{
		Handler:            manager.MetricsMiddleware(manager.CORSMiddleware(router.Handler)),
		Name:               "tower",
		MaxRequestBodySize: 10 * 1024 * 1024 * 1024,
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

func (s *Server) startRequestSweep() {
	time.Sleep(5 * s.timings[TRequestHeartbeat])
	pulse := time.NewTicker(s.timings[TRequestSweep])

	sweep := func() {
		s.state.lock.Lock()
		defer s.state.lock.Unlock()
		for _, rr := range s.state.Requests {
			log := logging.AddLogRef(s.log, rr.SDHash)
			if rr.Stage == StageDone || rr.Stage == StageCompleted || rr.Stage == StageFailedFatally || rr.Stage == StagePending {
				continue
			}
			log.Debug(
				"is request timed out?",
				"is", rr.TimedOut(s.timings[TRequestTimeoutBase]), "base", s.timings[TRequestTimeoutBase],
				"st", rr.TsStarted, "hb", rr.TsHeartbeat, "upd", rr.TsUpdated,
			)
			if rr.Stage == StageFailed || rr.TimedOut(s.timings[TRequestTimeoutBase]) {
				rr.FailedAttempts++
				if rr.FailedAttempts >= maxFailedAttempts {
					if rr.transcodingRequest != nil {
						rr.transcodingRequest.Reject()
					}
					log.Info(
						"failure number exceeded, discarding request",
						// "url", rr.URL, "ref", ref, "worker_id", rr.WorkerID,
						// "failures_count", rr.FailedAttempts, "max_failures", maxFailedAttempts,
						"request", rr,
					)
					rr.Stage = StageFailedFatally
				} else {
					log.Info(
						"re-publishing request",
						// "stage", rr.Stage, "updated", rr.TsUpdated, "started", rr.TsStarted,
						"request", rr,
					)
					rr.Stage = StagePending
					s.publishRequest(rr)
				}
			}
		}
	}
	for {
		select {
		case <-s.stopChan:
			return
		case <-pulse.C:
			sweep()
		}
	}
}

func (s *Server) startWatchingWorkerStatus() {
	pulse := time.NewTicker(s.timings[TWorkerStatus])
	for {
		select {
		case <-s.stopChan:
			return
		case <-pulse.C:
			s.registry.Lock()
			s.registry.capacity = 0
			s.registry.available = 0
			for id, c := range s.registry.workers {
				if time.Since(c.lastSeen) > s.timings[TWorkerStatusTimeout] {
					delete(s.registry.workers, id)
					continue
				}
				s.registry.capacity += c.capacity
				s.registry.available += c.available
			}
			s.registry.Unlock()
			// s.log.Debug("registry updated", "capacity", s.registry.capacity, "available", s.registry.available, "workers", len(s.registry.workers))
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (s *Server) startConsumingWorkerStatus() error {
	return s.consumer.StartConsuming(
		func(d rabbitmq.Delivery) rabbitmq.Action {
			msg := MsgStatus{}
			err := json.Unmarshal(d.Body, &msg)
			if err != nil {
				s.log.Warn("botched message received", "err", err)
				return rabbitmq.NackDiscard
			}
			s.registry.Lock()
			defer s.registry.Unlock()
			c := &worker{
				id:        msg.ID,
				capacity:  msg.Capacity,
				available: msg.Available,
				lastSeen:  d.Timestamp,
			}
			s.registry.workers[msg.ID] = c
			return rabbitmq.Ack
		},
		workerStatusQueueName,
		[]string{workerStatusQueueName},
		rabbitmq.WithConsumeOptionsConcurrency(1),
		rabbitmq.WithConsumeOptionsBindingExchangeName("transcoder"),
		rabbitmq.WithConsumeOptionsBindingExchangeKind("direct"),
		// rabbitmq.WithConsumeOptionsConsumerName(responsesConsumerName),
		rabbitmq.WithConsumeOptionsBindingExchangeDurable,
		rabbitmq.WithConsumeOptionsQueueDurable,
	)
}

func (s *Server) startConsumingResponses() error {
	return s.consumer.StartConsuming(
		func(d rabbitmq.Delivery) rabbitmq.Action {
			log := s.log.With("type", d.Type)
			rr, err := s.authenticateMessage(d)
			if err != nil {
				log.Info("failed to authenticate message", "err", err)
				return rabbitmq.NackDiscard
			}
			log = logging.AddLogRef(log, rr.SDHash)

			// TODO: Verify worker ID match
			rr.WorkerID = getWorkerID(d)

			if rr.TsStarted.IsZero() {
				rr.TsStarted = d.Timestamp
			}

			switch WorkerMessageType(d.Type) {
			case tHeartbeat:
				rr.TsHeartbeat = d.Timestamp
				return rabbitmq.Ack
			case tPipelineUpdate:
				var msg mPipelineProgress
				err := json.Unmarshal(d.Body, &msg)
				if err != nil {
					log.Error("can't unmarshal received message", "err", err, "type", d.Type)
					return rabbitmq.NackDiscard
				}
				rr.Progress = msg.Percent
				rr.TsUpdated = d.Timestamp
				rr.Stage = msg.Stage
				log.Debug("progress received ", "msg", msg)

				return rabbitmq.Ack
			case tPipelineError:
				var msg mPipelineError
				err := json.Unmarshal(d.Body, &msg)
				if err != nil {
					log.Error("can't unmarshal received message", "err", err)
					return rabbitmq.NackDiscard
				}
				rr.Error = msg.Error
				rr.Stage = StageFailed
				log.Debug("request failed", "msg", msg)
				return rabbitmq.Ack
			default:
				log.Warn("unknown message type received", "body", d.Body)
				return rabbitmq.NackDiscard
			}
		},
		responsesQueueName,
		[]string{responsesQueueName},
		rabbitmq.WithConsumeOptionsConcurrency(3),
		rabbitmq.WithConsumeOptionsBindingExchangeDurable,
		rabbitmq.WithConsumeOptionsBindingExchangeName("transcoder"),
		rabbitmq.WithConsumeOptionsBindingExchangeKind("direct"),
		// rabbitmq.WithConsumeOptionsConsumerName(responsesConsumerName),
		rabbitmq.WithConsumeOptionsQueueDurable,
	)
}

func (s *Server) authenticateMessage(d rabbitmq.Delivery) (*RunningRequest, error) {
	ref := getRequestRef(d)
	if ref == "" {
		return nil, errors.New("no request referred")
	}
	log := logging.AddLogRef(s.log, ref).With("type", d.Type)
	s.state.lock.RLock()
	defer s.state.lock.RUnlock()
	req, ok := s.state.Requests[ref]
	if !ok {
		log.Warn("referred request not found", "ref", ref)
		return nil, errors.New("no request referred")
	}
	if req.CallbackToken != getWorkerKey(d) {
		log.Info("worker key mismatch", "remote_key", getWorkerKey(d), "local_key", req.CallbackToken)
		// return nil, errors.New("worker key mismatch")
	}
	return req, nil
}

func getWorkerKey(d rabbitmq.Delivery) string {
	v, ok := d.Headers[headerRequestKey].(string)
	if !ok {
		return ""
	}
	return v
}

func getWorkerID(d rabbitmq.Delivery) string {
	v, ok := d.Headers[headerWorkerID].(string)
	if !ok {
		return ""
	}
	return v
}

func getRequestRef(d rabbitmq.Delivery) string {
	v, ok := d.Headers[headerRequestRef].(string)
	if !ok {
		return ""
	}
	return v
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
