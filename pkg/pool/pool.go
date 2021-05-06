package pool

import (
	"container/ring"
	"time"

	"github.com/lbryio/transcoder/pkg/mfr"
)

type queueKeeper struct {
	name   string
	queue  *mfr.Queue
	keeper Gatekeeper
}

type Pool struct {
	queues   []*queueKeeper
	out      chan *mfr.Item
	stopChan chan interface{}
}

// Gatekeeper defines a function that checks if supplied queue item and its value should be admitted to the queue.
type Gatekeeper func(key string, value interface{}, queue *mfr.Queue) error

func NewPool() *Pool {
	pool := &Pool{
		queues:   []*queueKeeper{},
		out:      make(chan *mfr.Item),
		stopChan: make(chan interface{}, 1),
	}
	return pool
}

// AddQueue adds a queue and its gatekeeper function to the pool.
func (p *Pool) AddQueue(name string, k Gatekeeper) {
	p.queues = append(p.queues, &queueKeeper{name: name, queue: mfr.NewQueue(), keeper: k})
}

// Enter attempts to admit an item to the first queue that would accept it.
// Queues are traversed in the same order they are added.
// If gatekeeper returns an error, admission stops and the error is returned to the caller.
func (p *Pool) Enter(key string, value interface{}) error {
	for _, qk := range p.queues {
		err := qk.keeper(key, value, qk.queue)
		if err != nil {
			return err
		}
	}
	return nil
}

// StartOut will launch the cycle of retrieving items out of queues. Should be called after at least one `AddQueue` call.
// Queues are pooled sequentially.
func (p *Pool) StartOut() {
	qr := ring.New(len(p.queues))
	for i := 0; i < qr.Len(); i++ {
		qr.Value = p.queues[i]
		qr = qr.Next()
	}
	for {
		qr = qr.Next()
		select {
		case <-p.stopChan:
			close(p.out)
			return
		default:
		}

		item := qr.Value.(*queueKeeper).queue.Pop()
		if item == nil {
			continue
		}
		p.out <- item
	}
}

func (p *Pool) Out() <-chan *mfr.Item {
	return p.out
}

func (p *Pool) Next() *mfr.Item {
	select {
	case e := <-p.out:
		return e
	case <-time.After(10 * time.Millisecond):
		return nil
	}
}

// Stop stops the Out.
func (p *Pool) Stop() {
	p.stopChan <- true
}
