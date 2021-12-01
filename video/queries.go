package video

import (
	"context"
	"fmt"
)

var (
	allVideoColumns = `url, sd_hash, type, path, remote_path,
		created_at, channel,
		last_accessed, access_count,
		size, checksum`
	queryVideoGet = fmt.Sprintf(`select %v from videos where sd_hash = $1 limit 1`, allVideoColumns)
	queryVideoAdd = `
		insert into videos (
			url, sd_hash, type, path, channel, size, checksum, created_at
		) values (
			$1, $2, $3, $4, $5, $6, $7, datetime('now')
		)`
	queryVideoUpdateAccess     = `update videos set last_accessed = datetime('now'), access_count = access_count + 1 where sd_hash = $2`
	queryVideoUpdateRemotePath = `update videos set remote_path = $1 where sd_hash = $2`
	queryVideoUpdatePath       = `update videos set path = $1 where sd_hash = $2`
	queryVideoLeastAccessed    = `
		select strftime('%s', 'now') - strftime('%s', last_accessed) las from videos
		where las > 3600 * 24 * 2 order by -las`
	queryVideoDelete         = `delete from videos where sd_hash = $1`
	queryVideoListAll        = fmt.Sprintf(`select %s from videos`, allVideoColumns)
	queryVideoListLocalOnly  = fmt.Sprintf(`select %s from videos where path != "" and remote_path = ""`, allVideoColumns)
	queryVideoListLocal      = fmt.Sprintf(`select %s from videos where path != "" and remote_path != ""`, allVideoColumns)
	queryVideoListRemoteOnly = fmt.Sprintf(`select %s from videos where path = "" and remote_path != ""`, allVideoColumns)
)

type AddParams struct {
	URL      string
	SDHash   string
	Type     string
	Path     string
	Channel  string
	Size     int64
	Checksum string
}

func (q *Queries) Add(ctx context.Context, arg AddParams) (*Video, error) {
	res, err := q.db.ExecContext(
		ctx, queryVideoAdd,
		arg.URL, arg.SDHash, arg.Type, arg.Path, arg.Channel, arg.Size, arg.Checksum,
	)
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
	var (
		i   Video
		err error
	)

	row := q.db.QueryRowContext(ctx, queryVideoGet, sdHash)
	if i, err = scan(row); err != nil {
		return nil, err
	}

	_, err = q.db.ExecContext(ctx, queryVideoUpdateAccess, sdHash)
	if err != nil {
		return nil, err
	}

	return &i, nil
}

func (q *Queries) ListAll(ctx context.Context) ([]*Video, error) {
	var (
		err  error
		list []*Video
	)

	rows, err := q.db.QueryContext(ctx, queryVideoListAll)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var i Video
		if i, err = scan(rows); err != nil {
			return nil, err
		}
		list = append(list, &i)
	}

	return list, nil
}

func (q *Queries) ListLocal(ctx context.Context) ([]*Video, error) {
	var (
		err  error
		list []*Video
	)

	rows, err := q.db.QueryContext(ctx, queryVideoListLocal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var i Video
		if i, err = scan(rows); err != nil {
			return nil, err
		}
		list = append(list, &i)
	}

	return list, nil
}

func (q *Queries) ListLocalOnly(ctx context.Context) ([]*Video, error) {
	var (
		err  error
		list []*Video
	)

	rows, err := q.db.QueryContext(ctx, queryVideoListLocalOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var i Video
		if i, err = scan(rows); err != nil {
			return nil, err
		}
		list = append(list, &i)
	}

	return list, nil
}

func (q *Queries) ListRemoteOnly(ctx context.Context) ([]*Video, error) {
	var (
		err  error
		list []*Video
	)

	rows, err := q.db.QueryContext(ctx, queryVideoListRemoteOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var i Video
		if i, err = scan(rows); err != nil {
			return nil, err
		}
		list = append(list, &i)
	}

	return list, nil
}

func (q *Queries) UpdateRemotePath(ctx context.Context, sdHash, url string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	r, err := tx.ExecContext(ctx, queryVideoUpdateRemotePath, url, sdHash)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		tx.Rollback()
		return fmt.Errorf("video %v not found", sdHash)
	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

func (q *Queries) UpdatePath(ctx context.Context, sdHash, path string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	r, err := tx.ExecContext(ctx, queryVideoUpdatePath, path, sdHash)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		tx.Rollback()
		return fmt.Errorf("video %v not found", sdHash)
	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

func (q *Queries) Delete(ctx context.Context, sdHash string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	r, err := tx.ExecContext(ctx, queryVideoDelete, sdHash)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		tx.Rollback()
		return fmt.Errorf("video %v not found", sdHash)
	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scan(r rowScanner) (Video, error) {
	var i Video
	if err := r.Scan(
		&i.URL,
		&i.SDHash,
		&i.Type,
		&i.Path,
		&i.RemotePath,
		&i.CreatedAt,
		&i.Channel,
		&i.LastAccessed,
		&i.AccessCount,
		&i.Size,
		&i.Checksum,
	); err != nil {
		return i, err
	}
	return i, nil
}
