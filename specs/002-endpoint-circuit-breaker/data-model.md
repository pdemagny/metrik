# Data Model: Endpoint Circuit Breaker

All state is in memory. Nothing is persisted (spec Assumptions).

## Circuit breaker settings

Part of the configuration. They follow the same global → per-check rules as `alertThreshold`, `maxTries` and `requestTimeout`.

| Setting | Scope | Type | Default | Meaning |
|---|---|---|---|---|
| `failureThreshold` | global | integer ≥ 0 | `3` | Consecutive failed cycles before suspension. `0` disables the breaker. |
| `failureThreshold` | per check | integer ≥ 0, optional | inherit | Unset inherits the global value. `0` disables it for this check. `N` overrides. |
| `cooldown` | global | duration ≥ 0 | `60s` | How long a suspended check is skipped before its trial probe. |
| `cooldown` | per check | duration ≥ 0, optional | inherit | `0` or unset inherits the global value. |

**Validation** (FR-013): negative values are rejected at load and at reload. The error names the setting and, when it comes from a check, the check's name.

**Resolution**: `Config.resolve` copies the check and fills in the inherited values, as it already does for the other settings. A resolved check always has a concrete threshold and cool-down.

## Check circuit state

One entry per active check, keyed by **name + endpoint**.

| Field | Meaning |
|---|---|
| `state` | `closed`, `open` or `trial` |
| `failures` | consecutive failed cycles while closed |
| `openUntil` | end of the current cool-down (only meaningful when `open`) |
| `openedAt` | when the check was first suspended in this outage, used for the `suspended` duration on recovery |

### State transitions

```text
                failure (failures < threshold)
                ┌──────────┐
                ▼          │
 ┌────────┐  failure, failures == threshold  ┌────────┐
 │ closed │ ───────────────────────────────▶ │  open  │ ◀─┐
 └────────┘                                  └────────┘   │
   ▲  │ success → failures = 0                  │         │ trial failed
   │  └──────────┐                              │ cycle after openUntil
   │             ▼                              ▼         │
   │          (closed)                      ┌────────┐    │
   └──────────────── trial succeeded ────── │ trial  │ ───┘
                                            └────────┘
```

| From | Event | To | Log entry (level) | Alert |
|---|---|---|---|---|
| closed | failed cycle, `failures` < threshold | closed | — (the probe's own `check failed` / `application error`) | yes, from the probe |
| closed | failed cycle, `failures` reaches threshold | open | `circuit opened` (ERROR) | from the probe only |
| closed | successful cycle | closed (`failures`=0) | — | no |
| open | cycle before `openUntil` | open | `check skipped` (WARN) | no |
| open | first cycle at/after `openUntil` | trial | — (a probe starts) | — |
| trial | another cycle while the trial is in flight | trial | `check skipped` (WARN) | no |
| trial | trial succeeded | closed (`failures`=0) | `circuit closed` (INFO) | no |
| trial | trial failed | open (new `openUntil`) | `circuit reopened` (ERROR) | from the probe only |
| any | threshold resolves to 0 | — (no entry kept) | — | as today |

**Ignored events**: a non-trial probe result that arrives while the state is `open` or `trial`. It started before the circuit opened, and counting it would count that cycle twice.

**Lifecycle**: an entry is created the first time a check's cycle is recorded. It is dropped at the start of any cycle whose configuration no longer contains that name + endpoint (removed check or changed endpoint). Everything is lost on restart.
