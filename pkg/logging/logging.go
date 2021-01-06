package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	loggers     = map[string]*zap.SugaredLogger{}
	environment = EnvDebug

	EnvDebug = "debug"
	EnvProd  = "prod"
)

var Prod = zap.NewProductionConfig()
var Dev = zap.NewDevelopmentConfig()

func init() {
	Prod.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
}

func Create(name string, cfg zap.Config) *zap.SugaredLogger {
	l, _ := cfg.Build()
	return l.Named(name).Sugar()
}
