package ladder

import (
	"github.com/odyseeteam/transcoder/pkg/logging"
	"go.uber.org/zap"
)

var logger = logging.Create("formats", logging.Dev)

func SetLogger(l *zap.SugaredLogger) {
	logger = l
}
