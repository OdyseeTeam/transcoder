package tower

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/lbryio/transcoder/pkg/logging"

	"github.com/pkg/errors"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/wagslane/go-rabbitmq"
)

type rpc struct {
	id        string
	publisher rabbitmq.Publisher
	consumer  rabbitmq.Consumer
	backCh    *amqp.Channel
	stopChan  chan struct{}
	log       logging.KVLogger
}

type towerRPC struct {
	*rpc
	tasks     map[string]*activeTask
	tasksLock sync.Mutex
}

type workerRPC struct {
	*rpc
	capacity, available int
	eagerness           chan int
}

type activeTask struct {
	id       string
	workerID string
	payload  chan MsgTranscodingTask
	progress chan MsgWorkerProgress
	errChan  chan error
	done     chan MsgWorkerResult
	reject   chan struct{}
}

type workerTask struct {
	payload  MsgTranscodingTask
	progress chan taskProgress
	errChan  chan error
	result   chan taskResult
}

func newrpc(rmqAddr string, log logging.KVLogger) (*rpc, error) {
	s := &rpc{
		stopChan: make(chan struct{}),
		log:      log,
	}

	var err error
	s.publisher, err = rabbitmq.NewPublisher(rmqAddr, amqp.Config{})
	if err != nil {
		return nil, err
	}

	s.consumer, err = rabbitmq.NewConsumer(rmqAddr, amqp.Config{})
	if err != nil {
		return nil, err
	}

	amqpConn, err := amqp.DialConfig(rmqAddr, amqp.Config{})
	if err != nil {
		return nil, err
	}
	ch, err := amqpConn.Channel()
	if err != nil {
		return nil, err
	}
	s.backCh = ch
	returns := s.publisher.NotifyReturn()

	go func() {
		for r := range returns {
			s.log.Warn(fmt.Sprintf("message returned from server: %+v\n", r))
		}
	}()

	s.log.Info("rpc connection open", "rmq_addr", rmqAddr)
	return s, nil
}

func (t *activeTask) SendPayload(mtt MsgTranscodingTask) {
	mtt.TaskID = t.id
	t.payload <- mtt
}

func (s *rpc) Stop() {
	s.consumer.StopConsuming(s.id, false)
	s.consumer.Disconnect()
	s.publisher.StopPublishing()
}

func (s *rpc) startConsuming(queue string, handler rabbitmq.Handler, concurrency int, durable bool) error {
	routingKeys := []string{}
	opts := []func(*rabbitmq.ConsumeOptions){
		rabbitmq.WithConsumeOptionsConcurrency(concurrency),
	}
	if durable {
		opts = append(opts, rabbitmq.WithConsumeOptionsQueueDurable)
	}
	if queue == replyToQueueName {
		opts = append(opts, rabbitmq.WithConsumeOptionsConsumerAutoAck(true))
		routingKeys = []string{""}
	} else {
		// opts = append(opts, rabbitmq.WithConsumeOptionsBindingExchangeDurable)
		// opts = append(opts, rabbitmq.WithConsumeOptionsBindingExchangeKind("direct"))
		routingKeys = []string{queue}
	}
	s.log.Debug("consuming queue", "queue", queue, "routing_keys", routingKeys)
	return s.consumer.StartConsuming(handler, queue, routingKeys, opts...)
}

func (s *rpc) publish(queue string, message interface{}) error {
	ll := s.log.With("queue", queue, "message", message)
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	opts := []func(*rabbitmq.PublishOptions){
		rabbitmq.WithPublishOptionsTimestamp(time.Now()),
		rabbitmq.WithPublishOptionsContentType("application/json"),
		rabbitmq.WithPublishOptionsExpiration("0"),
		rabbitmq.WithPublishOptionsMandatory,
		rabbitmq.WithPublishOptionsPersistentDelivery,
	}
	if queue == workRequestsQueueName {
		opts = append(opts, rabbitmq.WithPublishOptionsReplyTo(replyToQueueName))
	}
	if queue == taskDoneQueueName {
		s.log.Info("publishing to queue")
	} else {
		s.log.Debug("publishing to queue")
	}
	return s.publisher.Publish(
		body,
		[]string{queue},
		opts...,
	)
}

func (s *towerRPC) declareQueues() error {
	queues := []string{workRequestsQueueName, taskProgressQueueName, taskErrorsQueueName, taskDoneQueueName, workerStatusQueueName}
	for _, q := range queues {
		if _, err := s.backCh.QueueDeclare(q, true, false, false, false, amqp.Table{}); err != nil {
			return err
		}
	}
	return nil
}

func (s *towerRPC) deleteQueues() error {
	queues := []string{workRequestsQueueName, taskProgressQueueName, taskErrorsQueueName, taskDoneQueueName, workerStatusQueueName}
	for _, q := range queues {
		if _, err := s.backCh.QueueDelete(q, false, false, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *towerRPC) startConsumingWorkRequests() (<-chan *activeTask, error) {
	tasks := make(chan *activeTask)

	// Start consuming task progress reports first
	var err error
	err = s.startConsuming(taskProgressQueueName, func(d rabbitmq.Delivery) rabbitmq.Action {
		msg := MsgWorkerProgress{}
		err := json.Unmarshal(d.Body, &msg)
		if err != nil {
			s.log.Warn("botched message received", "err", err)
			return rabbitmq.NackDiscard
		}
		at, ok := s.getActiveTask(msg.TaskID)
		if !ok {
			s.log.Warn("no matching task found", "task_id", msg.TaskID, "worker_id", msg.WorkerID)
			return rabbitmq.NackDiscard
		} else {
			at.progress <- msg
			return rabbitmq.Ack
		}
	}, 5, true)
	if err != nil {
		return nil, err
	}
	err = s.startConsuming(taskErrorsQueueName, func(d rabbitmq.Delivery) rabbitmq.Action {
		msg := MsgWorkerError{}
		err := json.Unmarshal(d.Body, &msg)
		if err != nil {
			s.log.Warn("botched message received", "err", err)
			return rabbitmq.NackDiscard
		}
		if at, ok := s.getActiveTask(msg.TaskID); !ok {
			return rabbitmq.NackDiscard
		} else {
			at.errChan <- errors.New(msg.Error)
			return rabbitmq.Ack
		}
	}, 5, true)
	if err != nil {
		return nil, err
	}
	err = s.startConsuming(taskDoneQueueName, func(d rabbitmq.Delivery) rabbitmq.Action {
		msg := MsgWorkerResult{}
		err := json.Unmarshal(d.Body, &msg)
		if err != nil {
			s.log.Warn("botched message received", "err", err)
			return rabbitmq.NackDiscard
		}
		if at, ok := s.getActiveTask(msg.TaskID); !ok {
			return rabbitmq.NackDiscard
		} else {
			at.done <- msg
			return rabbitmq.Ack
		}
	}, 5, true)
	if err != nil {
		return nil, err
	}

	// Start consuming work requests from workers
	return tasks, s.startConsuming(workRequestsQueueName, func(d rabbitmq.Delivery) rabbitmq.Action {
		s.log.Debug("got work request", "reply-to", d.ReplyTo)
		ws := MsgWorkerStatus{}
		err := json.Unmarshal(d.Body, &ws)
		if err != nil {
			s.log.Warn("botched message received", "err", err)
			return rabbitmq.NackDiscard
		}
		at := createActiveTask(ws.WorkerID)
		s.tasksLock.Lock()
		s.tasks[at.id] = at
		s.tasksLock.Unlock()
		go func() {
			for {
				select {
				case tt := <-at.payload:
					body, err := json.Marshal(tt)
					if err != nil {
						s.log.Error("failure publishing task", "err", err)
						select {
						case at.reject <- struct{}{}:
						default:
						}
						s.tasksLock.Lock()
						delete(s.tasks, at.id)
						s.tasksLock.Unlock()
						return
					}
					opts := []func(*rabbitmq.PublishOptions){
						rabbitmq.WithPublishOptionsTimestamp(time.Now()),
						rabbitmq.WithPublishOptionsContentType("application/json"),
						rabbitmq.WithPublishOptionsExchange(workersExchange),
					}
					s.log.Debug("publishing to queue", "queue", d.ReplyTo, "message", tt)
					err = s.publisher.Publish(
						body,
						[]string{d.ReplyTo},
						opts...,
					)
					if err != nil {
						s.log.Error("failure publishing task", "err", err)
						select {
						case at.reject <- struct{}{}:
						default:
						}
						s.tasksLock.Lock()
						delete(s.tasks, at.id)
						s.tasksLock.Unlock()
					}
					return
				case <-at.done:
					return
				}
			}
		}()
		s.log.Debug("sending task to worker", "worker_id", at.workerID)
		tasks <- at
		return rabbitmq.Ack
	}, 1, true)
}

func (s *towerRPC) getActiveTask(id string) (*activeTask, bool) {
	s.tasksLock.Lock()
	defer s.tasksLock.Unlock()
	at, ok := s.tasks[id]
	return at, ok

}
func (s *workerRPC) sendWorkRequest() error {
	return s.publish(workRequestsQueueName, &MsgWorkRequest{WorkerID: s.id})
	// msg := &MsgWorkRequest{WorkerID: s.id}
	// body, err := json.Marshal(msg)
	// if err != nil {
	// 	return err
	// }
	// return s.publisher.Publish(
	// 	body,
	// 	[]string{requestsQueueName},
	// 	// rabbitmq.WithPublishOptionsHeaders(rabbitmq.Table{"type": "request.new"}),
	// 	rabbitmq.WithPublishOptionsContentType("application/json"),
	// 	rabbitmq.WithPublishOptionsExchange("transcoder"),
	// 	rabbitmq.WithPublishOptionsMandatory,
	// 	rabbitmq.WithPublishOptionsPersistentDelivery,
	// )
}

func (s *workerRPC) sendWorkerStatus(capacity, available int) error {
	s.log.Debug("sending work request", "capacity", capacity, "available", available)
	return s.publish(workerStatusQueueName, MsgWorkerStatus{
		WorkerID:  s.id,
		Capacity:  capacity,
		Available: available,
		Timestamp: time.Now(),
	})
}

func (s *workerRPC) startWorking(concurrency int) (<-chan workerTask, error) {
	requests := make(chan workerTask)
	s.capacity = concurrency
	s.eagerness = make(chan int)

	workerQueueName := fmt.Sprintf("worker-tasks-%v", s.id)

	// Start listening for replies to work requests
	err := s.consumer.StartConsuming(
		func(d rabbitmq.Delivery) rabbitmq.Action {
			var mtt MsgTranscodingTask
			err := json.Unmarshal(d.Body, &mtt)
			if err != nil {
				s.log.Warn("botched message received", "err", err)
				return rabbitmq.Ack
			}
			log := logging.AddLogRef(s.log, mtt.Ref)
			log.Debug("message received", "msg", mtt)

			go func() {
				s.eagerness <- -1
				defer func() { s.eagerness <- 1 }()
				wt := createWorkerTask(mtt)
				requests <- wt
				for {
					select {
					case p := <-wt.progress:
						s.publish(taskProgressQueueName, &MsgWorkerProgress{
							WorkerID: s.id,
							TaskID:   mtt.TaskID,
							Stage:    p.Stage,
							Percent:  p.Percent,
						})
					case err := <-wt.errChan:
						s.publish(taskErrorsQueueName, &MsgWorkerError{
							WorkerID: s.id,
							TaskID:   mtt.TaskID,
							Error:    err.Error(),
						})
						return
					case r := <-wt.result:
						s.publish(taskDoneQueueName, &MsgWorkerResult{
							WorkerID:     s.id,
							TaskID:       mtt.TaskID,
							RemoteStream: r.remoteStream,
						})
						return
					case <-s.stopChan:
						s.publish(taskErrorsQueueName, &MsgWorkerError{
							WorkerID: s.id,
							TaskID:   mtt.TaskID,
							Error:    "exiting",
						})
						return
					}
				}
			}()
			return rabbitmq.Ack
		},
		workerQueueName,
		[]string{workerQueueName},
		rabbitmq.WithConsumeOptionsQueueExclusive,
		rabbitmq.WithConsumeOptionsQueueAutoDelete,
		rabbitmq.WithConsumeOptionsBindingExchangeDurable, // Survive rabbitmq restart
		rabbitmq.WithConsumeOptionsBindingExchangeName(workersExchange),
		rabbitmq.WithConsumeOptionsConsumerName(s.id),
	)
	if err != nil {
		return nil, errors.Wrap(err, "cannot bind to incoming work queue")
	}

	// Start sending work requests to the special reply-to queue,
	// which is used to ensure this specific worker gets its work request served
	// and not just any worker.
	go func() {
		s.log.Info("tracking pipelines")
		for {
			select {
			case val := <-s.eagerness:
				s.available += val
				// s.sendWorkerStatus(s.capacity, s.available)
				if val > 0 {
					// err := s.sendWorkRequest()
					msg := &MsgWorkRequest{WorkerID: s.id}
					body, _ := json.Marshal(msg)
					err := s.publisher.Publish(
						body,
						[]string{workRequestsQueueName},
						rabbitmq.WithPublishOptionsContentType("application/json"),
						rabbitmq.WithPublishOptionsMandatory,
						rabbitmq.WithPublishOptionsPersistentDelivery,
						rabbitmq.WithPublishOptionsReplyTo(workerQueueName),
					)
					if err != nil {
						s.log.Error("failure sending work request", "err", err)
					}
				}
			case <-s.stopChan:
				s.log.Info("eagerness done")
				// s.sendWorkerStatus(0, 0)
				return
			}
		}
	}()
	for i := 0; i < s.capacity; i++ {
		s.eagerness <- 1
	}
	return requests, nil
}

func createWorkerTask(mtt MsgTranscodingTask) workerTask {
	return workerTask{
		payload:  mtt,
		progress: make(chan taskProgress),
		result:   make(chan taskResult),
		errChan:  make(chan error),
	}
}

func createActiveTask(wid string) *activeTask {
	uuid, _ := generateUUID()
	return &activeTask{
		workerID: wid,
		id:       uuid,
		payload:  make(chan MsgTranscodingTask),
		progress: make(chan MsgWorkerProgress),
		errChan:  make(chan error),
		done:     make(chan MsgWorkerResult),
	}
}

func generateUUID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), err
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
