package queue

import (
	"database/sql"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/formats"
	"github.com/stretchr/testify/suite"
)

type QueueSuite struct {
	suite.Suite
	db *db.DB
}

func TestQueueSuite(t *testing.T) {
	suite.Run(t, new(QueueSuite))
}

func (s *QueueSuite) SetupSuite() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func (s *QueueSuite) SetupTest() {
	s.db = db.OpenTestDB()
	s.db.MigrateUp(InitialMigration)
}

func (s *QueueSuite) TearDownTest() {
	s.db.Cleanup()
}

func (s *QueueSuite) TestQueueAdd() {
	q := NewQueue(s.db)
	url := "lbry://" + db.RandomString(32)
	sdHash := db.RandomString(96)
	task, err := q.Add(url, sdHash, formats.TypeHLS)
	s.Require().NoError(err)
	s.Equal(url, task.URL)
	s.Equal(sdHash, task.SDHash)
	s.Equal(StatusNew, task.Status)
}

func (s *QueueSuite) TestQueueGetBySDHash() {
	q := NewQueue(s.db)
	url := "lbry://" + db.RandomString(32)
	sdHash := db.RandomString(96)

	task, err := q.GetBySDHash(sdHash)
	s.Require().Nil(task)

	task, err = q.Add(url, sdHash, formats.TypeHLS)
	s.Require().NoError(err)

	task, err = q.GetBySDHash(sdHash)
	s.Require().NoError(err)
	s.Equal(url, task.URL)
	s.Equal(sdHash, task.SDHash)
	s.Equal(StatusNew, task.Status)
}

func (s *QueueSuite) TestQueuePoll() {
	q := NewQueue(s.db)
	var (
		lastTask *Task
		err      error
	)
	for range [100]int{} {
		lastTask, err = q.Add(fmt.Sprintf("lbry://%v", db.RandomString(32)), db.RandomString(96), formats.TypeHLS)
		s.Require().NoError(err)
	}

	for i := range [100]int{} {
		task, err := q.Poll()
		s.Require().NoError(err)
		s.Equal(task.ID, lastTask.ID-uint32(i))
		s.EqualValues(sql.NullFloat64{Float64: 0, Valid: true}, task.Progress)
		s.Equal(StatusStarted, task.Status)
	}

	ts, err := q.List()
	s.Require().NoError(err)
	s.Require().Len(ts, 100)
	for _, t := range ts {
		s.EqualValues(sql.NullFloat64{Float64: 0, Valid: true}, t.Progress)
		s.Equal(StatusStarted, t.Status)
	}

	task, err := q.Poll()
	s.EqualError(err, sql.ErrNoRows.Error())
	s.Nil(task)
}

func (s *QueueSuite) TestQueueRelease() {
	q := NewQueue(s.db)
	_, err := q.Add(db.RandomString(32), db.RandomString(96), formats.TypeHLS)
	s.Require().NoError(err)

	pTask, err := q.Poll()
	s.Require().NoError(err)
	q.Release(pTask.ID)

	pTask, err = q.Get(pTask.ID)
	s.Require().NoError(err)
	s.Require().NotNil(pTask)
	s.Equal(StatusReleased, pTask.Status)

	pTask2, err := q.Poll()
	s.Require().NoError(err)
	s.Equal(pTask.ID, pTask2.ID)
	s.Equal(StatusStarted, pTask2.Status)
}

func (s *QueueSuite) TestQueueReject() {
	q := NewQueue(s.db)
	_, err := q.Add(db.RandomString(32), db.RandomString(96), formats.TypeHLS)
	s.Require().NoError(err)

	pTask, err := q.Poll()
	s.Require().NoError(err)
	err = q.Reject(pTask.ID)
	s.Require().NoError(err)

	pTask, err = q.Get(pTask.ID)
	s.Require().NoError(err)
	s.Require().NotNil(pTask)
	s.Equal(StatusRejected, pTask.Status)

	_, err = q.Poll()
	s.Require().Equal(sql.ErrNoRows, err)
}
