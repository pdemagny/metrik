# Bug Assessment: A zero or negative runInterval stops all probing without an error

- **Slug**: zero-run-interval
- **Created**: 2026-10-12
- **Source**: pasted text
- **Verdict**: valid
- **Severity**: low

## Report (verbatim or summarized)

> When runInterval is set to 0 metrik starts but never probes and reports no error. The value should be rejected at startup and on reload.

The same wording is in `docs/metrik-roadmap.md:20`, under "Zero or negative `runInterval`", so this assessment covers negative values too.

## Symptom

With a `runInterval` of zero or less, metrik keeps running, never starts a check cycle, and logs nothing. Expected: the value is rejected with an `invalid config` error at startup and on reload.

The report is true only in a narrower case than it states. Since commit `737cc49`, `Validate` rejects a `runInterval` below `maxTries` × `requestTimeout`, and that rule already catches a zero or negative `runInterval` whenever a positive `requestTimeout` applies, globally or on any check. The silent case remains when no positive `requestTimeout` applies anywhere: the global `requestTimeout` is zero or negative and no check sets its own.

| `runInterval` | `requestTimeout` (global) | Checks                            | Result today                                   |
| :------------ | :------------------------ | :-------------------------------- | :--------------------------------------------- |
| `0` or `-5s`  | `5s` (default)            | any                               | Rejected, but with the `maxTries x requestTimeout` message |
| `0` or `-5s`  | `0`                       | one sets `requestTimeout: 1s`     | Rejected through that check                    |
| `0` or `-5s`  | `0` or `-1s`              | none sets `requestTimeout`        | **Starts, never probes, logs nothing**         |

## Reproduction

Reproduced with the binary built from `main` (`73f25fa`, Go 1.27.1), from a scratch directory holding only this `metrik.yaml`:

```yaml
runInterval: "0s"
requestTimeout: "0s"
maxTries: 1
failureThreshold: 0
checks:
  - name: "x"
    endpoint: "http://127.0.0.1:9/live"
```

At startup:

1. Run `metrik run` in that directory for 5 seconds.
2. Result: no output at all, and the process is still running when killed (`timeout` exit 124). The same happens with `runInterval: "-5s"`, and with `runInterval: "0s"` plus `requestTimeout: "-1s"`.

On reload:

1. Start with `runInterval: "1s"` and `requestTimeout: "0s"`. The check is probed every second.
2. After 3.5 seconds, rewrite the file with `runInterval: "0s"`.
3. Result: one more probe runs (the tick already pending), then probing stops. No error is logged and the process keeps running.

For comparison, the cases that are already rejected:

- `metrik run -i 0` with the shipped config exits 1 with `runInterval (0s) must not be below maxTries x requestTimeout (3 x 5s)`.
- A reload from `2s`/`1s` to `0s`/`1s` exits 1 with `runInterval (0s) must not be below maxTries x requestTimeout (1 x 1s)`.

## Suspected Code Paths

- `apps/metrik/internal/runner/runner.go:33` — `time.Tick(current().RunInterval)`. `time.Tick` returns a nil channel when the duration is zero or negative, and a receive from a nil channel blocks forever. The `select` then only ever leaves through `ctx.Done()`.
- `apps/metrik/internal/config/config.go:31` — `Validate` has no check on the sign of `RunInterval`.
- `apps/metrik/internal/config/config.go:59` — `tooShort` returns `false` when `timeout <= 0`, so the only rule that mentions `runInterval` is skipped exactly when `requestTimeout` is not positive.
- `apps/metrik/cmd/metrik/run.go:22` — `load` calls `Validate` at startup and from `OnConfigChange`, so one new rule in `Validate` covers both paths. No change is needed here.

## Root Cause Hypothesis

`Validate` never requires `RunInterval` to be positive. A non-positive value is caught only as a side effect of the `maxTries` × `requestTimeout` rule, and `tooShort` skips that rule when the timeout is not positive. The value then reaches `time.Tick`, which returns a nil channel, and the run loop blocks without logging. Confidence: high. The behavior is documented for `time.Tick` and matches all five runs above.

## Proposed Remediation

**Preferred**: add an explicit rule at the top of `Config.Validate`, before the `tooShort` rule: return an error such as `runInterval must be positive, got 0s` when `cfg.RunInterval <= 0`. It follows the existing rules for `failureThreshold` and `cooldown`, it covers startup and reload because both go through `load`, and it gives the already-rejected cases a message that names the real problem. Placing it first matters: otherwise `-i 0` with default settings keeps reporting the `maxTries x requestTimeout` message.

**Alternatives**:
- Guard in `runner.Run` (return an error when the interval is not positive). It protects the loop from any caller, but it only runs once at startup unless the loop is restructured, so it does not cover reload, and it duplicates a rule that belongs with the other config rules.
- Fix only the "Zero `requestTimeout`" roadmap bug. Once a positive global `requestTimeout` is required, `tooShort` rejects every non-positive `runInterval`. The bug would be closed by coincidence, with a misleading message, and would reopen if that rule changed.

**Files likely to change**:
- `apps/metrik/internal/config/config.go`
- `apps/metrik/internal/config/config_test.go`
- `docs/metrik-configuration.md` (the sentence at line 30 that lists rejected values)
- `docs/metrik-roadmap.md` (move the row out of "Bugs to fix")

**Tests to add or update**:
- `TestValidate`: new cases for a zero and a negative `RunInterval` with a zero `RequestTimeout` (the silent case), and for a zero `RunInterval` with the default timeout, asserting the new message.
- `TestValidate`: existing cases that leave `RunInterval` at zero must set a positive value. `zeros and unset are valid` would otherwise fail, and the four `negative ...` cases would get the `runInterval` error before the one they assert.
- `TestExampleConfigLoads` already validates the shipped config (`15s`) and should keep passing unchanged.

## Risks & Considerations

- A config that sets a non-positive `runInterval` and currently starts (silently doing nothing) will now exit at startup. That is the intended outcome.
- Rejection on reload means `logger.Fatal`, so the process exits. This is how every other invalid value is handled today; the fix keeps that behavior and does not change it.
- The runner tests call `startCycle` directly and never go through `Validate`, so they are not affected.
- The "Zero `requestTimeout`" roadmap bug touches the same function and the same test table. Fixing both in one pass would avoid editing the `TestValidate` cases twice, but they are separate reports.

## Open Questions

- None.
