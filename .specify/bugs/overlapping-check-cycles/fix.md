# Bug Fix: A check is probed again while its previous probe is still in flight

- **Slug**: overlapping-check-cycles
- **Fixed**: 2026-10-11
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

`startCycle` now skips, with a `warn` entry, any check whose previous probe is still running, so a check never has two probes in flight. `Config.Validate` rejects a `runInterval` below `maxTries × requestTimeout`, globally and per check, and the default `runInterval` moves from 10s to 15s so the shipped configuration passes the new rule.

## Changes

| File | Change | Notes |
|------|--------|-------|
| `apps/metrik/internal/runner/runner.go` | modified | `Run` owns a `sync.Map` of in-flight check IDs and passes it to `startCycle`, which skips a check found in it before the breaker and the limiter, and releases the ID when the probe goroutine ends. |
| `apps/metrik/internal/config/config.go` | modified | `Validate` applies the new rule to the globals and to each resolved check; new `tooShort` helper. |
| `apps/metrik/cmd/metrik/root.go` | modified | `--runInterval` default 10s → 15s. |
| `apps/metrik/configs/metrik.yaml`, `dev-values.yaml`, `prod-values.yaml` | modified | `runInterval: "15s"`. |
| `apps/metrik/internal/runner/runner_test.go` | modified, tests added | New in-flight tests; existing call sites pass an in-flight set. |
| `apps/metrik/internal/config/config_test.go` | modified, tests added | New `TestValidate` cases; example config expectations. |
| `docs/metrik-configuration.md` | modified | New default, the validation rule, the skip behavior, the `reason` field of `check skipped`. |
| `docs/metrik-architecture.md` | modified | Diagram and paragraph show the in-flight skip. |
| `docs/metrik-deployment.md` | modified | Example value 15s. |
| `README.md` | modified | "every 10 seconds" → "every 15 seconds". |

## Diff Highlights (optional)

```go
// runner.go, per-check loop
key := chk.ID()
// Checked before the breaker, so a skipped cycle never uses up a trial probe.
if _, running := inFlight.Load(key); running {
	logger.Warn("check skipped",
		zap.String("name", chk.Name),
		zap.String("endpoint", chk.Endpoint),
		zap.String("reason", "previous probe still running"),
	)
	continue
}
```

```go
// config.go
if tries := max(cfg.MaxTries, 1); tooShort(cfg.RunInterval, tries, cfg.RequestTimeout) {
	return fmt.Errorf("runInterval (%s) must not be below maxTries x requestTimeout (%d x %s)", ...)
}
```

## Tests Added or Updated

- `runner_test.go::TestStartCycleSkipsInFlight` — two cycles back to back probe each check once, the second logs one `warn` skip per check with the in-flight reason and no `circuit` field; a third cycle after completion probes again.
- `runner_test.go::TestStartCycleFailedProbeReleases` — a failing probe releases its check, so it is probed on every sequential cycle.
- `runner_test.go::TestStartCycleInFlightSkipKeepsTrial` — an in-flight skip after the cool-down does not consume the trial probe; the next cycle runs the trial and closes the circuit.
- `runner_test.go::TestStartCyclesShareBound` — updated: the second overlapping cycle now uses different checks, so the shared `maxConcurrent` bound is still exercised; every path is probed once.
- `config_test.go::TestValidate` — seven new cases: interval below the product, equality, zero tries counted as one (pass and fail), per-check timeout override, per-check tries override, overrides within the rule.
- `config_test.go::TestExampleConfigLoads` — expects `runInterval` 15s and asserts the example file passes `Validate`.

## Local Verification

- Commands run: `mise run test` (race detector, every module) → all packages pass.
- Commands run: `go test -race -count=5 ./internal/runner/ ./internal/config/` → pass.
- Commands run: `mise run lint` → 0 issues in every module.
- Commands run: `mise run docs` → site builds.
- Manual checks: `metrik run --runInterval 10s` exits with `invalid config: runInterval (10s) must not be below maxTries x requestTimeout (3 x 5s)`; `metrik run` with the shipped defaults starts without error.
- Not done: an end-to-end run against `demo-api` with a hanging endpoint.

## Deviations from Assessment

- `README.md` was not in the assessment's file list. It stated the old 10-second cadence, so it was updated with the default.
- The rule is implemented by division (`runInterval / requestTimeout < maxTries`) instead of multiplication, to satisfy the `gosec` overflow lint. Two side effects: a zero or negative `requestTimeout` never triggers the rule, and a negative `runInterval` only triggers it when `requestTimeout` is positive. Both values belong to the deferred bugs.
- `TestStartCyclesShareBound` was changed to use two different check lists rather than only lowering the expected hit count, because with identical checks the second cycle would start no probe and the bound assertion would test nothing.

## Follow-ups

- Deferred bugs from the assessment: `runInterval` zero or negative blocks forever silently; `requestTimeout: 0` means no HTTP timeout. Also untriaged: `maxConcurrent` is not resized on reload.
- Two checks with the same name and endpoint share one ID, so the second is now skipped whenever the first is still running in the same cycle. They already shared one circuit. Rejecting duplicate checks in `Validate` would make this explicit.
- Upgrade note for deployments: a config with `runInterval` below `maxTries × requestTimeout` (including the old 10s / 5s / 3 defaults) now stops metrik at startup and on reload.
