# Contract: `cmd/run.go` test seams

Metrik exposes no new external interface; the CLI flags, config schema and log messages are unchanged. This contract fixes the **internal** signatures that tests depend on, so the refactor and the tests can be written against the same target. All are unexported, in package `cmd`, in `run.go`.

```go
// resolve returns a copy of c whose AlertThreshold, MaxTries and RequestTimeout
// fall back to the global value when unset (zero) on the check.
func (cfg Config) resolve(c *check.Check) *check.Check

// report logs the outcome of one probe at the right severity and routes
// error-level entries through alert.
func report(logger *zap.Logger, alert func(zapcore.Entry) error,
	res *check.Result, err error, alertThreshold time.Duration)

// startCycle probes every check in cfg once, holding a slot of limiterCh for
// each probe in flight. It returns once the last probe has started (or ctx is
// done); the returned WaitGroup is done when every started probe has been reported.
func startCycle(ctx context.Context, logger *zap.Logger, cfg Config,
	limiterCh chan bool, alert func(zapcore.Entry) error) *sync.WaitGroup
```

Amended by T029 (Phase 7, Constitution IV): `RunE` creates `limiterCh` once, with capacity
`MaxConcurrent`, and passes it to every cycle, so overlapping cycles share the bound.

## Obligations

- **Behavior preservation**: `RunE` becomes `case <-time.Tick(...): startCycle(ctx, logger, runConfig, notifyOnCritical)`. Log messages, fields, levels, limiter acquire/release points and `createClient(requestTimeout)` per check stay identical to 9ccb748.
- `resolve` must not mutate the configured check. Config reload rebuilds `runConfig.Checks`, and tests compare against the original.
- `report` builds the alert logger exactly as today: `zap.New(logger.Core(), zap.Hooks(alert))`.
- The `logger` passed to `report` is the per-check child logger, carrying the `name` and `endpoint` fields.
