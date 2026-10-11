# Bug Assessment: A check is probed again while its previous probe is still in flight

- **Slug**: overlapping-check-cycles
- **Created**: 2026-10-11
- **Source**: pasted text
- **Verdict**: valid
- **Severity**: medium

## Report (verbatim or summarized)

> runInterval can be smaller than requestTimeout which can lead to bugs where the next cycles starts probes whereas the previous cycles is still ongoing

## Symptom

When a probe takes longer than `runInterval`, the next cycle starts a second probe of the same check while the first is still running. Expected: a check has at most one probe in flight at a time.

The condition is wider than `runInterval < requestTimeout`. `requestTimeout` bounds one HTTP attempt, not one probe. A probe lasts up to `maxTries × requestTimeout` plus the backoff waits between attempts, plus any `Retry-After` delay a 429 response asks for. With the shipped defaults (`runInterval` 10s, `requestTimeout` 5s, `maxTries` 3) a probe of a hanging endpoint lasts about 15s to 17s, so the default configuration already overlaps.

Observable effects:

- The same endpoint receives several concurrent probes, which adds load to a service that is already slow.
- Each overlapping probe reports on its own, so one outage produces duplicate `check failed` entries and duplicate `ALERT` entries.
- The circuit breaker counts one failure per completed probe, in completion order, so `failureThreshold` no longer means "consecutive failed cycles" while probes overlap.
- Duplicate probes of a slow check hold `maxConcurrent` slots. Once the limiter is full, `startCycle` blocks and every other check is delayed.

Overlap is bounded: the shared limiter caps probes in flight at `maxConcurrent`, so there is no goroutine leak and no crash.

## Reproduction

Reproduced on a scratch copy of `apps/metrik` (no source file changed), with a test in package `runner`:

1. Serve an endpoint that answers a healthy payload after 300ms.
2. Call `runner.Run` with one check on that endpoint, `RunInterval: 100ms`, `RequestTimeout: 500ms`, `MaxTries: 1`, `MaxConcurrent: 10`.
3. Let it run for about one second and track the requests in flight on the server.

Result: 10 requests, with a peak of **3 concurrent probes of the same check**.

With the real binary and default settings, point a check at an endpoint that never answers: each probe lasts about 15s to 17s against a 10s interval. [NEEDS CLARIFICATION: not run end to end against `demo-api`; the figure is derived from the defaults and the backoff library's 500ms initial interval, ×1.5 multiplier and ±50% jitter.]

## Suspected Code Paths

- `apps/metrik/internal/runner/runner.go:31` — `Run` starts a cycle on every tick and ignores the `WaitGroup` that `startCycle` returns, so nothing ties a new cycle to the completion of the previous one.
- `apps/metrik/internal/runner/runner.go:56` — the per-check loop in `startCycle` gates a probe on the circuit breaker and the limiter only. Nothing records that a check already has a probe in flight.
- `apps/metrik/internal/runner/runner.go:23` — the comment "overlapping cycles share the maxConcurrent bound" shows overlap is expected by the current design and only bounded, not prevented.
- `apps/metrik/pkg/check/main.go:110` — `backoff.Retry` with `WithMaxTries(c.MaxTries)`: a probe's duration is several attempts plus waits, not one `RequestTimeout`.
- `apps/metrik/pkg/check/main.go:84` — a 429 with `Retry-After` makes the probe wait a server-chosen number of seconds before the next attempt.
- `apps/metrik/internal/config/config.go:31` — `Validate` checks only the circuit breaker settings. No rule relates `runInterval` to `requestTimeout` or `maxTries`.
- `apps/metrik/pkg/breaker/breaker.go:117` — `Record` already tolerates overlap for non-trial probes (ignores a result that lands on a non-closed circuit), and `Allow` lets only one trial probe run. The breaker stays consistent; only the meaning of the failure count drifts.
- `apps/metrik/internal/runner/runner_test.go:206` — `TestStartCyclesShareBound` asserts that two overlapping cycles probe every check twice, so the current behavior is locked in by a test.

## Root Cause Hypothesis

Confidence: high (reproduced). `startCycle` returns as soon as the last probe has *started*, and `Run` then waits one `runInterval` and starts the next cycle without checking whether earlier probes have finished. This was a deliberate choice, recorded as an observation in `specs/001-healthchecker-tests/research.md` ("cycles can overlap when a check outlasts `runInterval`") and handled defensively in `specs/002-endpoint-circuit-breaker` (shared limiter, breaker tolerant of overlap). The missing piece is a per-check guard: nothing stops a second probe of a check whose first probe is still running.

## Proposed Remediation

**Preferred**: skip a check whose previous probe is still in flight. Keep a set of in-flight check IDs in `runner.Run` (created once, next to `limiterCh` and `circuits`, and passed to `startCycle`). In the per-check loop, before the breaker and the limiter, skip the check and log a `check skipped` entry at `warn` level (decided) with a reason field when its ID is already in the set; otherwise add the ID and remove it when the probe goroutine finishes. Checks that answer in time keep their cadence, a slow check is probed back to back at its own pace, and it can no longer hold more than one limiter slot. `chk.ID()` (name + endpoint) is the natural key, matching the breaker.

The skip must not call `circuits.Allow`, or a skipped cycle could consume the single trial probe of an open circuit.

**Also required (decided)**: `Config.Validate` returns an error when `runInterval` is below `maxTries × requestTimeout`. The rule is evaluated on resolved values, once for the globals and once per check (`cfg.Resolve(c)`), so a per-check `maxTries` or `requestTimeout` override is covered, and `maxTries: 0` counts as 1 as it does at run time. Equality passes. An invalid file then stops metrik at startup, and also on a reload, because `load` in `cmd/metrik/run.go` calls `logger.Fatal` on a validation error.

The rule narrows the overlap but does not remove it: backoff waits and `Retry-After` delays come on top of `maxTries × requestTimeout`, so the in-flight guard is still needed.

**The shipped defaults fail this rule**: `runInterval` 10s against 3 × 5s = 15s. The fix must change them everywhere they are set: the flag defaults in `cmd/metrik/root.go`, `configs/metrik.yaml`, `configs/dev-values.yaml`, `configs/prod-values.yaml`, the docs tables and examples, and the expectations in `TestExampleConfigLoads`. The default `runInterval` is raised to 15s (decided); `requestTimeout` 5s and `maxTries` 3 stay.

**Alternatives**:
- Wait for the whole cycle: `startCycle(...).Wait()` in `Run`. A one-line change with no new state, but one hanging check then delays every other check by up to its full probe duration (about 17s with the defaults), which lowers detection speed for healthy and failing endpoints alike.
- Validation only, without the guard: not enough, since backoff and `Retry-After` still push a probe past `runInterval`.

**Files likely to change**:
- `apps/metrik/internal/runner/runner.go`
- `apps/metrik/internal/runner/runner_test.go`
- `apps/metrik/internal/config/config.go`
- `apps/metrik/internal/config/config_test.go`
- `apps/metrik/cmd/metrik/root.go` (flag defaults)
- `apps/metrik/configs/metrik.yaml`, `apps/metrik/configs/dev-values.yaml`, `apps/metrik/configs/prod-values.yaml`
- `docs/metrik-configuration.md` (new skip log line, the new validation rule, default values)
- `docs/metrik-deployment.md` (example values)
- `docs/metrik-architecture.md` (cycle description, if it mentions overlap)

**Tests to add or update**:
- New: two cycles started back to back on a slow server probe each check once, and the second cycle logs one skip per check.
- New: once the first probe has finished, the next cycle probes the check again (the in-flight entry is released, including when the probe fails).
- New: an in-flight skip does not consume the trial probe of an open circuit.
- New in `TestValidate`: global `runInterval` below `maxTries × requestTimeout` is rejected; equality passes; a per-check override that breaks the rule is rejected and names the check; `maxTries: 0` is treated as 1.
- Update: `TestExampleConfigLoads` for the new default values, and it should also assert the example file passes `Validate`.
- Update: `TestStartCyclesShareBound` currently wants every path hit twice across overlapping cycles; under the preferred fix it wants once. The `maxConcurrent` bound assertion stays.

## Risks & Considerations

- Behavior change: a slow check produces fewer probes and fewer alerts than today. Anyone counting `check failed` entries per interval will see lower numbers.
- Breaking config change: existing files with the old defaults (10s / 5s / 3) are rejected at startup after the upgrade, and a bad edit to a watched file kills a running process on reload. Deployments must update their values together with the binary.
- New log line: a check that is always slower than `runInterval` logs a skip warning every cycle. It should be distinguishable from the circuit breaker skip (same message with a different field, or a different message).
- The in-flight set is shared between the tick loop and the probe goroutines, so it needs the same care as the breaker map (mutex or `sync.Map`) and must pass `go test -race`.
- Config reloads: a check renamed or re-pointed while its probe is in flight gets a new ID, so the old entry is released by its own goroutine and no pruning is needed.

## Deferred (separate bugs, to raise after this fix)

- `runInterval: 0` or a negative value: `time.Tick` returns nil for a non-positive duration, so metrik blocks forever without probing and without an error.
- `requestTimeout: 0`: means no HTTP timeout, so a probe of a hanging endpoint holds its limiter slot indefinitely.

Also noticed, not yet triaged: `maxConcurrent` is read once at startup, so a reload does not resize the limiter.

## Decisions

- The in-flight skip is logged at `warn`.
- `runInterval` below `maxTries × requestTimeout` is a validation error, not a warning.
- The two validation gaps above are handled as separate bugs.
- The default `runInterval` moves from 10s to 15s so the shipped configuration passes the new rule.

## Open Questions

None.
