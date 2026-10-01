# Research: Endpoint Circuit Breaker

All Technical Context unknowns are resolved below. Each entry gives the decision, the rationale and
the alternatives considered.

## R1. Library or hand-rolled breaker

- **Decision**: Hand-roll a small breaker (about 80 lines) in `apps/metrik/cmd`. Add no dependency.
- **Rationale**: Constitution I says the stdlib comes first and every new module needs a justification. The behaviour we need is small: count consecutive failed *cycles*, open for a fixed cool-down, allow one trial, and close or reopen. It needs `sync.Mutex`, `time` and a map.
- **Alternatives considered**:
  - `sony/gobreaker`: counts failures per request through `Execute`. Mapping "one cycle = one failure" and "skip without using a concurrency slot" onto it needs wrapping that is as long as the hand-rolled version. It also adds a dependency and generic options that would never be used.
  - `failsafe-go` / `hystrix-go`: much larger. hystrix-go is unmaintained.

## R2. Where the breaker lives

- **Decision**: An unexported `circuits` type in a new file `apps/metrik/cmd/breaker.go`, tested in `apps/metrik/cmd/breaker_test.go`. `startCycle` is its only caller.
- **Rationale**: It is scheduling policy, like the concurrency limiter, which already lives in `cmd`. A new package or an interface for one caller is what Constitution I forbids (YAGNI).
- **Alternatives considered**: `pkg/check` mixes the "probe one endpoint" logic with cross-cycle state. `internal/breaker` is a package with one caller.
- **Superseded**: by spec 003 and Constitution v1.1.0 ("cmd holds CLI code only"). The breaker now lives in apps/metrik/pkg/breaker (exported API: specs/003-breaker-package/contracts/breaker-api.md), tested in pkg/breaker/breaker_test.go.

## R3. What counts as a failed cycle

- **Decision**: A cycle fails when `report` logs at error level: the probe errored after its retries, the health payload was empty, or the payload status is not 200. `report` returns that verdict as a `bool`, and the breaker consumes it.
- **Rationale**: This matches FR-001 exactly and keeps one source of truth for failure. Slow but healthy responses (WARN) count as successes.
- **Alternatives considered**: Re-deriving failure from `(result, err)` in `startCycle` would duplicate `report`'s branching. Counting individual retry attempts would contradict the spec assumption that a cycle counts as one failure.

## R4. Per-check "unset" vs "disabled" threshold

- **Decision**:
  - Global setting: `failureThreshold int`, default 3. `0` disables the breaker for every check that doesn't override it.
  - Per-check setting: `FailureThreshold *int`. `nil` inherits the global value, `0` disables the breaker for that check, and `N > 0` overrides it.
  - Cool-down: `time.Duration` both globally (default 60s) and per check. On a check, `0` inherits the global value.
- **Rationale**: Elsewhere in the code, a per-check zero means "inherit" (`resolve`), but the spec makes `0` mean "disabled". A pointer is the only way to tell "not set" apart from "set to 0" without a sentinel value. Viper's mapstructure decoding fills `*int` fields natively. The cool-down needs no "disabled" value, so it keeps the existing zero-means-inherit rule.
- **Alternatives considered**:
  - `-1` as "disabled": contradicts FR-013 (negative values are rejected) and is less readable in YAML.
  - A separate `circuitBreaker: false` boolean: a second setting for something the threshold already expresses.

## R5. Validation (FR-013)

- **Decision**: `Config.validate() error` rejects a negative global or per-check threshold or cool-down. The error names the setting and, for per-check settings, the check. It runs right after every `Unmarshal`, at startup and on reload, and goes through the existing `logger.Fatal` path, like decode errors.
- **Rationale**: Validation fails the same way the current decode errors do. No new error-handling strategy.
- **Alternatives considered**: Keeping the old config on an invalid reload would be nicer, but it changes reload semantics for every setting. That's out of scope.

## R6. State identity, reload and pruning (FR-014)

- **Decision**: State is keyed by `name + "\x00" + endpoint` in a map owned by the run loop. Each cycle, the breaker drops entries whose key is not among the current checks.
- **Rationale**: A check whose endpoint changed, or a new check, gets a new key, so it starts closed. Removed checks are pruned. Unchanged checks keep their state. No reload hook is needed.
- **Alternatives considered**: Rebuilding state inside `OnConfigChange` would couple the breaker to viper's callback goroutine.
- **Note**: `OnConfigChange` already writes `runConfig` while the loop reads it (a data race that exists today). The breaker neither fixes nor worsens it. It is flagged as out of scope.

## R7. Concurrency and overlapping cycles

- **Decision**: One `sync.Mutex` guards the map. `allow(key, now)` returns `(run bool, trial bool, remaining time.Duration)` and is called *before* taking a limiter slot, so suspended checks use no slot (FR-004). `record(key, trial, failed, now, threshold, cooldown)` returns the transition to log.
  - While a trial is in flight, other cycles skip the check.
  - A non-trial result that arrives while the circuit is open (started before it opened) is ignored, so it is never double-counted.
- **Rationale**: A single lock is enough at 10s cycles and tens of checks. Returning transitions keeps all logging in `startCycle`, next to the existing `report` call.
- **Alternatives considered**: Atomics per field (too complex for no measurable gain), or a goroutine owning the state behind channels (more code and harder to test).

## R8. Time source for tests

- **Decision**: `allow` and `record` take `now time.Time` as an argument. `startCycle` passes `time.Now()`.
- **Rationale**: Tests drive state transitions with literal times. No injectable clock field or interface.
- **Alternatives considered**: A `now func() time.Time` field: a test seam that also works, but it adds a field. Sleeping in tests: slow and flaky.

## R9. Logging and alerts (FR-008, FR-009, Constitution III)

- **Decision**: Alerting is unchanged. Every failed probe still goes through `report`'s alerting logger (decided with the maintainer during planning). The new entries use the plain child logger, which has no alert hook:
  - `circuit opened` / `circuit reopened`: ERROR, no alert.
  - `check skipped`: WARN.
  - `circuit closed`: INFO.

  All of them carry `name`, `endpoint` and `circuit`, plus `failures` / `cooldown` / `remaining` / `suspended` where relevant.
- **Rationale**: Principle III keeps holding as written (errors from failed probes still alert). The volume reduction comes from not probing during the cool-down.
- **Alternatives considered**: One alert per outage. Rejected by the maintainer because it would need a constitution amendment.

## R10. Demo checks that are meant to fail

- **Decision**: Remove "liveness not found" and "liveness connection refused" from `configs/metrik.yaml`, `configs/dev-values.yaml` and `configs/prod-values.yaml`. Update `TestExampleConfigLoads` to match.
- **Rationale**: The maintainer's direction. These two behaviours are covered by `TestRunVerdicts` (4xx is not retried), `TestRunConnectionRefused` and `TestStartCycleAlerts`. Leaving them would show a permanently open circuit in every environment. "liveness slow timeout" and "readiness flaky" still fail at random, so they demonstrate the breaker opening and closing.
- **Alternatives considered**: Keeping them with `failureThreshold: 0`. That keeps noise the tests already make redundant.
