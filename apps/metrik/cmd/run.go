package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"

	"metrik/pkg/breaker"
	"metrik/pkg/check"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type (
	Config struct {
		Debug            bool           `yaml:"debug"`
		Verbose          bool           `yaml:"verbose"`
		RunInterval      time.Duration  `yaml:"runInterval"`
		MaxConcurrent    int            `yaml:"maxConcurrent"`
		RequestTimeout   time.Duration  `yaml:"requestTimeout"`
		AlertThreshold   time.Duration  `yaml:"alertThreshold"`
		MaxTries         uint           `yaml:"maxTries"`
		FailureThreshold int            `yaml:"failureThreshold"`
		Cooldown         time.Duration  `yaml:"cooldown"`
		Checks           []*check.Check `yaml:"checks"`
	}
)

func NewRunCmd(ctx context.Context, logger *zap.Logger) *cobra.Command {
	runCmd := &cobra.Command{
		Use:     "run",
		Aliases: []string{"r"},
		Short:   "Run health checks.",
		Long:    "Run health checks.",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			var runConfig Config
			err := rootConfig.Unmarshal(&runConfig)
			if err != nil {
				logger.Fatal("unable to decode into struct", zap.Error(err))
			}
			if err := runConfig.validate(); err != nil {
				logger.Fatal("invalid config", zap.Error(err))
			}
			rootConfig.WatchConfig()
			rootConfig.OnConfigChange(func(_ fsnotify.Event) {
				logger.Debug("reloading config ...")
				err := rootConfig.Unmarshal(&runConfig)
				if err != nil {
					logger.Fatal("unable to decode into struct", zap.Error(err))
				}
				if err := runConfig.validate(); err != nil {
					logger.Fatal("invalid config", zap.Error(err))
				}
			})

			// One limiter for the whole run, so overlapping cycles share the maxConcurrent bound.
			limiterCh := make(chan bool, runConfig.MaxConcurrent)
			// Circuit state outlives cycles and config reloads.
			circuits := breaker.New()
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-time.Tick(runConfig.RunInterval):
					startCycle(ctx, logger, runConfig, limiterCh, circuits, notifyOnCritical(logger))
				}
			}
		},
	}

	return runCmd
}

// validate rejects settings that have no meaning, such as negative circuit
// breaker values.
func (cfg Config) validate() error {
	if cfg.FailureThreshold < 0 {
		return fmt.Errorf("failureThreshold must not be negative, got %d", cfg.FailureThreshold)
	}
	if cfg.Cooldown < 0 {
		return fmt.Errorf("cooldown must not be negative, got %s", cfg.Cooldown)
	}
	for _, c := range cfg.Checks {
		if c.FailureThreshold != nil && *c.FailureThreshold < 0 {
			return fmt.Errorf("check %q: failureThreshold must not be negative, got %d", c.Name, *c.FailureThreshold)
		}
		if c.Cooldown < 0 {
			return fmt.Errorf("check %q: cooldown must not be negative, got %s", c.Name, c.Cooldown)
		}
	}
	return nil
}

// resolve returns a copy of c whose AlertThreshold, MaxTries, RequestTimeout and
// Cooldown fall back to the global value when unset (zero) on the check, and
// whose FailureThreshold falls back to the global value when nil.
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
	// Always a fresh pointer, so the copy never aliases the input check.
	threshold := cfg.FailureThreshold
	if r.FailureThreshold != nil {
		threshold = *r.FailureThreshold
	}
	r.FailureThreshold = &threshold
	if r.Cooldown == 0 {
		r.Cooldown = cfg.Cooldown
	}
	return &r
}

// startCycle probes every check in cfg once, holding a slot of limiterCh for
// each probe in flight. Checks whose circuit is open are skipped without taking
// a slot, and every probe outcome is recorded in circuits. It returns once the
// last probe has started (or ctx is done); the returned WaitGroup is done when
// every started probe has been reported.
func startCycle(ctx context.Context, logger *zap.Logger, cfg Config, limiterCh chan bool, circuits *breaker.Breaker, alert func(zapcore.Entry) error) *sync.WaitGroup {
	wg := &sync.WaitGroup{}

	checks := make([]*check.Check, 0, len(cfg.Checks))
	keys := make(map[string]struct{}, len(cfg.Checks))
	for _, c := range cfg.Checks {
		chk := cfg.resolve(c)
		checks = append(checks, chk)
		if *chk.FailureThreshold > 0 {
			keys[chk.ID()] = struct{}{}
		}
	}
	circuits.Keep(keys)

	for _, chk := range checks {
		// A zero threshold disables the breaker: probe every cycle, record nothing.
		enabled := *chk.FailureThreshold > 0
		key := chk.ID()
		g := breaker.Gate{Run: true}
		if enabled {
			g = circuits.Allow(key, time.Now())
		}
		if !g.Run {
			logger.Warn("check skipped",
				zap.String("name", chk.Name),
				zap.String("endpoint", chk.Endpoint),
				zap.String("circuit", string(g.State)),
				zap.Duration("remaining", g.Remaining),
			)
			continue
		}

		select {
		case limiterCh <- true:
		case <-ctx.Done():
			return wg
		}
		wg.Add(1)

		client := createClient(chk.RequestTimeout)
		go func() {
			defer wg.Done()
			childlogger := logger.WithLazy(
				zap.String("name", chk.Name),
				zap.String("endpoint", chk.Endpoint),
			)
			childlogger.Info("check started")

			results, err := chk.Run(ctx, childlogger, client)
			<-limiterCh

			failed := report(childlogger, alert, results, err, chk.AlertThreshold)
			if !enabled {
				return
			}
			// Transitions go through childlogger, which has no alert hook: the failed probe already alerted.
			switch ch := circuits.Record(key, g.Trial, failed, *chk.FailureThreshold, chk.Cooldown, time.Now()); ch.Kind {
			case breaker.Opened:
				childlogger.Error("circuit opened",
					zap.String("circuit", string(breaker.Open)),
					zap.Int("failures", ch.Failures),
					zap.Duration("cooldown", chk.Cooldown),
				)
			case breaker.Reopened:
				childlogger.Error("circuit reopened",
					zap.String("circuit", string(breaker.Open)),
					zap.Duration("cooldown", chk.Cooldown),
				)
			case breaker.Recovered:
				childlogger.Info("circuit closed",
					zap.String("circuit", string(breaker.Closed)),
					zap.Duration("suspended", ch.Suspended),
				)
			case breaker.NoChange:
			}
		}()
	}

	return wg
}

// report logs the outcome of one probe at the right severity and routes
// error-level entries through alert. It returns true when the probe failed.
func report(logger *zap.Logger, alert func(zapcore.Entry) error, results *check.Result, err error, alertThreshold time.Duration) bool {
	alertLogger := zap.New(logger.Core(), zap.Hooks(alert))
	if err != nil {
		// results is nil when the request could not even be prepared.
		var fields []zap.Field
		if results != nil {
			fields = append(
				fields,
				zap.Int("code", results.Code),
				zap.Duration("duration", results.Duration),
			)
		}
		alertLogger.Error("check failed", append(fields, zap.Error(err))...)
		return true
	}

	// A JSON null payload decodes to a nil body.
	if results.Body == nil {
		alertLogger.Error(
			"application error",
			zap.Int("code", results.Code),
			zap.Duration("duration", results.Duration),
			zap.String("reason", "empty health payload"),
		)
		return true
	}

	if results.Body.Status != http.StatusOK {
		alertLogger.Error(
			"application error",
			zap.Int("code", results.Code),
			zap.Duration("duration", results.Duration),
			zap.Int("status", results.Body.Status),
			zap.String("message", results.Body.Message),
			zap.String("reason", results.Body.Reason),
		)
		return true
	}

	if results.Duration > alertThreshold {
		logger.Warn("slow response",
			zap.Int("code", results.Code),
			zap.Duration("duration", results.Duration))
	}

	logger.Info(
		"check completed",
		zap.Int("code", results.Code),
		zap.Duration("duration", results.Duration),
	)
	return false
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
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
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
