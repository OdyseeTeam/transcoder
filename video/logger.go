package video

import (
	"github.com/lbryio/transcoder/pkg/logging"
	"go.uber.org/zap"
)

var logger = logging.Create("video", logging.Dev)

func SetLogger(l *zap.SugaredLogger) {
	logger = l
}
