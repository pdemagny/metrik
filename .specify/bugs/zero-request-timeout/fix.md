# Bug Fix: A zero or negative requestTimeout lets a probe hang forever

- **Slug**: zero-request-timeout
- **Fixed**: 2026-10-12
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

`Config.Validate` now rejects a `requestTimeout` that is zero or negative, globally and on a check, so the HTTP client always gets a positive timeout. A check's `requestTimeout` is now a pointer: only an omitted value inherits the global one, and an explicit `0` is rejected instead of being silently replaced.

## Changes

| File | Change | Notes |
|------|--------|-------|
| `apps/metrik/pkg/check/main.go` | modified | `Check.RequestTimeout` is a `*time.Duration`, nil when omitted. |
| `apps/metrik/internal/config/config.go` | modified | Two new rules in `Validate`. `Resolve` falls back to the global value on nil and always returns a fresh pointer. |
| `apps/metrik/internal/runner/runner.go` | modified | Dereferences the resolved timeout at the `NewClient` call. |
| `apps/metrik/internal/config/config_test.go` | modified | Five new `TestValidate` cases, updated cases, alias check in `TestResolve`, by-value comparison in `TestExampleConfigLoads`. |
| `docs/metrik-configuration.md` | modified | The rejected-values paragraph, the per-check `requestTimeout` row, and the paragraph on per-check zero values. |
| `docs/metrik-roadmap.md` | modified | Removed the "Zero `requestTimeout`" row and the now empty "Bugs to fix" section. |

## Diff Highlights (optional)

`Validate`, after the `runInterval` rule:

```go
// A zero or negative timeout means no timeout to the HTTP client, so a probe could hang forever.
if cfg.RequestTimeout <= 0 {
	return fmt.Errorf("requestTimeout must be positive, got %s", cfg.RequestTimeout)
}
```

`Validate`, for each check:

```go
if c.RequestTimeout != nil && *c.RequestTimeout <= 0 {
	return fmt.Errorf("check %q: requestTimeout must be positive, got %s", c.Name, *c.RequestTimeout)
}
```

`Resolve`:

```go
timeout := cfg.RequestTimeout
if r.RequestTimeout != nil {
	timeout = *r.RequestTimeout
}
r.RequestTimeout = &timeout
```

## Tests Added or Updated

- `config_test.go::TestValidate/zero_global_timeout` — a zero global timeout is rejected.
- `config_test.go::TestValidate/negative_global_timeout` — a negative global timeout is rejected.
- `config_test.go::TestValidate/zero_global_timeout,_checks_with_their_own` — the global value is rejected even when every check overrides it.
- `config_test.go::TestValidate/zero_check_timeout` — an explicit zero on a check is rejected, not inherited.
- `config_test.go::TestValidate/negative_check_timeout` — a negative value on a check is rejected under a valid global.
- `config_test.go::TestValidate/zeros_and_unset_are_valid` — now sets a global timeout, and shows that an omitted per-check timeout stays valid.
- `config_test.go::TestValidate` — the four `negative ...` cases set a global timeout so they keep asserting their own error; the two cases with a per-check timeout pass a pointer.
- `config_test.go::TestResolve` — the resolved timeout is never nil and never aliases the input check's pointer.
- `config_test.go::TestExampleConfigLoads` — compares `RequestTimeout` by value and the other fields as a whole.

## Local Verification

- Commands run: `mise run test` (race detector, every module) → all packages pass.
- Commands run: `mise run lint` → 0 issues in every module.
- Commands run: `mise run docs` → site builds.
- Manual checks, with a binary built from the fixed tree and the assessment's setup (a local endpoint that accepts connections and never answers):
  - Global `0s` (with `maxConcurrent` 2 and 1) and global `-1s` → exit 1 with `requestTimeout must be positive, got <value>`.
  - Check `hang` with `requestTimeout` of `"-1s"`, `"0s"` and a bare `0` → exit 1 with `check "hang": requestTimeout must be positive, got <value>`.
  - Reload from `1s` to `0s` → `invalid config` and exit 1, instead of a probe that never ends.
  - Controls: global `1s` with no per-check value, and global `2s` with `1s` on the `hang` check → the hung endpoint is reported as `check failed` each time it is probed.
  - The shipped `configs/metrik.yaml` still starts.

## Deviations from Assessment

- None in the fix itself.
- The assessment left the empty "Bugs to fix" roadmap section as a wording choice. The section was removed, which restores the roadmap to its shape before the two bugs were listed.

## Follow-ups

- Done after this fix, as a separate change: `alertThreshold`, `maxTries` and `cooldown` on a check are pointers too, so only an omitted value inherits and an explicit zero is kept. Before that change they treated zero as "inherit", unlike `requestTimeout`.
- The transport still has no dial or response-header timeout. The client timeout covers both, so this is hardening only.
- The tests build the pointers with `new(expr)`, which needs Go 1.26 or later. The module pins 1.27.1.
