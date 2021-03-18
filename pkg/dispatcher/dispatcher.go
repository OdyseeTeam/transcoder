package dispatcher

import (
	"errors"
	"fmt"
	"sync"
)

var ErrInvalidPayload = errors.New("invalid payload")

type Task struct {
	Payload    interface{}
	Dispatcher *Dispatcher
	done       chan bool
}

type Workload interface {
	Do(Task) error
}

func Done(d chan bool) bool {
	select {
	case <-d:
		return true
	default:
		return false
	}
}

func NewWorker(id int, workerPool chan chan Task, wl Workload) Worker {
	return Worker{
		ID:       id,
		tasks:    make(chan Task),
		pool:     workerPool,
		stopChan: make(chan bool),
		workload: wl,
		wait:     sync.WaitGroup{},
	}
}

type Worker struct {
	ID       int
	tasks    chan Task
	pool     chan chan Task
	stopChan chan bool
	workload Workload
	wait     sync.WaitGroup
}

// Start starts reading from tasks channel
func (w *Worker) Start() {
	logger.Infow("started worker", "id", w.ID)
	go func() {
		w.wait.Add(1)
		for {
			w.pool <- w.tasks

			select {
			case t := <-w.tasks:
				ll := logger.With("wid", w.ID, "task", fmt.Sprintf("%+v", t))
				ll.Debugw("got task")
				err := w.workload.Do(t)
				if err != nil {
					ll.Errorw("workload errored", "err", err)
				} else {
					ll.Debugw("task done")
				}
				t.done <- true
			case <-w.stopChan:
				w.wait.Done()
				logger.Infow("stopped worker", "id", w.ID)
				return
			}
		}
	}()
}

// Stop stops the workload invocation cycle (it will finish the current workload)
func (w *Worker) Stop() {
	w.stopChan <- true
	w.wait.Wait()
}

type Dispatcher struct {
	workerPool chan chan Task
	workers    []*Worker
	tasks      chan Task
	stopChan   chan bool
}

func Start(workers int, wl Workload) Dispatcher {
	d := Dispatcher{
		workerPool: make(chan chan Task, 200),
		tasks:      make(chan Task, 2000),
		stopChan:   make(chan bool),
	}

	for i := 0; i < workers; i++ {
		w := NewWorker(i, d.workerPool, wl)
		d.workers = append(d.workers, &w)
		w.Start()
	}

	go func() {
		for {
			select {
			case task := <-d.tasks:
				logger.Debugw("received incoming task", "task", fmt.Sprintf("%+v", task))
				go func() {
					logger.Debugw("dispatching incoming task", "task", fmt.Sprintf("%+v", task))
					workerQueue := <-d.workerPool
					workerQueue <- task
				}()
			case <-d.stopChan:
				for _, w := range d.workers {
					w.Stop()
				}
				return
			}
		}
	}()

	return d
}

func (d *Dispatcher) Dispatch(payload interface{}) chan bool {
	done := make(chan bool, 1)
	d.tasks <- Task{Payload: payload, Dispatcher: d, done: done}
	return done
}

func (d Dispatcher) Stop() {
	d.stopChan <- true
}
