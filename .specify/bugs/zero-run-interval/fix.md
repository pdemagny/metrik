# Bug Fix: A zero or negative runInterval stops all probing without an error

- **Slug**: zero-run-interval
- **Fixed**: 2026-10-12
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

`Config.Validate` now rejects a `runInterval` that is zero or negative, before any other rule. The value can no longer reach `time.Tick`, so metrik exits with `invalid config` at startup and on reload instead of running without probing.

## Changes

| File | Change | Notes |
|------|--------|-------|
| `apps/metrik/internal/config/config.go` | modified | New first rule in `Validate`: `runInterval must be positive, got <value>`. Doc comment updated. |
| `apps/metrik/internal/config/config_test.go` | modified | Four new `TestValidate` cases; five existing cases now set a positive `RunInterval`. |
| `docs/metrik-configuration.md` | modified | The rejected-values paragraph names a zero or negative `runInterval`. |
| `docs/metrik-roadmap.md` | modified | Removed the "Zero or negative `runInterval`" row from "Bugs to fix". |

## Diff Highlights (optional)

```go
func (cfg Config) Validate() error {
	// A zero or negative interval never ticks, so no check would ever run.
	if cfg.RunInterval <= 0 {
		return fmt.Errorf("runInterval must be positive, got %s", cfg.RunInterval)
	}
```

## Tests Added or Updated

- `config_test.go::TestValidate/zero_run_interval` — a zero `RunInterval` with no timeout set (the silent case) is rejected.
- `config_test.go::TestValidate/negative_run_interval` — a negative `RunInterval` with no timeout set is rejected.
- `config_test.go::TestValidate/zero_run_interval,_negative_timeout` — the rule does not depend on the sign of `RequestTimeout`.
- `config_test.go::TestValidate/zero_run_interval,_positive_timeout` — the new message wins over the `maxTries x requestTimeout` message.
- `config_test.go::TestValidate` — `zeros and unset are valid` and the four `negative ...` cases now set `RunInterval: time.Second`, so they keep asserting what they did before.

## Local Verification

- Commands run: `go test -race -count=1 ./...` in `apps/metrik` (Go 1.27.1 through `mise exec`) → all packages pass.
- Commands run: `mise run lint` → 0 issues in every module.
- Commands run: `mise run docs` → site builds.
- Manual checks, with a binary built from the fixed tree and the scratch config from the assessment:
  - Startup with `runInterval`/`requestTimeout` of `0s`/`0s`, `-5s`/`0s` and `0s`/`-1s` → each exits 1 with `runInterval must be positive, got <value>`. All three ran silently before.
  - Reload from `1s`/`0s` to `0s`/`0s` → three probes, then exit 1 with the same message. Before, probing stopped and the process kept running.
  - `metrik run -i 0` with the shipped config → exit 1 with the new message instead of the `maxTries x requestTimeout` one.

## Deviations from Assessment

None.

## Follow-ups

- The "Zero `requestTimeout`" bug is still listed in the roadmap. It touches the same function and the same test table.
- The shell's default `go` is 1.27.2 while `GOROOT` points to the pinned 1.27.1, so a bare `go build` fails with a version mismatch. `mise exec -- go ...` and the `mise run` tasks work.
