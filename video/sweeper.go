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
	counters sync.Map
	swept    map[string]bool
}

type TallyItem struct {
	URL    string
	SDHash string
	Count  uint64
}

func NewSweeper() *sweeper {
	return &sweeper{
		counters: sync.Map{},
		swept:    map[string]bool{},
	}
}

func (s *sweeper) Inc(url, sdHash string) {
	ic := counter(0)
	i, _ := s.counters.LoadOrStore([2]string{url, sdHash}, &ic)
	c := i.(*counter)
	c.Inc()
}

func (s *sweeper) Top(n, lb int) []TallyItem {
	tally := []TallyItem{}
	s.counters.Range(func(k, v interface{}) bool {
		us := k.([2]string)
		ti := TallyItem{URL: us[0], SDHash: us[1], Count: v.(*counter).Value()}
		if ti.Count <= uint64(lb) || s.swept[ti.SDHash] {
			return true
		}
		tally = append(tally, ti)
		return true
	})

	// Descending sort
	sort.Slice(tally, func(i, j int) bool { return tally[i].Count > tally[j].Count })
	if n >= len(tally) {
		return tally
	}
	return tally[:n]
}

func (s *sweeper) Sweep(ti []TallyItem) {
	for _, i := range ti {
		s.swept[i.SDHash] = true
	}
}
