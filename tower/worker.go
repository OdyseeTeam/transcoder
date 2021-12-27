package tower

import (
	"errors"
	"sync"
	"time"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/storage"
)

type WorkerConfig struct {
	id       string
	rmqAddr  string
	poolSize int
	workDir  string
	log      logging.KVLogger
	timings  map[string]time.Duration
	s3       *storage.S3Driver
}

type Worker struct {
	*WorkerConfig
	rpc                 *workerRPC
	processor           Processor
	stopChan            chan struct{}
	workers             chan struct{}
	activePipelines     map[string]struct{}
	activePipelinesLock sync.Mutex
	requestHandler      requestHandler
	bgTasks             *sync.WaitGroup
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
	p, err := newPipeline(config.workDir, config.s3, enc, w.log)
	if err != nil {
		return nil, err
	}
	w.processor = p

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
	w.log.Info("worker configured", "id", w.id)
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

func (c *Worker) handleRequest(wt workerTask) {
	mtt := wt.payload
	log := logging.AddLogRef(c.log, mtt.SDHash).With("url", mtt.URL)

	log.Info("task received, starting", "msg", mtt)
	c.processor.Process(c.stopChan, wt)
	// for {
	// 	select {
	// 	case <-c.stopChan:
	// 		task.cleanup()
	// 		return
	// 	case <-tc.TaskDone:
	// 		err := <-tc.Errc
	// 		if err != nil {
	// 			log.Error("processor failed", "err", err)
	// 			wt.errors <- err
	// 		} else {
	// 			wt.done
	// 		}
	// 		task.cleanup()
	// 		return
	// 	case p := <-tc.Progress:
	// 		log.Debug("processor progress received", "progress", p)
	// 		wt.progress <- p
	// 	}
	// }
}

func (c *Worker) StartWorkers() error {
	taskChan, err := c.rpc.startWorking(c.poolSize)
	if err != nil {
		return err
	}
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
