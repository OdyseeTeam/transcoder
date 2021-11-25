package zapadapter

import (
	"context"

	"github.com/lbryio/transcoder/pkg/logging"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"logur.dev/logur"
)

// kvLogger is a Logur adapter for Uber's Zap.
type kvLogger struct {
	logger *zap.SugaredLogger
	core   zapcore.Core
}

// NewKV returns a new Logur kvLogger.
// If kvLogger is nil, a default instance is created.
func NewKV(logger *zap.Logger) *kvLogger {
	if logger == nil {
		logger = zap.L()
	}
	logger = logger.WithOptions(zap.AddCallerSkip(1))

	return &kvLogger{
		logger: logger.Sugar(),
		core:   logger.Core(),
	}
}

// Trace implements the Logur kvLogger interface.
func (l *kvLogger) Trace(msg string, keyvals ...interface{}) {
	// Fall back to Debug
	l.logger.Debugw(msg, keyvals...)
}

// Debug implements the Logur kvLogger interface.
func (l *kvLogger) Debug(msg string, keyvals ...interface{}) {
	if !l.core.Enabled(zap.DebugLevel) {
		return
	}
	l.logger.Debugw(msg, keyvals...)
}

// Info implements the Logur kvLogger interface.
func (l *kvLogger) Info(msg string, keyvals ...interface{}) {
	if !l.core.Enabled(zap.InfoLevel) {
		return
	}
	l.logger.Infow(msg, keyvals...)
}

// Warn implements the Logur kvLogger interface.
func (l *kvLogger) Warn(msg string, keyvals ...interface{}) {
	if !l.core.Enabled(zap.WarnLevel) {
		return
	}
	l.logger.Warnw(msg, keyvals...)
}

// Error implements the Logur kvLogger interface.
func (l *kvLogger) Error(msg string, keyvals ...interface{}) {
	if !l.core.Enabled(zap.ErrorLevel) {
		return
	}
	l.logger.Errorw(msg, keyvals...)
}

func (l *kvLogger) TraceContext(_ context.Context, msg string, keyvals ...interface{}) {
	l.Trace(msg, keyvals...)
}

func (l *kvLogger) DebugContext(_ context.Context, msg string, keyvals ...interface{}) {
	l.Debug(msg, keyvals...)
}

func (l *kvLogger) InfoContext(_ context.Context, msg string, keyvals ...interface{}) {
	l.Info(msg, keyvals...)
}

func (l *kvLogger) WarnContext(_ context.Context, msg string, keyvals ...interface{}) {
	l.Warn(msg, keyvals...)
}

func (l *kvLogger) ErrorContext(_ context.Context, msg string, keyvals ...interface{}) {
	l.Error(msg, keyvals...)
}

// ...
func (l *kvLogger) With(keyvals ...interface{}) logging.KVLogger {
	return NewKV(l.logger.With(keyvals...).Desugar())
}

// LevelEnabled implements the Logur LevelEnabler interface.
func (l *kvLogger) LevelEnabled(level logur.Level) bool {
	switch level {
	case logur.Trace:
		return l.core.Enabled(zap.DebugLevel)
	case logur.Debug:
		return l.core.Enabled(zap.DebugLevel)
	case logur.Info:
		return l.core.Enabled(zap.InfoLevel)
	case logur.Warn:
		return l.core.Enabled(zap.WarnLevel)
	case logur.Error:
		return l.core.Enabled(zap.ErrorLevel)
	}

	return true
}
