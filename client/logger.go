package client

import (
	"github.com/lbryio/transcoder/pkg/logging"
	"go.uber.org/zap"
)

var logger = logging.Create("client", logging.Dev)

func SetLogger(l *zap.SugaredLogger) {
	logger = l
}
