package queue

import (
	"database/sql"

	"github.com/lbryio/transcoder/pkg/worker"
	"github.com/pkg/errors"
)

type Poller struct {
	queue               *Queue
	incomingTasks       chan *Task
	incomingTaskCounter uint64
	isShutdown          bool
}

func (p *Poller) Process() error {
	if p.IsShutdown() {
		return worker.ErrShutdown
	}

	t, err := p.queue.Poll()
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return errors.Wrap(worker.FatalError, err.Error())
	}
	p.incomingTasks <- t
	p.incomingTaskCounter++
	return nil
}

func (p *Poller) Shutdown() {
	logger.Infow("poller shutting down, no more tasks will be sent to the workers")
	p.isShutdown = true
}

func (p *Poller) IsShutdown() bool {
	return p.isShutdown
}

func (p Poller) IncomingTasks() <-chan *Task {
	return p.incomingTasks
}

func (p Poller) StartTask(t *Task) error {
	return p.queue.Start(t.ID)
}

func (p Poller) ProgressTask(t *Task, progress float64) error {
	return p.queue.UpdateProgress(t.ID, progress)
}

func (p Poller) RejectTask(t *Task) error {
	return p.queue.Reject(t.ID)
}

func (p Poller) ReleaseTask(t *Task) error {
	return p.queue.Release(t.ID)
}

func (p Poller) CompleteTask(t *Task) {
	p.queue.Complete(t.ID)
}
