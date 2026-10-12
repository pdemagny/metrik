# Bug Verification: A zero or negative runInterval stops all probing without an error

- **Slug**: zero-run-interval
- **Tested**: 2026-10-12
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

## Summary

The symptom no longer reproduces: every configuration that ran silently in the assessment now exits 1 with `runInterval must be positive`, at startup and on reload. The four new test cases fail against the pre-fix `config.go` and pass against the fix, and no regression was found.

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction at startup (post-fix) | `metrik run` with the assessment's scratch config, `runInterval`/`requestTimeout` of `0s`/`0s`, `-5s`/`0s`, `0s`/`-1s` | pass | Each exits 1 with the new message. All three ran silently before the fix. |
| Reproduction on reload (post-fix) | Start with `1s`/`0s`, rewrite the file to `0s`/`0s` after 3.5s | pass | Three probes, then `invalid config` and exit 1. Before the fix, probing stopped and the process kept running. |
| Valid config still runs | `metrik run` with `1s`/`0s` for 4s | pass | Three probes started, still running when killed. |
| New / updated tests | `go test -race -count=1 -v -run 'TestValidate\|TestExampleConfigLoads' ./internal/config/` | pass | 16 `TestValidate` cases, including the 4 new ones, plus `TestExampleConfigLoads`. |
| Pre-fix control | Same `TestValidate` run in a scratch copy with `config.go` taken from `HEAD` | pass | The 4 new cases fail and the other 12 pass, so the new cases do detect the bug. |
| Regression suite | `mise run test` (race detector, all three modules) | pass | |
| Lint | `mise run lint` | pass | 0 issues in every module. |

## Output Excerpts

Startup, `runInterval: "0s"` and `requestTimeout: "0s"`:

```text
{"level":"fatal",...,"msg":"invalid config","error":"runInterval must be positive, got 0s"}
exit=1
```

Reload from `1s`/`0s` to `0s`/`0s`:

```text
{"level":"fatal",...,"msg":"invalid config","error":"runInterval must be positive, got 0s"}
exit=1   (3 "check started" entries before the reload)
```

Pre-fix control (new tests, `config.go` from `HEAD`):

```text
--- FAIL: TestValidate/zero_run_interval
--- FAIL: TestValidate/negative_run_interval
--- FAIL: TestValidate/zero_run_interval,_negative_timeout
--- FAIL: TestValidate/zero_run_interval,_positive_timeout
FAIL	metrik/internal/config
```

Regression suite:

```text
ok  	demo-api/internal/router
ok  	metrik/internal/config
ok  	metrik/internal/runner
ok  	metrik/pkg/breaker
ok  	metrik/pkg/check
```

## Residual Risks

- The reload path is checked by hand only. No automated test covers `load` in `cmd/metrik/run.go`, which has no test files; the unit tests cover `Validate` alone.
- The value was set through the config file and the `-i` flag. The `METRIK_RUNINTERVAL` environment variable was not exercised; it reaches `Validate` through the same decode step.
- Not run in the kind cluster. The Helm values set `runInterval: "15s"`, which is unaffected.
- A zero `requestTimeout` is still accepted. That is the separate bug still listed in the roadmap.

## Recommendation

Close the bug. The reproduction from the assessment was exercised end to end at startup and on reload, and the fix holds.
