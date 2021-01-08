package queue

import (
	"database/sql"
)

const (
	StatusNew       = "new"
	StatusPending   = "pending"
	StatusStarted   = "started"
	StatusRejected  = "rejected"
	StatusReleased  = "released"
	StatusCompleted = "completed"
)

type Task struct {
	ID        uint32
	SDHash    string
	CreatedAt string
	URL       string
	Progress  sql.NullFloat64
	StartedAt sql.NullString
	Status    string
	Type      string
}
