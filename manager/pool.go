package manager

import (
	"container/ring"
	"time"

	"github.com/OdyseeTeam/transcoder/pkg/mfr"
	"github.com/OdyseeTeam/transcoder/pkg/resolve"
	"github.com/prometheus/client_golang/prometheus"
)

var pollTimeout = 50 * time.Millisecond

type level struct {
	name    string
	queue   *mfr.Queue
	keeper  Gatekeeper
	minHits uint
}

// Pool contains queues which can admit items based on gatekeeper functions.
type Pool struct {
	levels   []*level
	out      chan *mfr.Item
	stopChan chan interface{}
}

// Gatekeeper defines a function that checks if supplied queue item and its value should be admitted to the queue.
type Gatekeeper func(key string, value interface{}, queue *mfr.Queue) bool

func NewPool() *Pool {
	pool := &Pool{
		levels:   []*level{},
		out:      make(chan *mfr.Item),
		stopChan: make(chan interface{}, 1),
	}
	return pool
}

// AddQueue adds a queue and its gatekeeper function to the pool.
func (p *Pool) AddQueue(name string, minHits uint, k Gatekeeper) {
	p.levels = append(p.levels, &level{name: name, queue: mfr.NewQueue(), keeper: k, minHits: minHits})
}

// Admit retries to put item into the first queue that would accept it.
// Queues are traversed in the same order they are added.
// If gatekeeper returns an error, admission stops and the error is returned to the caller.
func (p *Pool) Admit(key string, value interface{}) error {
	ll := logger.With("key", key)
	for i, level := range p.levels {
		ll.Debugw("checking level", "level", level.name)
		q := level.queue
		_, s := level.queue.Get(key)

		mql := QueueLength.With(prometheus.Labels{"queue": level.name})
		mqh := QueueHits.With(prometheus.Labels{"queue": level.name})
		switch s {
		case mfr.StatusNone:
			if level.keeper(key, value, level.queue) {
				mql.Inc()
				mqh.Inc()
				if i == len(p.levels)-1 {
					return resolve.ErrTranscodingForbidden
				}
				return resolve.ErrTranscodingQueued
			}
		case mfr.StatusActive:
			mqh.Inc()
			q.Hit(key, value)
			return resolve.ErrTranscodingUnderway
		case mfr.StatusQueued:
			mqh.Inc()
			q.Hit(key, value)
			return resolve.ErrTranscodingQueued
		case mfr.StatusDone:
			mqh.Inc()
			q.Hit(key, value)
			// This is to prevent race conditions when the item has been transcoded already
			// while the request is still in flight.
			return resolve.ErrTranscodingUnderway
		}
	}
	ll.Debug("suitable level not found")
	return resolve.ErrChannelNotEnabled
}

// Start will launch the cycle of retrieving items out of queues. Should be called after at least one `AddQueue` call.
// Queues are pooled sequentially.
func (p *Pool) Start() {
	r := ring.New(len(p.levels))
	for i := 0; i < r.Len(); i++ {
		r.Value = p.levels[i]
		r = r.Next()
	}
	for {
		r = r.Next()
		select {
		case <-p.stopChan:
			close(p.out)
			return
		default:
		}

		l := r.Value.(*level)
		item := l.queue.MinPop(l.minHits)
		if item == nil {
			// Non-stop polling will cause excessive CPU load.
			time.Sleep(pollTimeout)
			continue
		}
		logger.Named("pool").Debugf("popping item %v", item.Value)
		QueueLength.With(prometheus.Labels{"queue": l.name}).Dec()
		QueueItemAge.With(prometheus.Labels{"queue": l.name}).Observe(float64(item.Age()))
		p.out <- item
	}
}

func (p *Pool) Out() <-chan *mfr.Item {
	return p.out
}

// Next returns the next item in the queue almost in a non-blocking way.
func (p *Pool) Next() *mfr.Item {
	select {
	case e := <-p.out:
		return e
	case <-time.After(pollTimeout + 50*time.Millisecond):
		return nil
	}
}

// Stop stops the queue polling routine.
func (p *Pool) Stop() {
	p.stopChan <- true
}
