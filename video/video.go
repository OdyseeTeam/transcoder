package video

import (
	"context"

	"github.com/lbryio/transcoder/db"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var loggerName = "video"
var logger = zap.NewExample().Sugar().Named(loggerName)

func InitDebugLogger() {
	l, _ := zap.NewDevelopment()
	l = l.Named(loggerName)
	logger = l.Sugar()
	logger.Debugw("logger configured", "mode", "debug")
}

func InitProductionLogger() {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	l, _ := cfg.Build()
	l = l.Named(loggerName)
	logger = l.Sugar()
	logger.Infow("logger configured", "mode", "production")
}

// Library contains methods for accessing videos database.
type Library struct {
	queries Queries
}

func NewLibrary(db *db.DB) *Library {
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
