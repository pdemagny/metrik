// Package runner probes the configured checks on every run interval, within
// the concurrency limit, through the circuit breaker, and reports each outcome.
package runner

import (
	"context"
	"net/http"
	"sync"
	"time"

	"metrik/internal/config"
	"metrik/pkg/breaker"
	"metrik/pkg/check"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Run probes the checks of current() on every run interval until ctx is done.
// current is read on each tick, so configuration reloads apply to the next cycle.
func Run(ctx context.Context, logger *zap.Logger, current func() config.Config) error {
	cfg := current()
	// One limiter for the whole run, so overlapping cycles share the maxConcurrent bound.
	limiterCh := make(chan bool, cfg.MaxConcurrent)
	// Circuit state outlives cycles and config reloads.
	circuits := breaker.New()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.Tick(current().RunInterval):
			startCycle(ctx, logger, current(), limiterCh, circuits, notifyOnCritical(logger))
		}
	}
}

// startCycle probes every check in cfg once, holding a slot of limiterCh for
// each probe in flight. Checks whose circuit is open are skipped without taking
// a slot, and every probe outcome is recorded in circuits. It returns once the
// last probe has started (or ctx is done); the returned WaitGroup is done when
// every started probe has been reported.
func startCycle(ctx context.Context, logger *zap.Logger, cfg config.Config, limiterCh chan bool, circuits *breaker.Breaker, alert func(zapcore.Entry) error) *sync.WaitGroup {
	wg := &sync.WaitGroup{}

	checks := make([]*check.Check, 0, len(cfg.Checks))
	keys := make(map[string]struct{}, len(cfg.Checks))
	for _, c := range cfg.Checks {
		chk := cfg.Resolve(c)
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

		client := check.NewClient(chk.RequestTimeout)
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
