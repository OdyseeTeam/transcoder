package dispatcher

import (
	"errors"
	"fmt"
	"sync"
)

const (
	TaskFailed = iota
	TaskDone
	TaskActive
	TaskPending
	TaskDropped
)

var ErrInvalidPayload = errors.New("invalid payload")

type Task struct {
	Payload    interface{}
	Dispatcher *Dispatcher
	result     *Result
}

type Result struct {
	Status int
	Error  error
}

type Workload interface {
	Do(Task) error
}

const (
	sigStop = iota
	sigDoAndStop
)

type Worker struct {
	id      string
	tasks   chan Task
	pool    chan chan Task
	sigChan chan int
	wl      Workload
	gwait   *sync.WaitGroup
	wait    *sync.WaitGroup
}

func (r Result) Failed() bool {
	return r.Status == TaskFailed
}

func (r Result) Done() bool {
	return r.Status == TaskDone
}

func NewWorker(id int, workerPool chan chan Task, wl Workload, gwait *sync.WaitGroup) Worker {
	return Worker{
		id:      fmt.Sprintf("%T#%v", wl, id),
		tasks:   make(chan Task),
		pool:    workerPool,
		sigChan: make(chan int),
		wl:      wl,
		gwait:   gwait,
		wait:    &sync.WaitGroup{},
	}
}

// Start starts reading from tasks channel
func (w *Worker) Start() {
	logger.Infof("spawned dispatch worker %v", w.id)
	w.gwait.Add(1)
	go func() {
		for {
			w.pool <- w.tasks

			select {
			case t := <-w.tasks:
				t.result.Status = TaskActive
				ll := logger.With("wid", w.id, "task", fmt.Sprintf("%+v", t))
				ll.Debugw("worker got a task")
				DispatcherTasksActive.Inc()
				err := w.wl.Do(t)
				DispatcherTasksActive.Dec()
				if err != nil {
					t.result.Status = TaskFailed
					t.result.Error = err
					DispatcherTasksFailed.WithLabelValues(w.id).Inc()
					ll.Errorw("workload failed", "err", err)
				} else {
					DispatcherTasksDone.WithLabelValues(w.id).Inc()
					ll.Debugw("worker done a task")
				}
				t.result.Status = TaskDone
			case sig := <-w.sigChan:
				if sig == sigStop {
					close(w.tasks)
					w.gwait.Done()
					logger.Infof("stopped dispatch worker %v", w.id)
					return
				}
			}
		}
	}()
}

// Stop stops the wl invocation cycle (it will finish the current wl).
func (w *Worker) Stop() {
	w.sigChan <- sigStop
}

type Dispatcher struct {
	workerPool chan chan Task
	workers    []*Worker
	tasks      chan Task
	sigChan    chan int
	gwait      *sync.WaitGroup
}

func Start(workers int, wl Workload) Dispatcher {
	d := Dispatcher{
		workerPool: make(chan chan Task, 100000),
		tasks:      make(chan Task, 100000),
		sigChan:    make(chan int, 1),
		gwait:      &sync.WaitGroup{},
	}

	for i := 0; i < workers; i++ {
		w := NewWorker(i, d.workerPool, wl, d.gwait)
		d.workers = append(d.workers, &w)
		w.Start()
	}

	var cstop bool

	go func() {
		for {
			select {
			case task := <-d.tasks:
				DispatcherQueueLength.Dec()
				logger.Debugw("dispatching incoming task", "task", fmt.Sprintf("%+v", task))
				wq := <-d.workerPool
				wq <- task
			case sig := <-d.sigChan:
				if sig == sigDoAndStop {
					cstop = true
				} else if sig == sigStop {
					for _, w := range d.workers {
						w.Stop()
					}
					return
				}
			default:
				if cstop {
					logger.Debug("do-and-stop in progress, sending out signals")
					for _, w := range d.workers {
						logger.Debugf("sent stop signal to %v", w.id)
						w.Stop()
					}
					return
				}
			}

		}
	}()

	return d
}

func (d *Dispatcher) Dispatch(payload interface{}) *Result {
	r := &Result{Status: TaskPending}
	d.tasks <- Task{Payload: payload, Dispatcher: d, result: r}
	DispatcherQueueLength.Inc()
	DispatcherTasksQueued.Inc()
	return r
}

func (d *Dispatcher) TryDispatch(payload interface{}) *Result {
	r := &Result{Status: TaskPending}
	select {
	case d.tasks <- Task{Payload: payload, Dispatcher: d, result: r}:
		DispatcherQueueLength.Inc()
		DispatcherTasksQueued.Inc()
	default:
		DispatcherTasksDropped.Inc()
		r.Status = TaskDropped
	}
	return r
}

func (d Dispatcher) Stop() {
	d.sigChan <- sigStop
	d.gwait.Wait()
	logger.Infof("all %v workers are stopped", len(d.workers))
}

func (d Dispatcher) DoAndStop() {
	logger.Infof("waiting for %v workers to be done with their workload queues", len(d.workers))
	d.sigChan <- sigDoAndStop
	d.gwait.Wait()
	logger.Infof("all %v workers are stopped", len(d.workers))
}
