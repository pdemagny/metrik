package cmd

import (
	"context"
	"crypto/tls"
	"metrik/pkg/check"
	"net/http"
	"sync"
	"time"

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

func NewRunCmd(ctx context.Context, logger *zap.Logger) *cobra.Command {
	var (
		runCmd = &cobra.Command{
			Use:     "run",
			Aliases: []string{"r"},
			Short:   "Run health checks.",
			Long:    "Run health checks.",
			Args:    cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
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

				// One limiter for the whole run, so overlapping cycles share the maxConcurrent bound.
				limiterCh := make(chan bool, runConfig.MaxConcurrent)
				for {
					select {
					case <-ctx.Done():
						return nil
					case <-time.Tick(runConfig.RunInterval):
						startCycle(ctx, logger, runConfig, limiterCh, notifyOnCritical(logger))
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
	r := *c
	if r.AlertThreshold == 0 {
		r.AlertThreshold = cfg.AlertThreshold
	}
	if r.MaxTries == 0 {
		r.MaxTries = cfg.MaxTries
	}
	// Zero tries means unlimited retries to the backoff library; always keep them bounded.
	if r.MaxTries == 0 {
		r.MaxTries = 1
	}
	if r.RequestTimeout == 0 {
		r.RequestTimeout = cfg.RequestTimeout
	}
	return &r
}

// startCycle probes every check in cfg once, holding a slot of limiterCh for
// each probe in flight. It returns once the last probe has started (or ctx is
// done); the returned WaitGroup is done when every started probe has been reported.
func startCycle(ctx context.Context, logger *zap.Logger, cfg Config, limiterCh chan bool, alert func(zapcore.Entry) error) *sync.WaitGroup {
	wg := &sync.WaitGroup{}

	for _, c := range cfg.Checks {
		select {
		case limiterCh <- true:
		case <-ctx.Done():
			return wg
		}
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
		// results is nil when the request could not even be prepared.
		var fields []zap.Field
		if results != nil {
			fields = append(fields,
				zap.Int("code", results.Code),
				zap.Duration("duration", results.Duration),
			)
		}
		alertLogger.Error("check failed", append(fields, zap.Error(err))...)
		return
	}

	// A JSON null payload decodes to a nil body.
	if results.Body == nil {
		alertLogger.Error("application error",
			zap.Int("code", results.Code),
			zap.Duration("duration", results.Duration),
			zap.String("reason", "empty health payload"),
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
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: timeout,
	}
}

// notifyOnCritical returns an alert hook that records an "ALERT" entry through
// logger for every error-level entry. logger must not carry the hook itself.
func notifyOnCritical(logger *zap.Logger) func(zapcore.Entry) error {
	return func(entry zapcore.Entry) error {
		if entry.Level >= zapcore.ErrorLevel {
			// Send a notification to Slack, PagerDuty, etc.
			logger.Info("ALERT", zap.String("alert", entry.Message))
		}
		return nil
	}
}
