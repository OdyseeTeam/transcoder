package video

import (
	"context"
	"database/sql"

	"go.uber.org/zap"
)

var logger = zap.NewExample().Sugar().Named("video")

// Library contains methods for accessing videos database.
type Library struct {
	queries Queries
}

func NewLibrary(db *sql.DB) *Library {
	return &Library{queries: Queries{db}}
}

// Add records data about video into database.
func (q Library) Add(url, sdHash, _type, path string) (*Video, error) {
	tp := AddParams{URL: url, SDHash: sdHash, Type: _type, Path: path}
	return q.queries.Add(context.Background(), tp)
}

func (q Library) Get(sdHash string) (*Video, error) {
	return q.queries.Get(context.Background(), sdHash)
}
