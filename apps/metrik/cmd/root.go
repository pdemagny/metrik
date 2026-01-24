package cmd

import (
	"context"
	"strings"
	"time"

	loggerctx "metrik/internal/context"
	logger "metrik/internal/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	rootConfig     = viper.New()
	debug, verbose bool
	usageMessages  = map[string]string{
		"debug":          "Activate debug mode, printing data structures and useful debugging messages.",
		"verbose":        "Enable more verbose output.",
		"runInterval":    "Interval between each check execution.",
		"maxConcurrent":  "Maximum number of checks that can run concurrently.",
		"requestTimeout": "Timeout for each request.",
		"alertThreshold": "Duration after which a check is considered failed.",
		"maxTries":       "Number of retries to do when a request fails.",
	}
	rootCmd = &cobra.Command{
		Use:   "metrik",
		Short: "Metrik healthchecker.",
		Long:  "Command-Line Interface for the Metrik healthchecker.",
	}
	globalFlags = pflag.NewFlagSet("global", pflag.ContinueOnError)
)

func Execute() {
	// Logger
	logger := logger.CreateLogger()
	defer logger.Sync() //nolint:errcheck
	zap.RedirectStdLog(logger)

	// Context
	ctx := loggerctx.NewContextWithLogger(context.Background(), logger)
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
	}()

	// Config
	rootConfig.SetConfigType("yaml")
	rootConfig.SetConfigName("metrik")
	rootConfig.AddConfigPath("/config")
	rootConfig.AddConfigPath("./configs")
	rootConfig.AddConfigPath(".")
	rootConfig.SetEnvPrefix("metrik")
	rootConfig.AutomaticEnv()
	rootConfig.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	err := rootConfig.ReadInConfig()
	if err != nil {
		logger.Fatal("error reading config file", zap.Error(err))
	}

	// Flags
	globalFlags.BoolVar(&debug, "debug", false, usageMessages["debug"])
	globalFlags.BoolVarP(&verbose, "verbose", "v", true, usageMessages["verbose"])
	globalFlags.DurationP("checkInterval", "i", 10*time.Second, usageMessages["checkInterval"])
	globalFlags.IntP("maxConcurrent", "c", 10, usageMessages["maxConcurrent"])
	globalFlags.DurationP("requestTimeout", "t", 5*time.Second, usageMessages["requestTimeout"])
	globalFlags.DurationP("alertThreshold", "a", 3*time.Second, usageMessages["alertThreshold"])
	globalFlags.IntP("maxTries", "m", 3, usageMessages["maxTries"])
	rootCmd.PersistentFlags().AddFlagSet(globalFlags)
	err = rootConfig.BindPFlags(globalFlags)
	if err != nil {
		logger.Fatal("unable to bind flags", zap.Error(err))
	}

	// Commands
	rootCmd.AddCommand(NewRunCmd(ctx))
	rootCmd.AddCommand(NewVersionCmd())

	cobra.CheckErr(rootCmd.Execute())
}
