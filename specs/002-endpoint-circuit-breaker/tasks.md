---

description: "Task list for the endpoint circuit breaker"
---

# Tasks: Endpoint Circuit Breaker

**Input**: Design documents from `/specs/002-endpoint-circuit-breaker/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/configuration.md, contracts/log-events.md, quickstart.md

**Tests**: Included. The constitution ("Development Workflow & Quality Gates") requires Go tests for new or changed logic, and plan.md lists them as a quality gate. Within each story, write the tests first and confirm they fail before implementing.

**Organization**: Tasks are grouped by user story so that each story can be implemented and tested on its own.

**Note (spec 003)**: the breaker described in T010–T025 now lives in apps/metrik/pkg/breaker with exported names (see specs/003-breaker-package/research.md R2); cmd/breaker*.go no longer exists.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: The user story the task belongs to (US1–US4 from spec.md)

## Path Conventions

- Single Go module `apps/metrik` (see plan.md → Project Structure). `apps/api` and `pulumi/` are untouched.
- Run Go commands from `apps/metrik`. Run `mise run lint` and `mise run test` from the repository root.
- Code style: match the surrounding code. Use zap fields, wrap errors with `fmt.Errorf`, no new dependencies (research R1), no new packages or interfaces (research R2).

---

## Phase 1: Setup (demo config cleanup)

**Purpose**: Apply the maintainer's direction from planning (research R10) before any feature work, so later config edits land on the trimmed files.

- [X] T001 [P] In `apps/metrik/configs/metrik.yaml`, delete the `liveness not found` check (its `name` and `endpoint` lines) and the half-commented `liveness connection refused` check (its active `- name:` line plus the commented `endpoint` / `maxTries` lines). Keep every other line, including the commented-out examples.
- [X] T002 [P] In `apps/metrik/configs/dev-values.yaml`, delete the `liveness not found` and `liveness connection refused` entries (with their `endpoint` / `maxTries` lines) from `configmap.data."metrik.yaml".checks`.
- [X] T003 [P] In `apps/metrik/configs/prod-values.yaml`, delete the same two entries as T002 from `configmap.data."metrik.yaml".checks`.
- [X] T004 Update `TestExampleConfigLoads` in `apps/metrik/cmd/run_test.go` so that `want` holds only `{Name: "liveness slow warn", Endpoint: "http://localhost:8080/v1/slow", AlertThreshold: 3 * time.Second}`. Remove the comment about the half-commented check. Run `go test ./cmd/ -run TestExampleConfigLoads` (depends on T001).

**Checkpoint**: `grep -n "notfound\|8081" apps/metrik/configs/*.yaml` prints nothing, and `go test ./...` passes.

---

## Phase 2: Foundational (blocking prerequisites)

**Purpose**: Settings, resolution, the failure verdict and the empty breaker type that every story builds on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T005 [P] In `apps/metrik/pkg/check/main.go`, add two fields to `Check`, after `RequestTimeout`:
  - `FailureThreshold *int \`yaml:"failureThreshold,omitempty"\``
  - `Cooldown time.Duration \`yaml:"cooldown,omitempty"\``

  Per data-model.md: on a check, `FailureThreshold` is "optional; Unset inherits the global value. `0` disables it for this check. `N` overrides"; `Cooldown` is "`0` or unset inherits the global value".
- [X] T006 [P] In `apps/metrik/cmd/root.go`, register two persistent flags after `maxTries`, before `BindPFlags`:
  - `flags.Int("failureThreshold", 3, "Consecutive failed cycles before a check is suspended (0 disables the circuit breaker).")`
  - `flags.Duration("cooldown", 60*time.Second, "How long a suspended check is skipped before a trial probe.")`

  No shorthand. The env vars `METRIK_FAILURETHRESHOLD` and `METRIK_COOLDOWN` then work through the existing `AutomaticEnv` (contracts/configuration.md).
- [X] T007 In `apps/metrik/cmd/run.go`, add these fields to `Config`, after `MaxTries`:
  - `FailureThreshold int \`yaml:"failureThreshold"\``
  - `Cooldown time.Duration \`yaml:"cooldown"\``

  Extend `resolve` so that:
  - when `r.FailureThreshold == nil`, it gets a pointer to a *new* int holding `cfg.FailureThreshold`. It must never alias a pointer shared with the input or the config.
  - when `r.Cooldown == 0`, it gets `cfg.Cooldown`.

  Update the `resolve` doc comment to list the two new settings (depends on T005).
- [X] T008 Add `TestResolve` cases in `apps/metrik/cmd/run_test.go`:
  - The global config sets `FailureThreshold: 3, Cooldown: time.Minute`. Assert that the resolved threshold and cool-down are inherited when unset, that a per-check `FailureThreshold` of `0` stays `0`, and that `5` stays `5`.
  - A per-check `Cooldown` of `2 * time.Minute` overrides the global value.
  - The resolved pointer is not the input's pointer.

  Keep the existing "input not mutated" assertion working: the struct still compares with `!=` (depends on T007).
- [X] T009 In `apps/metrik/cmd/run.go`, change `report` to return `failed bool`. It returns `true` on each of its three error-level branches (probe error, nil body, body status ≠ 200) and `false` otherwise (healthy, including slow), per research R3. Keep the call in `startCycle` compiling, ignoring the value for now. In `apps/metrik/cmd/run_test.go`, assert the returned value in `TestReport` (add a `failed bool` column), `TestReportNullBody` (`true`) and `TestReportNilResult` (`true`).
- [X] T010 Create `apps/metrik/cmd/breaker.go` containing only the types and constructor, with no behaviour yet:
  - `type circuitState string` with constants `closed = "closed"`, `open = "open"` and `trial = "trial"` (unexported names; pick ones that don't collide in package `cmd`, e.g. `stateClosed` / `stateOpen` / `stateTrial`)
  - `type circuit struct { state circuitState; failures int; openUntil, openedAt time.Time }`
  - `type circuits struct { mu sync.Mutex; m map[string]*circuit }` with `func newCircuits() *circuits`
  - `func circuitKey(c *check.Check) string { return c.Name + "\x00" + c.Endpoint }` (research R6)

  Add a short doc comment pointing to data-model.md for the state machine.

**Checkpoint**: `go build ./... && go test ./...` passes in `apps/metrik`, and behaviour is unchanged.

---

## Phase 3: User Story 1 - Stop probing an endpoint that keeps failing (Priority: P1) 🎯 MVP

**Goal**: after `failureThreshold` consecutive failed cycles, a check is skipped (no request, no concurrency slot) until its cool-down ends. Other checks are unaffected.

**Independent Test**: one check against an always-failing endpoint and one against a healthy endpoint. Once the threshold is reached, the failing endpoint gets no requests during the cool-down, and the healthy one is probed every cycle.

### Tests for User Story 1 ⚠️ (write first, confirm they fail)

- [X] T011 [P] [US1] In a new `apps/metrik/cmd/breaker_test.go`, add a table-driven `TestCircuitOpens` that drives `allow` / `record` with literal times (research R8: `now := time.Unix(0, 0)`, then `now.Add(...)`). Use threshold 3 and cool-down 60s. Cases:
  - 2 failures → still `closed`, and `allow` returns run.
  - 2 failures, then a success, then 2 failures → still `closed` (the count was reset).
  - 3 failures → `allow` at `+10s` returns no run, state `open`, `remaining == 50s`.
  - Two keys: opening one leaves the other `closed`.
- [X] T012 [P] [US1] In `apps/metrik/cmd/run_test.go`, add `TestStartCycleSkipsOpenCircuit`:
  - An `httptest` server returns 500 on `/bad` and a healthy JSON payload on `/good`, counting hits per path.
  - Config: `MaxTries: 1, FailureThreshold: 2, Cooldown: time.Hour, MaxConcurrent: 2`.
  - Run 3 cycles sequentially with one shared `newCircuits()` (`startCycle(...).Wait()` each).
  - Assert `/bad` was hit 2 times and `/good` 3 times. Assert exactly one `check skipped` entry at WARN level, with `name` = the bad check and `circuit` = `"open"`.
- [X] T013 [P] [US1] In `apps/metrik/cmd/run_test.go`, add `TestStartCycleSkipUsesNoSlot` for FR-004:
  - Open a check's circuit (threshold 1, one failed cycle).
  - Fill `limiterCh` to capacity by sending `true` until it is full.
  - Call `startCycle` with only that check and a `t.Context()`.
  - Assert it returns and `Wait()` completes within 1s (use a `done` channel with `select` and `time.After`), proving the skip never tried to take a slot.

### Implementation for User Story 1

- [X] T014 [US1] In `apps/metrik/cmd/breaker.go`, implement the two methods below. Both lock `mu` and lazily create the entry in `closed`.
  - `func (cs *circuits) allow(key string, now time.Time) gate`, with `type gate struct { run, trial bool; state circuitState; remaining time.Duration }`:
    - `closed` → `{run: true}`.
    - `open` and `now.Before(openUntil)` → `{state: open, remaining: openUntil.Sub(now)}`.
    - Leave the cool-down-elapsed branch returning `{run: true}` for now (US2 refines it).
  - `func (cs *circuits) record(key string, trial, failed bool, threshold int, cooldown time.Duration, now time.Time) change`, with `type change struct { kind changeKind; failures int; suspended time.Duration }` and `changeKind` constants `noChange`, `opened`, `reopened`, `recovered`. For the `closed` state:
    - success → `failures = 0`.
    - failure → `failures++`; on reaching `threshold`, set `state = open`, `openUntil = now.Add(cooldown)`, `openedAt = now`, and return `{kind: opened, failures: failures}`.

  Make T011 pass.
- [X] T015 [US1] Wire the breaker into `startCycle` in `apps/metrik/cmd/run.go`:
  - Add a `circuits *circuits` parameter, before `alert`.
  - Inside the loop, resolve `chk` *before* the limiter `select`, compute `key := circuitKey(chk)`, and call `g := circuits.allow(key, time.Now())`.
  - If `!g.run`, log `logger.Warn("check skipped", zap.String("name", c.Name), zap.String("endpoint", c.Endpoint), zap.String("circuit", string(g.state)), zap.Duration("remaining", g.remaining))` and `continue`, without touching `limiterCh` or `wg`.
  - In the goroutine, capture `failed := report(...)`, then call `circuits.record(key, g.trial, failed, *chk.FailureThreshold, chk.Cooldown, time.Now())`.
  - In `NewRunCmd`, create `circuits := newCircuits()` once, next to `limiterCh`, and pass it on every tick.
  - Update every existing `startCycle` call in `apps/metrik/cmd/run_test.go` to pass `newCircuits()`, and give their configs a `FailureThreshold` (e.g. 3) so that `resolve` produces a non-nil pointer.

  Make T012 and T013 pass.
- [X] T016 [US1] Add `func (cs *circuits) keep(keys map[string]struct{})` to `apps/metrik/cmd/breaker.go`. It deletes every entry whose key is not in `keys` (research R6, FR-014). At the start of `startCycle`, build the set from the resolved checks and call it. Add `TestCircuitKeep` in `apps/metrik/cmd/breaker_test.go`:
  - An open circuit for key A is dropped when A is not kept.
  - A check whose endpoint changed (new key) starts `closed`.
  - A kept key keeps its `open` state.

**Checkpoint**: US1 works on its own. A failing check stops being probed after the threshold and stays skipped for the cool-down. Run `go test -race ./...`.

---

## Phase 4: User Story 2 - Resume an endpoint automatically once it recovers (Priority: P1)

**Goal**: once the cool-down has elapsed, exactly one trial probe runs. Success returns the check to normal; failure suspends it for another full cool-down.

**Independent Test**: suspend a check against a controllable endpoint, make it healthy, wait out the cool-down, and confirm that one trial probe happens and normal probing resumes. Repeat with the endpoint still failing and confirm the check is suspended again.

### Tests for User Story 2 ⚠️

- [X] T017 [P] [US2] Add a table-driven `TestCircuitTrial` in `apps/metrik/cmd/breaker_test.go`, with literal times, threshold 3 and cool-down 60s, starting from an `open` circuit opened at `t0`:
  - `allow` at `t0+60s` returns `{run: true, trial: true}`, and the state becomes `trial`.
  - A second `allow` before the trial is recorded returns `{run: false, state: trial, remaining: 0}`.
  - `record(trial=true, failed=false)` at `t0+61s` → `closed`, `failures == 0`, `kind == recovered`, `suspended == 61s`.
  - `record(trial=true, failed=true)` at `t0+61s` → `open`, `openUntil == t0+121s`, `kind == reopened`. A single failure is enough; the threshold doesn't apply again.
  - A non-trial `record(trial=false, failed=false)` while `open` → `kind == noChange`, and the state stays `open` (data-model.md "Ignored events").
- [X] T018 [P] [US2] Add `TestStartCycleTrialRecovers` in `apps/metrik/cmd/run_test.go`:
  - A server whose health is toggled by an `atomic.Bool` counts hits.
  - Config: `MaxTries: 1, FailureThreshold: 1, Cooldown: 30 * time.Millisecond`.
  - Sequence: an unhealthy cycle (1 hit, the circuit opens); an immediate cycle (skipped, still 1 hit); sleep 40ms; a cycle while still unhealthy (trial, 2 hits, reopened); sleep 40ms; set healthy; a trial cycle (3 hits, closes); a normal cycle (4 hits).
  - Assert the hit counts after each step.

### Implementation for User Story 2

- [X] T019 [US2] In `apps/metrik/cmd/breaker.go`, complete `allow`:
  - `open` with `!now.Before(openUntil)` → set `state = trial`, return `{run: true, trial: true}`.
  - `trial` → `{state: trial}` (no run, `remaining` 0).

  Complete `record`:
  - When `trial` is false and the state is not `closed` → return `noChange`, with no mutation.
  - When `trial` is true:
    - success → `closed`, `failures = 0`, return `{kind: recovered, suspended: now.Sub(openedAt)}`.
    - failure → `open`, `openUntil = now.Add(cooldown)` (keep `openedAt`), return `{kind: reopened}`.

  Make T017 and T018 pass.

**Checkpoint**: US1 + US2 make up the MVP. A failing endpoint is suspended and comes back on its own. Run `go test -race ./...`.

---

## Phase 5: User Story 3 - See and get alerted on suspension and recovery (Priority: P2)

**Goal**: logs show every state change, per contracts/log-events.md. Failed probes keep raising their alerts, and the new entries never add one (FR-008, FR-009).

**Independent Test**: drive a check through failure → suspension → skipped cycles → failed trial → successful trial, and compare the log entries and alert count against contracts/log-events.md.

### Tests for User Story 3 ⚠️

- [X] T020 [US3] Add `TestStartCycleCircuitLogs` in `apps/metrik/cmd/run_test.go`. Use a toggleable server as in T018, `FailureThreshold: 2`, `Cooldown: 30 * time.Millisecond`, `newObserved` and `newAlertRecorder`. Drive the sequence: fail, fail, skip, (sleep) failed trial, (sleep) successful trial. Assert:
  - The entries `circuit opened` (ERROR, `circuit: "open"`, `failures: int64(2)`, `cooldown` equal to the 30ms duration), `check skipped` (WARN, `circuit: "open"`), `circuit reopened` (ERROR, `circuit: "open"`) and `circuit closed` (INFO, `circuit: "closed"`, `suspended` > 0) each appear exactly once, and every one carries `name` and `endpoint`.
  - The alert recorder counted exactly 3: the three failed probes (two before opening, plus the failed trial). It must not count the `circuit opened` / `circuit reopened` entries.

### Implementation for User Story 3

- [X] T021 [US3] In the `startCycle` goroutine in `apps/metrik/cmd/run.go`, switch on the `change` returned by `record` and log through `childlogger`, which has **no** alert hook. Never use `report`'s alert logger here.
  - `opened` → `childlogger.Error("circuit opened", zap.String("circuit", "open"), zap.Int("failures", ch.failures), zap.Duration("cooldown", chk.Cooldown))`
  - `reopened` → `childlogger.Error("circuit reopened", zap.String("circuit", "open"), zap.Duration("cooldown", chk.Cooldown))`
  - `recovered` → `childlogger.Info("circuit closed", zap.String("circuit", "closed"), zap.Duration("suspended", ch.suspended))`
  - `noChange` → nothing.

  Make T020 pass.

**Checkpoint**: every circuit transition is visible in the logs, and alert volume follows research R9.

---

## Phase 6: User Story 4 - Tune or disable the circuit breaker per check (Priority: P3)

**Goal**: global and per-check thresholds and cool-downs, a per-check opt-out with `failureThreshold: 0`, and rejection of negative values (FR-010, FR-011, FR-013).

**Independent Test**: configure a global threshold and cool-down, one check with an override and one check with `failureThreshold: 0`, and confirm that each follows its own settings. A negative value fails at load.

### Tests for User Story 4 ⚠️

- [X] T022 [P] [US4] Add a table-driven `TestValidate` in `apps/metrik/cmd/run_test.go`, covering each row of contracts/configuration.md → Validation:
  - global `FailureThreshold: -1` → error contains `failureThreshold` and `must not be negative`
  - global `Cooldown: -5 * time.Second` → error contains `cooldown` and `must not be negative`
  - check `x` with `FailureThreshold` pointing to `-1` → error contains `check "x"`, `failureThreshold` and `must not be negative`
  - check `x` with `Cooldown: -5 * time.Second` → error contains `check "x"`, `cooldown` and `must not be negative`
  - a valid config with zeros and a nil pointer → `nil`
- [X] T023 [P] [US4] Add `TestStartCycleBreakerDisabled` in `apps/metrik/cmd/run_test.go`:
  - An always-500 server, `FailureThreshold: 3` globally, and the check sets `FailureThreshold` to a pointer to `0`.
  - Run 5 cycles. Assert 5 hits, 5 alerts, and no entry whose message starts with `circuit` or equals `check skipped` (SC-006).

### Implementation for User Story 4

- [X] T024 [US4] In `apps/metrik/cmd/run.go`, add `func (cfg Config) validate() error`. It returns the first problem found, using exactly these formats:
  - `fmt.Errorf("failureThreshold must not be negative, got %d", ...)` and `fmt.Errorf("cooldown must not be negative, got %s", ...)` for the global settings.
  - `fmt.Errorf("check %q: failureThreshold must not be negative, got %d", ...)` and `fmt.Errorf("check %q: cooldown must not be negative, got %s", ...)` for each check (only when its pointer is non-nil, for the threshold).

  In `NewRunCmd`, call it right after *both* `rootConfig.Unmarshal(&runConfig)` calls (startup and `OnConfigChange`): `if err := runConfig.validate(); err != nil { logger.Fatal("invalid config", zap.Error(err)) }`. Make T022 pass.
- [X] T025 [US4] In `startCycle` in `apps/metrik/cmd/run.go`, when `*chk.FailureThreshold == 0`, bypass the breaker entirely: don't call `allow` or `record`, and leave the key out of the set passed to `keep` (any stale state is pruned). Probing and `report` stay exactly as they are today. Make T023 pass.

**Checkpoint**: all four stories work. Run `go test -race ./...`.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T026 [P] In `apps/metrik/configs/metrik.yaml`, add the global keys after `maxTries`, with comments:
  - `failureThreshold: 3 # consecutive failed cycles before a check is skipped; 0 disables the circuit breaker.`
  - `cooldown: "60s" # how long a suspended check is skipped before one trial probe.`

  Add a commented per-check example (`# failureThreshold: 0 # never skip this check`) under one of the checks. Then extend the globals assertion in `TestExampleConfigLoads` in `apps/metrik/cmd/run_test.go` to require `FailureThreshold == 3` and `Cooldown == time.Minute` (FR-015, Constitution II).
- [X] T027 [P] Add the same two global keys (`failureThreshold: 3`, `cooldown: "60s"`) to the embedded `metrik.yaml` in both `apps/metrik/configs/dev-values.yaml` and `apps/metrik/configs/prod-values.yaml`, after `maxTries`.
- [X] T028 [P] Update `README.md`:
  - Add a "Configuration" subsection under "Usage" that documents `failureThreshold` / `cooldown`, with their flag, env var (`METRIK_FAILURETHRESHOLD`, `METRIK_COOLDOWN`), defaults, per-check override and `0` = disabled. Link to `apps/metrik/configs/metrik.yaml`.
  - Remove the "Add a Circuit Breaker mecanism." line from "Roadmap".

  Keep the README's existing tone and heading style (Constitution: the README must stay accurate).
- [X] T029 From the repository root, run `mise run lint` and `mise run test`. Both must pass with `0 issues.` in every module. Fix any finding in the files touched above. Don't add `//nolint`.
- [X] T030 Walk through `specs/002-endpoint-circuit-breaker/quickstart.md` sections 2 and 3 by hand. Confirm the log sequence matches contracts/log-events.md. Note any deviation in the PR description rather than changing the contracts silently.

---

## Dependencies & Execution Order

### Phase dependencies

- **Setup (Phase 1)**: none. T004 depends on T001.
- **Foundational (Phase 2)**: after Setup. T007 depends on T005, T008 on T007, T010 on T005. It blocks every story.
- **US1 (Phase 3)**: after Foundational.
- **US2 (Phase 4)**: after US1. It refines `allow` / `record` from T014 and uses the `startCycle` wiring from T015.
- **US3 (Phase 5)**: after US2 (needs the `reopened` / `recovered` transitions).
- **US4 (Phase 6)**: after US1. T024 (validation) only needs Foundational. T025 needs the T015 wiring. It can run alongside US2 / US3 if `run.go` edits are coordinated.
- **Polish (Phase 7)**: after all stories. T026, T027 and T028 touch different files and can run in parallel. T029 and T030 come last.

### Within each story

Tests come first and must fail. Then `breaker.go` changes, then `startCycle` wiring, then run `go test -race ./...` at the checkpoint.

### Parallel opportunities

- T001, T002 and T003: three different config files.
- T005 and T006: `pkg/check/main.go` and `cmd/root.go`.
- In US1: T011 (`breaker_test.go`) alongside T012 / T013 (`run_test.go`). T012 and T013 both edit `run_test.go`, so they are only parallel if appended carefully.
- In US2: T017 (`breaker_test.go`) alongside T018 (`run_test.go`).
- In US4: T022 / T023 (tests) can be written while US2 / US3 are in progress.
- In Polish: T026, T027 and T028.

---

## Parallel Example: User Story 1

```text
# Tests together (different files):
Task: "T011 [US1] TestCircuitOpens in apps/metrik/cmd/breaker_test.go"
Task: "T012 [US1] TestStartCycleSkipsOpenCircuit in apps/metrik/cmd/run_test.go"

# Then, in order (same files):
Task: "T014 [US1] allow/record closed→open in apps/metrik/cmd/breaker.go"
Task: "T015 [US1] wire circuits into startCycle in apps/metrik/cmd/run.go"
Task: "T016 [US1] keep() pruning in apps/metrik/cmd/breaker.go"
```

## Parallel Example: User Story 2

```text
Task: "T017 [US2] TestCircuitTrial in apps/metrik/cmd/breaker_test.go"
Task: "T018 [US2] TestStartCycleTrialRecovers in apps/metrik/cmd/run_test.go"
```

---

## Implementation Strategy

### MVP first (US1 + US2, both P1)

1. Phase 1 (trim the demo configs) and Phase 2 (settings and types). Behaviour is unchanged at this point.
2. US1: failing checks get suspended. This alone doesn't recover, so don't release it without US2.
3. US2: the trial probe and recovery. **Stop and validate**: quickstart.md §2 steps 1–3.

### Incremental delivery

4. US3: transition logs (operator visibility, alert volume verified).
5. US4: validation and per-check opt-out (SC-006 regression check).
6. Polish: documented defaults, README, lint/test gate, manual quickstart.

### Commits (AGENTS.md: one logical change each, Conventional Commits, only when the maintainer asks)

Suggested split:
- `chore(metrik): drop always-failing demo checks` (Phase 1)
- `feat(metrik): add circuit breaker settings` (Phase 2)
- `feat(metrik): suspend checks after consecutive failures` (US1)
- `feat(metrik): resume suspended checks after a trial probe` (US2)
- `feat(metrik): log circuit breaker transitions` (US3)
- `feat(metrik): validate and allow disabling the circuit breaker` (US4)
- `docs: document circuit breaker settings` (Polish)

---

## Notes

- [P] tasks touch different files and have no dependency on an incomplete task.
- Every `startCycle` test must share one `newCircuits()` across its cycles. A fresh one per cycle hides the feature.
- Known flakiness risk: T018 and T020 use 30ms cool-downs with 40ms sleeps. If the race detector makes them flaky on CI, widen to 100ms / 150ms rather than adding a clock seam (research R8).
- Out of scope (plan.md → pre-existing gaps): SIGTERM handling, the `OnConfigChange` data race, and `time.Tick` per loop iteration.
