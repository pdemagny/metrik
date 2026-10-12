# Bug Verification: A zero or negative requestTimeout lets a probe hang forever

- **Slug**: zero-request-timeout
- **Tested**: 2026-10-12
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

## Summary

The symptom no longer reproduces: every configuration from the assessment that left a probe hanging now exits 1 with `requestTimeout must be positive`, at startup and on reload. The five new test cases fail without the two new `Validate` rules and pass with them, and no regression was found.

## Checks Performed

All reproduction runs use a binary built from the fixed tree and the assessment's setup: a local endpoint on `127.0.0.1:18999` that accepts connections and never answers.

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction, global value (post-fix) | `metrik run` with global `requestTimeout` of `0s` (`maxConcurrent` 2 and 1) and `-1s` | pass | Each exits 1 with the global message. Before the fix, the probe never ended in all three. |
| Reproduction, per-check value (post-fix) | `metrik run` with a valid global and `requestTimeout: "-1s"`, then `"0s"`, on the `hang` check | pass | Each exits 1 with the `check "hang"` message. |
| Reproduction on reload (post-fix) | Start with `1s`, rewrite the file to `0s` after 5s | pass | The check fails and alerts first, then `invalid config` and exit 1. |
| Flag and environment overrides | Valid file, with `-t 0`, then with `METRIK_REQUESTTIMEOUT=0s` | pass | Both exit 1 with the global message. |
| Valid config still runs | Global `1s`, no per-check value, 7s | pass | The hung endpoint gives `check failed` and `ALERT` 3 times; the other check is probed 6 times. |
| New / updated tests | `go test -race -count=1 -v -run 'TestValidate\|TestResolve\|TestExampleConfigLoads' ./internal/config/` | pass | 21 `TestValidate` cases including the 5 new ones, `TestResolve`, and `TestExampleConfigLoads`. |
| Control without the fix | `TestValidate` in a scratch copy of the fixed tree with the two new `Validate` rules removed | pass | The 5 new cases fail and the others pass, so the new cases do detect the bug. |
| Regression suite | `mise run test` (race detector, all three modules) | pass | |
| Lint | `mise run lint` | pass | 0 issues in every module. |
| Helm values | `helm template` with `configs/dev-values.yaml` | pass | The rendered config sets `requestTimeout: "5s"` globally and `"1s"` on one check, both valid. Rendered only, not deployed. |

## Output Excerpts

Startup, global `requestTimeout: "0s"`:

```text
"error":"requestTimeout must be positive, got 0s"
exit=1
```

Startup, `requestTimeout: "0s"` on the `hang` check:

```text
"error":"check \"hang\": requestTimeout must be positive, got 0s"
exit=1
```

Control run, global `1s` (7 seconds):

```text
3 "msg":"check failed","name":"hang"
6 "msg":"check failed","name":"ok"
9 "msg":"ALERT"
```

Control without the two new rules:

```text
--- FAIL: TestValidate/zero_global_timeout
--- FAIL: TestValidate/negative_global_timeout
--- FAIL: TestValidate/zero_global_timeout,_checks_with_their_own
--- FAIL: TestValidate/zero_check_timeout
--- FAIL: TestValidate/negative_check_timeout
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

- The reload path is checked by hand only. `cmd/metrik` has no test files; the unit tests cover `Validate` and `Resolve`.
- When these checks ran, no automated test decoded a per-check `requestTimeout: 0` from YAML; that path was exercised with the real binary. `TestDecodeCheckSettings`, added afterwards with the per-check inheritance change, now covers it.
- Not run in the kind cluster. The chart was rendered, not deployed.
- A config that sets `requestTimeout: 0` on a check to mean "use the global value" is now rejected. This is intended, and none of the shipped configs do it.
- `runner.go` dereferences the resolved timeout. It is always non-nil after `Resolve`, which `TestResolve` asserts.

## Recommendation

Close the bug. The reproduction from the assessment was exercised end to end at startup and on reload, for global and per-check values, and the fix holds.
