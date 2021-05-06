package pool

import (
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/lbryio/transcoder/pkg/mfr"

	"github.com/stretchr/testify/suite"
)

var errUnderway = errors.New("underway")
var errForbidden = errors.New("forbidden")

type poolSuite struct {
	suite.Suite
}

type claim struct {
	sdHash, url string
}

func isLevel5(key string) bool {
	return rand.Intn(2) == 0
}

func isChannelEnabled(key string) bool {
	return rand.Intn(2) == 0
}

func TestQueueSuite(t *testing.T) {
	suite.Run(t, new(poolSuite))
}

func (s *poolSuite) TestPool() {
	var p1, p2, p3 int
	rand.Seed(time.Now().UnixNano())
	pool := NewPool()

	s.Nil(pool.Next())

	// Level 5 channel queue
	pool.AddQueue("level5", func(k string, v interface{}, q *mfr.Queue) error {
		if q.Exists(k) || isLevel5(k) {
			p1++
			q.Hit(k, v)
			return errUnderway
		}
		return nil
	})
	// Hardcoded channel queue
	pool.AddQueue("hardcoded", func(k string, v interface{}, q *mfr.Queue) error {
		if q.Exists(k) || isChannelEnabled(k) {
			p2++
			q.Hit(k, v)
			return errUnderway
		}
		return nil
	})
	// Common queue
	pool.AddQueue("common", func(k string, v interface{}, q *mfr.Queue) error {
		q.Hit(k, v)
		p3++
		return errForbidden
	})

	go pool.StartOut()

	s.Nil(pool.Next())

	for range [1000]int{} {
		c := claim{randomString(96), randomString(25)}
		pool.Enter(c.url, c)
	}

	s.GreaterOrEqual(pool.queues[0].queue.Hits(), uint(1))
	s.GreaterOrEqual(pool.queues[1].queue.Hits(), uint(1))
	s.GreaterOrEqual(pool.queues[2].queue.Hits(), uint(1))

	total := 0
	for total < 1000 {
		e := pool.Next()
		s.Require().NotNil(e, "pool is exhausted with %v hits", total)
		total += int(e.Hits())
	}
	s.Equal(total, 1000)
}

func randomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	b := make([]rune, n)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}
