package queue

import (
	"fmt"
	"testing"
	"time"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/formats"
	"github.com/stretchr/testify/suite"
)

func TestPollerSuite(t *testing.T) {
	suite.Run(t, new(PollerSuite))
}

type PollerSuite struct {
	suite.Suite
	db *db.DB
}

func (s *PollerSuite) SetupTest() {
	s.db = db.OpenTestDB()
	s.db.MigrateUp(InitialMigration)
}

func (s *PollerSuite) StartPollerWorker(p *Poller, q *Queue, wf func(*Task)) {
	for t := range p.IncomingTasks() {
		time.Sleep(10 * time.Millisecond)
		wf(t)
	}
}

func (s *PollerSuite) TestStartPoller() {
	q := NewQueue(s.db)
	p := q.StartPoller(5)
	for range [5]bool{} {
		go s.StartPollerWorker(p, q, func(_ *Task) {})
	}

	for range [10]int{} {
		_, err := q.Add(fmt.Sprintf("lbry://%v", db.RandomString(32)), db.RandomString(96), formats.TypeHLS)
		s.Require().NoError(err)
	}

	for {
		if p.incomingTaskCounter == 10 {
			break
		}
	}

	ts, err := q.List()
	s.Require().NoError(err)
	s.Require().Len(ts, 10)

	for _, t := range ts {
		s.Equal(StatusPending, t.Status)
	}
}

func (s *PollerSuite) TestPollerShutdown() {
	q := NewQueue(s.db)
	p := q.StartPoller(5)
	for range [5]bool{} {
		go s.StartPollerWorker(p, q, func(_ *Task) {})
	}

	for range [20]int{} {
		_, err := q.Add(fmt.Sprintf("lbry://%v", db.RandomString(32)), db.RandomString(96), formats.TypeHLS)
		s.Require().NoError(err)
	}

	for {
		if p.incomingTaskCounter == 10 {
			p.Shutdown()
			break
		}
	}

	ts, err := q.List()
	s.Require().NoError(err)
	s.Require().Len(ts, 20)

	for _, t := range ts[10:] {
		s.Equal(StatusNew, t.Status)
	}
	for _, t := range ts[:10] {
		s.Equal(StatusPending, t.Status)
	}
}
