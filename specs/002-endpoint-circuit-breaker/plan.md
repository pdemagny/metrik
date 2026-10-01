# Implementation Plan: Endpoint Circuit Breaker

**Branch**: `002-endpoint-circuit-breaker` | **Date**: 2026-10-01 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/002-endpoint-circuit-breaker/spec.md`, plus the
maintainer's note: "Since we now have tests covering those dev checks, we can remove them."

## Summary

Metrik will stop probing a check after `failureThreshold` failed cycles in a row (default 3). It
skips the check for `cooldown` (default 60s), sends one trial probe, and then either resumes
normal probing or skips the check for another cool-down.

The breaker is a small hand-rolled state map in `apps/metrik/pkg/breaker` (moved there by spec 003), consulted by `startCycle`
before it takes a concurrency slot. `report` returns whether the cycle failed, and the breaker
records that verdict.

Alerting is unchanged: every failed probe still alerts. The reduction in requests and alerts
comes from the skipped cycles.

The two always-failing demo checks ("liveness not found" and "liveness connection refused") are
removed from the example, dev and prod configs, because tests already cover their behaviour.

## Technical Context

**Language/Version**: Go 1.27.1 (pinned in `.mise.toml`)

**Primary Dependencies**: existing only (cobra, viper, zap, backoff/v5, fsnotify). No new module ([research R1](research.md#r1-library-or-hand-rolled-breaker)).

**Storage**: N/A. State is in memory and lost on restart.

**Testing**: `go test` with the race detector (`mise run test`). Logs are asserted with `zaptest/observer`, and endpoints are simulated with `httptest`.

**Target Platform**: Linux container (ko image) on Kubernetes; local runs with `air`.

**Project Type**: CLI daemon (`metrik run`).

**Performance Goals**: no measurable overhead per cycle. One mutex-guarded map lookup per check per cycle.

**Constraints**:
- Suspended checks must not use a `maxConcurrent` slot.
- Existing alert semantics are preserved.
- Configs without the new keys keep working.

**Scale/Scope**: tens of checks at a 10s interval. About 80 lines of production code plus tests, 2 new config keys, and 3 config files trimmed.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Evidence |
|---|---|---|
| I. Simplicity First | ✅ | No new dependency (R1). No new package or interface; one unexported type with one caller (R2). Time is passed as an argument instead of an injected clock (R8). (The location was revised by spec 003 under Constitution v1.1.0.) |
| II. Configuration Over Code | ✅ | `failureThreshold` and `cooldown` can be set globally (YAML, flag, env) and per check (YAML). The defaults are documented in `configs/metrik.yaml` and the README (FR-015). |
| III. Observable by Default | ✅ | The 4 new structured entries carry `name`, `endpoint` and `circuit` ([log-events](contracts/log-events.md)). Failed probes still log at ERROR and alert. The new ERROR entries (`circuit opened` / `circuit reopened`) describe a state change, not a probe outcome, so they don't trigger the alert a second time. A skipped cycle has no response time or status code because no request is made. The maintainer chose per-failure alerts over one alert per outage to keep this principle unamended. |
| IV. Resilient, Bounded Checking | ✅ | The feature reduces load on failing services. The breaker never adds requests: at most one trial per cool-down. `maxConcurrent` and `maxTries` still bound everything. Context propagation is unchanged. |
| V. Declarative, Reproducible Delivery | ✅ | Only Helm values files change (`configs/*-values.yaml`). No template, chart or Pulumi change. |
| Quality gates | ✅ planned | The breaker logic, `validate`, the `resolve` extension and the `startCycle` integration get table-driven tests. Lint must stay at 0 issues. The README is updated in the same change. |

**Pre-existing gaps, out of scope** (not introduced or worsened by this feature):
- `metrik run` doesn't handle SIGINT/SIGTERM (principle IV); the context is never cancelled by a signal.
- `OnConfigChange` writes `runConfig` while the run loop reads it (a data race).
- `time.Tick` is created on every loop iteration.

Post-design re-check: still ✅. Nothing in Phase 1 adds a package, dependency, interface or unconfigurable value.

## Project Structure

### Documentation (this feature)

```text
specs/002-endpoint-circuit-breaker/
├── plan.md              # This file
├── research.md          # Phase 0: decisions R1–R10
├── data-model.md        # Phase 1: settings, circuit state, transitions
├── quickstart.md        # Phase 1: validation guide
├── contracts/
│   ├── configuration.md # Phase 1: config keys, flags, env, validation
│   └── log-events.md    # Phase 1: new log entries and alert behaviour
├── checklists/
│   └── requirements.md  # From /speckit-specify
└── tasks.md             # Phase 2 (/speckit-tasks; not created here)
```

### Source Code (repository root)

```text
apps/metrik/
├── cmd/
│   ├── root.go           # + failureThreshold / cooldown flags (defaults 3 / 60s)
│   ├── run.go            # Config fields + validate(); resolve() fills breaker settings;
│   │                     # startCycle consults circuits before taking a slot;
│   │                     # report() returns failed bool
│   └── run_test.go       # + validate / resolve cases, startCycle skip/trial cases,
│                         #   TestExampleConfigLoads updated for trimmed example
├── pkg/breaker/          # moved here by spec 003
│   ├── breaker.go        # NEW: Breaker type: Allow/Record/Keep, transitions
│   └── breaker_test.go   # NEW: table-driven transition tests (literal times)
├── pkg/check/main.go     # Check gains FailureThreshold *int, Cooldown time.Duration
└── configs/
    ├── metrik.yaml       # − 2 failing demo checks; + documented failureThreshold/cooldown
    ├── dev-values.yaml   # − 2 failing demo checks
    └── prod-values.yaml  # − 2 failing demo checks
README.md                 # + Configuration subsection (breaker settings); Roadmap: drop "Circuit Breaker"
```

**Structure Decision**: The single existing module `apps/metrik` is extended in place. The breaker sits beside `startCycle` in `cmd` (R2). `apps/api` and `pulumi/` are untouched.

## Complexity Tracking

No constitution violations. Nothing to justify.
