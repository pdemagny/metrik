package main

import (
	"context"

	"metrik/internal/config"
	"metrik/internal/runner"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func NewRunCmd(ctx context.Context, logger *zap.Logger) *cobra.Command {
	runCmd := &cobra.Command{
		Use:     "run",
		Aliases: []string{"r"},
		Short:   "Run health checks.",
		Long:    "Run health checks.",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			var runConfig config.Config
			err := rootConfig.Unmarshal(&runConfig)
			if err != nil {
				logger.Fatal("unable to decode into struct", zap.Error(err))
			}
			if err := runConfig.Validate(); err != nil {
				logger.Fatal("invalid config", zap.Error(err))
			}
			rootConfig.WatchConfig()
			rootConfig.OnConfigChange(func(_ fsnotify.Event) {
				logger.Debug("reloading config ...")
				err := rootConfig.Unmarshal(&runConfig)
				if err != nil {
					logger.Fatal("unable to decode into struct", zap.Error(err))
				}
				if err := runConfig.Validate(); err != nil {
					logger.Fatal("invalid config", zap.Error(err))
				}
			})

			return runner.Run(ctx, logger, func() config.Config { return runConfig })
		},
	}

	return runCmd
}
