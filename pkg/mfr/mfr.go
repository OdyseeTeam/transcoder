package mfr

import (
	"container/list"
	"sync"
)

const (
	red   = iota // in processing
	green        // ready for processing
	blue         // processed
)

// Item is a queue storage unit.
type Item struct {
	key       string
	Value     interface{}
	posParent *list.Element
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

// Hit puts Item stored at `key` higher up in the queue, or adds it to the bottom of the pile if the item is not present.
func (q *Queue) Hit(key string, value interface{}) {
	q.mu.Lock()
	if item, ok := q.entries[key]; ok {
		q.increment(item)
	} else {
		q.add(key, value)
	}
	q.mu.Unlock()
}

// Exists check if item with a given key is registered in the queue.
func (q *Queue) Exists(key string) bool {
	q.mu.Lock()
	_, ok := q.entries[key]
	q.mu.Unlock()
	return ok
}

// Peek returns the top-most item of the queue without marking it as being processed.
func (q *Queue) Peek() *Item {
	return q.pop(false)
}

// Pop returns the top-most item of the queue and marks it as being processed so consecutive calls will return subsequent items.
func (q *Queue) Pop() *Item {
	return q.pop(true)
}

func (q *Queue) pop(lockItem bool) *Item {
	var i *Item
	top := q.positions.Back()

	for top != nil && i == nil {
		pos := top.Value.(*Position)
		q.mu.Lock()
		for it, status := range pos.entries {
			if status == red || status == blue {
				continue
			}
			i = it
			if lockItem {
				pos.entries[it] = red
			}
			break
		}
		q.mu.Unlock()
		top = top.Prev()
	}
	return i
}

// Release returns items back into the queue for future possibility to be `Pop`ped again.
func (q *Queue) Release(key string) {
	q.setStatus(key, green)
}

// Fold marks the queue item as fully processed.
func (q *Queue) Fold(key string) {
	q.setStatus(key, blue)
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

func (q *Queue) add(key string, value interface{}) {
	posParent := q.positions.Front()
	item := &Item{
		key:       key,
		Value:     value,
		posParent: posParent,
	}
	posParent.Value.(*Position).entries[item] = green
	q.entries[key] = item
	q.size++
	q.hits++
}

func (q *Queue) increment(item *Item) {
	pos := item.posParent.Value.(*Position)
	nextFreq := pos.freq + 1
	delete(pos.entries, item)

	nextPosParent := item.posParent.Next()
	if nextPosParent == nil || nextPosParent.Value.(*Position).freq > nextFreq {
		nextPosParent = q.positions.InsertAfter(&Position{freq: nextFreq, entries: map[*Item]int{}}, item.posParent)
	}
	nextPosParent.Value.(*Position).entries[item] = green
	item.posParent = nextPosParent
	q.hits++
}

// Hits returns the number of hits for the item.
func (i *Item) Hits() uint {
	return i.posParent.Value.(*Position).freq
}
