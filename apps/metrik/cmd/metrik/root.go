package main

import (
	"context"
	_ "embed"
	"strings"
	"time"

	log "metrik/internal/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

//go:embed version.txt
var version string

var (
	rootConfig     = viper.New()
	debug, verbose bool
	rootCmd        = &cobra.Command{
		Use:     "metrik",
		Short:   "Metrik healthchecker.",
		Long:    "Command-Line Interface for the Metrik healthchecker.",
		Version: strings.TrimSpace(version),
	}
)

func main() {
	// Logger
	logger := log.CreateLogger()
	defer logger.Sync()
	zap.RedirectStdLog(logger)

	// Context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	flags := rootCmd.PersistentFlags()
	flags.BoolVar(&debug, "debug", false, "Activate debug mode, printing data structures and useful debugging messages.")
	flags.BoolVarP(&verbose, "verbose", "v", true, "Enable more verbose output.")
	flags.DurationP("runInterval", "i", 15*time.Second, "Interval between each check execution.")
	flags.IntP("maxConcurrent", "c", 10, "Maximum number of checks that can run concurrently.")
	flags.DurationP("requestTimeout", "t", 5*time.Second, "Timeout for each request.")
	flags.DurationP("alertThreshold", "a", 3*time.Second, "Duration after which a check is considered failed.")
	flags.IntP("maxTries", "m", 3, "Number of retries to do when a request fails.")
	flags.Int("failureThreshold", 3, "Consecutive failed cycles before a check is suspended (0 disables the circuit breaker).")
	flags.Duration("cooldown", 60*time.Second, "How long a suspended check is skipped before a trial probe.")
	err = rootConfig.BindPFlags(flags)
	if err != nil {
		logger.Fatal("unable to bind flags", zap.Error(err))
	}

	// Commands
	rootCmd.AddCommand(NewRunCmd(ctx, logger))

	cobra.CheckErr(rootCmd.Execute())
}
