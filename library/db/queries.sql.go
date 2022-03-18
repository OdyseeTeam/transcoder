// Code generated by sqlc. DO NOT EDIT.
// source: queries.sql

package db

import (
	"context"
	"database/sql"
)

const addChannel = `-- name: AddChannel :one
INSERT into channels (
    url, claim_id, priority
) VALUES (
    $1, $2, $3
)
RETURNING id, created_at, url, claim_id, priority
`

type AddChannelParams struct {
	URL      string
	ClaimID  string
	Priority ChannelPriority
}

func (q *Queries) AddChannel(ctx context.Context, arg AddChannelParams) (Channel, error) {
	row := q.db.QueryRowContext(ctx, addChannel, arg.URL, arg.ClaimID, arg.Priority)
	var i Channel
	err := row.Scan(
		&i.ID,
		&i.CreatedAt,
		&i.URL,
		&i.ClaimID,
		&i.Priority,
	)
	return i, err
}

const addVideo = `-- name: AddVideo :one
INSERT INTO videos (
  tid, sd_hash, url, channel, storage, path, size, checksum
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, created_at, updated_at, accessed_at, access_count, tid, url, sd_hash, channel, storage, path, size, checksum
`

type AddVideoParams struct {
	TID      string
	SDHash   string
	URL      string
	Channel  string
	Storage  string
	Path     string
	Size     int64
	Checksum sql.NullString
}

func (q *Queries) AddVideo(ctx context.Context, arg AddVideoParams) (Video, error) {
	row := q.db.QueryRowContext(ctx, addVideo,
		arg.TID,
		arg.SDHash,
		arg.URL,
		arg.Channel,
		arg.Storage,
		arg.Path,
		arg.Size,
		arg.Checksum,
	)
	var i Video
	err := row.Scan(
		&i.ID,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.AccessedAt,
		&i.AccessCount,
		&i.TID,
		&i.URL,
		&i.SDHash,
		&i.Channel,
		&i.Storage,
		&i.Path,
		&i.Size,
		&i.Checksum,
	)
	return i, err
}

const deleteVideo = `-- name: DeleteVideo :exec
DELETE from videos
WHERE tid = $1
`

func (q *Queries) DeleteVideo(ctx context.Context, tid string) error {
	_, err := q.db.ExecContext(ctx, deleteVideo, tid)
	return err
}

const getAllChannels = `-- name: GetAllChannels :many
SELECT id, created_at, url, claim_id, priority from channels
`

func (q *Queries) GetAllChannels(ctx context.Context) ([]Channel, error) {
	rows, err := q.db.QueryContext(ctx, getAllChannels)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Channel
	for rows.Next() {
		var i Channel
		if err := rows.Scan(
			&i.ID,
			&i.CreatedAt,
			&i.URL,
			&i.ClaimID,
			&i.Priority,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getAllVideos = `-- name: GetAllVideos :many
SELECT id, created_at, updated_at, accessed_at, access_count, tid, url, sd_hash, channel, storage, path, size, checksum FROM videos
`

func (q *Queries) GetAllVideos(ctx context.Context) ([]Video, error) {
	rows, err := q.db.QueryContext(ctx, getAllVideos)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Video
	for rows.Next() {
		var i Video
		if err := rows.Scan(
			&i.ID,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.AccessedAt,
			&i.AccessCount,
			&i.TID,
			&i.URL,
			&i.SDHash,
			&i.Channel,
			&i.Storage,
			&i.Path,
			&i.Size,
			&i.Checksum,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getAllVideosForStorage = `-- name: GetAllVideosForStorage :many
SELECT id, created_at, updated_at, accessed_at, access_count, tid, url, sd_hash, channel, storage, path, size, checksum FROM videos
WHERE storage = $1
`

func (q *Queries) GetAllVideosForStorage(ctx context.Context, storage string) ([]Video, error) {
	rows, err := q.db.QueryContext(ctx, getAllVideosForStorage, storage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Video
	for rows.Next() {
		var i Video
		if err := rows.Scan(
			&i.ID,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.AccessedAt,
			&i.AccessCount,
			&i.TID,
			&i.URL,
			&i.SDHash,
			&i.Channel,
			&i.Storage,
			&i.Path,
			&i.Size,
			&i.Checksum,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getChannel = `-- name: GetChannel :one
SELECT id, created_at, url, claim_id, priority from channels
WHERE claim_id = $1
`

func (q *Queries) GetChannel(ctx context.Context, claimID string) (Channel, error) {
	row := q.db.QueryRowContext(ctx, getChannel, claimID)
	var i Channel
	err := row.Scan(
		&i.ID,
		&i.CreatedAt,
		&i.URL,
		&i.ClaimID,
		&i.Priority,
	)
	return i, err
}

const getVideo = `-- name: GetVideo :one
SELECT id, created_at, updated_at, accessed_at, access_count, tid, url, sd_hash, channel, storage, path, size, checksum FROM videos
WHERE sd_hash = $1 LIMIT 1
`

func (q *Queries) GetVideo(ctx context.Context, sdHash string) (Video, error) {
	row := q.db.QueryRowContext(ctx, getVideo, sdHash)
	var i Video
	err := row.Scan(
		&i.ID,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.AccessedAt,
		&i.AccessCount,
		&i.TID,
		&i.URL,
		&i.SDHash,
		&i.Channel,
		&i.Storage,
		&i.Path,
		&i.Size,
		&i.Checksum,
	)
	return i, err
}

const recordVideoAccess = `-- name: RecordVideoAccess :exec
UPDATE videos
SET last_accessed = NOW(), access_count = access_count + 1
WHERE sd_hash = $1
`

func (q *Queries) RecordVideoAccess(ctx context.Context, sdHash string) error {
	_, err := q.db.ExecContext(ctx, recordVideoAccess, sdHash)
	return err
}