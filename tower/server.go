package tower

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
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

type ServerConfig struct {
	rmqAddr        string
	workDir        string
	httpServerBind string
	httpServerURL  string
	log            logging.KVLogger
	videoManager   *manager.VideoManager
	timings        map[string]time.Duration
	state          *State
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

const (
	TWorkerWait          = "worker_wait"
	TRequestPick         = "request_pick"
	TRequestSweep        = "request_sweep"
	TWorkerStatus        = "worker_status"
	TWorkerStatusTimeout = "worker_status_timeout"
)

func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		rmqAddr:        "amqp://guest:guest@localhost/",
		httpServerBind: ":18080",
		log:            logging.NoopKVLogger{},
		state:          &State{lock: sync.RWMutex{}, Requests: map[string]*RunningRequest{}},
		timings: Timings{
			TWorkerWait:          1000 * time.Millisecond,
			TRequestPick:         500 * time.Millisecond,
			TRequestSweep:        10 * time.Second,
			TWorkerStatus:        200 * time.Millisecond,
			TWorkerStatusTimeout: 10 * time.Second,
		},
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

func (c *ServerConfig) RestoreState(file string) *ServerConfig {
	state, err := RestoreState(file)
	if err != nil {
		panic(err)
	}
	c.state = state
	return c
}

func (c *ServerConfig) RMQAddr(addr string) *ServerConfig {
	c.rmqAddr = addr
	return c
}

func NewServer(config *ServerConfig) (*Server, error) {
	server := Server{
		ServerConfig: config,
		registry:     &workerRegistry{workers: map[string]*worker{}},
		stopChan:     make(chan interface{}),
	}

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
			fmt.Printf("message returned from server: %+v\n", r.ReplyText)
		}
	}()

	return &server, nil
}

func (s *Server) StartAll() error {
	if s.videoManager == nil {
		return errors.New("VideoManager is not configured")
	}

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
}

func (s *Server) InitiateRequest(incomingRequest *manager.TranscodingRequest) error {
	rr := &RunningRequest{
		URL:                incomingRequest.URI,
		SDHash:             incomingRequest.SDHash,
		Ref:                incomingRequest.SDHash,
		Stage:              StagePending,
		CallbackToken:      crypto.RandString(32),
		TsStarted:          time.Now(),
		transcodingRequest: incomingRequest,
	}
	s.state.lock.Lock()
	s.state.Requests[incomingRequest.SDHash] = rr
	s.state.lock.Unlock()

	s.log.Info("initiating request", "url", rr.URL, "sd_hash", rr.SDHash)
	return s.publishRequest(rr)
}

func (s *Server) declareQueues() error {
	_, err := s.backCh.QueueDeclare(requestsQueueName, true, false, false, false, amqp.Table{}) // amqp.Table{"x-max-length": 3})
	return err
}

func (s *Server) deleteQueues() error {
	if _, err := s.backCh.QueueDelete(requestsQueueName, false, false, false); err != nil {
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
		CallbackURL: s.httpServerURL + rr.Ref,
		Key:         rr.CallbackToken,
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
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
			s.log.Info("no workers available, waiting")
			time.Sleep(s.timings[TWorkerWait])
		}
		s.log.Info("workers available, starting", "available", s.registry.available)
		select {
		case <-s.stopChan:
			return
		case r := <-requests:
			if r != nil {
				s.log.Info("picked up transcoding request", "url", r.URI)
				if err := s.InitiateRequest(r); err != nil {
					r.Release()
					s.log.Error("failed to send request", "url", r.URI, "err", err)
				}
			}
		}
		time.Sleep(s.timings[TRequestPick])
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
			s.state.lock.RLock()
			rr, ok := s.state.Requests[ls.SDHash]
			s.state.lock.RUnlock()
			if !ok {
				s.log.Error("uploader callback received but no corresponding request found", "ref", ls.SDHash)
				return
			}
			rr.Uploaded = true

			if _, err := s.videoManager.Library().AddLightLocalStream(rr.URL, "", ls); err != nil {
				rr.Stage = StageFailed
				rr.Error = err.Error()
				s.log.Error("failed to add stream", "ref", ls.SDHash, "err", err)
				return
			}
			rr.Stage = StageCompleted
		},
	)
	manager.AttachVideoHandler(router, "", s.videoManager.Library().Path(), s.videoManager, s.log.With("manager.http"))

	s.log.Info("starting tower http server", "addr", s.httpServerBind, "url", s.httpServerURL)
	l, err := net.Listen("tcp", s.httpServerBind)
	if err != nil {
		return err
	}

	// TODO: Cleanup middleware attachment.
	httpServer := &fasthttp.Server{
		Handler: manager.MetricsMiddleware(manager.CORSMiddleware(router.Handler)),
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
	pulse := time.NewTicker(s.timings[TRequestSweep])

	sweep := func() {
		for ref, rr := range s.state.Requests {
			if rr.Stage == StageFailed || rr.TimedOut() {
				rr.FailedAttempts++
				if rr.FailedAttempts >= maxFailedAttempts {
					if rr.transcodingRequest != nil {
						rr.transcodingRequest.Reject()
					}
					delete(s.state.Requests, ref)
					s.log.Info(
						"failure number exceeded, discarding request",
						"url", rr.URL, "ref", ref, "worker_id", rr.WorkerID)
				} else {
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
			s.state.lock.Lock()
			sweep()
			s.state.lock.Unlock()
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
	)
}

func (s *Server) startConsumingResponses() error {
	return s.consumer.StartConsuming(
		func(d rabbitmq.Delivery) rabbitmq.Action {
			ll := s.log.With("type", d.Type)
			rr, err := s.authenticateMessage(d)
			if err != nil {
				ll.Info("failed to authenticate message", "err", err)
				return rabbitmq.NackDiscard
			}

			rr.WorkerID = getWorkerID(d)

			switch WorkerMessageType(d.Type) {
			case tHeartbeat:
				rr.TsHeartbeat = d.Timestamp
				return rabbitmq.Ack
			case tPipelineUpdate:
				var msg pipelineProgress
				err := json.Unmarshal(d.Body, &msg)
				if err != nil {
					ll.Error("can't unmarshal received message", "err", err, "type", d.Type)
					return rabbitmq.NackDiscard
				}
				rr.Progress = msg.Percent
				if msg.Error != nil {
					rr.Error = msg.Error.Error()
				}
				rr.TsUpdated = d.Timestamp
				rr.Stage = msg.Stage

				ll.Debug("progress received ", "worker_id", rr.WorkerID, "sd_hash", rr.SDHash, "stage", rr.Stage, "progress", rr.Progress, "url", rr.URL)

				// Clean streams that have been saved into database alread
				if rr.Stage == StageCompleted && time.Since(rr.TsUpdated) > 24*time.Hour {
					s.state.lock.Lock()
					delete(s.state.Requests, rr.Ref)
					s.state.lock.Unlock()
				}
				return rabbitmq.Ack
			case tPipelineGone:
				rr.Stage = StageFailed
				return rabbitmq.Ack
			default:
				ll.Warn("unknown message type received", "body", d.Body, "type", d.Type)
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
		rabbitmq.WithConsumeOptionsQuorum,
	)
}

func (s *Server) authenticateMessage(d rabbitmq.Delivery) (*RunningRequest, error) {
	ll := s.log.With("type", d.Type)
	ref := getRequestRef(d)
	if ref == "" {
		return nil, errors.New("no request referred")
	}
	req, ok := s.state.Requests[ref]
	if !ok {
		ll.Warn("referred request not found", "ref", ref)
		return nil, errors.New("no request referred")
	}
	if req.CallbackToken != getWorkerKey(d) {
		ll.Warn("worker key mismatch", "ref", ref, "remote_key", getWorkerKey(d), "local_key", req.CallbackToken)
		return nil, errors.New("worker key mismatch")
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
