# Implementation Plan: Healthchecker Automated Tests

**Branch**: `worktree-001-healthchecker-tests` | **Date**: 2026-09-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/001-healthchecker-tests/spec.md`

## Summary

Add the first Go test suite for `apps/metrik`. It has three layers:

1. `pkg/check` tests drive `Check.Run` against `httptest` servers and assert the verdict, the attempt count and the retry timing.
2. `cmd` tests cover per-check settings resolution, loading of the example config, result classification with alerting, and the `maxConcurrent` bound.
3. A few `t.Skip`-ped tests pin the known constitution divergences (KD-1..3) without fixing them.

To make layer 2 reachable, `cmd/run.go` gets a behavior-preserving extraction of three unexported functions: `resolve`, `report` and `startCycle`. No new module dependencies are added. See [research.md](./research.md).

## Technical Context

**Language/Version**: Go 1.26.1 toolchain (`.mise.toml`); module declares `go 1.25.6`

**Primary Dependencies**: existing only: `cenkalti/backoff/v5`, `zap` (+ `zaptest/observer`, same module), `viper`, `cobra`; stdlib `testing`, `net/http/httptest`, `sync/atomic`

**Storage**: N/A

**Testing**: `go test -race` and `golangci-lint` v2.11.4, run through `mise exec --` because a v1 binary shadows it on PATH

**Target Platform**: Linux/macOS developer machines. Tests use loopback only.

**Project Type**: CLI (cobra) long-running health-check daemon

**Performance Goals**: full suite < 30 s (SC-003); expected ≈ 3–5 s with `t.Parallel()`

**Constraints**: offline, ephemeral ports only, no timing flakiness (20× `-race` runs), no observable behavior change in production code

**Scale/Scope**: 2 packages under test (`pkg/check` 110 LOC, `cmd/run.go` 178 LOC). About 25 test cases in total, including 3 skipped known-defect tests.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle / Gate | Status | Notes |
|------------------|--------|-------|
| I. Simplicity First: stdlib first, no new deps, no YAGNI knobs | ✅ | Only stdlib and the zap/viper modules already required. No backoff injection seam (research R2). The only new parameter, `alert`, exists so tests can observe alerts, and production passes the existing hook. |
| I. Independent modules | ✅ | Changes confined to `apps/metrik` (+ one lint-config line). |
| II. Configuration over code | ✅ | No new settings. The example config is asserted as-is. |
| III. Observable by default | ✅ (tested) | Tests assert log level and structured fields. The pre-existing `fmt.Printf` in `notifyOnCritical` is **reported, not changed** (out of scope). |
| IV. Resilient, bounded checking | ✅ (tested) | The suite enforces bounded retries, no retry on 4xx, cancellation and the concurrency bound. Divergences KD-1..3 are pinned as skipped tests. |
| V. Declarative delivery | ✅ N/A | No chart, image or Pulumi change. |
| Gate: `go test ./...` passes | ✅ planned | Includes `-race`. |
| Gate: `golangci-lint` no **new** findings | ⚠️ → ✅ | Baseline already has 13 findings. New cmd tests import `metrik/pkg/check`, which the strict depguard allow-list rejects (as it already does for `run.go`). The fix is to add `metrik/pkg` and `metrik/internal` to the allow-list, a one-line change that also clears the existing depguard findings on those imports. The `run.go` extraction also clears the existing `deferInLoop` finding. |
| Workflow: no commit without request; one logical change per commit | ✅ | Suggested split in Complexity Tracking below. Nothing is committed until the maintainer asks. |

**Post-design re-check (after Phase 1)**: all rows unchanged; no violations to justify.

## Project Structure

### Documentation (this feature)

```text
specs/001-healthchecker-tests/
├── plan.md              # This file
├── research.md          # Phase 0: decisions R1–R8, known defects KD-1..3
├── data-model.md        # Phase 1: entities and verdict/severity tables
├── quickstart.md        # Phase 1: how to run and validate the suite
├── contracts/
│   └── test-seams.md    # Phase 1: signatures of the extracted run.go functions
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks), not created here
```

### Source Code (repository root)

```text
.golangci.yaml                     # depguard allow: + metrik/pkg, metrik/internal
apps/metrik/
├── configs/metrik.yaml            # read by config test (unchanged)
├── cmd/
│   ├── run.go                     # extract resolve / report / startCycle (no behavior change)
│   └── run_test.go                # NEW: US2 + US3 + KD-2, KD-3
└── pkg/check/
    ├── main.go                    # unchanged
    └── main_test.go               # NEW: US1 + edge cases + KD-1 (package check)
```

**Structure Decision**: standard Go layout with `_test.go` files next to the code they test, in the same package (white-box). `cmd` tests must reach the unexported `resolve`/`report`/`startCycle` functions. `pkg/check` tests only need exported API, but staying in-package avoids a depguard import finding. No `tests/` tree: Go convention and the constitution both favor co-located tests.

## Complexity Tracking

No constitution violations. Suggested commit split (one logical change each, for when the maintainer asks):

1. `chore(lint): allow metrik internal packages in depguard`
2. `refactor(metrik): extract run cycle helpers for testability`
3. `test(check): cover probe verdicts and retry rules`
4. `test(metrik): cover settings resolution, classification and concurrency`
