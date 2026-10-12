# Bug Assessment: A zero or negative requestTimeout lets a probe hang forever

- **Slug**: zero-request-timeout
- **Created**: 2026-10-12
- **Source**: pasted text
- **Verdict**: valid
- **Severity**: medium

## Report (verbatim or summarized)

> When requestTimeout is set to 0 The HTTP request has no timeout, so a probe of an endpoint that never answers holds its `maxConcurrent` slot indefinitely. The value should be rejected at startup and on reload.

The same wording is in `docs/metrik-roadmap.md:20`, under "Zero `requestTimeout`".

## Symptom

With a `requestTimeout` of zero, a probe of an endpoint that accepts the connection and never answers does not end. It holds its `maxConcurrent` slot until metrik stops. Expected: the value is rejected with an `invalid config` error at startup and on reload.

The report is accurate, and the problem is wider in two ways:

- **Negative values behave the same.** `http.Client` applies its timeout only when it is positive, so `-1s` also means no timeout.
- **A negative per-check value bypasses a valid global.** `Resolve` falls back to the global value only when the check's value is zero, so `requestTimeout: "-1s"` on a check gives that check no timeout even with a global `5s`.

A per-check `requestTimeout` of zero does not cause the hang: `Resolve` treats it as unset and uses the global value. It is still a surprise, because `requestTimeout: "0s"` written on a check is silently replaced by the global value. The remediation below rejects it.

Observable effects, beyond the held slot:

- The hung endpoint never produces `check failed` or `ALERT`. The probe has no outcome, so the outage metrik exists to detect is not reported.
- The check is never probed again. Every later cycle logs `check skipped` with `previous probe still running`, at `warn` level.
- Once hung probes hold every `maxConcurrent` slot, `startCycle` blocks on the limiter and all checks stop, with no further log entry.

## Reproduction

Reproduced with the binary built from `main` (`b9a223b`, Go 1.27.1). A local listener on `127.0.0.1:18999` accepts connections and never answers. Config, from a scratch directory:

```yaml
runInterval: "1s"
requestTimeout: "0s"
maxConcurrent: 2
maxTries: 1
failureThreshold: 0
checks:
  - name: "hang"
    endpoint: "http://127.0.0.1:18999/live"
  - name: "ok"
    endpoint: "http://127.0.0.1:9/live"   # connection refused, fails at once
```

Each run lasted 7 seconds.

| Case | Setting | `hang` check | `ok` check |
| :--- | :------ | :----------- | :--------- |
| A | global `0s`, `maxConcurrent: 2` | 1 `check started`, then 5 `check skipped`; no `check failed`, no `ALERT` | probed 6 times |
| B | global `0s`, `maxConcurrent: 1` | 1 `check started`, then nothing | never probed; no log entry at all |
| C | global `-1s`, `maxConcurrent: 2` | same as A | probed 6 times |
| D | global `1s`, `hang` sets `requestTimeout: "-1s"` | same as A | probed 6 times |
| E (control) | global `1s`, `maxConcurrent: 2` | 3 `check failed` with `ALERT` | probed 6 times |

On reload (one `hang` check, `runInterval: "2s"`):

1. Start with `requestTimeout: "1s"`. The check fails and alerts every 2 seconds.
2. After 5 seconds, rewrite the file with `requestTimeout: "0s"`.
3. Result: the next cycle starts one probe that never ends. Later cycles log `check skipped`. No error is logged and no further `ALERT` is raised.

None of these configurations is rejected today.

## Suspected Code Paths

- `apps/metrik/pkg/check/main.go:53` — `Timeout: timeout` in `NewClient`. A zero or negative `http.Client.Timeout` means no timeout. The transport sets no dial or response-header timeout either, so nothing else bounds the request.
- `apps/metrik/internal/runner/runner.go:95` — `check.NewClient(chk.RequestTimeout)` passes the resolved value through unchecked. The slot taken at line 88 is released only at line 106, after `chk.Run` returns.
- `apps/metrik/internal/config/config.go:83` — `cmp.Or(r.RequestTimeout, cfg.RequestTimeout)` keeps a negative per-check value, because only zero counts as unset.
- `apps/metrik/internal/config/config.go:33` — `Validate` has no rule on `RequestTimeout`, global or per check.
- `apps/metrik/internal/config/config.go:64` — `tooShort` returns `false` when `timeout <= 0`, so the one rule that reads `RequestTimeout` skips these values.

## Root Cause Hypothesis

`Validate` never checks the sign of `RequestTimeout`. A zero or negative value reaches `http.Client.Timeout`, where it disables the timeout, and the probe goroutine blocks in `client.Do` while holding a limiter slot and its in-flight entry. Confidence: high. The five runs above match, and the control run with a positive timeout behaves correctly.

## Proposed Remediation

**Preferred**: require every `requestTimeout` that is written in the config to be positive, and let only an omitted per-check value inherit the global one.

- The global `requestTimeout` must be positive, with a message such as `requestTimeout must be positive, got 0s`.
- A check's own `requestTimeout`, when set, must be positive too, with a message such as `check "x": requestTimeout must be positive, got 0s`. Omitting it means "inherit the global value".

To tell an omitted value from an explicit zero, `Check.RequestTimeout` becomes a `*time.Duration`, nil when omitted, the same way `Check.FailureThreshold` is a `*int`. `Resolve` falls back to the global value when it is nil and always returns a non-nil pointer, and `startCycle` dereferences it for `NewClient`. Checked in a scratch copy: viper decodes an omitted key (or a key with an empty value) to nil, and `"0s"`, a bare `0` and `"1s"` to non-nil values.

The two rules go in `Config.Validate`, after the `runInterval must be positive` rule and before the `maxTries` × `requestTimeout` rule. Both go through `load`, so startup and reload are covered without touching `run.go`. Keeping the `runInterval` rule first preserves the messages asserted by the existing zero-`runInterval` tests.

**Alternatives**:
- Keep `time.Duration` on the check, reject only negative per-check values, and keep zero as "inherit". It is a smaller change, but an explicit `requestTimeout: "0s"` on a check would silently become the global value. Rejected for that reason.
- Validate only the resolved value of each check. It catches the same probes, but a config with a zero global and no checks, or checks that all override it, would pass, which contradicts the report ("the value should be rejected").
- Clamp in `NewClient` (fall back to a default when the timeout is not positive). It hides a config error instead of reporting it, and the effective timeout would differ from the one the `maxTries` × `requestTimeout` rule validated.
- Add transport-level timeouts (`ResponseHeaderTimeout`, dial timeout). Useful hardening, but it does not reject the value and it changes probe behavior for valid configs. Out of scope here.

**Files likely to change**:
- `apps/metrik/pkg/check/main.go` (the field type)
- `apps/metrik/internal/config/config.go` (`Validate` and `Resolve`)
- `apps/metrik/internal/runner/runner.go` (dereference at the `NewClient` call)
- `apps/metrik/internal/config/config_test.go`
- `docs/metrik-configuration.md` (the rejected-values paragraph and the per-check `requestTimeout` row)
- `docs/metrik-roadmap.md` (remove the row; it is the last one, so the "Bugs to fix" section becomes empty)

**Tests to add or update**:
- `TestValidate`: new cases for a zero global timeout, a negative global timeout, and a zero and a negative per-check timeout under a valid global, each asserting its message.
- `TestValidate`: a case showing that an omitted per-check timeout with a positive global stays valid.
- `TestValidate`: the existing cases that set a positive `RunInterval` and no `RequestTimeout` must set one (`zeros and unset are valid` and the four `negative ...` cases), or they would get the new error first. The zero and negative `runInterval` cases are unaffected as long as that rule stays first.
- `TestValidate` and `TestResolve`: cases that set a per-check `RequestTimeout` must pass a pointer. `TestResolve` should also assert that the resolved pointer is never nil and never aliases the input check.
- `TestExampleConfigLoads` compares each decoded check to an expected struct with `!=`. With a pointer field that comparison fails for the check that sets `requestTimeout: "1s"`, so the test must compare that field by value.

## Risks & Considerations

- A config that sets `requestTimeout: 0` to mean "no timeout" will now exit at startup. There is no supported way to disable the timeout afterwards. That matches the report's intent.
- Rejection on reload means `logger.Fatal`, so the process exits. Every other invalid value is handled this way today.
- After the fix, the `timeout <= 0` guard in `tooShort` cannot be reached from `Validate` with a validated config. It still protects the division, so it should stay.
- A check that sets `requestTimeout: 0` to mean "use the global value" will now be rejected. The shipped config, the Helm values and the docs example never do this.
- `Check` is in `pkg/check`, so the field type change is visible to any code that imports the package. Nothing outside `apps/metrik` does today.
- The runner tests set only the global `RequestTimeout` and call `startCycle` directly. They should keep compiling and passing unchanged.
- `alertThreshold`, `maxTries` and `cooldown` on a check keep treating zero as "inherit". After the fix `requestTimeout` behaves differently from them. Aligning them is a separate change.
- Removing the last row leaves an empty "Bugs to fix" table in the roadmap. The fix should remove the section or leave a placeholder; either is a wording choice.

## Open Questions

- None.
