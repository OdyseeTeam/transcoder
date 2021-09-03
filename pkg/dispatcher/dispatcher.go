// Package dispatcher provides a convenient interface for parallelizing long tasks and keeping them at bay.

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

// Worker can be any object that is capable to do `Work()`.
type Worker interface {
	Work(Task) error
}

// Task represents a unit of work.
// Each worker should accept it as an argument.
// Example:
//  func (w encoderWorker) Work(t dispatcher.Task) error {
//		r := t.Payload.(*manager.TranscodingRequest)
//  ...
type Task struct {
	Payload    interface{}
	Dispatcher *Dispatcher
	result     *Result
}

// Result is a result of Task execution.
// TODO: setting/returning this needs to be implemented better using channels.
type Result struct {
	Status int
	Error  error
	value  chan interface{}
}

const (
	sigStop = iota
	sigDoAndStop
)

type agent struct {
	id      string
	tasks   chan Task
	pool    chan chan Task
	sigChan chan int
	worker  Worker
	gwait   *sync.WaitGroup
	wait    *sync.WaitGroup
}

func (t Task) SetResult(v interface{}) {
	t.result.value <- v
}

func (r Result) Failed() bool {
	return r.Status == TaskFailed
}

func (r Result) Done() bool {
	return r.Status == TaskDone
}

func (r Result) Value() interface{} {
	return <-r.value
}

func newAgent(id int, agentPool chan chan Task, worker Worker, gwait *sync.WaitGroup) agent {
	return agent{
		id:      fmt.Sprintf("%T#%v", worker, id),
		tasks:   make(chan Task),
		pool:    agentPool,
		sigChan: make(chan int),
		worker:  worker,
		gwait:   gwait,
		wait:    &sync.WaitGroup{},
	}
}

// Start starts reading from tasks channel
func (a *agent) Start() {
	logger.Infof("spawned dispatch agent %v", a.id)
	a.gwait.Add(1)
	go func() {
		for {
			a.pool <- a.tasks

			select {
			case t := <-a.tasks:
				t.result.Status = TaskActive
				ll := logger.With("wid", a.id, "task", fmt.Sprintf("%+v", t))
				ll.Debugw("agent got a task")
				DispatcherTasksActive.Inc()
				err := a.worker.Work(t)
				DispatcherTasksActive.Dec()
				if err != nil {
					t.result.Status = TaskFailed
					t.result.Error = err
					DispatcherTasksFailed.WithLabelValues(a.id).Inc()
					ll.Errorw("workload failed", "err", err)
				} else {
					DispatcherTasksDone.WithLabelValues(a.id).Inc()
					ll.Debugw("agent done a task")
				}
				t.result.Status = TaskDone
			case sig := <-a.sigChan:
				if sig == sigStop {
					close(a.tasks)
					a.gwait.Done()
					logger.Infof("stopped dispatch agent %v", a.id)
					return
				}
			}
		}
	}()
}

// Stop stops the worker invocation cycle (it will finish the current worker).
func (a *agent) Stop() {
	a.sigChan <- sigStop
}

type Dispatcher struct {
	agentPool     chan chan Task
	agents        []*agent
	incomingTasks chan Task
	sigChan       chan int
	gwait         *sync.WaitGroup
}

// Start spawns a pool of workers.
// tasksBuffer sets how many tasks should be pre-emptively put into each worker's
// incoming queue. Set to 0 for prevent greedy tasks assignment (this will make `Dispatch` blocking).
func Start(parallel int, worker Worker, tasksBuffer int) Dispatcher {
	d := Dispatcher{
		agentPool:     make(chan chan Task, 1000),
		incomingTasks: make(chan Task, tasksBuffer),
		sigChan:       make(chan int, 1),
		gwait:         &sync.WaitGroup{},
	}

	for i := 0; i < parallel; i++ {
		a := newAgent(i, d.agentPool, worker, d.gwait)
		d.agents = append(d.agents, &a)
		a.Start()
	}

	go func() {
		for {
			select {
			case task := <-d.incomingTasks:
				DispatcherQueueLength.Dec()
				logger.Debugw("dispatching incoming task", "task", fmt.Sprintf("%+v", task))
				agentQueue := <-d.agentPool
				agentQueue <- task
			case sig := <-d.sigChan:
				if sig == sigStop {
					for _, a := range d.agents {
						a.Stop()
					}
					return
				}
			}
		}
	}()

	return d
}

// Dispatch takes `payload`, wraps it into a `Task` and dispatches to the first available `Worker`.
func (d *Dispatcher) Dispatch(payload interface{}) *Result {
	r := &Result{Status: TaskPending, value: make(chan interface{}, 1)}
	d.incomingTasks <- Task{Payload: payload, Dispatcher: d, result: r}
	DispatcherQueueLength.Inc()
	DispatcherTasksQueued.Inc()
	return r
}

func (d Dispatcher) Stop() {
	d.sigChan <- sigStop
	d.gwait.Wait()
	logger.Infof("all %v agents are stopped", len(d.agents))
}
