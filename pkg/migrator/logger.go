package migrator

import (
	"github.com/OdyseeTeam/transcoder/pkg/logging"

	"go.uber.org/zap"
)

var logger = logging.Create("migrator", logging.Dev)

func SetLogger(l *zap.SugaredLogger) {
	logger = l
}
