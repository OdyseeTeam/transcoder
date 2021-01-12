package dispatcher

import "fmt"

type Task struct {
	URL, SDHash string
}

type Workload interface {
	Do(Task) error
}

func NewWorker(id int, workers chan chan Task, wl Workload) Worker {
	return Worker{
		ID:       id,
		tasks:    make(chan Task),
		workers:  workers,
		stopChan: make(chan bool),
		workload: wl,
	}
}

func NewDispatcher() Dispatcher {
	return Dispatcher{
		workers:  make(chan chan Task, 200),
		tasks:    make(chan Task, 2000),
		stopChan: make(chan bool),
	}
}

type Worker struct {
	ID       int
	tasks    chan Task
	workers  chan chan Task
	stopChan chan bool
	workload Workload
}

// Start starts reading from tasks channel
func (w *Worker) Start() {
	go func() {
		for {
			w.workers <- w.tasks

			select {
			case task := <-w.tasks:
				logger.Debugw("got task", "wid", w.ID, "task", fmt.Sprintf("%+v", task))
				err := w.workload.Do(task)
				if err != nil {
					logger.Errorw("workload errored", "err", err, "wid", w.ID, "task", fmt.Sprintf("%+v", task))
				}
			case <-w.stopChan:
				return
			}
		}
	}()
}

// Stop stops the workload invocation cycle.
func (w *Worker) Stop() {
	w.stopChan <- true
}

type Dispatcher struct {
	workers  chan chan Task
	tasks    chan Task
	stopChan chan bool
}

func (d Dispatcher) Start(workers int, wl Workload) {
	for i := 0; i < workers; i++ {
		w := NewWorker(i, d.workers, wl)
		w.Start()
	}

	go func() {
		for {
			select {
			case task := <-d.tasks:
				logger.Debugw("received incoming task", "task", fmt.Sprintf("%+v", task))
				go func() {
					logger.Debugw("dispatching incoming task", "task", fmt.Sprintf("%+v", task))
					workTasks := <-d.workers
					workTasks <- task
				}()
			case <-d.stopChan:
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
