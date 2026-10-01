package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func CreateLogger() *zap.Logger {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.RFC3339TimeEncoder

	config := zap.Config{
		Level:             zap.NewAtomicLevelAt(getLogLevelFromEnv()),
		DisableCaller:     true,
		DisableStacktrace: true,
		Encoding:          "json",
		EncoderConfig:     encoderCfg,
		OutputPaths: []string{
			"stdout",
		},
		ErrorOutputPaths: []string{
			"stdout",
		},
	}

	return zap.Must(config.Build())
}

func getLogLevelFromEnv() zapcore.Level {
	level, err := zapcore.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		return zap.InfoLevel
	}
	return level
}
