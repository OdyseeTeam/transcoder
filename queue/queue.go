package queue

import (
	"context"
	"time"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/pkg/worker"

	_ "github.com/mattn/go-sqlite3" // sqlite
)

type Queue struct {
	queries Queries
}

func NewQueue(db *db.DB) *Queue {
	return &Queue{queries: Queries{db}}
}

func (q Queue) Add(url, sdHash, _type string) (*Task, error) {
	tp := AddParams{URL: url, SDHash: sdHash, Type: formats.TypeHLS}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return q.queries.Add(ctx, tp)
}

func (q Queue) Poll() (*Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return q.queries.Poll(ctx)
}

func (q Queue) Release(id uint32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return q.queries.Release(ctx, id)
}

func (q Queue) Get(id uint32) (*Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return q.queries.Get(ctx, id)
}

func (q Queue) GetBySDHash(sdHash string) (*Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return q.queries.GetBySDHash(ctx, sdHash)
}

func (q Queue) List() ([]*Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return q.queries.List(ctx)
}

func (q Queue) Reject(id uint32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return q.queries.updateStatus(ctx, id, StatusRejected)
}

func (q Queue) Start(id uint32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return q.queries.updateStatus(ctx, id, StatusStarted)
}

func (q Queue) Complete(id uint32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return q.queries.updateStatus(ctx, id, StatusCompleted)
}

func (q Queue) UpdateProgress(id uint32, progress float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return q.queries.updateProgress(ctx, id, progress)
}

func (q *Queue) StartPoller(workers int) *Poller {
	p := &Poller{
		queue:         q,
		incomingTasks: make(chan *Task, 1000),
	}
	w := worker.NewTicker(p, 1*time.Second)
	w.Start()
	return p
}
