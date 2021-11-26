package tower

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/rabbitmq/amqp091-go"
	"github.com/wagslane/go-rabbitmq"
)

type WorkerConfig struct {
	rmqAddr  string
	poolSize int
	workDir  string
	log      logging.KVLogger
	timings  map[string]time.Duration
}

type Worker struct {
	*WorkerConfig
	publisher           rabbitmq.Publisher
	consumer            *rabbitmq.Consumer
	id                  string
	processor           Processor
	stopChan            chan struct{}
	workers             chan struct{}
	activePipelines     map[string]struct{}
	activePipelinesLock sync.Mutex
	requestHandler      requestHandler
	bgTasks             *sync.WaitGroup
}

type Processor interface {
	Process(stop chan struct{}, t *task) taskControl
}

type requestHandler func(MsgRequest)

func DefaultWorkerConfig() *WorkerConfig {
	return &WorkerConfig{
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

	worker := Worker{
		WorkerConfig:        config,
		stopChan:            make(chan struct{}),
		activePipelines:     map[string]struct{}{},
		activePipelinesLock: sync.Mutex{},
		bgTasks:             &sync.WaitGroup{},
	}
	p, err := newPipeline(config.workDir, enc, worker.log)
	if err != nil {
		return nil, err
	}
	worker.processor = p
	worker.requestHandler = worker.handleRequest

	if worker.id == "" {
		worker.generateID()
	}

	publisher, err := rabbitmq.NewPublisher(worker.rmqAddr, amqp091.Config{})
	if err != nil {
		return nil, err
	}
	worker.publisher = publisher

	returns := publisher.NotifyReturn()
	go func() {
		for r := range returns {
			worker.log.Warn(fmt.Sprintf("message returned from server: %+v\n", r))
		}
	}()

	worker.workers = make(chan struct{}, worker.poolSize)
	for i := 0; i < worker.poolSize; i++ {
		worker.workers <- struct{}{}
	}

	return &worker, nil
}

func (c *WorkerConfig) RMQAddr(addr string) *WorkerConfig {
	c.rmqAddr = addr
	return c
}

func (c *WorkerConfig) WorkDir(workDir string) *WorkerConfig {
	c.workDir = workDir
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

func (c *Worker) SetID(id string) {
	if c.id != "" {
		return
	}
	c.id = id
}

func (c *Worker) generateID() {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	c.SetID(fmt.Sprintf("%v-%v-%v", hostname, time.Now().UnixNano(), randomdata.Number(100, 200)))
}

func (c *Worker) handleRequest(req MsgRequest) {
	log := logging.AddLogRef(c.log, req.SDHash).With("url", req.URL)

	task := &task{url: req.URL, sdHash: req.SDHash, callbackURL: req.CallbackURL, token: req.Key}
	pulse := time.NewTicker(c.timings[TRequestHeartbeat])

	log.Info("task received, starting", "msg", req)

	tc := c.processor.Process(c.stopChan, task)
	for {
		select {
		case <-c.stopChan:
			task.cleanup()
			return
		case <-tc.TaskDone:
			err := <-tc.Errc
			if err != nil {
				log.Error("processor failed", "err", err)
				c.Respond(req, tPipelineError, mPipelineError{Error: err.Error()})
			}
			task.cleanup()
			return
		case p := <-tc.Progress:
			c.Respond(req, tPipelineUpdate, p)
			log.Debug("processor progress received", "progress", p)
		case <-pulse.C:
			c.Respond(req, tHeartbeat, struct{}{})
		}
	}
}

func (c *Worker) StartWorkers() {
	// Interval for unsubscribing from the queue when no workers available
	pulse := time.NewTicker(c.timings[TWorkerStatus] / 2)

	go func() {
		running := false
		for {
			select {
			case <-c.stopChan:
				return
			case <-pulse.C:
				if running && len(c.workers) == 0 {
					c.log.Debug("destroying consumer")
					c.DestroyConsumer()
					running = false
				} else if !running && len(c.workers) > 0 {
					c.log.Debug("initializing consumer")
					if err := c.InitConsumer(); err != nil {
						c.log.Error("consumer init failure", err)
						return
					}
					if err := c.startConsuming(); err != nil {
						c.log.Error("consumer start failure", err)
						return
					}
					running = true
				}
			}
		}
	}()
}

func (c *Worker) Respond(msg MsgRequest, mtype WorkerMessageType, message interface{}) error {
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return c.publisher.Publish(
		body,
		[]string{responsesQueueName},
		rabbitmq.WithPublishOptionsType(string(mtype)),
		rabbitmq.WithPublishOptionsHeaders(rabbitmq.Table{
			headerWorkerID:   c.id,
			headerRequestKey: msg.Key,
			headerRequestRef: msg.Ref,
		}),
		rabbitmq.WithPublishOptionsTimestamp(time.Now()),
		rabbitmq.WithPublishOptionsContentType("application/json"),
		rabbitmq.WithPublishOptionsExchange("transcoder"),
		rabbitmq.WithPublishOptionsMandatory,
		rabbitmq.WithPublishOptionsPersistentDelivery,
	)
}

func (c *Worker) Stop() {
	close(c.stopChan)
	c.DestroyConsumer()
}

func (c *Worker) InitConsumer() error {
	consumer, err := rabbitmq.NewConsumer(c.rmqAddr, amqp091.Config{})
	if err != nil {
		return err
	}
	c.consumer = &consumer
	return nil
}

func (c *Worker) DestroyConsumer() {
	if c.consumer != nil {
		c.consumer.StopConsuming(c.id, false)
		c.consumer.Disconnect()
		c.consumer = nil
	}
}

func (c *Worker) startConsuming() error {
	return c.consumer.StartConsuming(
		func(d rabbitmq.Delivery) rabbitmq.Action {
			var msg MsgRequest
			err := json.Unmarshal(d.Body, &msg)
			if err != nil {
				c.log.Warn("botched message received", "err", err)
				return rabbitmq.NackDiscard
			}
			log := logging.AddLogRef(c.log, msg.SDHash)
			log.Info("message received", "msg", msg)

			c.activePipelinesLock.Lock()
			defer c.activePipelinesLock.Unlock()
			if _, ok := c.activePipelines[msg.SDHash]; ok {
				log.Info("duplicate transcoding request received")
				return rabbitmq.NackDiscard
			}
			c.activePipelines[msg.Ref] = struct{}{}

			select {
			case <-c.stopChan:
				return rabbitmq.NackRequeue
			case <-c.workers:
				log.Debug("checked out worker")
				go func() {
					defer func() {
						c.activePipelinesLock.Lock()
						defer c.activePipelinesLock.Unlock()
						delete(c.activePipelines, msg.Ref)
						c.workers <- struct{}{}
						log.Debug("checked worker back in")
					}()
					c.requestHandler(msg)
				}()
				return rabbitmq.Ack
			default:
				log.Debug("worker pool exhausted")
				return rabbitmq.NackRequeue
			}
		},
		requestsQueueName,
		[]string{requestsQueueName},
		rabbitmq.WithConsumeOptionsConcurrency(1),
		rabbitmq.WithConsumeOptionsBindingExchangeDurable,
		rabbitmq.WithConsumeOptionsBindingExchangeName("transcoder"),
		rabbitmq.WithConsumeOptionsBindingExchangeKind("direct"),
		rabbitmq.WithConsumeOptionsConsumerName(c.id),
		rabbitmq.WithConsumeOptionsQueueDurable,
	)
}

func (c *Worker) StartSendingStatus() {
	pulse := time.NewTicker(c.timings[TWorkerStatus])
	for {
		select {
		case <-c.stopChan:
			return
		case <-pulse.C:
			if err := c.sendStatus(); err != nil {
				c.log.Error("worker status send error", "err", err)
			}
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (c *Worker) sendStatus() error {
	msg := MsgStatus{
		ID:        c.id,
		Capacity:  c.poolSize,
		Available: len(c.workers),
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	// c.log.Debug("sending worker status", "msg", msg)

	return c.publisher.Publish(
		body,
		[]string{workerStatusQueueName},
		rabbitmq.WithPublishOptionsTimestamp(time.Now()),
		rabbitmq.WithPublishOptionsContentType("application/json"),
		rabbitmq.WithPublishOptionsExchange("transcoder"),
		rabbitmq.WithPublishOptionsExpiration("0"),
	)
}
