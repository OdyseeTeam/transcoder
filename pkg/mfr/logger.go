package mfr

import (
	"github.com/OdyseeTeam/transcoder/pkg/logging"

	"go.uber.org/zap"
)

var logger = logging.Create("mfr", logging.Dev)

func SetLogger(l *zap.SugaredLogger) {
	logger = l
}
