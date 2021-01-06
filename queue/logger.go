package queue

import (
	"github.com/lbryio/transcoder/pkg/logging"
	"go.uber.org/zap"
)

var logger = logging.Create("queue", logging.Dev)

func SetLogger(l *zap.SugaredLogger) {
	logger = l
}
