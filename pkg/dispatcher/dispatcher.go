package dispatcher

import (
	"fmt"
	"sync"
)

type Task struct {
	URL, SDHash string
}

type Workload interface {
	Do(Task) error
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

func New() Dispatcher {
	return Dispatcher{
		workerPool: make(chan chan Task, 200),
		tasks:      make(chan Task, 2000),
		stopChan:   make(chan bool),
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
	go func() {
		w.wait.Add(1)
		for {
			w.pool <- w.tasks

			select {
			case task := <-w.tasks:
				logger.Debugw("got task", "wid", w.ID, "task", fmt.Sprintf("%+v", task))
				err := w.workload.Do(task)
				if err != nil {
					logger.Errorw("workload errored", "err", err, "wid", w.ID, "task", fmt.Sprintf("%+v", task))
				}
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

func (d Dispatcher) Start(workers int, wl Workload) {
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
}

func (d Dispatcher) Dispatch(t Task) {
	d.tasks <- t
}

func (d Dispatcher) Stop() {
	d.stopChan <- true
}
