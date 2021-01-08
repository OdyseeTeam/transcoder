package queue

import (
	"context"
	"database/sql"
	"fmt"
)

var (
	queryTaskGet         = `select id, sd_hash, created_at, url, progress, started_at, type, status from tasks where id = $1`
	queryTaskGetBySDHash = `select id, sd_hash, created_at, url, progress, started_at, type, status from tasks where sd_hash = $1`
	queryList            = `select id, sd_hash, created_at, url, progress, started_at, type, status from tasks`
	queryTaskAdd         = `
		insert into tasks (
			url, sd_hash, type, status, created_at
		) values (
			$1, $2, $3, "new", datetime('now')
		);
	`
	queryTaskPoll = `
		select id, sd_hash, created_at, url, progress, started_at, type, status from tasks
		where status in ("new", "released") order by id desc limit 1
	`
	queryTaskMarkStarted = fmt.Sprintf(
		`update tasks set started_at = datetime('now'), progress = 0, status = "%v" where id = $1`,
		StatusStarted)
	queryTaskMarkReleased = fmt.Sprintf(
		`update tasks set started_at = null, progress = null, status = "%v" where id = $1`,
		StatusReleased)
	queryUpdateProgress = `update tasks set progress = $1 where id = $2`
	queryUpdateStatus   = `update tasks set status = $1 where id = $2`
)

type rowScanner interface {
	Scan(dest ...interface{}) error
}

type AddParams struct {
	URL    string
	SDHash string
	Type   string
}

type GetParams struct {
	URL    string
	SDHash string
	ID     int
}

func (q *Queries) Add(ctx context.Context, arg AddParams) (*Task, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, queryTaskAdd, arg.URL, arg.SDHash, arg.Type)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	lastID, err := res.LastInsertId()
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return q.Get(ctx, uint32(lastID))
}

// Poll pops an unprocessed task from the queue and marks it as started. It is assumed that task poller
// will eventually mark task as rejected, completed or failed.
func (q *Queries) Poll(ctx context.Context) (*Task, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	row := tx.QueryRowContext(ctx, queryTaskPoll)
	i, err := scan(row)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	err = q.updateStatusTx(tx, ctx, i.ID, StatusPending)
	if err != nil {
		tx.Rollback()
		return &i, err
	}
	err = tx.Commit()
	if err != nil {
		return &i, err
	}

	i.Progress = sql.NullFloat64{Float64: 0, Valid: true}
	i.Status = StatusPending
	return &i, err
}

func (q *Queries) Release(ctx context.Context, id uint32) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, queryTaskMarkReleased, id)
	if err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

func (q *Queries) updateProgress(ctx context.Context, id uint32, progress float64) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	row := tx.QueryRowContext(ctx, queryTaskGet, id)
	i, err := scan(row)
	if i.Status != StatusStarted {
		tx.Rollback()
		return fmt.Errorf("wrong status for progressing task: %v", i.Status)
	}
	_, err = tx.ExecContext(ctx, queryUpdateProgress, progress, id)
	if err != nil {
		tx.Rollback()
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

func (q *Queries) updateStatus(ctx context.Context, id uint32, status string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	err = q.updateStatusTx(tx, ctx, id, status)
	if err != nil {
		tx.Rollback()
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

func (q *Queries) updateStatusTx(tx *sql.Tx, ctx context.Context, id uint32, status string) error {
	r, err := tx.ExecContext(ctx, queryUpdateStatus, status, id)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("task %v not found", id)
	}
	return nil
}

func (q *Queries) Get(ctx context.Context, id uint32) (*Task, error) {
	row := q.db.QueryRowContext(ctx, queryTaskGet, id)
	i, err := scan(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return &i, err
	}
	return &i, nil
}

func (q *Queries) GetBySDHash(ctx context.Context, sdHash string) (*Task, error) {
	row := q.db.QueryRowContext(ctx, queryTaskGetBySDHash, sdHash)
	i, err := scan(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return &i, err
	}
	return &i, nil
}

func (q *Queries) List(ctx context.Context) ([]*Task, error) {
	var tasks []*Task

	rows, err := q.db.QueryContext(ctx, queryList)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		i, err := scan(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, &i)
	}
	return tasks, nil
}

func scan(r rowScanner) (Task, error) {
	var i Task
	if err := r.Scan(
		&i.ID,
		&i.SDHash,
		&i.CreatedAt,
		&i.URL,
		&i.Progress,
		&i.StartedAt,
		&i.Type,
		&i.Status,
	); err != nil {
		return i, err
	}
	return i, nil
}
