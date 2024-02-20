package manager

import (
	"testing"

	"github.com/Pallinder/go-randomdata"
	"github.com/odyseeteam/transcoder/pkg/logging"
	"github.com/odyseeteam/transcoder/pkg/mfr"

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
	sampleSize := 200
	mfr.SetLogger(logging.Create("mfr", logging.Prod))

	var p1, p2, p3 int
	pool := NewPool()

	s.Nil(pool.Next())

	// Level 5 channel queue
	pool.AddQueue("level5", 0, func(k string, v interface{}, q *mfr.Queue) bool {
		// Randomly determined as level 5
		if isLevel5(k) {
			p1++
			q.Hit(k, v)
			return true
		}
		return false
	})
	// Hardcoded channel queue
	pool.AddQueue("hardcoded", 0, func(k string, v interface{}, q *mfr.Queue) bool {
		// Randomly enabled
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

	for i := 0; i < sampleSize; i++ {
		c := &element{randomdata.Alphanumeric(96), randomdata.Alphanumeric(25)}
		pool.Admit(c.url, c)
	}

	s.GreaterOrEqual(p1, 1)
	s.GreaterOrEqual(p2, 1)
	s.GreaterOrEqual(p3, 1)

	total := 0
	for e := range pool.Out() {
		s.Require().NotNil(e, "pool is exhausted with %v hits", total)
		total += int(e.Hits())
		if total >= sampleSize {
			break
		}
	}
	s.Equal(sampleSize, total)
}

func (s *poolSuite) TestPoolMinHits() {
	pool := NewPool()

	pool.AddQueue("common", 10, func(k string, v interface{}, q *mfr.Queue) bool {
		q.Hit(k, v)
		return true
	})

	go pool.Start()
	s.Nil(pool.Next())

	c := &element{randomdata.Alphanumeric(96), randomdata.Alphanumeric(25)}
	pool.Admit(c.url, c)
	s.Nil(pool.Next())

	for range [8]int{} {
		pool.Admit(c.url, c)
	}
	s.Nil(pool.Next())

	pool.Admit(c.url, c)

	e := pool.Next()
	s.Require().NotNil(e)
	s.Equal(c, e.Value.(*element))

	pool.Admit(c.url, c)
	s.Nil(pool.Next())
}
