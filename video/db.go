package video

import (
	"context"
	"database/sql"

	_ "github.com/mattn/go-sqlite3" // sqlite
)

type DBTX interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

func New(db DBTX) *Queries {
	return &Queries{db: db}
}

type Queries struct {
	db DBTX
}

// func (q *Queries) WithTx(tx *sql.Tx) *Queries {
// 	return &Queries{
// 		db: tx,
// 	}
// }
