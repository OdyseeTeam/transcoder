package dispatcher

import (
	"github.com/OdyseeTeam/transcoder/pkg/logging"
	"go.uber.org/zap"
)

var logger = logging.Create("dispatcher", logging.Dev)

func SetLogger(l *zap.SugaredLogger) {
	logger = l
}
