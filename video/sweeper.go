package video

import (
	"sort"
	"sync"
	"sync/atomic"
)

// A Counter is a thread-safe counter implementation
type counter uint64

// Incr method increments the counter by some value
func (c *counter) Inc() {
	atomic.AddUint64((*uint64)(c), 1)
}

// Value method returns the counter's current value
func (c *counter) Value() uint64 {
	return atomic.LoadUint64((*uint64)(c))
}

type sweeper struct {
	// counters sync.Map
	mu       *sync.Mutex
	counters map[string]*TallyItem
	swept    map[string]bool
}

type TallyItem struct {
	URL    string
	SDHash string
	Count  uint64
}

func NewSweeper() *sweeper {
	return &sweeper{
		counters: map[string]*TallyItem{},
		swept:    map[string]bool{},
		mu:       &sync.Mutex{},
	}
}

func (s *sweeper) Inc(url, sdHash string) {
	s.mu.Lock()
	if _, ok := s.counters[url]; !ok {
		s.counters[url] = &TallyItem{URL: url, SDHash: sdHash, Count: 1}
	} else {
		s.counters[url].Count++
	}
	s.mu.Unlock()
}

func (s *sweeper) Top(n, lb int) []*TallyItem {
	tally := []*TallyItem{}
	for _, v := range s.counters {
		if v.Count >= uint64(lb) && !s.swept[v.SDHash] {
			tally = append(tally, v)
		}
	}

	// Descending sort
	sort.Slice(tally, func(i, j int) bool { return tally[i].Count > tally[j].Count })
	if n >= len(tally) {
		return tally
	}
	return tally[:n]
}

func (s *sweeper) Sweep(ti []*TallyItem) {
	for _, i := range ti {
		s.swept[i.SDHash] = true
	}
}
