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
	URL   string
	Count uint64
}

func NewSweeper() *sweeper {
	return &sweeper{
		counters: sync.Map{},
		swept:    map[string]bool{},
	}
}

func (s *sweeper) Inc(url string) {
	ic := counter(0)
	i, _ := s.counters.LoadOrStore(url, &ic)
	c := i.(*counter)
	c.Inc()
}

func (s *sweeper) Top(n int, sweep bool) []TallyItem {
	tally := []TallyItem{}
	s.counters.Range(func(k, v interface{}) bool {
		ti := TallyItem{k.(string), v.(*counter).Value()}
		if s.swept[ti.URL] {
			return true
		}
		tally = append(tally, ti)
		if sweep {
			s.swept[ti.URL] = true
		}
		return true
	})
	sort.Slice(tally, func(i, j int) bool { return tally[i].Count < tally[j].Count })
	if n >= len(tally) {
		return tally
	}
	return tally[:n]
}
