# Implementation Plan: Circuit Breaker as a Standalone Package

**Branch**: `003-breaker-package` | **Date**: 2026-10-01 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/003-breaker-package/spec.md`

## Summary

Move feature 002's circuit breaker out of `apps/metrik/cmd` into a new stdlib-only package
`apps/metrik/pkg/breaker`, as Constitution v1.1.0 requires ("`cmd` holds CLI code only").

The API is the existing one, exported one-to-one: `Breaker`, `New`, `Allow`, `Record`, `Keep`,
`Gate`, `Change`, `Kind`, `State`. Check identity becomes `(*check.Check).ID()`. Logging stays in
`cmd`, so operator-visible behaviour is unchanged.

The breaker's tests move with it as an external test package. The rest of the non-CLI logic in
`cmd` is recorded below as a justified, deferred gap.

## Technical Context

**Language/Version**: Go 1.27.1 (pinned in `.mise.toml`)

**Primary Dependencies**: none new. `pkg/breaker` imports only `sync` and `time`. `cmd` keeps cobra, viper, zap and fsnotify.

**Storage**: N/A. State is in memory (unchanged from 002).

**Testing**: `go test -race` via `mise run test`. Breaker tests run on their own in `pkg/breaker` (`package breaker_test`).

**Target Platform**: Linux container on Kubernetes; local runs (unchanged).

**Project Type**: CLI daemon plus internal library packages (`apps/metrik/pkg/*`).

**Performance Goals**: no change. Same lock, same map, same call count per cycle.

**Constraints**:
- Byte-for-byte identical operator behaviour: config, flags, env, log messages/levels/fields, alerts (FR-003).
- No new dependency.
- The breaker must not import `cmd`, `check`, zap, viper or cobra (FR-002).

**Scale/Scope**: about 140 lines moved and renamed, about 110 lines of tests moved, a few call sites in `cmd/run.go` and `cmd/run_test.go`, one new method in `pkg/check`, and pointer updates in feature 002's docs.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.* Constitution v1.1.0.

| Principle | Status | Evidence |
|---|---|---|
| I. Simplicity First | ✅ with a recorded gap | The new package is exactly what the v1.1.0 rule requires, and it is explicitly allowed with a single caller. No new interface, no new options, API exported one-to-one (R2). `breaker` doesn't import `cmd` (R1, data-model dependency rule). The remaining non-CLI code in `cmd` is a known deviation, justified in Complexity Tracking (R6). |
| II. Configuration Over Code | ✅ | No setting added or changed. Keys, defaults, flags and env stay those of 002. |
| III. Observable by Default | ✅ | Log entries and alerts are produced by the same `startCycle` code with renamed identifiers (R5). Quickstart §3 replays 002's scenario. |
| IV. Resilient, Bounded Checking | ✅ | No change to timeouts, retries, concurrency or context handling. Concurrency safety is unchanged (same mutex), checked with `-race`. |
| V. Declarative, Reproducible Delivery | ✅ | No chart, values, Pulumi or image change. |
| Quality gates | ✅ planned | Breaker tests move with it, plus a `TestID` for `check.ID`. Lint at 0 issues. The README is unaffected (no operator-visible change). |

Post-design re-check: ✅. Phase 1 added only `check.ID()` and renames, and every remaining violation is listed below.

## Project Structure

### Documentation (this feature)

```text
specs/003-breaker-package/
├── plan.md              # This file
├── research.md          # R1–R7
├── data-model.md        # Old → new names; dependency rule
├── quickstart.md        # Validation guide
├── contracts/
│   └── breaker-api.md   # Exported API of pkg/breaker + check.ID
├── checklists/
│   └── requirements.md  # From /speckit-specify
└── tasks.md             # Phase 2 (/speckit-tasks; not created here)
```

### Source Code (repository root)

```text
apps/metrik/
├── pkg/
│   ├── breaker/
│   │   ├── breaker.go        # NEW (moved from cmd/breaker.go, exported names)
│   │   └── breaker_test.go   # NEW (moved from cmd/breaker_test.go, package breaker_test)
│   └── check/
│       ├── main.go           # + func (c *Check) ID() string
│       └── main_test.go      # + TestID
└── cmd/
    ├── breaker.go            # DELETED
    ├── breaker_test.go       # DELETED
    ├── run.go                # startCycle/NewRunCmd use breaker.New/Allow/Record/Keep, check.ID
    └── run_test.go           # newCircuits() → breaker.New()
specs/002-endpoint-circuit-breaker/
├── research.md               # R2 marked superseded → spec 003
├── plan.md                   # structure tree + note
├── tasks.md                  # path note
└── quickstart.md             # §1 test location
```

**Structure Decision**: Add `apps/metrik/pkg/breaker` beside `pkg/check` in the existing `apps/metrik` module. `apps/api` and `pulumi/` are untouched.

## Complexity Tracking

> **Resolved** (Constitution v1.2.0): the follow-up refactor moved `Config`/`Validate`/`Resolve` to
> `apps/metrik/internal/config`, `startCycle`/`report`/`notifyOnCritical` and the run loop to
> `apps/metrik/internal/runner`, and `createClient` to `pkg/check.NewClient`. The commands now live
> in `apps/metrik/cmd/metrik`. The row below is kept as history.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| Constitution I v1.1.0 ("`cmd` MUST contain only CLI code"): these remain in `apps/metrik/cmd/run.go`: `Config.validate`, `Config.resolve`, `startCycle`, `report`, `createClient`, `notifyOnCritical`, and the settings part of `Config` | The maintainer scoped this feature to the breaker only (spec FR-006, Q1: C). Moving the scheduler, reporting, HTTP client and alert hook means splitting the roughly 700-line `run_test.go` and designing new exported APIs for the run loop. | Doing it here would turn a low-risk move into a redesign. It is deferred to a follow-up feature seeded from this list. The constitution's Sync Impact Report lists the same items. |
