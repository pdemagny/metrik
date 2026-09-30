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
						startCycle(ctx, logger, runConfig, notifyOnCritical)
					}
				}
			},
		}
	)

	return runCmd
}

// resolve returns a copy of c whose AlertThreshold, MaxTries and RequestTimeout
// fall back to the global value when unset (zero) on the check.
func (cfg Config) resolve(c *check.Check) *check.Check {
	r := check.NewCheck(c.Name, c.Endpoint, cfg.AlertThreshold, cfg.MaxTries)
	r.RequestTimeout = cfg.RequestTimeout
	if c.AlertThreshold != 0 {
		r.AlertThreshold = c.AlertThreshold
	}
	if c.MaxTries != 0 {
		r.MaxTries = c.MaxTries
	}
	if c.RequestTimeout != 0 {
		r.RequestTimeout = c.RequestTimeout
	}
	return r
}

// startCycle probes every check in cfg once, with at most cfg.MaxConcurrent
// probes in flight. It returns once the last probe has started; the returned
// WaitGroup is done when every probe has been reported.
func startCycle(ctx context.Context, logger *zap.Logger, cfg Config, alert func(zapcore.Entry) error) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	limiterCh := make(chan bool, cfg.MaxConcurrent)
	defer close(limiterCh)

	for _, c := range cfg.Checks {
		limiterCh <- true
		wg.Add(1)

		chk := cfg.resolve(c)
		client := createClient(chk.RequestTimeout)
		go func() {
			defer wg.Done()
			childlogger := logger.WithLazy(
				zap.String("name", c.Name),
				zap.String("endpoint", c.Endpoint),
			)
			childlogger.Info("check started")

			results, err := chk.Run(ctx, childlogger, client)
			<-limiterCh

			report(childlogger, alert, results, err, chk.AlertThreshold)
		}()
	}

	return wg
}

// report logs the outcome of one probe at the right severity and routes
// error-level entries through alert.
func report(logger *zap.Logger, alert func(zapcore.Entry) error, results *check.Result, err error, alertThreshold time.Duration) {
	alertLogger := zap.New(logger.Core(), zap.Hooks(alert))
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
		logger.Warn("slow response",
			zap.Int("code", results.Code),
			zap.Duration("duration", results.Duration))
	}

	logger.Info("check completed",
		zap.Int("code", results.Code),
		zap.Duration("duration", results.Duration),
	)
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
