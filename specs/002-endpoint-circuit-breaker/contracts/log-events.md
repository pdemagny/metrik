# Contract: Log Events

Metrik's logs are its operator interface (Constitution III). The breaker adds four structured JSON entries. Every one of them carries the existing per-check fields `name` and `endpoint`, plus a `circuit` field giving the state after the event.

Existing entries (`check started`, `check completed`, `slow response`, `check failed`, `application error`, `ALERT`) are unchanged. Failed probes keep raising `ALERT`, including failed trial probes.

| `msg` | Level | When | Extra fields | Raises `ALERT` |
|---|---|---|---|---|
| `circuit opened` | error | a closed check reaches its failure threshold | `circuit: "open"`, `failures` (int), `cooldown` (duration) | no |
| `check skipped` | warn | a cycle skips a suspended check, or one whose trial is in flight | `circuit: "open"` or `"trial"`, `remaining` (duration, `0s` while a trial is in flight) | no |
| `circuit reopened` | error | a trial probe failed | `circuit: "open"`, `cooldown` (duration) | no (the failed probe's own entry already alerted) |
| `circuit closed` | info | a trial probe succeeded | `circuit: "closed"`, `suspended` (duration since `circuit opened`) | no |

## Example sequence (threshold 3, cool-down 60s, run interval 10s)

```text
t=0s   check failed      error  → ALERT
t=10s  check failed      error  → ALERT
t=20s  check failed      error  → ALERT
t=20s  circuit opened    error  failures=3 cooldown=60s
t=30s  check skipped     warn   circuit=open remaining=50s
 …     (one per cycle)
t=80s  check started            (trial)
t=80s  check completed   info
t=80s  circuit closed    info   suspended=60s
```

`check started` is not logged for skipped cycles, because no probe runs.
