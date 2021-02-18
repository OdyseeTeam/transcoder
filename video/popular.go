package video

import (
	"sync/atomic"

	cmap "github.com/orcaman/concurrent-map"
)

type Sweeper struct {
	popular cmap.ConcurrentMap
}

type item struct {
	hits  uint64
	swept bool
}

func NewSweeper() *Sweeper {
	return &Sweeper{
		popular: cmap.New(),
	}
}

func (s *Sweeper) Hit(url string) {
	if ie, ok := s.popular.Get(url); ok {
		i := ie.(*item)
		atomic.AddUint64(&i.hits, 1)
	} else {
		s.popular.SetIfAbsent(url, &item{hits: 1})
	}
}
