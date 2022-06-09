package priority

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/suite"
)

type prioritySuite struct {
	suite.Suite
	queue *Queue
}

func TestPrioritySuite(t *testing.T) {
	suite.Run(t, new(prioritySuite))
}

func (s *prioritySuite) SetupTest() {
	rand.Seed(time.Now().UnixNano())

	ropts := &redis.Options{
		Addr:     "localhost:6379",
		Password: "odyredis", // no password set
		DB:       0,          // use default DB
	}
	q := NewQueue(ropts)

	s.queue = q
}

func (s *prioritySuite) TestIncPop() {
	popItem1 := Item{URL: fmt.Sprintf("lbry://%s?%s", randomdata.Alphanumeric(25), randomdata.Alphanumeric(96))}
	popItem2 := Item{URL: fmt.Sprintf("lbry://%s?%s", randomdata.Alphanumeric(25), randomdata.Alphanumeric(96))}
	popItem3 := Item{URL: fmt.Sprintf("lbry://%s?%s", randomdata.Alphanumeric(25), randomdata.Alphanumeric(96))}

	q := s.queue

	wg := &sync.WaitGroup{}
	wg.Add(4)
	go func() {
		defer wg.Done()
		for range [1000]bool{} {
			q.Inc(popItem1)
		}
	}()
	go func() {
		defer wg.Done()
		for range [999]bool{} {
			q.Inc(popItem2)
		}
	}()
	go func() {
		defer wg.Done()
		for range [900]bool{} {
			q.Inc(popItem3)
		}
	}()
	go func() {
		defer wg.Done()
		for range [500]bool{} {
			q.Inc(Item{URL: fmt.Sprintf("lbry://%s?%s", randomdata.Alphanumeric(25), randomdata.Alphanumeric(96))})
		}
	}()
	wg.Wait()

	q.rdb.Wait(context.Background(), 1, 5*time.Second)
	pos1, err := q.Pop()
	s.Require().NoError(err)
	s.Equal(popItem1, pos1.Item)
	s.EqualValues(1000, pos1.Score)

	pos2, err := q.Pop()
	s.Require().NoError(err)
	s.Equal(popItem2, pos2.Item)
	s.EqualValues(999, pos2.Score)

	pos3, err := q.Pop()
	s.Require().NoError(err)
	s.Equal(popItem3, pos3.Item)
	s.EqualValues(900, pos3.Score)
}
