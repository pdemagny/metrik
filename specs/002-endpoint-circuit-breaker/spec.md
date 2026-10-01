# Feature Specification: Endpoint Circuit Breaker

**Feature Branch**: `002-endpoint-circuit-breaker`

**Created**: 2026-10-01

**Status**: Draft

**Input**: User description: "Add a circuit breaker so failing endpoints are temporarily skipped"

## User Scenarios & Testing *(mandatory)*

The "users" of this feature are the operators who run Metrik and receive its alerts. Today a
down endpoint is probed every cycle with the full retry budget, and every cycle raises a new
alert. During an outage this wastes probe capacity, keeps hammering a service that is already
failing, and buries the first alert under identical repeats.

### User Story 1 - Stop probing an endpoint that keeps failing (Priority: P1)

An endpoint goes down. After it has failed several cycles in a row, Metrik stops probing it for
a cool-down period instead of retrying it every cycle, while the other checks keep running
normally.

**Why this priority**: this is the feature itself. It stops Metrik from adding load to a failing
service and frees probe slots for healthy checks.

**Independent Test**: point one check at an endpoint that always fails and another at one that
always succeeds. Once the failure threshold is reached, the failing endpoint receives no requests
during the cool-down, and the healthy check runs every cycle.

**Acceptance Scenarios**:

1. **Given** a check whose last cycles ended in failure, one fewer than the failure threshold, **When** the next cycle also fails, **Then** the check is suspended and is not probed in the following cycles until the cool-down has elapsed.
2. **Given** a suspended check, **When** a cycle runs during the cool-down, **Then** the endpoint receives no request and the cycle records that the check was skipped.
3. **Given** a check that fails some cycles but succeeds before reaching the threshold, **When** it succeeds, **Then** its consecutive-failure count resets and it is never suspended.
4. **Given** one suspended check and other healthy checks, **When** cycles run, **Then** the healthy checks are probed on their normal schedule and are not affected.

---

### User Story 2 - Resume an endpoint automatically once it recovers (Priority: P1)

After the cool-down, Metrik sends a single trial probe to the suspended endpoint. If it succeeds,
the check returns to normal. If it fails, the check is suspended for another cool-down.

**Why this priority**: without automatic recovery, a suspended endpoint would never be monitored
again. Stories 1 and 2 together are the minimum viable feature.

**Independent Test**: suspend a check against a controllable endpoint, make the endpoint healthy,
wait for the cool-down, and confirm that one trial probe is made and normal probing resumes.
Repeat with the endpoint still failing and confirm it is suspended again.

**Acceptance Scenarios**:

1. **Given** a suspended check whose cool-down has elapsed, **When** the next cycle runs, **Then** exactly one trial probe is sent to the endpoint.
2. **Given** a trial probe, **When** it succeeds, **Then** the check returns to normal probing and its consecutive-failure count is zero.
3. **Given** a trial probe, **When** it fails, **Then** the check is suspended again for a full cool-down, without needing the full threshold of failures again.

---

### User Story 3 - See and get alerted on suspension and recovery (Priority: P2)

An operator watching the logs sees when a check is suspended, when it is skipped, and when it
recovers. Every failed probe still raises an alert, as it does today. Because suspended cycles send
no probe, they raise no alert, so alerts slow from one per cycle to one per cool-down while the
endpoint stays down.

**Why this priority**: suspension must not hide an outage. Operators need the state changes, but
Stories 1–2 already deliver the reduction in requests and alerts.

**Independent Test**: drive a check through failure → suspension → skipped cycles → recovery and
inspect the emitted log entries and alerts.

**Acceptance Scenarios**:

1. **Given** a check that reaches the failure threshold, **When** it is suspended, **Then** an error-level entry names the check, its endpoint and how long it is suspended for. The failed probe that triggered the suspension raises its usual alert, and the suspension entry raises no extra one.
2. **Given** a suspended check, **When** a cycle skips it, **Then** a warning-level entry records the skip, and no alert is raised.
3. **Given** a suspended check, **When** a trial probe succeeds, **Then** an info-level entry records that the check recovered, including how long it was suspended.
4. **Given** a trial probe that fails, **When** the check is suspended again, **Then** the failed probe raises its usual alert and an error-level entry records the new suspension, with no extra alert.

---

### User Story 4 - Tune or disable the circuit breaker per check (Priority: P3)

An operator sets the failure threshold and cool-down globally, overrides them for specific
checks, or turns the circuit breaker off for a check that must be probed every cycle.

**Why this priority**: the defaults suit most checks. Tuning and opting out are refinements.

**Independent Test**: configure a global threshold and cool-down, one check with an override and
one check with the circuit breaker disabled, and confirm each follows its own settings.

**Acceptance Scenarios**:

1. **Given** a global failure threshold and cool-down, **When** a check sets neither, **Then** it uses the global values.
2. **Given** a check that overrides the threshold or cool-down, **When** it fails, **Then** its own values apply, and other checks keep the global values.
3. **Given** a check with the circuit breaker disabled, **When** it fails any number of cycles in a row, **Then** it is probed every cycle and alerts as it does today.

---

### Edge Cases

- **Configuration reload**: when the configuration is reloaded, checks that still exist with the same endpoint keep their state. A check whose endpoint changed, or a newly added check, starts in the normal state. Removed checks drop their state.
- **Shutdown during a trial probe**: the probe is cancelled like any other probe and the check stays suspended. Its state is not persisted (see Assumptions).
- **Threshold of 1**: a single failed cycle suspends the check.
- **Cool-down shorter than the run interval**: the trial probe happens on the first cycle after the cool-down ends, so the effective cool-down is rounded up to the next cycle.
- **Slow but successful responses**: they count as successes for the circuit breaker. They still raise the existing slow-response warning.
- **Rate-limited responses**: a probe that ends in failure after honouring the endpoint's retry delay counts as one failed cycle, like any other failure.
- **Overlapping cycles**: if a check's previous probe is still running when the next cycle starts, its circuit state is updated once per completed probe and never double-counted.
- **Invalid settings**: a negative threshold or a negative cool-down is rejected when the configuration is loaded, with an error naming the check and the setting.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST keep, for each configured check, a count of consecutive cycles that ended in failure. A cycle ends in failure when its probe outcome is logged at error level today: the request failed after its retries, the endpoint returned an error status, or the health payload reported unhealthy or was empty.
- **FR-002**: The system MUST reset that count to zero whenever a cycle for that check succeeds.
- **FR-003**: When the count reaches the check's failure threshold, the system MUST suspend the check for its cool-down period.
- **FR-004**: While a check is suspended, the system MUST NOT send any request to its endpoint, and MUST NOT use a concurrency slot for it.
- **FR-005**: After the cool-down has elapsed, on the next cycle, the system MUST send exactly one trial probe for the check. The trial probe uses the check's normal retry and timeout settings.
- **FR-006**: If the trial probe succeeds, the system MUST return the check to normal probing with a failure count of zero. If it fails, the system MUST suspend the check again for a full cool-down.
- **FR-007**: Suspending, skipping or resuming one check MUST NOT change when or how any other check is probed.
- **FR-008**: Every failed probe, including a failed trial probe, MUST keep raising its alert as it does today. Suspension and re-suspension MUST be logged at error level without raising an additional alert. Skipped cycles MUST NOT raise alerts.
- **FR-009**: The system MUST log each skipped cycle at warning level, and recovery after a successful trial at info level. Every entry MUST include the check name, the endpoint and the circuit state, plus the remaining suspension (on skip) or total suspended time (on recovery).
- **FR-010**: Operators MUST be able to set the failure threshold and cool-down globally and override them per check, through the configuration file or environment variables, like the other check settings.
- **FR-011**: Operators MUST be able to disable the circuit breaker for a single check. A disabled check MUST behave exactly as checks do today.
- **FR-012**: The circuit breaker MUST be on by default for every check. It uses the default threshold and cool-down (see Assumptions) unless they are configured. Setting a check's threshold to 0 disables it for that check.
- **FR-013**: The system MUST reject a negative failure threshold or cool-down at configuration load, with an error that names the check and the setting.
- **FR-014**: On configuration reload, the system MUST keep the circuit state of checks whose name and endpoint are unchanged, and reset it for checks whose endpoint changed or that were added.
- **FR-015**: The default threshold, cool-down and how to disable the breaker MUST be documented in the example configuration and the README.

### Key Entities

- **Check circuit state**: the live state of one check's circuit breaker. It holds the state (normal, suspended or trial), the count of consecutive failed cycles, when the current suspension ends, when the current outage began, and whether the outage has already been alerted. It is identified by the check's name and endpoint, and lives in memory only.
- **Circuit breaker settings**: the failure threshold (consecutive failed cycles before suspension, where 0 disables the breaker) and the cool-down (how long a check stays suspended before a trial probe). Global defaults can be overridden per check, following the same rules as the existing alert threshold, retry and timeout settings.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: During a sustained outage of one endpoint, the number of requests Metrik sends to it drops by at least 80% compared with today, with the default settings.
- **SC-002**: During a sustained outage of one endpoint, with the default settings, operators receive at most one alert per cool-down for that endpoint after the first suspension, instead of one per cycle. That is at least 80% fewer alerts.
- **SC-003**: Once a failing endpoint recovers, Metrik resumes normal probing of it within one cool-down plus one run interval.
- **SC-004**: While one or more endpoints are suspended, every other check keeps running on its normal schedule. No healthy check misses a cycle because of a suspended one.
- **SC-005**: An operator can tell from the logs alone which checks are suspended, since when, and when each one recovered, without reading the code.
- **SC-006**: Checks with the circuit breaker disabled produce the same probes, logs and alerts as before the feature.

## Assumptions

- Circuit state is kept in memory and resets when Metrik restarts. A restart briefly re-probes endpoints that were suspended. That is acceptable at Metrik's scale and avoids adding storage.
- Default failure threshold: 3 consecutive failed cycles. Default cool-down: 60 seconds, which is 6 cycles at the default 10-second run interval.
- A "failed cycle" counts the whole cycle, after its retries, as one failure. Individual retry attempts within a cycle are not counted separately.
- Setting the failure threshold to 0 disables the circuit breaker for that check. This follows the existing convention of using zero-valued settings for "unset/default".
- There is one Metrik instance per environment. Sharing circuit state between several instances is out of scope.
- The demo checks that are meant to fail every cycle ("liveness not found" and "liveness connection refused") are removed from the example, dev and prod configurations. Automated tests already cover their behaviour (client errors are not retried, and refused connections are retried then alert). The randomly failing demo checks ("liveness slow timeout" and "readiness flaky") stay, and now also show the breaker opening and recovering.
- Exposing circuit state outside the logs (an API endpoint, metrics or a dashboard) is out of scope for this feature.
- Alerts go through Metrik's existing alerting path. No new notification channel is added.
