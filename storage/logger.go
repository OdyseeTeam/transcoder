package storage

import (
	"github.com/OdyseeTeam/transcoder/pkg/logging"
	"go.uber.org/zap"
)

var logger = logging.Create("storage", logging.Dev)

func SetLogger(l *zap.SugaredLogger) {
	logger = l
}
