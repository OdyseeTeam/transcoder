package tower

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/rabbitmq/amqp091-go"
	"github.com/wagslane/go-rabbitmq"
)

const (
	TPipelineHeartbeat = "processor_heartbeat"
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
	publisher      rabbitmq.Publisher
	consumer       *rabbitmq.Consumer
	id             string
	processor      Processor
	stopChan       chan interface{}
	workers        chan struct{}
	requestHandler requestHandler
}

type Processor interface {
	Process(stop chan interface{}, t *task) (<-chan interface{}, <-chan pipelineProgress)
}

type requestHandler func(MsgRequest)

func DefaultWorkerConfig() *WorkerConfig {
	return &WorkerConfig{
		rmqAddr: "amqp://guest:guest@localhost/",
		log:     logging.NoopKVLogger{},
		timings: Timings{
			TPipelineHeartbeat: 30 * time.Second,
			TWorkerStatus:      500 * time.Millisecond,
		},
	}
}

// NewWorker creates a new worker connecting to AMQP server.
func NewWorker(config *WorkerConfig) (*Worker, error) {
	enc, err := encoder.NewEncoder(encoder.Configure().Log(config.log))
	if err != nil {
		return nil, err
	}
	p, err := newPipeline(config.workDir, enc, config.timings[TPipelineHeartbeat])
	if err != nil {
		return nil, err
	}
	worker := Worker{
		WorkerConfig: config,
		processor:    p,
		stopChan:     make(chan interface{}),
	}
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
			fmt.Printf("worker got return from server: %+v\n", r)
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
	ll := c.log.With("url", req.URL)
	task := &task{url: req.URL, callbackURL: req.CallbackURL, token: req.Key}

	ll.Info("task received, starting")
	heartbeat, progress := c.processor.Process(c.stopChan, task)

	for {
		select {
		case <-c.stopChan:
			return
		case _, ok := <-heartbeat:
			if ok {
				ll.Debug("heartbeat received")
				c.Respond(req, tHeartbeat, nil)
			} else {
				c.Respond(req, tPipelineGone, nil)
			}
		case p, ok := <-progress:
			if !ok {
				c.Respond(req, tPipelineGone, mPipelineError{Error: "progress channel closed"})
			} else {
				llp := ll.With("progress", p)
				c.Respond(req, tPipelineUpdate, p)
				if p.Error != nil {
					llp.Error("processor errored", "err", p.Error.Error())
					task.cleanup()
					return
				} else if p.Stage == StageDone {
					llp.Debug("processor done")
					task.cleanup()
					return
				}
				llp.Debug("processor progressed")
			}
		case <-time.After(c.timings[TPipelineHeartbeat] * 2):
			c.Respond(req, tPipelineGone, mPipelineError{Error: "heartbeat timeout"})
		}
	}
}

func (c *Worker) StartWorkers() error {
	if err := c.InitConsumer(); err != nil {
		return err
	}
	if err := c.startConsuming(); err != nil {
		return err
	}
	return nil
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
	c.StopConsumer()
}

func (c *Worker) InitConsumer() error {
	consumer, err := rabbitmq.NewConsumer(c.rmqAddr, amqp091.Config{})
	if err != nil {
		return err
	}
	c.consumer = &consumer
	return nil
}

func (c *Worker) StopConsumer() {
	if c.consumer != nil {
		c.consumer.StopConsuming(c.id, false)
		c.consumer.Disconnect()
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
			c.log.Debug("message received", "msg", msg)

			select {
			case <-c.stopChan:
				return rabbitmq.NackRequeue
			case <-c.workers:
				// Run as normal
				c.log.Debug("checked out worker")
				go func() {
					defer func() {
						c.workers <- struct{}{}
						c.log.Debug("checked worker back in")
					}()
					c.requestHandler(msg)
				}()
				return rabbitmq.Ack
			default:
				// No workers available
				// c.log.Debug("worker pool exhausted")
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
		rabbitmq.WithConsumeOptionsQuorum,
		// rabbitmq.WithConsumeOptionsQueueArgs(rabbitmq.Table{"x-max-length": 3}),
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
