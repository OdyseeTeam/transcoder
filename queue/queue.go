package queue

import (
	"context"
	"time"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/pkg/worker"
	_ "github.com/mattn/go-sqlite3" // sqlite
	"go.uber.org/zap"
)

var logger = zap.NewExample().Sugar().Named("queue")

type Queue struct {
	queries Queries
}

func NewQueue(db *db.DB) *Queue {
	return &Queue{queries: Queries{db}}
}

func (q Queue) Add(url, sdHash, _type string) (*Task, error) {
	tp := AddParams{URL: url, SDHash: sdHash, Type: formats.TypeHLS}
	return q.queries.Add(context.Background(), tp)
}

func (q Queue) Poll() (*Task, error) {
	return q.queries.Poll(context.Background())
}

func (q Queue) Release(id uint32) error {
	return q.queries.Release(context.Background(), id)
}

func (q Queue) Get(id uint32) (*Task, error) {
	return q.queries.Get(context.Background(), id)
}

func (q Queue) GetBySDHash(sdHash string) (*Task, error) {
	return q.queries.GetBySDHash(context.Background(), sdHash)
}

func (q Queue) List() ([]*Task, error) {
	return q.queries.List(context.Background())
}

func (q Queue) Reject(id uint32) error {
	return q.queries.updateStatus(context.Background(), id, StatusRejected)
}

func (q Queue) Complete(id uint32) error {
	return q.queries.updateStatus(context.Background(), id, StatusCompleted)
}

func (q *Queue) StartPoller(workers int) *Poller {
	p := &Poller{
		queue:         q,
		incomingTasks: make(chan *Task, workers),
	}
	w := worker.NewTicker(p, 100*time.Millisecond)
	w.Start()
	return p
}
