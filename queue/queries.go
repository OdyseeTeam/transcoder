package queue

import (
	"context"
	"database/sql"
	"fmt"
)

const (
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
	queryTaskStart      = `update tasks set started_at = datetime('now'), progress = 0, status = "started" where id = $1`
	queryTaskRelease    = `update tasks set started_at = null, progress = null, status = "released" where id = $1`
	queryUpdateProgress = `update tasks set progress = datetime('now') where id = $1`
	queryUpdateStatus   = `update tasks set status = $2 where id = $1`
)

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
	var i Task

	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	row := tx.QueryRowContext(ctx, queryTaskPoll)
	err = row.Scan(
		&i.ID,
		&i.SDHash,
		&i.CreatedAt,
		&i.URL,
		&i.Progress,
		&i.StartedAt,
		&i.Type,
		&i.Status,
	)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	_, err = tx.ExecContext(ctx, queryTaskStart, i.ID)
	if err != nil {
		tx.Rollback()
		return &i, err
	}
	err = tx.Commit()
	if err != nil {
		return &i, err
	}

	i.Progress = sql.NullFloat64{Float64: 0, Valid: true}
	i.Status = StatusStarted
	return &i, err
}

func (q *Queries) Release(ctx context.Context, id uint32) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, queryTaskRelease, id)
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

func (q *Queries) updateStatus(ctx context.Context, id uint32, status string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	r, err := tx.ExecContext(ctx, queryUpdateStatus, status, id)
	if err != nil {
		tx.Rollback()
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		tx.Rollback()
		return err
	}
	if n == 0 {
		tx.Rollback()
		return fmt.Errorf("task %v not found", id)
	}
	tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

func (q *Queries) Get(ctx context.Context, id uint32) (*Task, error) {
	var i Task

	row := q.db.QueryRowContext(ctx, queryTaskGet, id)
	err := row.Scan(
		&i.ID,
		&i.SDHash,
		&i.CreatedAt,
		&i.URL,
		&i.Progress,
		&i.StartedAt,
		&i.Type,
		&i.Status,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return &i, err
	}
	return &i, nil
}

func (q *Queries) GetBySDHash(ctx context.Context, sdHash string) (*Task, error) {
	var i Task

	row := q.db.QueryRowContext(ctx, queryTaskGetBySDHash, sdHash)
	err := row.Scan(
		&i.ID,
		&i.SDHash,
		&i.CreatedAt,
		&i.URL,
		&i.Progress,
		&i.StartedAt,
		&i.Type,
		&i.Status,
	)
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
		var i Task
		if err := rows.Scan(
			&i.ID,
			&i.SDHash,
			&i.CreatedAt,
			&i.URL,
			&i.Progress,
			&i.StartedAt,
			&i.Type,
			&i.Status,
		); err != nil {
			return nil, err
		}
		tasks = append(tasks, &i)
	}
	return tasks, nil
}
