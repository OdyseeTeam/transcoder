package tower

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
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
	tasks *taskList
}

type workerRPC struct {
	*rpc
	capacity, available int
	capacityChan        chan int
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

func newTowerRPC(rmqAddr string, tasks *taskList, log logging.KVLogger) (*towerRPC, error) {
	rpc, err := newrpc(rmqAddr, log)
	if err != nil {
		return nil, err
	}
	t := &towerRPC{
		rpc:   rpc,
		tasks: tasks,
	}
	return t, nil
}

func newWorkerRPC(rmqAddr string, log logging.KVLogger) (*workerRPC, error) {
	rpc, err := newrpc(rmqAddr, log)
	if err != nil {
		return nil, err
	}
	w := &workerRPC{
		rpc: rpc,
	}
	return w, nil
}

func (s *rpc) Stop() {
	s.log.Info("stopping RPC")
	close(s.stopChan)
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

func (s *towerRPC) getMessageData(d rabbitmq.Delivery, m interface{}) (*activeTask, error) {
	err := json.Unmarshal(d.Body, m)
	if err != nil {
		s.log.Warn("cannot parse incoming message", "err", err)
		return nil, errors.Wrap(err, "cannot parse incoming message")
	}
	tid := getTaskID(d)
	wid := getWorkerID(d)
	at, ok := s.tasks.getActive(tid)
	if !ok {
		s.log.Warn("no matching task found", "task_id", tid, "worker_id", wid)
		return nil, errors.New("no matching task found")
	}
	return at, nil
}

func (s *rpc) consumeTaskReports(queue string, handler rabbitmq.Handler) error {
	routingKeys := []string{queue}
	opts := []func(*rabbitmq.ConsumeOptions){
		rabbitmq.WithConsumeOptionsConcurrency(5),
		rabbitmq.WithConsumeOptionsQueueDurable,
	}
	s.log.Debug("consuming queue", "queue", queue, "routing_keys", routingKeys)
	return s.consumer.StartConsuming(handler, queue, routingKeys, opts...)
}

func (s *towerRPC) startConsumingWorkRequests() (<-chan *activeTask, error) {
	activeTaskChan := make(chan *activeTask)
	restoreChan := make(chan *activeTask)
	retryChan := make(chan *activeTask)
	err := s.tasks.restore(restoreChan)
	if err != nil {
		return nil, err
	}

	go func() {
		for at := range restoreChan {
			s.log.Info("task restored", "task_id", at.id)
			activeTaskChan <- at
		}
	}()

	go func() {
		retry := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-retry.C:
				retryAttemptChan := make(chan *activeTask)
				go func() {
					for at := range retryAttemptChan {
						s.log.Info("retrying task", "task_id", at.id)
						retryChan <- at
					}
				}()
				err := s.tasks.loadRetriable(retryAttemptChan)
				if err != nil {
					s.log.Error("cannot load retriable tasks", "err", err)
				}
				close(retryAttemptChan)
			case <-s.stopChan:
				retry.Stop()
				return
			}
		}
	}()

	// Start consuming task progress reports first
	err = s.consumeTaskReports(taskProgressQueueName, func(d rabbitmq.Delivery) rabbitmq.Action {
		msg := MsgWorkerProgress{}
		at, err := s.getMessageData(d, &msg)
		if err != nil {
			return rabbitmq.NackDiscard
		}
		s.log.Debug("task progress received", "task_id", at.id, "stage", msg.Stage, "percent", msg.Percent)
		_, err = at.RecordProgress(msg)
		if err != nil {
			s.log.Warn("error recording task progress", "task_id", at.id, "err", err)
		}
		return rabbitmq.Ack
	})
	if err != nil {
		return nil, err
	}
	err = s.consumeTaskReports(taskErrorsQueueName, func(d rabbitmq.Delivery) rabbitmq.Action {
		msg := MsgWorkerError{}
		at, err := s.getMessageData(d, &msg)
		if err != nil {
			return rabbitmq.NackDiscard
		}
		s.log.Info("task error received", "task_id", at.id, "err", msg.Error, "fatal", msg.Fatal)
		_, err = at.SetError(msg)
		if err != nil {
			s.log.Warn("error setting task error", "task_id", at.id, "err", err)
		}
		return rabbitmq.Ack
	})
	if err != nil {
		return nil, err
	}
	err = s.consumeTaskReports(taskDoneQueueName, func(d rabbitmq.Delivery) rabbitmq.Action {
		msg := MsgWorkerResult{}
		at, err := s.getMessageData(d, &msg)
		if err != nil {
			return rabbitmq.NackDiscard
		}
		s.log.Info("task result message received", "task_id", at.id)
		_, err = at.MarkDone(msg)
		if err != nil {
			s.log.Warn("error marking task as done", "task_id", at.id, "err", err)
		}
		return rabbitmq.Ack
	})
	if err != nil {
		return nil, err
	}

	// Start consuming work requests from workers
	err = s.startConsuming(workRequestsQueueName, func(d rabbitmq.Delivery) rabbitmq.Action {
		s.log.Debug("got work request", "reply-to", d.ReplyTo)
		ws := MsgWorkerStatus{}
		err := json.Unmarshal(d.Body, &ws)
		if err != nil {
			s.log.Warn("botched message received", "err", err)
			return rabbitmq.NackDiscard
		}
		var at *activeTask
		select {
		case at = <-retryChan:
			at.workerID = ws.WorkerID
			go func() {
				at.SendPayload(at.exPayload)
			}()
		default:
			at = s.tasks.newActiveTask(ws.WorkerID, "", nil)
		}
		// s.log.Debug("sending task to worker", "worker_id", at.workerID, "task_id", at.id)
		activeTaskChan <- at
		go func() {
			for {
				select {
				case mtt := <-at.payload:
					body, err := json.Marshal(mtt)
					if err != nil {
						s.log.Error("failure publishing task", "err", err)
						s.tasks.delete(at.id)
						return
					}
					opts := []func(*rabbitmq.PublishOptions){
						rabbitmq.WithPublishOptionsTimestamp(time.Now()),
						rabbitmq.WithPublishOptionsContentType("application/json"),
						rabbitmq.WithPublishOptionsExchange(workersExchange),
					}
					s.log.Info("publishing task", "worker_id", at.workerID, "task_id", at.id, "payload", mtt)
					err = s.publisher.Publish(
						body,
						[]string{d.ReplyTo},
						opts...,
					)
					if err != nil {
						s.log.Error("failure publishing task", "err", err)
						s.tasks.delete(at.id)
						return
					}
					s.tasks.commit(at.id, mtt)
					return
				case <-at.done:
					return
				}
			}
		}()

		return rabbitmq.Ack
	}, 1, true)
	if err != nil {
		return nil, err
	}

	return activeTaskChan, nil
}

func (s *workerRPC) publishResponse(queue string, taskID string, message interface{}) error {
	headers := rabbitmq.Table{headerTaskID: taskID, headerWorkerID: s.id}
	ll := s.log.With("queue", queue, "message", message, "headers", headers)
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	opts := []func(*rabbitmq.PublishOptions){
		rabbitmq.WithPublishOptionsTimestamp(time.Now()),
		rabbitmq.WithPublishOptionsContentType("application/json"),
		rabbitmq.WithPublishOptionsExpiration("0"),
		rabbitmq.WithPublishOptionsMandatory,
		rabbitmq.WithPublishOptionsHeaders(headers),
		rabbitmq.WithPublishOptionsPersistentDelivery,
	}
	if queue == taskDoneQueueName || queue == taskErrorsQueueName {
		ll.Info("publishing to queue")
	} else {
		ll.Debug("publishing to queue")
	}
	return s.publisher.Publish(
		body,
		[]string{queue},
		opts...,
	)
}
func (s *workerRPC) workerQueueName() string {
	return fmt.Sprintf("worker-tasks-%v", s.id)
}

func (s *workerRPC) sendWorkRequest() error {
	msg := &MsgWorkRequest{WorkerID: s.id}
	body, _ := json.Marshal(msg)
	return s.publisher.Publish(
		body,
		[]string{workRequestsQueueName},
		rabbitmq.WithPublishOptionsContentType("application/json"),
		rabbitmq.WithPublishOptionsMandatory,
		rabbitmq.WithPublishOptionsHeaders(rabbitmq.Table{headerWorkerID: s.id}),
		rabbitmq.WithPublishOptionsPersistentDelivery,
		rabbitmq.WithPublishOptionsReplyTo(s.workerQueueName()),
	)
}

// func (s *workerRPC) sendWorkerStatus(capacity, available int) error {
// 	s.log.Debug("sending work request", "capacity", capacity, "available", available)
// 	return s.publish(workerStatusQueueName, MsgWorkerStatus{
// 		WorkerID:  s.id,
// 		Capacity:  capacity,
// 		Available: available,
// 		Timestamp: time.Now(),
// 	})
// }

func (s *workerRPC) startWorking(concurrency int) (<-chan workerTask, error) {
	requests := make(chan workerTask)
	s.capacity = concurrency
	s.capacityChan = make(chan int)

	// Start listening for replies to work requests
	err := s.consumer.StartConsuming(
		func(d rabbitmq.Delivery) rabbitmq.Action {
			var mtt MsgTranscodingTask
			err := json.Unmarshal(d.Body, &mtt)
			if err != nil {
				s.log.Warn("botched message received", "err", err)
				return rabbitmq.Ack
			}
			log := logging.AddLogRef(s.log, mtt.SDHash)
			log.Info("work message received", "msg", mtt)

			go func() {
				s.capacityChan <- -1
				defer func() { s.capacityChan <- 1 }()
				wt := createWorkerTask(mtt)
				requests <- wt

				for {
					var err error
					select {
					case p := <-wt.progress:
						err = s.publishResponse(taskProgressQueueName, mtt.TaskID, &MsgWorkerProgress{
							Stage:   p.Stage,
							Percent: p.Percent,
						})
						if err != nil {
							s.log.Warn("error publishing task progress", "err", err)
						}
					case te := <-wt.errors:
						err = s.publishResponse(taskErrorsQueueName, mtt.TaskID, &MsgWorkerError{
							Error: te.err.Error(),
							Fatal: te.fatal,
						})
						if err != nil {
							s.log.Error("error publishing task error", "err", err)
						}
						return
					case r := <-wt.result:
						err = s.publishResponse(taskDoneQueueName, mtt.TaskID, &MsgWorkerResult{RemoteStream: r.remoteStream})
						if err != nil {
							s.log.Error("error publishing task result", "err", err)
						}
						return
					case <-s.stopChan:
						s.log.Info("worker exiting")
						err = s.publishResponse(taskErrorsQueueName, mtt.TaskID, &MsgWorkerError{Error: "worker exiting"})
						if err != nil {
							s.log.Warn("error while publishing exit message", "err", err)
						}
						return
					}
				}
			}()
			return rabbitmq.Ack
		},
		s.workerQueueName(),
		[]string{s.workerQueueName()},
		// rabbitmq.WithConsumeOptionsQueueExclusive,
		rabbitmq.WithConsumeOptionsQueueAutoDelete,
		rabbitmq.WithConsumeOptionsBindingExchangeDurable, // Survive rabbitmq restart
		rabbitmq.WithConsumeOptionsBindingExchangeName(workersExchange),
		rabbitmq.WithConsumeOptionsConsumerName(s.id),
	)
	if err != nil {
		return nil, errors.Wrap(err, "cannot bind to incoming work queue")
	}
	s.log.Info("consuming work queue", "queue", s.workerQueueName())

	go func() {
		for {
			select {
			case val := <-s.capacityChan:
				s.available += val
				// s.sendWorkerStatus(s.capacity, s.available)
				if val > 0 {
					s.log.Info("sending work request")
					err := s.sendWorkRequest()
					if err != nil {
						s.log.Error("failure sending work request", "err", err)
						os.Exit(1)
					}
				}
			case <-s.stopChan:
				s.log.Info("stopping sending work requests")
				return
			}
		}
	}()
	for i := 0; i < s.capacity; i++ {
		s.capacityChan <- 1
	}

	return requests, nil
}

func createWorkerTask(mtt MsgTranscodingTask) workerTask {
	return workerTask{
		payload:  mtt,
		progress: make(chan taskProgress),
		result:   make(chan taskResult),
		errors:   make(chan taskError),
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

func getWorkerID(d rabbitmq.Delivery) string {
	v, ok := d.Headers[headerWorkerID].(string)
	if !ok {
		return ""
	}
	return v
}

func getTaskID(d rabbitmq.Delivery) string {
	v, ok := d.Headers[headerTaskID].(string)
	if !ok {
		return ""
	}
	return v
}
