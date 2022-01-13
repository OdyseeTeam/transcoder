package tower

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/tower/queue"

	"github.com/oklog/ulid/v2"
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
	tasks      *taskList
	retryTasks *retryTasks
	randPool   sync.Pool

	videoManager *manager.VideoManager
}

type workerRPC struct {
	*rpc
	capacity, available int
	capacityChan        chan int
	sessionID           string
}

type retryTasks struct {
	sync.RWMutex
	workers map[string]chan *activeTask
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
		rpc:        rpc,
		tasks:      tasks,
		retryTasks: &retryTasks{workers: map[string]chan *activeTask{}},
		randPool: sync.Pool{
			New: func() interface{} {
				return rand.New(rand.NewSource(time.Now().UnixNano()))
			},
		},
	}
	err = t.declareQueues()
	if err != nil {
		return nil, err
	}
	return t, nil
}

func newWorkerRPC(rmqAddr string, log logging.KVLogger) (*workerRPC, error) {
	rpc, err := newrpc(rmqAddr, log)
	if err != nil {
		return nil, err
	}
	w := &workerRPC{
		rpc:       rpc,
		sessionID: time.Now().Format(time.RFC3339),
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
	routingKeys = []string{queue}
	s.log.Debug("consuming queue", "queue", queue, "routing_keys", routingKeys)
	return s.consumer.StartConsuming(handler, queue, routingKeys, opts...)
}

func (s *towerRPC) declareQueues() error {
	queues := []string{workRequestsQueue, taskStatusQueue, workerHandshakeQueue}
	for _, q := range queues {
		if _, err := s.backCh.QueueDeclare(q, true, false, false, false, amqp.Table{}); err != nil {
			return err
		}
	}
	return nil
}

func (s *towerRPC) deleteQueues() error {
	queues := []string{workRequestsQueue, taskStatusQueue, workerHandshakeQueue}
	for _, q := range queues {
		if _, err := s.backCh.QueueDelete(q, false, false, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *towerRPC) getMsgMeta(d rabbitmq.Delivery) (*workerMsgMeta, error) {
	wid, ok := d.Headers[headerWorkerID].(string)
	if !ok {
		return nil, errors.New("worker id missing")
	}
	tid, ok := d.Headers[headerTaskID].(string)
	if !ok {
		return nil, errors.New("task id missing")
	}
	mType, ok := d.Headers[headerMessageType].(string)
	if !ok {
		return nil, errors.New("message type missing")
	}
	return &workerMsgMeta{wid: wid, tid: tid, mType: mType}, nil
}

func (s *rpc) consumeTaskStatuses(queue string, handler rabbitmq.Handler) error {
	routingKeys := []string{queue}
	opts := []func(*rabbitmq.ConsumeOptions){
		rabbitmq.WithConsumeOptionsConcurrency(1),
		rabbitmq.WithConsumeOptionsQueueDurable,
	}
	s.log.Debug("consuming queue", "queue", queue, "routing_keys", routingKeys)
	return s.consumer.StartConsuming(handler, queue, routingKeys, opts...)
}

func (s *towerRPC) handleTaskStatus(at *activeTask, msgi interface{}, meta *workerMsgMeta) {
	ll := s.log.With("tid", at.id, "wid", at.workerID)
	switch meta.mType {
	case mTypeError:
		msg := msgi.(*MsgWorkerError)
		ll.Info("task error received", "err", msg.Error, "fatal", msg.Fatal)
		_, err := at.SetError(*msg)
		if err != nil {
			ll.Warn("error setting task error", "err", err)
		}
	case mTypeProgress:
		msg := msgi.(*MsgWorkerProgress)
		s.log.Debug("task progress received", "tid", at.id, "stage", msg.Stage, "percent", msg.Percent)
		at.RecordProgress(*msg)
	case mTypeSuccess:
		msg := msgi.(*MsgWorkerSuccess)
		s.log.Info("task result message received", "tid", at.id)
		_, err := at.MarkDone(*msg)
		if err != nil {
			s.log.Warn("error marking task as done", "tid", at.id, "err", err)
		}
	default:
		s.log.Error("unknown message type", "type", meta.mType)
	}
}

func (s *towerRPC) startConsumingWorkRequests() (<-chan *activeTask, error) {
	activeTaskChan := make(chan *activeTask)
	// retryChan := make(chan *activeTask)

	restoreChan, err := s.tasks.restore()
	if err != nil {
		return nil, err
	}

	go func() {
		for at := range restoreChan {
			at.restored = true
			s.log.Info("task restored", "tid", at.id)
			activeTaskChan <- at
		}
		s.log.Info("done restoring tasks")
	}()

	// go func() {
	// 	retry := time.NewTicker(1 * time.Second)
	// 	for {
	// 		select {
	// 		case <-retry.C:
	// 			retryAttemptChan := make(chan *activeTask)
	// 			go func() {
	// 				for at := range retryAttemptChan {
	// 					s.log.Info("retrying task", "tid", at.id)
	// 					retryChan <- at
	// 				}
	// 			}()
	// 			err := s.tasks.loadRetriable(retryAttemptChan)
	// 			if err != nil {
	// 				s.log.Error("cannot load retriable tasks", "err", err)
	// 			}
	// 			close(retryAttemptChan)
	// 		case <-s.stopChan:
	// 			retry.Stop()
	// 			return
	// 		}
	// 	}
	// }()

	// Start consuming task progress reports first
	err = s.consumeTaskStatuses(taskStatusQueue, func(d rabbitmq.Delivery) rabbitmq.Action {
		var msg interface{}
		meta, err := s.getMsgMeta(d)
		if err != nil {
			s.log.Error("error getting message metadata", "err", err)
			return rabbitmq.NackDiscard
		}

		switch meta.mType {
		case mTypeError:
			msg = &MsgWorkerError{}
		case mTypeProgress:
			msg = &MsgWorkerProgress{}
		case mTypeSuccess:
			msg = &MsgWorkerSuccess{}
		default:
			s.log.Error("unknown message type", "type", meta.mType)
		}

		err = json.Unmarshal(d.Body, msg)
		if err != nil {
			s.log.Error("cannot parse incoming message", "err", err)
			return rabbitmq.NackDiscard
		}

		at, ok := s.tasks.get(meta.tid)
		if !ok {
			s.log.Error("no matching active task found", "tid", meta.tid, "wid", meta.wid)
			return rabbitmq.NackDiscard
		}
		s.handleTaskStatus(at, msg, meta)

		return rabbitmq.Ack
	})
	if err != nil {
		return nil, err
	}

	err = s.startConsuming(workerHandshakeQueue, func(d rabbitmq.Delivery) rabbitmq.Action {
		s.log.Info("got worker handshake", "reply-to", d.ReplyTo)
		mwh := MsgWorkerHandshake{}
		err := json.Unmarshal(d.Body, &mwh)
		if err != nil {
			s.log.Warn("botched message received", "err", err)
			return rabbitmq.NackDiscard
		}
		ll := s.log.With("wid", mwh.WorkerID)

		s.retryTasks.Lock()
		retryChan, ok := s.retryTasks.workers[mwh.WorkerID]
		if !ok {
			retryChan = make(chan *activeTask)
			s.retryTasks.workers[mwh.WorkerID] = retryChan
		}
		s.retryTasks.Unlock()
		ll.Info("retrieving running tasks")

		runningTasks, err := s.tasks.q.GetActiveTasksForWorker(context.Background(), mwh.WorkerID)
		if err != nil {
			ll.Error("failed to retrieve running tasks", "err", err)
		} else {
			ll.Info("running tasks retrieved", "count", len(runningTasks))
		}
		for _, dbt := range runningTasks {
			at, ok := s.tasks.get(dbt.ULID)
			if !ok {
				// All running tasks should've been restored at the beginning of this function,
				// something's wrong if they weren't
				ll.Error("no corresponding active task found", "db_tid", dbt.ULID, "err", err)
				continue
			}
			retryChan <- at
		}
		return rabbitmq.Ack
	}, 1, true)
	if err != nil {
		return nil, err
	}

	// Start consuming work requests from workers
	err = s.startConsuming(workRequestsQueue, func(d rabbitmq.Delivery) rabbitmq.Action {
		s.log.Info("got work request", "reply-to", d.ReplyTo)
		wr := MsgWorkerRequest{}
		err := json.Unmarshal(d.Body, &wr)
		if err != nil {
			s.log.Warn("botched message received", "err", err)
			return rabbitmq.NackDiscard
		}
		// select {
		// case at = <-retryChan:
		// 	at.workerID = ws.WorkerID
		// 	go func() {
		// 		at.SendPayload(at.exPayload)
		// 	}()
		// default:
		// 	at = s.tasks.newActiveTask(ws.WorkerID, "", nil)
		// }
		s.retryTasks.RLock()
		retryChan := s.retryTasks.workers[wr.WorkerID]
		s.retryTasks.RUnlock()

		select {
		case at := <-retryChan:
			ll := s.log.With("wid", at.workerID, "tid", at.id)
			if at.exPayload == nil {
				ll.Error("empty payload for retried task", "err", err)
				return rabbitmq.NackDiscard
			}
			mtt := at.exPayload
			s.publishTask(d.ReplyTo, *mtt)
			if err != nil {
				ll.Error("failure publishing task", "err", err)
				return rabbitmq.NackDiscard
			}
			ll.Info("re-published task", "payload", mtt)
		default:
			at := s.tasks.newEmptyTask(wr.WorkerID, s.generateULID())
			s.dispatchActiveTask(d.ReplyTo, activeTaskChan, at)
		}
		// s.log.Debug("sending task to worker", "wid", at.workerID, "tid", at.id)

		return rabbitmq.Ack
	}, 1, true)
	if err != nil {
		return nil, err
	}

	return activeTaskChan, nil
}

func (s *towerRPC) dispatchActiveTask(wrkQueue string, activeTaskChan chan *activeTask, at *activeTask) {
	activeTaskChan <- at
	go func() {
		for {
			select {
			case mtt := <-at.payload:
				at.exPayload = &mtt
				dbt, err := s.tasks.q.GetRunnableTaskByPayload(context.Background(), queue.GetRunnableTaskByPayloadParams{
					URL:    mtt.URL,
					SDHash: mtt.SDHash,
				})
				if err == nil {
					s.log.Warn("payload already found running in the database", "db_task", dbt)
					s.dispatchActiveTask(wrkQueue, activeTaskChan, at)
					return
				}

				_, err = s.tasks.q.CreateTask(context.Background(), queue.CreateTaskParams{
					ULID:   at.id,
					Worker: at.workerID,
					URL:    mtt.URL,
					SDHash: mtt.SDHash,
				})
				if err != nil {
					s.log.Error("error saving task to db", "err", err, "ulid", at.id)
					at = s.tasks.newEmptyTask(at.workerID, s.generateULID())
					s.dispatchActiveTask(wrkQueue, activeTaskChan, at)
					return
				}
				s.tasks.insert(at)
				s.publishTask(wrkQueue, mtt)
				if err != nil {
					s.log.Error("failure publishing task", "err", err)
					s.tasks.delete(at.id)
					return
				}
				s.log.Info("published task", "wid", at.workerID, "tid", at.id, "payload", mtt)
				return
			case <-at.success:
				return
			}
		}
	}()
}

func (s *towerRPC) publishTask(wrkQueue string, mtt MsgTranscodingTask) error {
	body, err := json.Marshal(mtt)
	if err != nil {
		return err
	}
	opts := []func(*rabbitmq.PublishOptions){
		rabbitmq.WithPublishOptionsTimestamp(time.Now()),
		rabbitmq.WithPublishOptionsContentType("application/json"),
		rabbitmq.WithPublishOptionsExchange(workersExchange),
	}

	return s.publisher.Publish(
		body,
		[]string{wrkQueue},
		opts...,
	)
}

func (s *workerRPC) sendTaskStatus(queue, tid, mType string, message interface{}) error {
	headers := rabbitmq.Table{headerTaskID: tid, headerWorkerID: s.id, headerMessageType: mType}
	ll := s.log.With("type", mType, "message", message, "headers", headers)
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

	ll.Debug("sending task status")
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
	msg := &MsgWorkerRequest{
		WorkerID:  s.id,
		SessionID: s.sessionID,
	}
	body, _ := json.Marshal(msg)
	return s.publisher.Publish(
		body,
		[]string{workRequestsQueue},
		rabbitmq.WithPublishOptionsContentType("application/json"),
		rabbitmq.WithPublishOptionsMandatory,
		rabbitmq.WithPublishOptionsHeaders(rabbitmq.Table{headerWorkerID: s.id}),
		rabbitmq.WithPublishOptionsPersistentDelivery,
		rabbitmq.WithPublishOptionsReplyTo(s.workerQueueName()),
	)
}

func (s *workerRPC) sendWorkerHandshake() error {
	msg := MsgWorkerHandshake{
		WorkerID:  s.id,
		Capacity:  s.capacity,
		Available: s.available,
		SessionID: s.sessionID,
	}
	s.log.Info("sending worker handshake", "msg", msg)
	body, _ := json.Marshal(msg)
	return s.publisher.Publish(
		body,
		[]string{workerHandshakeQueue},
		rabbitmq.WithPublishOptionsContentType("application/json"),
		rabbitmq.WithPublishOptionsMandatory,
		rabbitmq.WithPublishOptionsHeaders(rabbitmq.Table{headerWorkerID: s.id}),
		rabbitmq.WithPublishOptionsPersistentDelivery,
		rabbitmq.WithPublishOptionsReplyTo(s.workerQueueName()),
	)
}

func (s *workerRPC) startWorking(concurrency int) (<-chan workerTask, error) {
	requests := make(chan workerTask)
	s.capacity = concurrency
	s.capacityChan = make(chan int)

	err := s.sendWorkerHandshake()
	if err != nil {
		return nil, err
	}

	// Start listening for replies to work requests
	err = s.consumer.StartConsuming(
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
						err = s.sendTaskStatus(taskStatusQueue, mtt.TaskID, mTypeProgress, &MsgWorkerProgress{
							Stage:   p.Stage,
							Percent: p.Percent,
						})
						if err != nil {
							s.log.Warn("error publishing task progress", "err", err)
						}
					case te := <-wt.errors:
						err = s.sendTaskStatus(taskStatusQueue, mtt.TaskID, mTypeError, &MsgWorkerError{
							Error: te.err.Error(),
							Fatal: te.fatal,
						})
						if err != nil {
							s.log.Error("error publishing task error", "err", err)
						}
						return
					case r := <-wt.result:
						err = s.sendTaskStatus(taskStatusQueue, mtt.TaskID, mTypeSuccess, &MsgWorkerSuccess{RemoteStream: r.remoteStream})
						if err != nil {
							s.log.Error("error publishing task result", "err", err)
						}
						return
					case <-s.stopChan:
						s.log.Info("worker exiting")
						err = s.sendTaskStatus(taskStatusQueue, mtt.TaskID, mTypeError, &MsgWorkerError{Error: "worker exiting"})
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
				if val > 0 {
					s.log.Info("sending work request", "available", s.available)
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

func (s *towerRPC) generateULID() string {
	t := time.Now()
	r := s.randPool.Get().(*rand.Rand)
	return ulid.MustNew(ulid.Timestamp(t), ulid.Monotonic(r, 0)).String()
}
