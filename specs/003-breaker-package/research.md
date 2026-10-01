# Research: Circuit Breaker as a Standalone Package

The Technical Context has no open unknowns. Each entry gives the
decision, the rationale and the alternatives considered.

## R1. Package location and name

- **Decision**: `apps/metrik/pkg/breaker` (package `breaker`), next to `pkg/check`.
- **Rationale**: This is the location the maintainer named (spec Assumptions), and it is what Constitution v1.1.0, Principle I, now requires. depguard already allows `metrik/pkg`, so no lint configuration changes.
- **Alternatives considered**: `internal/breaker`. The constitution allows `internal/` for code that is not meant to be imported, but the maintainer asked for `pkg`, and nothing outside the module could import it anyway.

## R2. Exported API: minimal rename of what exists

- **Decision**: Export the existing unexported API one-to-one, with names that read well from outside the package (spec edge case "Naming"):

  | Before (`cmd`) | After (`breaker`) |
  |---|---|
  | `circuits`, `newCircuits()` | `Breaker`, `New()` |
  | `(*circuits).allow` / `record` / `keep` | `(*Breaker).Allow` / `Record` / `Keep` |
  | `gate{run, trial, state, remaining}` | `Gate{Run, Trial, State, Remaining}` |
  | `change{kind, failures, suspended}` | `Change{Kind, Failures, Suspended}` |
  | `changeKind`: `noChange`, `opened`, `reopened`, `recovered` | `Kind`: `NoChange`, `Opened`, `Reopened`, `Recovered` |
  | `circuitState`: `stateClosed`, `stateOpen`, `stateTrial` | `State`: `Closed`, `Open`, `Trial` |
  | `circuit` (per-key state) | `circuit` (stays unexported) |

  Signatures, semantics and locking stay exactly the same. Full contract: [contracts/breaker-api.md](contracts/breaker-api.md).
- **Rationale**: This is a pure move (FR-003). Reshaping the API would add risk and review cost with no behaviour gain. Constitution I applies: no new abstractions.
- **Alternatives considered**:
  - A `Settings{Threshold, Cooldown}` parameter instead of two positional arguments to `Record`. It reads slightly better, but it's a new type for one call site. Rejected for now.
  - An interface for the breaker, so `cmd` could swap implementations. There is a single implementation, so that would be YAGNI.

## R3. Check identity (`circuitKey`)

- **Decision**: Replace `cmd.circuitKey(*check.Check)` with a method `func (c *Check) ID() string` in `pkg/check`, returning `Name + "\x00" + Endpoint`. `breaker` keeps working with opaque string keys and does not import `check`.
- **Rationale**: Constitution I says logic belongs in `pkg`, and how a check is identified is a property of the check. Keeping `breaker` free of `check` means it depends only on the stdlib (FR-002).
- **Alternatives considered**:
  - Keep `circuitKey` in `cmd`. A small leftover, but it would be one more known gap created by this very feature.
  - `breaker.Key(name, endpoint)`. That would make the breaker know what a check is made of.

## R4. Test placement and package

- **Decision**: Move `cmd/breaker_test.go` to `pkg/breaker/breaker_test.go` as an *external* test package (`package breaker_test`). `TestCircuitOpens`, `TestCircuitTrial` and `TestCircuitKeep` keep their cases. Only identifiers change: exported names and the `breaker.` prefix. Add a small `TestID` table to `pkg/check/main_test.go`.
- **Rationale**: An external test proves the exported API is enough for every transition (SC-002). The tests then build without `cmd` (US1). The `startCycle` tests in `cmd/run_test.go` stay where they are, because `startCycle` stays (FR-006). They only switch `newCircuits()` to `breaker.New()`.
- **Alternatives considered**: An internal test package (`package breaker`). Fewer edits, but it wouldn't show that the public API is sufficient.

## R5. Logging stays in `cmd`

- **Decision**: `breaker` returns `Gate` and `Change`. `startCycle` keeps turning them into the `check skipped` / `circuit opened` / `circuit reopened` / `circuit closed` entries, using `string(breaker.Open)` and so on for the `circuit` field.
- **Rationale**: FR-002 (no logging in the breaker) and FR-003 (identical log output). Today's `startCycle` code keeps working with renamed identifiers.
- **Alternatives considered**: Logging inside the breaker through a passed-in logger. That would break FR-002 and change who owns the alerting rules.

## R6. Recording the remaining gap (FR-006, SC-006)

- **Decision**: The plan's Complexity Tracking records every non-command declaration left in `cmd` after this change, as the justified deviation from Constitution I v1.1.0, deferred to a follow-up feature:
  - `Config` (its settings, and its decoding, which is wiring)
  - `Config.validate`
  - `Config.resolve`
  - `startCycle`
  - `report`
  - `createClient`
  - `notifyOnCritical`
- **Rationale**: The constitution is a MUST, so code left out of compliance needs a recorded justification. The maintainer scoped this feature with Q1: C. The constitution's Sync Impact Report lists the same items.
- **Alternatives considered**: A separate `KNOWN_GAPS.md`. The plan's Complexity Tracking is the place the constitution's Governance section names.

## R7. Feature 002 documents

- **Decision**: Update feature 002 in place (FR-008):
  - Research R2: add "Superseded by spec 003 (Constitution v1.1.0)" and the new location.
  - plan.md: the Project Structure tree, plus a Constitution Check note.
  - tasks.md: a note at the top saying file paths in `cmd/breaker*.go` now live in `pkg/breaker`.
  - quickstart.md §1: the test location.

  Contracts and the data model stay unchanged, because operator behaviour and states are identical.
- **Rationale**: Feature 002 is uncommitted and describes code that will no longer exist where it says. A short pointer is enough; rewriting history isn't needed.
