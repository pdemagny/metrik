# Bug Verification: A check is probed again while its previous probe is still in flight

- **Slug**: overlapping-check-cycles
- **Tested**: 2026-10-11
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

## Summary

The symptom no longer reproduces: the assessment's reproduction now peaks at one probe in flight (three before the fix), and the real binary run against an endpoint that never answers peaks at one concurrent connection where the pre-fix binary reached two. The new validation rule rejects a short `runInterval` at startup. No regression was found in the test suites or the linter.

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction (post-fix) | The assessment's `TestReproOverlap` (300ms endpoint, `RunInterval` 100ms, `RequestTimeout` 500ms) run with `-race` on a scratch copy of the fixed module, with an added assertion that the peak is 1 | pass | 3 probes, peak 1. Before the fix: 10 probes, peak 3. |
| End to end, hanging endpoint, post-fix binary | Fixed binary run 13s against a local TCP server that never answers; `runInterval` 2s, `requestTimeout` 1s, `maxTries` 2, breaker disabled | pass | Peak of 1 concurrent connection; 3 probes started, 3 cycles skipped with the `warn` entry. |
| End to end, same setup, pre-fix binary | Binary built from `HEAD` (`git archive`) in a scratch directory, same server and config | reproduces the bug | Peak of 2 concurrent connections, 6 probes started, no skip. Confirms the setup exercises the bug. |
| Validation at startup | Fixed binary with `--runInterval 1s` on the config above, and with `--runInterval 10s` on the shipped config | pass | Both exit 1 with `invalid config`. |
| Shipped defaults start | Fixed binary with the shipped config, no flags (run during the fix step) | pass | No validation error. |
| New / updated tests | `go test -race -count=1 -v ./internal/runner/ ./internal/config/` | pass | The 4 runner tests and 12 `TestValidate` cases named in `fix.md`, plus `TestExampleConfigLoads`. |
| Regression suite | `mise run test` (race detector, all three modules) | pass | |
| Lint | `mise run lint` | pass | 0 issues in every module. |
| Reload with an invalid file | — | not-run | Reload goes through the same `load` closure as startup; not exercised on a live process. |
| `demo-api` end to end | — | not-run | The hanging TCP server stands in for it; `demo-api`'s `/v1/slow` is random, so it gives no deterministic overlap. |

## Output Excerpts

Reproduction on the fixed code:

```
overlap_repro_test.go:43: hits=3 peak concurrent probes of the SAME check=1
--- PASS: TestReproOverlap (1.45s)
```

Real binary against the hanging endpoint, 13 seconds each:

```
== pre:  connections=11 peak_concurrent=2
   check started: 6   check skipped: 0   check failed: 5   ALERT: 5
== post: connections=6  peak_concurrent=1
   check started: 3   check skipped: 3   check failed: 3   ALERT: 3
```

```json
{"level":"warn","msg":"check skipped","name":"hang","endpoint":"http://127.0.0.1:18081/hang","reason":"previous probe still running"}
```

Validation:

```json
{"level":"fatal","msg":"invalid config","error":"runInterval (1s) must not be below maxTries x requestTimeout (2 x 1s)"}
{"level":"fatal","msg":"invalid config","error":"runInterval (10s) must not be below maxTries x requestTimeout (3 x 5s)"}
```

## Residual Risks

- A check that is always slower than `runInterval` logs a skip warning on every skipped cycle (3 of 6 cycles in the end-to-end run). This is the intended signal, but it adds log volume.
- Alert volume drops for a hanging endpoint (5 alerts became 3 over the same 13 seconds). Anything counting `check failed` or `ALERT` entries per interval will see fewer.
- Existing deployments whose config has `runInterval` below `maxTries × requestTimeout`, including the old defaults, stop at startup after the upgrade. The reload path with such a file was not exercised live.
- The interaction with the circuit breaker is covered by a unit test only (`TestStartCycleInFlightSkipKeepsTrial`); the end-to-end run had the breaker disabled.
- The rule ignores a zero or negative `requestTimeout`, which is one of the two bugs deferred to the roadmap.

## Recommendation

Close the bug — verified with the assessment's reproduction and with the real binary before and after the fix. The changes are uncommitted in the working tree and await review. The two deferred validation bugs are tracked in `docs/metrik-roadmap.md`.
