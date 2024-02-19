package mfr

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

const (
	StatusNone   = iota
	StatusQueued // waiting to get to the top
	StatusActive // being processed
	StatusDone   // done processing
)

// Item is a queue storage unit.
type Item struct {
	key       string
	Value     interface{}
	queue     *Queue
	posParent *list.Element
	created   time.Time
}

type Position struct {
	entries map[*Item]int
	freq    uint
}

// Queue stores a priority queue with Items with most Hits being at the top.
type Queue struct {
	entries   map[string]*Item
	positions *list.List
	size      uint
	hits      uint
	mu        sync.RWMutex
}

var now = time.Now

// NewQueue initializes an empty priority queue suitable for registering Hits right away.
func NewQueue() *Queue {
	queue := &Queue{
		positions: list.New(),
		entries:   map[string]*Item{},
		mu:        sync.RWMutex{},
	}
	queue.positions.PushFront(&Position{freq: 1, entries: map[*Item]int{}})
	return queue
}

// Hit puts Item stoStatusActive at `key` higher up in the queue, or inserts it to the bottom of the pile if the item is not present.
func (q *Queue) Hit(key string, value interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if item, ok := q.entries[key]; ok {
		q.increment(item)
		logger.Debugw("increment", "key", key, "pointer", fmt.Sprintf("%p", value), "hits", item.Hits())
	} else {
		logger.Debugw("insert", "key", key, "pointer", fmt.Sprintf("%p", value))
		q.insert(key, value)
	}
}

// Get retrieves item by key along with its processing status.
func (q *Queue) Get(key string) (*Item, int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if e, ok := q.entries[key]; ok {

		return e, e.posParent.Value.(*Position).entries[e]
	}
	return nil, StatusNone
}

// Peek returns the top-most item of the queue without marking it as being processed.
func (q *Queue) Peek() *Item {
	return q.pop(false, 0)
}

// Pop returns the top-most item of the queue and marks it as being processed so consecutive calls will return subsequent items.
func (q *Queue) Pop() *Item {
	return q.pop(true, 0)
}

// MinPeek returns the top-most item of the queue if it has a required minimum of hits, without marking it as being processed.
func (q *Queue) MinPeek(minHits uint) *Item {
	return q.pop(false, minHits)
}

// MinPop returns the top-most item of the queue if it has a required minimum of hits
// and marks it as being processed so consecutive calls will return subsequent items.
func (q *Queue) MinPop(minHits uint) *Item {
	return q.pop(true, minHits)
}

func (q *Queue) pop(lockItem bool, minHits uint) *Item {
	var (
		i, it  *Item
		status int
	)
	top := q.positions.Back()

	for top != nil && i == nil {
		pos := top.Value.(*Position)
		q.mu.Lock()
		for it, status = range pos.entries {
			if it.Hits() < minHits {
				q.mu.Unlock()
				return nil
			}
			if status == StatusActive || status == StatusDone {
				continue
			}
			i = it
			logger.Debugw("pop candidate", "key", i.key, "status", pos.entries[i], "neighbors", fmt.Sprintf("%v", pos.entries), "q", fmt.Sprintf("%p", q))
			if lockItem {
				pos.entries[i] = StatusActive
			}
			break
		}
		q.mu.Unlock()
		top = top.Prev()
	}
	if i != nil {
		logger.Debugw("pop", "key", i.key, "pointer", fmt.Sprintf("%p", i.Value), "hits", i.Hits())
	}
	return i
}

// Release returns the item back into the queue for future possibility to be `Pop`ped again.
func (q *Queue) Release(key string) {
	q.setStatus(key, StatusQueued)
}

// Done marks the queue item as fully processed.
func (q *Queue) Done(key string) {
	q.setStatus(key, StatusDone)
}

func (q *Queue) Hits() uint {
	return q.hits
}

func (q *Queue) setStatus(key string, status int) {
	item := q.entries[key]
	if item == nil {
		return
	}
	q.mu.Lock()
	item.posParent.Value.(*Position).entries[item] = status
	q.mu.Unlock()
}

func (q *Queue) insert(key string, value interface{}) {
	posParent := q.positions.Front()
	item := &Item{
		key:       key,
		Value:     value,
		queue:     q,
		posParent: posParent,
		created:   now(),
	}
	posParent.Value.(*Position).entries[item] = StatusQueued
	q.entries[key] = item
	q.size++
	q.hits++
}

func (q *Queue) increment(item *Item) {
	pos := item.posParent.Value.(*Position)
	status := pos.entries[item]
	nextFreq := pos.freq + 1
	delete(pos.entries, item)

	nextPosParent := item.posParent.Next()
	if nextPosParent == nil || nextPosParent.Value.(*Position).freq > nextFreq {
		nextPosParent = q.positions.InsertAfter(&Position{freq: nextFreq, entries: map[*Item]int{}}, item.posParent)
	}
	nextPosParent.Value.(*Position).entries[item] = status
	item.posParent = nextPosParent
	q.hits++
}

func (q *Queue) Size() uint {
	return q.size
}

// Hits returns the number of hits for the item.
func (i *Item) Hits() uint {
	return i.posParent.Value.(*Position).freq
}

// Release returns the item back into the queue for future possibility to be `Pop`ped again (it won't stop registering hits).
func (i *Item) Release() {
	logger.Debugw("release", "key", i.key)
	i.queue.Release(i.key)
}

// Done marks the item as fully processed (it won't stop registering hits).
func (i *Item) Done() {
	logger.Debugw("done", "key", i.key)
	i.queue.Done(i.key)
}

// Age returns how many seconds have passed since the item was created.
func (i *Item) Age() int {
	return int(time.Since(i.created).Seconds())
}
