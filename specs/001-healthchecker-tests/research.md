# Research: Healthchecker Automated Tests

All Technical Context unknowns are resolved below. Sources: `apps/metrik` code at 9ccb748,
`github.com/cenkalti/backoff/v5@v5.0.3/retry.go`, and `go.uber.org/zap@v1.27.1/zaptest/observer`.

## R1. Simulating endpoints offline

- **Decision**: use `net/http/httptest.NewServer`, with one handler per scenario. Each handler counts attempts with an `atomic.Int32`. For "connection refused", start a server, keep its URL, then `Close()` it.
- **Rationale**: it's in the standard library (Principle I), binds to an ephemeral loopback port (FR-010: no fixed ports), and exercises the real `http.Client` path, including `errors.Is(err, syscall.ECONNREFUSED)`.
- **Alternatives considered**: a custom `http.RoundTripper` stub. It's faster, but it skips real dial errors and timeouts, which are the behaviors under test. Mock libraries were rejected because they add a dependency.

## R2. Keeping retry tests fast without touching production code

- **Decision**: don't inject a backoff. Tests use `maxTries ≤ 3` and `t.Parallel()`. The default exponential backoff (500 ms initial, ×1.5, ±50 % jitter) costs about 1.3 s of wall time per retrying test, and the tests overlap when run in parallel.
- **Rationale**: this needs no production change (FR-012) and no test-only knob (Principle I / YAGNI). The expected package wall time is about 3 s, well under SC-003's 30 s.
- **Alternatives considered**: an unexported `backOff` field on `Check`, set to `&backoff.ZeroBackOff{}` in tests. It's faster, but it adds a seam only tests use. Revisit only if the suite nears the time budget.

## R3. Retry semantics to assert (backoff v5 `Retry`)

Order inside the loop: success → `MaxTries` reached → `Permanent` → context cancelled → next backoff → `RetryAfterError` replaces delay → wait (cancellable).

- 4xx (≠429) → `backoff.Permanent` → exactly 1 attempt; error unwrapped to `"client error"`.
- 429 with `Retry-After: 1` → next wait = 1 s (backoff reset). Assert elapsed between attempts ≥ 1 s.
- 429 without `Retry-After` → header parse fails → falls into the 4xx branch → permanent, 1 attempt.
- Network error / timeout / body decode error → retried until `MaxTries`.
- `MaxTries == 0` means **unlimited** tries, bounded only by the 15-minute `DefaultMaxElapsedTime`. It's reachable only if the global `maxTries` is 0. Report it as an observation, not a test (Principle IV says "bounded").
- Context cancelled during the backoff wait → returns `context.Canceled` without waiting out the delay.

## R4. Observing logs and alerts

- **Decision**: build the logger with `zaptest/observer.New(zap.DebugLevel)` and assert on `ObservedLogs.FilterMessage(...)`, checking level and fields. For alerts, pass a recording function in place of `notifyOnCritical` (see R5).
- **Rationale**: it's already part of the `go.uber.org/zap` module, so there's no new `go.mod` entry. Assertions check structured fields (Principle III), not string output.
- **Alternatives considered**: capturing stdout. That's brittle, and `notifyOnCritical` writes with `fmt.Printf`, which would mix with the log output.

## R5. Making run-cycle logic reachable (behavior-preserving refactor of `cmd/run.go`)

The logic currently lives inline in the `RunE` closure, which runs an infinite `time.Tick` loop. The plan extracts three unexported functions **in the same file**, with no behavior change:

| Function | Moves out of `RunE` | Used by tests for |
|----------|---------------------|-------------------|
| `(cfg Config) resolve(c *check.Check) *check.Check` | the three "per-check value else global" blocks, returning a check with `AlertThreshold`, `MaxTries` and `RequestTimeout` set | US2 |
| `report(logger *zap.Logger, alert func(zapcore.Entry) error, res *check.Result, err error, alertThreshold time.Duration)` | the classification body of the per-check goroutine | US3 scenarios 1–4 |
| `startCycle(ctx, logger, cfg Config, alert func(zapcore.Entry) error) *sync.WaitGroup` | one tick: limiter, goroutine fan-out, limiter release | US3 scenario 5 (`startCycle(...).Wait()`) |

- **Why `startCycle` returns the WaitGroup instead of waiting**: today the tick loop blocks only until the last check has *started* (on the limiter send), not until all checks finish. Returning the WaitGroup keeps that exactly. `RunE` ignores it, and tests call `.Wait()`. The existing `go func(){ wg.Wait() }()` does nothing and is dropped.
- **Why `alert` is a parameter**: it's the only way to observe the alert hook without capturing stdout. Production passes `notifyOnCritical`, so behavior is unchanged.
- **Alternatives considered**: making `startCycle` block until all checks complete. It's cleaner, but it changes timing when a cycle outlasts `runInterval`, which breaks FR-012. The overlap is recorded as an observation instead.

## R6. Loading the example configuration

- **Decision**: a test in `cmd` creates `viper.New()`, calls `SetConfigFile("../configs/metrik.yaml")`, then `ReadInConfig()` and `Unmarshal(&Config{})`.
- **Rationale**: it's the same decoder `RunE` uses: mapstructure with case-insensitive field matching and the string→`time.Duration` hook. It avoids `Execute()`, which calls `logger.Fatal` and binds global flags.
- **Expected content** (current file): runInterval 10s, maxConcurrent 10, requestTimeout 5s, alertThreshold 3s, maxTries 3. There are **3 checks**, and the third ("liveness connection refused") has an **empty endpoint** because its lines are half commented out. The test asserts the file as it is; the empty endpoint is reported as an observation.

## R7. Handling known defects (spec FR-013, Assumptions)

- **Decision**: each divergence from the constitution gets a real test that asserts the *intended* behavior. The test starts with `t.Skip("KNOWN DEFECT KD-n: …")`, so deleting the skip turns it into a regression test.
- **Rationale**: a panic in a `report` goroutine would crash the whole test binary. A skip keeps the suite green, and `go test -v` shows every gap.

**Update (Phase 7, T025–T027)**: all three defects are now fixed, and their tests run without `t.Skip`.

| ID | Divergence | Where | Constitution | Pinned by |
|----|------------|-------|--------------|---------------------|
| KD-1 | 5xx with a decodable JSON body counts as success, so it isn't retried | `pkg/check` `Run` | IV (5xx must be retried) | `TestRunServerErrorIsRetried` (`pkg/check/main_test.go`) |
| KD-2 | Body `null` sets `Result.Body` to nil, then `report` dereferences it and panics | `cmd` `report` | IV (graceful failure) | `TestReportNullBody` (`cmd/run_test.go`) |
| KD-3 | A request that can't be built (malformed URL) returns a nil `*Result`, then `report` reads `res.Code` and panics | `cmd` `report` | IV | `TestReportNilResult` (`cmd/run_test.go`) |

**Observations** to list in the final report, not tested: `maxTries: 0` globally allows unlimited retries; cycles can overlap when a check outlasts `runInterval`; `notifyOnCritical` uses `fmt.Printf` (Principle III); the example config has a check with no endpoint; `logger.Error("unable to create request: %s\n", …)` passes a printf-style format string to zap.

## R8. Quality gates

- `cd apps/metrik && go test -race -count=1 ./...` must pass (SC-004 uses `-count=20`).
- `go test -cover ./pkg/check ./cmd` checks SC-002.
- `golangci-lint run` from `apps/metrik` uses the repo `.golangci.yaml`. Test files follow the same rules.
