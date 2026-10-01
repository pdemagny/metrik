---

description: "Task list for moving the circuit breaker into pkg/breaker"
---

# Tasks: Circuit Breaker as a Standalone Package

**Input**: Design documents from `/specs/003-breaker-package/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/breaker-api.md, quickstart.md

**Tests**: Included. The constitution requires tests for logic in `pkg/`. Here that means moving the existing breaker tests and adding `TestID`. This is a refactor: there are no new behaviours to drive test-first, and the existing suite is the safety net. It must stay green after every phase.

**Organization**: Tasks are grouped by user story (US1–US3 from spec.md).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: The user story the task belongs to

## Path Conventions

- Go module `apps/metrik`. Run Go commands from `apps/metrik`, and `mise run lint` / `mise run test` from the repository root.
- Rename table: [research.md → R2](research.md#r2-exported-api-minimal-rename-of-what-exists). API: [contracts/breaker-api.md](contracts/breaker-api.md).
- Pure move: no change to logic, locking, comments' meaning, log messages, log fields or config.

---

## Phase 1: Setup (baseline)

**Purpose**: Prove the starting point is green, and capture what operators see today so that US2 can be compared against it.

- [X] T001 From the repository root, run `mise run lint` and `mise run test`. Both must report `0 issues.` and pass before any change. If not, stop and report: the refactor must start from green.
- [X] T002 Capture the operator-visible baseline in the gitignored `apps/metrik/tmp/003-baseline/`:
  1. Build the binary: `(cd apps/metrik && go build -o tmp/003-baseline/metrik .)`.
  2. Write `tmp/003-baseline/metrik.yaml` with the scratch config from specs/002-endpoint-circuit-breaker/quickstart.md §2 step 1.
  3. Run it for 30s with the API stopped, starting the API (`apps/api`) at about 17s.
  4. Save `run.log`, plus `summary.txt` with one line per JSON entry: `level msg sorted-field-keys`. Exclude `ts` and the field values.

  Delete the binary afterwards. Keep the two text files for T010.

---

## Phase 2: Foundational

**Purpose**: The check identity that both the breaker's caller and its tests rely on.

**⚠️ CRITICAL**: US2 depends on this phase.

- [X] T003 In `apps/metrik/pkg/check/main.go`, add this method below the `Check` type:

  ```go
  // ID identifies a check by name and endpoint, so a changed endpoint is a new check.
  func (c *Check) ID() string { return c.Name + "\x00" + c.Endpoint }
  ```

  In `apps/metrik/pkg/check/main_test.go`, add a table-driven `TestID` that asserts:
  - two checks with the same name and endpoint get equal IDs
  - a different endpoint gives a different ID
  - a different name gives a different ID
  - `{Name: "a\x00b", Endpoint: "c"}` and `{Name: "a", Endpoint: "b\x00c"}` are documented as colliding. Write a comment saying why that's acceptable: names and URLs never contain NUL. Do *not* "fix" it.

  Run `go test ./pkg/check/`.

**Checkpoint**: `go test -race ./...` passes, and nothing else has changed yet.

---

## Phase 3: User Story 1 - Find and test the circuit breaker on its own (Priority: P1) 🎯 MVP

**Goal**: `pkg/breaker` exists, is stdlib-only, and carries all of the breaker's tests. During this phase `cmd` still compiles against its old copy, so the story can be verified on its own.

**Independent Test**: specs/003-breaker-package/quickstart.md §1. The `pkg/breaker` tests pass on their own, and `go list -deps` shows no `metrik/cmd`, `metrik/pkg/check`, zap, viper or cobra.

- [X] T004 [US1] Create `apps/metrik/pkg/breaker/breaker.go` (package `breaker`) from the current `apps/metrik/cmd/breaker.go`, applying the R2 rename table exactly:
  - `circuitState` → `State` (`stateClosed`/`stateOpen`/`stateTrial` → `Closed`/`Open`/`Trial`, with the string values `"closed"`/`"open"`/`"trial"` unchanged)
  - `circuits` → `Breaker`, `newCircuits` → `New`
  - methods `allow`/`record`/`keep` → `Allow`/`Record`/`Keep`; `get` stays unexported
  - `gate{run, trial, state, remaining}` → `Gate{Run, Trial, State, Remaining}`
  - `change{kind, failures, suspended}` → `Change{Kind, Failures, Suspended}`
  - `changeKind` → `Kind` (`noChange`/`opened`/`reopened`/`recovered` → `NoChange`/`Opened`/`Reopened`/`Recovered`)
  - `circuit` stays unexported, with unexported fields

  Other rules:
  - Do **not** copy `circuitKey`, and do **not** import `metrik/pkg/check`. Imports are `sync` and `time` only.
  - Add a package doc comment: "Package breaker suspends checks that keep failing and resumes them after a trial probe. See specs/002-endpoint-circuit-breaker/data-model.md for the state machine."
  - Rewrite the doc comments to use the new names, including those required by contracts/breaker-api.md: `Allow` moves Open→Trial once; `Record`'s `trial` must be the `Gate.Trial` from `Allow`; a threshold of 0 is bypassed by callers; `Breaker` is safe for concurrent use.
  - Keep every statement of logic identical.
- [X] T005 [US1] Create `apps/metrik/pkg/breaker/breaker_test.go` as **external** package `breaker_test`, from the current `apps/metrik/cmd/breaker_test.go`:
  - Rename the tests `TestCircuitOpens` → `TestOpens`, `TestCircuitTrial` → `TestTrial` and `TestCircuitKeep` → `TestKeep`, and keep every case.
  - Replace the identifiers with their `breaker.`-qualified exported forms: `newCircuits()` → `breaker.New()`, `gate{...}` → `breaker.Gate{...}`, `change{kind: opened, ...}` → `breaker.Change{Kind: breaker.Opened, ...}`, `stateOpen` → `breaker.Open`, `cs.allow` → `cs.Allow`, and so on.
  - Keep the `t0`/`at` helpers and the `testThreshold`/`testCooldown` constants local to the test file.

  Depends on T004.
- [X] T006 [US1] Verify US1 from `apps/metrik`:
  - `go test -race -count=1 ./pkg/breaker/` passes.
  - `go list -deps ./pkg/breaker | grep -E '^metrik/|zap|viper|cobra'` prints only `metrik/pkg/breaker`.
  - `go vet ./...` passes, with `cmd` still on its own copy.

**Checkpoint**: the breaker stands alone and is fully tested.

---

## Phase 4: User Story 2 - Operators see no difference (Priority: P1)

**Goal**: `cmd` uses `pkg/breaker` and `check.ID()`. The old copy is deleted, and behaviour is identical to the T002 baseline.

**Independent Test**: specs/003-breaker-package/quickstart.md §2–§4, plus a replay compared with the T002 baseline.

- [X] T007 [US2] Switch `apps/metrik/cmd/run.go` to the package. Change identifiers only; log messages, fields and levels stay the same.
  - Import `metrik/pkg/breaker`. In `NewRunCmd`, `circuits := newCircuits()` becomes `circuits := breaker.New()`.
  - In the `startCycle` signature, `circuits *circuits` becomes `circuits *breaker.Breaker`.
  - **Rename the local `breaker` bool in `startCycle` to `enabled`**, because it would shadow the package. Update its comment ("A zero threshold disables the breaker…") and both of its uses (`if breaker {` and `if !breaker {`).
  - `circuitKey(chk)` → `chk.ID()`, in both the `keys` loop and the probe loop.
  - `gate{run: true}` → `breaker.Gate{Run: true}`; `g.run`/`g.trial`/`g.state`/`g.remaining` → `g.Run`/`g.Trial`/`g.State`/`g.Remaining`.
  - `circuits.allow`/`record`/`keep` → `circuits.Allow`/`Record`/`Keep`.
  - In the transition switch: `ch.kind` → `ch.Kind`; the cases become `breaker.Opened`, `breaker.Reopened`, `breaker.Recovered` and `breaker.NoChange`; `ch.failures`/`ch.suspended` → `ch.Failures`/`ch.Suspended`; `string(stateOpen)`/`string(stateClosed)` → `string(breaker.Open)`/`string(breaker.Closed)`.

  Update the `startCycle` doc comment if it names the old type.
- [X] T008 [US2] In `apps/metrik/cmd/run_test.go`, replace every `newCircuits()` with `breaker.New()` and add the `metrik/pkg/breaker` import. Change nothing else. Every test keeps its name, cases and assertions (SC-001).
- [X] T009 [US2] Delete `apps/metrik/cmd/breaker.go` and `apps/metrik/cmd/breaker_test.go` (`git rm` is not needed: they were never committed; plain `rm`). Then run `gofmt -l .`, `go vet ./...` and `go test -race -count=1 ./...` from `apps/metrik`. All must be clean. Depends on T007 and T008.
- [X] T010 [US2] Replay the T002 scenario with a freshly built binary into `apps/metrik/tmp/003-after/`, producing `summary.txt` the same way. Then `diff apps/metrik/tmp/003-baseline/summary.txt apps/metrik/tmp/003-after/summary.txt`.
  - Expected: identical. Timing-driven differences in the *number* of `check skipped` lines (±1 at the edges) are acceptable. Any other difference in messages, levels, field keys or `ALERT` count is a regression to fix in `cmd/run.go`.
  - Delete both `tmp/003-*` directories afterwards.

**Checkpoint**: quickstart §2 (`ls cmd/breaker*.go` and the grep both print nothing), §3 and §4 (`git diff --stat -- apps/metrik/go.mod apps/metrik/go.sum` is empty) all pass.

---

## Phase 5: User Story 3 - The "command-line layer is CLI only" rule is written down (Priority: P2)

**Goal**: the rule is in the constitution, and every remaining exception is listed.

**Independent Test**: specs/003-breaker-package/quickstart.md §5.

- [X] T011 [P] [US3] Read `.specify/memory/constitution.md`. Confirm the version is `1.1.0`, and that Principle I contains both new bullets: `cmd` MUST contain only CLI code, and a library package with `cmd` as its only caller is allowed and MUST NOT import `cmd`. This was done by `/speckit-constitution`, so verify only. Edit it only if it's missing, and report that.
- [X] T012 [US3] From the repository root, run `grep -nE '^func|^type|^var|^\t[A-Za-z]+ +(struct|=)' apps/metrik/cmd/*.go | grep -v _test`.
  - For each declaration that is not CLI code (`Execute`, `NewRunCmd`, `NewVersionCmd`, `rootConfig`, `rootCmd`, `debug`, `verbose`, `version` are CLI), confirm it appears in the Complexity Tracking table of `specs/003-breaker-package/plan.md`.
  - Add any missing item to the table, and to the constitution's Sync Impact Report follow-up list in `.specify/memory/constitution.md`.
  - Remove the "apps/metrik/cmd/breaker.go moves to apps/metrik/pkg/breaker under spec 003" follow-up line from that report, because it's now done.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: keep feature 002's documents true to the code (FR-008), then run the final gate.

- [X] T013 [P] In `specs/002-endpoint-circuit-breaker/research.md`, append to the R2 entry: `- **Superseded**: by spec 003 and Constitution v1.1.0 ("cmd holds CLI code only"). The breaker now lives in apps/metrik/pkg/breaker (exported API: specs/003-breaker-package/contracts/breaker-api.md), tested in pkg/breaker/breaker_test.go.` Leave the original text as history.
- [X] T014 [P] In `specs/002-endpoint-circuit-breaker/plan.md`:
  - In the Summary, replace "a small hand-rolled state map in `apps/metrik/cmd`" with "a small hand-rolled state map in `apps/metrik/pkg/breaker` (moved there by spec 003)".
  - In the Source Code tree, replace the `breaker.go` and `breaker_test.go` lines under `cmd/` with a `pkg/breaker/` entry holding both files, annotated `# moved here by spec 003`.
  - In the Constitution Check row for principle I, append "(the location was revised by spec 003 under Constitution v1.1.0)".
- [X] T015 [P] In `specs/002-endpoint-circuit-breaker/tasks.md`, insert this line right after the `**Organization**` paragraph: `**Note (spec 003)**: the breaker described in T010–T025 now lives in apps/metrik/pkg/breaker with exported names (see specs/003-breaker-package/research.md R2); cmd/breaker*.go no longer exists.`
- [X] T016 [P] In `specs/002-endpoint-circuit-breaker/quickstart.md` §1, change "the new breaker tests in `apps/metrik/cmd`" to "the breaker tests in `apps/metrik/pkg/breaker` and the `startCycle` tests in `apps/metrik/cmd`".
- [X] T017 Final gate from the repository root:
  - `mise run lint` → `0 issues.` in every module. Fix any findings without `//nolint`.
  - `mise run test` passes.
  - specs/003-breaker-package/quickstart.md §1–§5 all match their expected output.
  - `git status --short` shows only the files this tasks list names, plus the untracked `specs/003-breaker-package/`. Report anything else without touching it.

---

## Dependencies & Execution Order

- **Setup (T001–T002)**: first. T002 needs T001 to be green.
- **Foundational (T003)**: after Setup. It blocks US2 (which needs `check.ID`). It doesn't block US1.
- **US1 (T004–T006)**: after Setup. T005 depends on T004, and T006 on both. It can run alongside T003.
- **US2 (T007–T010)**: after T003 and US1. T007 and T008 edit different files but must land together before T009 (the package won't compile otherwise). T010 is last.
- **US3 (T011–T012)**: T011 can run at any time. T012 needs US2 done, so that `cmd` is in its final state.
- **Polish (T013–T017)**: T013–T016 are independent docs and can run at any time after the plan. T017 is last.

### Parallel opportunities

- T003 (pkg/check) alongside T004–T005 (pkg/breaker).
- T011 alongside anything.
- T013, T014, T015 and T016: four different documents.

## Parallel Example: User Story 1

```text
Task: "T003 check.ID + TestID in apps/metrik/pkg/check/"     # foundational, different package
Task: "T004 [US1] create apps/metrik/pkg/breaker/breaker.go"
# then
Task: "T005 [US1] create apps/metrik/pkg/breaker/breaker_test.go"
```

## Parallel Example: Polish

```text
Task: "T013 002 research.md R2 superseded note"
Task: "T014 002 plan.md summary/tree/check note"
Task: "T015 002 tasks.md note"
Task: "T016 002 quickstart.md §1 location"
```

---

## Implementation Strategy

### MVP (US1 + US2, both P1)

1. T001–T003: green baseline, captured behaviour, `check.ID`.
2. US1: the package exists and is tested on its own, while `cmd` still uses its old copy.
3. US2: switch `cmd` over, delete the copy, diff against the baseline. **Stop and validate.**

### Then

4. US3: verify the rule and the completeness of the gap list.
5. Polish: feature 002 docs, final gate.

### Commits (only when the maintainer asks; one logical change each)

Suggested, on top of feature 002's commits:
- `docs(constitution): amend to v1.1.0 (cmd holds CLI code only)`. Remove the Sync Impact Report first, as the Governance section requires.
- `feat(check): identify checks by name and endpoint` (T003)
- `refactor(metrik): move the circuit breaker to pkg/breaker` (T004–T012)
- `docs(specs): point feature 002 docs at pkg/breaker` (T013–T016, together with the spec 003 documents)

If feature 002 isn't committed yet, the maintainer may prefer to fold T004–T009 into 002's feature commit so that `cmd/breaker.go` never appears in history.

---

## Notes

- The local variable `breaker` in `startCycle` **must** be renamed (T007). It shadows the package import and won't compile otherwise.
- `pkg/breaker` must never import `metrik/pkg/check` (FR-002). The caller passes `chk.ID()`.
- Out of scope (plan.md → Complexity Tracking): moving `startCycle`, `report`, `resolve`/`validate`, `createClient` and `notifyOnCritical` out of `cmd`.
