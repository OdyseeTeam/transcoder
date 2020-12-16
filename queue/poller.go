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
}

func (p *Poller) Process() error {
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

func (p Poller) Shutdown() {
	close(p.incomingTasks)
}

func (p Poller) IncomingTasks() <-chan *Task {
	return p.incomingTasks
}

func (p Poller) RejectTask(t *Task) error {
	return p.queue.Reject(t.ID)
}

func (p Poller) ReleaseTask(t *Task) {
	p.queue.Release(t.ID)
}

func (p Poller) CompleteTask(t *Task) {
	p.queue.Complete(t.ID)
}
