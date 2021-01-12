package video

import (
	"context"
	"time"
)

const (
	queryVideoGet = `select
		url, sd_hash, type, path,
		created_at, channel, last_accessed
		from video where sd_hash = $1 limit 1`
	queryVideoAdd = `
		insert into video (
			url, sd_hash, type, path, channel, created_at
		) values (
			$1, $2, $3, $4, $5, datetime('now')
		);
	`
	queryVideoUpdateLastAccessed = `update video set last_accessed = $1 where sd_hash = $2`
)

type AddParams struct {
	URL     string
	SDHash  string
	Type    string
	Path    string
	Channel string
}

func (q *Queries) Add(ctx context.Context, arg AddParams) (*Video, error) {
	res, err := q.db.ExecContext(ctx, queryVideoAdd, arg.URL, arg.SDHash, arg.Type, arg.Path, arg.Channel)
	if err != nil {
		return nil, err
	}
	_, err = res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return q.Get(ctx, arg.SDHash)
}

func (q *Queries) Get(ctx context.Context, sdHash string) (*Video, error) {
	var i Video

	row := q.db.QueryRowContext(ctx, queryVideoGet, sdHash)
	err := row.Scan(
		&i.URL,
		&i.SDHash,
		&i.Type,
		&i.Path,
		&i.CreatedAt,
		&i.Channel,
		&i.LastAccessed,
	)
	if err != nil {
		return nil, err
	}

	_, err = q.db.ExecContext(ctx, queryVideoUpdateLastAccessed, time.Now(), sdHash)
	if err != nil {
		return nil, err
	}

	return &i, nil
}
