# Feature Specification: Circuit Breaker as a Standalone Package

**Feature Branch**: `003-breaker-package`

**Created**: 2026-10-01

**Status**: Draft

**Input**: User description: "update the circuit breaker feature to be a breaker package in pkg instead of living in cmd, which is supposed to be only CLI code."

## User Scenarios & Testing *(mandatory)*

This feature has two kinds of users:
- **Maintainers and contributors**, who read, change and test Metrik's code. This refactor is for them.
- **Operators**, who run Metrik. They must not notice any change.

Feature 002 (endpoint circuit breaker) put the circuit breaker's state tracking inside the
command-line layer. The maintainer's rule is that the command-line layer only handles commands:
parsing flags, loading configuration and starting the run. Health-checking logic belongs in the
library layer, next to the existing probe logic.

### User Story 1 - Find and test the circuit breaker on its own (Priority: P1)

A maintainer wants to change how the circuit breaker decides to suspend or resume a check. They
find it as a self-contained component in the library layer, change it, and run its tests without
building or touching the command-line layer.

**Why this priority**: this is the request itself. Putting the breaker where the project's layout
says it belongs makes it easier to find, review and reuse.

**Independent Test**: run the breaker's own tests on their own. They cover every state transition
from feature 002's data model (closed, open, trial, re-open, recovery, ignored stale results,
pruning) and pass without compiling any command-line code.

**Acceptance Scenarios**:

1. **Given** the codebase after this change, **When** a maintainer looks for the circuit breaker, **Then** it is a dedicated breaker component in the library layer, and the command-line layer has no circuit state of its own.
2. **Given** the breaker component, **When** its tests are run on their own, **Then** they cover every transition listed in feature 002's data model and pass.
3. **Given** the breaker component, **When** its dependencies are inspected, **Then** it depends neither on the command-line layer, nor on configuration loading, nor on logging.

---

### User Story 2 - Operators see no difference (Priority: P1)

An operator upgrades to the refactored version. The configuration, flags, environment variables,
log entries and alerts stay exactly as feature 002 defined them.

**Why this priority**: this is a pure refactor. Any behaviour change would be a regression.

**Independent Test**: repeat feature 002's quickstart (outage, suspension, failed trial, recovery,
breaker disabled, invalid value on reload) and compare the log entries, levels, fields and alerts
with the contracts of feature 002.

**Acceptance Scenarios**:

1. **Given** an existing configuration, **When** the refactored Metrik runs it, **Then** it loads with the same keys, defaults, flags and environment variables.
2. **Given** feature 002's outage-and-recovery scenario, **When** it is replayed, **Then** the same sequence of log entries is produced, with the same messages, levels and fields, and the same number of alerts.
3. **Given** the full automated test suite of feature 002, **When** it is run after the change, **Then** every test passes. The only changes allowed in tests are to where they live and to the names they refer to.

---

### User Story 3 - The "command-line layer is CLI only" rule is written down (Priority: P2)

A contributor planning a future feature reads the project's governing principles and finds the
rule that the command-line layer holds only command handling, with health-checking logic in the
library layer. They also find the list of existing code that doesn't follow the rule yet, so it
can be fixed later instead of copied.

**Why this priority**: the rule is what prevents the next feature from repeating feature 002's
placement. The code move (Stories 1–2) delivers value without it.

**Independent Test**: read the governing principles and confirm the rule is stated and testable.
Then confirm that every non-command declaration still in the command-line layer appears in the
known-gap list.

**Acceptance Scenarios**:

1. **Given** the project's governing principles, **When** a contributor reads them, **Then** they state that the command-line layer holds only command definitions, flag and configuration wiring, and run start-up and shutdown.
2. **Given** the command-line layer after this change, **When** its remaining non-command logic is listed (cycle scheduling, result reporting, check settings resolution and validation, HTTP client setup, alert hook), **Then** each item appears in a documented known-gap list that is left for a follow-up feature.

---

### Edge Cases

- **Concurrent use**: the breaker is used from many probes at once. It must stay safe under concurrent use, with no data races detected by the race-enabled test run.
- **Identity of a check**: today a check is identified by name and endpoint. The breaker must keep that identity rule, so a configuration reload keeps, resets and prunes state exactly as in feature 002.
- **Disabled breaker**: a check with a failure threshold of 0 must still bypass the breaker entirely.
- **Stale results**: trial and non-trial results that arrive after pruning or after the circuit opened must still be ignored as in feature 002.
- **Naming**: the breaker's public names should read naturally from outside the component, without repeating the component's name.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The circuit breaker's state tracking and decisions MUST live in a dedicated breaker component in the library layer, not in the command-line layer.
- **FR-002**: The breaker component MUST NOT depend on the command-line layer, on configuration loading, or on logging. It takes the check identity, the outcome, the settings and the current time as inputs, and returns decisions and transitions. Callers do the logging.
- **FR-003**: Operator-visible behaviour MUST stay identical to feature 002: configuration keys, defaults, flags, environment variables, validation messages, log entries (message, level, fields) and alerts.
- **FR-004**: The breaker's tests MUST move with it and run on their own. The command-line layer keeps only the tests of what it still contains.
- **FR-005**: The breaker MUST remain safe for concurrent use and keep the identity, reload, pruning, disabled-breaker and stale-result rules of feature 002.
- **FR-006**: This change MUST move only the circuit breaker. The rest of the health-checking logic in the command-line layer (cycle scheduling, result reporting, check settings resolution and validation, HTTP client setup, alert hook) stays where it is. It MUST be recorded as a known gap for a follow-up feature.
- **FR-009**: The project's governing principles MUST be amended to state that the command-line layer holds only command definitions, flag and configuration wiring, and run start-up and shutdown, and that health-checking logic belongs in the library layer. The amendment follows the project's amendment procedure and reconciles the "one caller" simplicity rule that led feature 002 to keep the breaker in the command-line layer.
- **FR-007**: The change MUST NOT add any third-party dependency, and the full lint and test gate MUST pass.
- **FR-008**: Feature 002's design documents (plan, research decision on where the breaker lives, tasks) MUST be updated to reflect the new location and the reason for it, so they don't contradict the code.

### Key Entities

- **Breaker component**: the library-layer unit that holds every check's circuit state and decides whether a check is probed in a cycle. It has the same states, transitions and identity rules as feature 002's "Check circuit state".
- **Command-line layer**: command definitions, flag and configuration wiring, and run start-up. Within the agreed scope, it uses library components but holds no health-checking logic itself.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of feature 002's automated tests pass after the change, edited only for location and names.
- **SC-002**: The breaker's tests can be run on their own and cover 100% of the transitions listed in feature 002's data model.
- **SC-003**: Replaying feature 002's quickstart produces the same sequence of log entries, with the same messages, levels and fields, and the same number of alerts as before the change.
- **SC-004**: The command-line layer contains no circuit-state data or transition logic: zero breaker state types or methods remain there.
- **SC-005**: No new third-party dependency is added, and the lint gate reports 0 issues in every module.
- **SC-006**: The governing principles state the CLI-only rule. Every non-command declaration left in the command-line layer is listed as a known gap, and none is missing from the list.

## Assumptions

- Feature 002 is implemented but not committed. This refactor builds on that working tree and can be committed separately from it, or folded into it, at the maintainer's choice.
- The maintainer named the location: a `breaker` package under `apps/metrik/pkg`, next to the existing `check` package. This is recorded as a stakeholder requirement, not a design choice left to planning.
- Feature 002's research decision R2 kept the breaker in the command-line layer because it had a single caller (Constitution I, YAGNI). The maintainer's rule that the command-line layer holds only CLI code overrides that decision. The rule is recorded as a constitution amendment (FR-009), made with `/speckit-constitution`, as a MINOR version bump since it adds guidance.
- Moving the remaining non-CLI logic out of the command-line layer is out of scope. It becomes a follow-up feature, seeded from the known-gap list.
- The way a check is identified (name and endpoint) remains the caller's choice. The breaker works with an opaque identity string.
- No behaviour, configuration or documentation visible to operators changes, so the README and the example configs are not affected.
