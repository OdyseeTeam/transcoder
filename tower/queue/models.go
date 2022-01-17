// Code generated by sqlc. DO NOT EDIT.

package queue

import (
	"database/sql"
	"fmt"
	"time"
)

type Status string

const (
	StatusNew        Status = "new"
	StatusProcessing Status = "processing"
	StatusRetrying   Status = "retrying"
	StatusErrored    Status = "errored"
	StatusFailed     Status = "failed"
	StatusDone       Status = "done"
)

func (e *Status) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = Status(s)
	case string:
		*e = Status(s)
	default:
		return fmt.Errorf("unsupported scan type for Status: %T", src)
	}
	return nil
}

type Task struct {
	ID            int32
	CreatedAt     time.Time
	UpdatedAt     sql.NullTime
	ULID          string
	Status        Status
	Retries       sql.NullInt32
	Stage         sql.NullString
	StageProgress sql.NullInt32
	Error         sql.NullString
	Worker        string
	URL           string
	SDHash        string
	Result        sql.NullString
}
