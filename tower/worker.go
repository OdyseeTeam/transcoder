package tower

import (
	"errors"
	"sync"
	"time"

	"github.com/fasthttp/router"
	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/tower/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

type WorkerConfig struct {
	id             string
	rmqAddr        string
	poolSize       int
	workDir        string
	httpServerBind string
	log            logging.KVLogger
	timings        map[string]time.Duration
	s3             *storage.S3Driver
}

type Worker struct {
	*WorkerConfig
	rpc                 *workerRPC
	processor           Processor
	stopChan            chan struct{}
	activePipelines     map[string]struct{}
	activePipelinesLock sync.Mutex
	bgTasks             *sync.WaitGroup
	httpServer          *fasthttp.Server
}

type Processor interface {
	Process(stop chan struct{}, t workerTask)
}

type requestHandler func(MsgTranscodingTask)

func DefaultWorkerConfig() *WorkerConfig {
	return &WorkerConfig{
		id:      "unknown",
		rmqAddr: "amqp://guest:guest@localhost/",
		log:     logging.NoopKVLogger{},
		timings: defaultTimings(),
	}
}

// NewWorker creates a new worker connecting to AMQP server.
func NewWorker(config *WorkerConfig) (*Worker, error) {
	enc, err := encoder.NewEncoder(encoder.Configure().Log(config.log))
	if err != nil {
		return nil, err
	}
	if config.s3 == nil {
		return nil, errors.New("s3 configuration not set")
	}
	w := Worker{
		WorkerConfig:        config,
		stopChan:            make(chan struct{}),
		activePipelines:     map[string]struct{}{},
		activePipelinesLock: sync.Mutex{},
		bgTasks:             &sync.WaitGroup{},
	}

	rpc, err := newrpc(w.rmqAddr, w.log)
	if err != nil {
		return nil, err
	}
	w.rpc = &workerRPC{rpc: rpc}
	if config.id == "" {
		return nil, errors.New("no worker ID set")
	}
	w.id = config.id
	w.rpc.id = config.id

	p, err := newPipeline(config.workDir, w.id, config.s3, enc, w.log)
	if err != nil {
		return nil, err
	}
	w.processor = p

	w.log.Info("worker configured", "id", w.id)
	if w.httpServerBind != "" {
		if err := w.startHttpServer(); err != nil {
			return nil, err
		}
	}
	return &w, nil
}

func (c *WorkerConfig) RMQAddr(addr string) *WorkerConfig {
	c.rmqAddr = addr
	return c
}

func (c *WorkerConfig) WorkDir(workDir string) *WorkerConfig {
	c.workDir = workDir
	return c
}

func (c *WorkerConfig) S3Driver(s3 *storage.S3Driver) *WorkerConfig {
	c.s3 = s3
	return c
}

func (c *WorkerConfig) Timings(t Timings) *WorkerConfig {
	for k, v := range t {
		c.timings[k] = v
	}
	return c
}

func (c *WorkerConfig) Logger(logger logging.KVLogger) *WorkerConfig {
	c.log = logger
	return c
}

func (c *WorkerConfig) PoolSize(poolSize int) *WorkerConfig {
	c.poolSize = poolSize
	return c
}

func (c *WorkerConfig) WorkerID(id string) *WorkerConfig {
	c.id = id
	return c
}

func (c *WorkerConfig) HttpServerBind(bind string) *WorkerConfig {
	c.httpServerBind = bind
	return c
}

func (c *Worker) handleRequest(wt workerTask) {
	mtt := wt.payload
	log := logging.AddLogRef(c.log, mtt.SDHash).With("url", mtt.URL)

	log.Info("task received, starting", "msg", mtt)
	c.processor.Process(c.stopChan, wt)
}

func (c *Worker) startHttpServer() error {
	router := router.New()

	metrics.RegisterWorkerMetrics()
	router.GET("/metrics", fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler()))

	c.log.Info("starting worker http server", "addr", c.httpServerBind)
	httpServer := &fasthttp.Server{
		Handler:          router.Handler,
		Name:             "worker",
		DisableKeepalive: true,
	}

	c.httpServer = httpServer
	go func() {
		err := httpServer.ListenAndServe(c.httpServerBind)
		if err != nil {
			c.log.Error("http server error", "err", err)
			close(c.stopChan)
		}
	}()
	go func() {
		<-c.stopChan
		c.log.Info("shutting down worker http server", "addr", c.httpServerBind)
		httpServer.Shutdown()
	}()

	return nil
}

func (c *Worker) StartWorkers() error {
	taskChan, err := c.rpc.startWorking(c.poolSize)
	if err != nil {
		return err
	}
	metrics.WorkerCapability.WithLabelValues(metrics.WorkerStatusCapacity).Set(float64(c.poolSize))
	go func() {
		for {
			select {
			case wt := <-taskChan:
				c.handleRequest(wt)
			case <-c.stopChan:
				return
			}
		}
	}()
	return nil
}

func (c *Worker) Stop() {
	close(c.stopChan)
	c.rpc.consumer.Disconnect()
	c.rpc.publisher.StopPublishing()
}
