package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var global *zap.Logger

func Init(env string) {
	var cfg zap.Config
	if env == "production" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	l, err := cfg.Build(zap.AddCallerSkip(0))
	if err != nil {
		panic(err)
	}
	global = l
}

func L() *zap.Logger {
	if global == nil {
		l, _ := zap.NewDevelopment()
		global = l
	}
	return global
}

func Sync() {
	if global != nil {
		_ = global.Sync()
	}
}
