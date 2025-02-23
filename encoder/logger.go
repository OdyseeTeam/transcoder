package encoder

import (
	"github.com/OdyseeTeam/transcoder/pkg/logging"

	"go.uber.org/zap"
)

var logger = logging.Create("encoder", logging.Dev) // nolint:unused

func SetLogger(l *zap.SugaredLogger) {
	logger = l
}
