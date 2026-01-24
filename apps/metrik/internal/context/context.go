package context

import (
	"context"

	"go.uber.org/zap"
)

type loggerKey struct{}

func NewContextWithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	ctx = context.WithValue(ctx, loggerKey{}, logger)
	return ctx
}

func LoggerFromContext(ctx context.Context) *zap.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*zap.Logger); ok {
		return logger
	}
	return zap.NewNop()
}
