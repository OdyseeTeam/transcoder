package manager

import (
	"math/rand"
	"testing"
	"time"

	"github.com/lbryio/transcoder/pkg/mfr"

	"github.com/stretchr/testify/suite"
)

type poolSuite struct {
	suite.Suite
}

type element struct {
	sdHash, url string
}

func TestPoolSuite(t *testing.T) {
	suite.Run(t, new(poolSuite))
}

func (s *poolSuite) TestPool() {
	var p1, p2, p3 int
	rand.Seed(time.Now().UnixNano())
	pool := NewPool()

	s.Nil(pool.Next())

	// Level 5 channel queue
	pool.AddQueue("level5", 0, func(k string, v interface{}, q *mfr.Queue) bool {
		if isLevel5(k) {
			p1++
			q.Hit(k, v)
			return true
		}
		return false
	})
	// Hardcoded channel queue
	pool.AddQueue("hardcoded", 0, func(k string, v interface{}, q *mfr.Queue) bool {
		if isChannelEnabled(k) {
			p2++
			q.Hit(k, v)
			return true
		}
		return false
	})
	// Common queue
	pool.AddQueue("common", 0, func(k string, v interface{}, q *mfr.Queue) bool {
		q.Hit(k, v)
		p3++
		return true
	})

	go pool.Start()

	s.Nil(pool.Next())

	for range [1000]int{} {
		c := element{randomString(96), randomString(25)}
		pool.Admit(c.url, c)
	}

	s.GreaterOrEqual(pool.levels[0].queue.Hits(), uint(1))
	s.GreaterOrEqual(pool.levels[1].queue.Hits(), uint(1))
	s.GreaterOrEqual(pool.levels[2].queue.Hits(), uint(1))

	total := 0
	for total < 1000 {
		e := pool.Next()
		s.Require().NotNil(e, "pool is exhausted with %v hits", total)
		total += int(e.Hits())
	}
	s.Equal(total, 1000)
}

func (s *poolSuite) TestPoolMinHits() {
	pool := NewPool()

	pool.AddQueue("common", 10, func(k string, v interface{}, q *mfr.Queue) bool {
		q.Hit(k, v)
		return true
	})

	go pool.Start()
	s.Nil(pool.Next())

	c := element{randomString(96), randomString(25)}
	pool.Admit(c.url, c)
	s.Nil(pool.Next())

	for range [8]int{} {
		pool.Admit(c.url, c)
	}
	s.Nil(pool.Next())

	pool.Admit(c.url, c)

	go func() {
		e := pool.Next()
		s.Require().NotNil(e)
		s.Equal(c, e.Value.(element))
	}()

	go func() {
		s.Nil(pool.Next())
	}()
}
