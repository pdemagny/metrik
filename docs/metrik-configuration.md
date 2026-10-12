---
title: Configuration
weight: 20
---

# Configuration

Metrik reads a `metrik.yaml` file, looked up in this order: `/config`, `./configs`, the current
directory. The file is required. A documented example lives in
[`apps/metrik/configs/metrik.yaml`](https://github.com/pdemagny/metrik/blob/main/apps/metrik/configs/metrik.yaml).

Global settings can be overridden by a flag or a `METRIK_`-prefixed environment variable (the
setting name in upper case, for example `METRIK_RUNINTERVAL`). Changes to the file are picked up
while running and apply from the next cycle; an invalid reloaded file stops the process.

## Global settings

| Setting            | Type     | Default | Flag                             | Environment variable      | Description                                                              |
| :----------------- | :------- | :------ | :------------------------------- | :------------------------ | :----------------------------------------------------------------------- |
| `debug`            | bool     | `false` | `--debug`                        | `METRIK_DEBUG`            | Print data structures and debugging messages.                            |
| `verbose`          | bool     | `true`  | `--verbose`, `-v`                | `METRIK_VERBOSE`          | Enable more verbose output.                                              |
| `runInterval`      | duration | `15s`   | `--runInterval`, `-i`            | `METRIK_RUNINTERVAL`      | Interval between two check cycles.                                       |
| `maxConcurrent`    | int      | `10`    | `--maxConcurrent`, `-c`          | `METRIK_MAXCONCURRENT`    | Maximum number of checks running at the same time.                       |
| `requestTimeout`   | duration | `5s`    | `--requestTimeout`, `-t`         | `METRIK_REQUESTTIMEOUT`   | Timeout of each HTTP request.                                            |
| `alertThreshold`   | duration | `3s`    | `--alertThreshold`, `-a`         | `METRIK_ALERTTHRESHOLD`   | Response time above which a warning is logged.                           |
| `maxTries`         | uint     | `3`     | `--maxTries`, `-m`               | `METRIK_MAXTRIES`         | Total attempts per check on transient failures. `0` is treated as `1`.   |
| `failureThreshold` | int      | `3`     | `--failureThreshold`             | `METRIK_FAILURETHRESHOLD` | Consecutive failed cycles before a check is skipped. `0` disables it.    |
| `cooldown`         | duration | `60s`   | `--cooldown`                     | `METRIK_COOLDOWN`         | How long a suspended check is skipped before one trial probe.            |

Durations use Go syntax (`500ms`, `5s`, `1m30s`). Negative `failureThreshold` and `cooldown`
values, global or per check, are rejected at startup and on reload. So is a zero or negative
`runInterval` or `requestTimeout`, global or per check, and a `runInterval` below `maxTries` ×
`requestTimeout`, checked for the global values and for each check with its overrides, so that a
probe can use all its tries within one interval.

Backoff waits and `Retry-After` delays can still make a probe outlast `runInterval`. A check whose
previous probe is still running is skipped for that cycle, so a check is never probed twice at once.

Values are resolved in this order: flag, environment variable, config file, default. The defaults
above are the flag defaults.

> [!NOTE]
> `LOG_LEVEL` (`debug`, `info`, `warn`, `error`; default `info`) is a plain environment variable
> read by the logger. It is not part of `metrik.yaml`.

## Checks

`checks` is the list of endpoints to probe.

| Field              | Type     | Required | Default                | Description                                                  |
| :----------------- | :------- | :------: | :--------------------- | :----------------------------------------------------------- |
| `name`             | string   |    ✅     |                        | Name used in logs. With `endpoint`, it identifies the check. |
| `endpoint`         | string   |    ✅     |                        | URL probed with an HTTP `GET`.                               |
| `alertThreshold`   | duration |    ❌     | global `alertThreshold`   | Per-check slow response threshold.                        |
| `maxTries`         | uint     |    ❌     | global `maxTries`         | Per-check attempts.                                       |
| `requestTimeout`   | duration |    ❌     | global `requestTimeout`  | Per-check request timeout. Must be positive when set.      |
| `failureThreshold` | int      |    ❌     | global `failureThreshold` | Per-check breaker threshold; `0` never suspends the check. |
| `cooldown`         | duration |    ❌     | global `cooldown`        | Per-check cooldown.                                         |

A per-check value of zero (or an omitted one) falls back to the global value. `failureThreshold`
and `requestTimeout` are the exceptions: only an omitted value inherits. An explicit
`failureThreshold` of `0` disables the breaker for that check, and an explicit `requestTimeout` of
`0` is rejected.

```yaml
runInterval: "15s"
maxConcurrent: 10
requestTimeout: "5s"
alertThreshold: "3s"
maxTries: 3
failureThreshold: 3
cooldown: "60s"
checks:
  - name: "api liveness"
    endpoint: "http://localhost:8080/v1/.well-known/live"
  - name: "api slow"
    endpoint: "http://localhost:8080/v1/slow"
    requestTimeout: "1s"
    failureThreshold: 0 # never skip this check
```

## Health endpoint contract

A check passes when the endpoint answers with a status below 400 (redirects are not followed, so a
3xx result must itself carry the body) and a JSON body whose `status` is `200`:

```json
{ "message": "readiness", "status": 503, "reason": "database service not ready yet" }
```

| Outcome                                | Behaviour                                                   |
| :------------------------------------- | :---------------------------------------------------------- |
| Network error, timeout, 5xx            | Retried with exponential backoff, up to `maxTries`.         |
| 429 with a `Retry-After` header        | Retried after the delay the server asks for.                |
| Other 4xx                              | Failed immediately, never retried.                          |
| Body unreadable, empty, or `status` ≠ 200 | Logged as an `application error`.                        |
| Response time above `alertThreshold`   | `slow response` warning; the check still passes.            |

## Circuit breaker

A check whose probes fail `failureThreshold` cycles in a row is skipped for `cooldown`, then probed
once. If that trial succeeds, the check is probed normally again. If it fails, the check is
skipped for another `cooldown`. Every failed probe still sends an alert; skipped cycles don't.

Set `failureThreshold: 0` on a check (or globally) to probe it every cycle, without the circuit
breaker.

## Log events

Logs are JSON on stdout.

| Message                                      | Level | Fields                                           |
| :------------------------------------------- | :---- | :----------------------------------------------- |
| `check started`                              | info  | `name`, `endpoint`                               |
| `check completed`                            | info  | `code`, `duration`                               |
| `slow response`                              | warn  | `code`, `duration`                               |
| `retrying ...`                               | info  | `error`, `duration` (backoff)                    |
| `check failed`                               | error | `code`, `duration`, `error`                      |
| `application error`                          | error | `code`, `duration`, `status`, `message`, `reason`|
| `check skipped`                              | warn  | `circuit`, `remaining` (circuit open), or `reason` (previous probe still running) |
| `circuit opened` / `reopened` / `closed`     | error / error / info | `circuit`, `failures`, `cooldown`, `suspended` |
| `ALERT`                                      | info  | `alert` (the error message that triggered it)    |

Every error-level entry about a probe is followed by an `ALERT` entry. This is the hook where a
real notification channel will plug in (see the [roadmap](metrik-roadmap.md)).
