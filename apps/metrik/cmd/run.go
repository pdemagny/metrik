package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"metrik/pkg/check"
	"net/http"
	"sync"
	"time"

	loggerctx "metrik/internal/context"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type (
	Config struct {
		Debug          bool           `yaml:"debug"`
		Verbose        bool           `yaml:"verbose"`
		RunInterval    time.Duration  `yaml:"runInterval"`
		MaxConcurrent  int            `yaml:"maxConcurrent"`
		RequestTimeout time.Duration  `yaml:"requestTimeout"`
		AlertThreshold time.Duration  `yaml:"alertThreshold"`
		MaxTries       uint           `yaml:"maxTries"`
		Checks         []*check.Check `yaml:"checks"`
	}
)

func NewRunCmd(ctx context.Context) *cobra.Command {
	var (
		runCmd = &cobra.Command{
			Use:     "run",
			Aliases: []string{"r"},
			Short:   "Run health checks.",
			Long:    "Run health checks.",
			Args:    cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				var logger *zap.Logger
				logger = loggerctx.LoggerFromContext(ctx)

				var runConfig Config
				err := rootConfig.Unmarshal(&runConfig)
				if err != nil {
					logger.Fatal("unable to decode into struct", zap.Error(err))
				}
				rootConfig.WatchConfig()
				rootConfig.OnConfigChange(func(in fsnotify.Event) {
					logger.Debug("reloading config ...")
					err := rootConfig.Unmarshal(&runConfig)
					if err != nil {
						logger.Fatal("unable to decode into struct", zap.Error(err))
					}
				})

				for {
					select {
					case <-ctx.Done():
						return nil
					case <-time.Tick(runConfig.RunInterval):
						var wg sync.WaitGroup
						limiterCh := make(chan bool, runConfig.MaxConcurrent)
						defer close(limiterCh)

						for _, c := range runConfig.Checks {
							limiterCh <- true
							wg.Add(1)

							var alertThreshold time.Duration
							if c.AlertThreshold != 0 {
								alertThreshold = c.AlertThreshold
							} else {
								alertThreshold = runConfig.AlertThreshold
							}

							var maxTries uint
							if c.MaxTries != 0 {
								maxTries = c.MaxTries
							} else {
								maxTries = runConfig.MaxTries
							}

							var requestTimeout time.Duration
							if c.RequestTimeout != 0 {
								requestTimeout = c.RequestTimeout
							} else {
								requestTimeout = runConfig.RequestTimeout
							}

							client := createClient(requestTimeout)
							check := check.NewCheck(c.Name, c.Endpoint, alertThreshold, maxTries)
							go func() {
								defer wg.Done()
								childlogger := logger.WithLazy(
									zap.String("name", c.Name),
									zap.String("endpoint", c.Endpoint),
								)
								childlogger.Info("check started")

								results, err := check.Run(ctx, childlogger, client)
								<-limiterCh

								alertLogger := zap.New(childlogger.Core(), zap.Hooks(notifyOnCritical))
								if err != nil {
									alertLogger.Error("check failed",
										zap.Int("code", results.Code),
										zap.Duration("duration", results.Duration),
										zap.Error(err),
									)
									return
								}

								if results.Body.Status != http.StatusOK {
									alertLogger.Error("application error",
										zap.Int("code", results.Code),
										zap.Duration("duration", results.Duration),
										zap.Int("status", results.Body.Status),
										zap.String("message", results.Body.Message),
										zap.String("reason", results.Body.Reason),
									)
									return
								}

								if results.Duration > alertThreshold {
									childlogger.Warn("slow response",
										zap.Int("code", results.Code),
										zap.Duration("duration", results.Duration))
								}

								childlogger.Info("check completed",
									zap.Int("code", results.Code),
									zap.Duration("duration", results.Duration),
								)

							}()
						}

						go func() {
							wg.Wait()
						}()
					}
				}
			},
		}
	)

	return runCmd
}

func createClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false,
			DisableKeepAlives:   false,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: timeout,
	}
}

func notifyOnCritical(entry zapcore.Entry) error {
	if entry.Level >= zapcore.ErrorLevel {
		// Send a notification to Slack, PagerDuty, etc.Config.
		fmt.Printf("ALERT! %s\n", entry.Message)
	}
	return nil
}
