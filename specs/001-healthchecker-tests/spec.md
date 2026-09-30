# Feature Specification: Healthchecker Automated Tests

**Feature Branch**: `worktree-001-healthchecker-tests`

**Created**: 2026-09-30

**Status**: Draft

**Input**: User description: "Write tests for the metrik healthchecker (do it in a worktree)"

## User Scenarios & Testing *(mandatory)*

The "users" of this feature are the Metrik maintainer and contributors. They need an automated
safety net that proves the healthchecker reports endpoint health correctly, so they can change it
without silently breaking alerting.

### User Story 1 - Verify the outcome of a single health probe (Priority: P1)

A maintainer changes how a single endpoint is probed and wants proof that every kind of endpoint
response still gets the right verdict: healthy, application-reported failure, client error,
rate-limited, unreachable, or unreadable response.

**Why this priority**: probing one endpoint is the core of the product. If a verdict is wrong,
alerts are either missed or false, and nothing else matters.

**Independent Test**: run the probe tests alone against simulated endpoints. They pass or fail
without any live service, cluster or network access.

**Acceptance Scenarios**:

1. **Given** an endpoint that answers success with a well-formed health payload, **When** it is probed, **Then** the probe succeeds and reports the status code, the response time and the payload's status, message and reason.
2. **Given** an endpoint that answers with a client error (4xx other than 429), **When** it is probed, **Then** the probe fails after exactly one attempt, with no retry.
3. **Given** an endpoint that answers "too many requests" with a retry delay, **When** it is probed, **Then** the probe waits at least that delay before trying again.
4. **Given** an endpoint that refuses connections or does not answer within the request timeout, **When** it is probed with a retry limit of N, **Then** the probe makes exactly N attempts and then reports failure.
5. **Given** an endpoint that fails transiently and then recovers before the retry limit, **When** it is probed, **Then** the probe succeeds and reports the successful attempt's result.
6. **Given** an endpoint whose success response body is not a readable health payload, **When** it is probed, **Then** the probe treats it as a failure and retries within the limit.
7. **Given** a probe in progress, **When** the surrounding run is cancelled (shutdown), **Then** the probe stops promptly and does not keep retrying.

---

### User Story 2 - Verify per-check settings override global defaults (Priority: P2)

An operator sets global defaults (alert threshold, retry limit, request timeout) and overrides
some of them per check. The maintainer wants proof that each check uses its own value when one is
set and falls back to the global default otherwise.

**Why this priority**: operators tune checks through configuration alone, without rebuilding.
If an override is silently ignored, checks alert at the wrong time or retry the wrong number of
times.

**Independent Test**: resolve the settings for a set of checks with and without overrides, and
compare them to the expected values. No endpoint is needed.

**Acceptance Scenarios**:

1. **Given** a check with no overrides, **When** its settings are resolved, **Then** it uses the global alert threshold, retry limit and request timeout.
2. **Given** a check that overrides one setting, **When** its settings are resolved, **Then** that setting uses the check's value and the others use the global defaults.
3. **Given** the example configuration shipped with the project, **When** it is loaded, **Then** it parses without error into the expected global defaults and the expected list of checks.

---

### User Story 3 - Verify the run's classification, alerting and concurrency bound (Priority: P3)

The maintainer wants proof that a full run over several checks puts each result in the right
severity (success, slow response warning, application error, failed check), raises an alert only
for errors, and never runs more checks at the same time than the configured maximum.

**Why this priority**: this is where probe results become operator-visible signals. It depends on
Stories 1 and 2 being reliable.

**Independent Test**: run one cycle over simulated endpoints with a captured log output and an
observable alert sink. Then inspect the severities and alerts recorded, and the peak number of
checks in flight.

**Acceptance Scenarios**:

1. **Given** a healthy endpoint that answers faster than its alert threshold, **When** a run cycle executes, **Then** a "completed" entry is logged at info level and no alert is raised.
2. **Given** a healthy endpoint that answers slower than its alert threshold, **When** a run cycle executes, **Then** a "slow response" entry is logged at warning level and no alert is raised.
3. **Given** an endpoint whose payload reports a non-OK application status, **When** a run cycle executes, **Then** an "application error" entry is logged at error level with the payload's status, message and reason, and an alert is raised.
4. **Given** an endpoint whose probe fails, **When** a run cycle executes, **Then** a "check failed" entry is logged at error level and an alert is raised.
5. **Given** more checks than the concurrency maximum M, **When** a run cycle executes, **Then** no more than M checks are in flight at any moment, and every check still completes.

---

### Edge Cases

- A "too many requests" answer without a usable retry delay: it is treated as a client error, with no retry.
- A redirect answer: redirects are not followed. The answer is judged on its own merits and has no readable payload, so it is treated as a failure.
- A server error (5xx) whose body is a well-formed health payload: the constitution requires 5xx to be retried. The tests must pin down which behavior is expected (see Assumptions).
- A success answer with an empty body, or a payload that is literally empty/null: the run must not crash while classifying the result.
- A probe that cannot even be prepared (malformed endpoint address): the probe fails cleanly and the run logs a failure without crashing.
- A retry limit of 1: exactly one attempt, with no retry.
- Two consecutive probes of the same check: the results of one attempt do not leak into the next.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The test suite MUST cover every probe verdict listed in User Story 1, including the number of attempts made.
- **FR-002**: The test suite MUST verify that client errors (4xx other than rate limiting) are never retried, and that transient failures (unreachable, timed out, unreadable response) are retried up to the configured limit and no further.
- **FR-003**: The test suite MUST verify that a rate-limited response with a retry delay waits at least that delay before the next attempt.
- **FR-004**: The test suite MUST verify that cancelling a run stops in-flight probes promptly.
- **FR-005**: The test suite MUST verify how per-check overrides are resolved against global defaults for alert threshold, retry limit and request timeout.
- **FR-006**: The test suite MUST verify that the example configuration shipped with the project loads successfully.
- **FR-007**: The test suite MUST verify how a run classifies results by severity, and that alerts are raised only for error-level outcomes.
- **FR-008**: The test suite MUST verify that concurrent checks never exceed the configured maximum.
- **FR-009**: The test suite MUST cover every edge case listed above, each with an explicit expected outcome.
- **FR-010**: The test suite MUST run fully offline and self-contained: no live services, no cluster, no fixed network ports that could collide, and no reliance on wall-clock luck (no flaky timing).
- **FR-011**: The test suite MUST run with the project's standard test command in the healthchecker module, and MUST pass the project's lint gate with no new findings.
- **FR-012**: Where production code must be restructured only to make logic reachable by tests (for example, isolating settings resolution or result classification), the restructuring MUST NOT change observable behavior.
- **FR-013**: Any case where current behavior contradicts the project constitution MUST be surfaced explicitly in the test suite and reported to the maintainer. It MUST NOT be silently encoded as correct.

### Key Entities

- **Check**: one configured endpoint to watch. It has a name, an address, and optional overrides for alert threshold, retry limit and request timeout.
- **Global defaults**: run-wide settings (run interval, concurrency maximum, request timeout, alert threshold, retry limit) that apply to a check unless it overrides them.
- **Probe result**: the outcome of probing one check: status code, response time, and the health payload (status, message, optional reason).
- **Outcome severity**: how the run classifies a probe result: completed (info), slow response (warning), application error (error + alert), check failed (error + alert).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of the acceptance scenarios and edge cases in this spec have at least one corresponding automated test.
- **SC-002**: At least 90% of the statements in the probing component, and at least 80% in the settings-resolution and classification logic, are exercised by the suite.
- **SC-003**: The full healthchecker suite completes in under 30 seconds on a developer laptop.
- **SC-004**: The suite passes 20 consecutive runs with no flaky failures, including when run with the race detector enabled.
- **SC-005**: Deliberately breaking a retry rule (for example, retrying client errors) or the concurrency bound makes at least one test fail.
- **SC-006**: Every constitution divergence found while writing the tests is listed in the final report, with its test reference.

## Assumptions

- Scope is the healthchecker app only (`apps/metrik`). The companion demo API (`apps/api`) and the IaC are out of scope.
- Tests assert the **intended** behavior defined by the constitution (Principle IV: bounded retries, transient failures and 5xx retried, 4xx never retried). Where current code diverges (for example, a 5xx with a readable payload is currently accepted without retry; an empty/null payload or an unpreparable request can crash the run), fixing the defect is **out of scope**. The test for it is kept but marked as a known failure with a note, so the suite stays green and the gap stays visible. The maintainer decides later whether to fix it. This can be revisited in `/speckit-clarify`.
- Small, behavior-preserving restructurings of the run command are acceptable to make settings resolution, classification and the concurrency bound testable in isolation.
- The config-reload-on-file-change behavior and the long-running interval loop are covered only as far as a single run cycle. Hot-reload is out of scope.
- The standard test tooling of the language is sufficient. No new test dependencies are added unless the plan justifies them (constitution Principle I).
- Retry back-off delays may be shortened in tests, through configuration or injection, to meet SC-003 without changing production defaults.
