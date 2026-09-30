# Data Model: Healthchecker Automated Tests

No persistent data. These are the in-memory entities the tests build and assert on. The Go types already exist; none are added or changed.

## Check (`pkg/check.Check`)

| Field | Type | Source | Rule under test |
|-------|------|--------|-----------------|
| `Name` | string | YAML `name` | Carried into log fields `name` |
| `Endpoint` | string | YAML `endpoint` | Carried into log fields `endpoint`; empty or malformed → probe failure (KD-3 if unpreparable) |
| `AlertThreshold` | duration | YAML `alertThreshold`, 0 = inherit | See Resolution |
| `MaxTries` | uint | YAML `maxTries`, 0 = inherit | See Resolution; `1` → exactly one attempt |
| `RequestTimeout` | duration | YAML `requestTimeout`, 0 = inherit | See Resolution; drives client timeout |

## Global defaults (`cmd.Config`)

`RunInterval`, `MaxConcurrent`, `RequestTimeout`, `AlertThreshold`, `MaxTries`, `Checks []*Check`, plus `Debug`/`Verbose`.

**Resolution rule** (`Config.resolve`), for each of AlertThreshold, MaxTries and RequestTimeout: `effective = check value if non-zero, else global value`. Test matrix: no override / one override each / all overridden.

## Probe result (`pkg/check.Result` + `APIResponse`)

| Field | Meaning |
|-------|---------|
| `Code` | HTTP status of the **last** attempt that received a response |
| `Duration` | response time of that attempt |
| `Body.Status` / `Body.Message` / `Body.Reason` | decoded health payload; `Body` is nil if the payload was literally `null` (KD-2) |

## Probe verdict table (`Check.Run`): assertions for US1

| Endpoint behavior | Attempts (maxTries=N) | `err` | Result |
|-------------------|-----------------------|-------|--------|
| 200 + valid payload | 1 | nil | Code 200, Body decoded |
| 4xx ≠ 429 | 1 | `"client error"` | Code set |
| 429 + `Retry-After: s` | ≤ N, gap ≥ s between attempts | retry-after error if still 429 at N | Code 429 |
| 429, no `Retry-After` | 1 | `"client error"` | Code 429 |
| connection refused | N | dial error (`ECONNREFUSED`) | zero Code |
| exceeds client timeout | N | timeout error | zero Code |
| fails k < N times then 200 | k+1 | nil | from successful attempt |
| 2xx/3xx, body not JSON or empty | N | decode error | Code set |
| ctx cancelled during backoff | stops < backoff delay | `context.Canceled` | n/a |
| 5xx + valid payload | **intended**: N, error. **Actual**: 1, nil | KD-1 (skipped test) | |

## Outcome severity (`report`): assertions for US3

States are evaluated in this order; the first match wins:

| # | Condition | Log message | Level | Alert hook called |
|---|-----------|-------------|-------|-------------------|
| 1 | `err != nil` | `check failed` (+code, duration, error) | ERROR | yes |
| 2 | `Body.Status != 200` | `application error` (+status, message, reason) | ERROR | yes |
| 3 | `Duration > alertThreshold` | `slow response` | WARN | no |
| 4 | always after 3 (or instead of it) | `check completed` | INFO | no |

Note: when state 3 matches, `check completed` is still logged afterwards. Tests assert both entries.

## Cycle (`startCycle`)

Invariant: the number of checks with an in-flight probe is ≤ `MaxConcurrent` at every instant, and every configured check is probed exactly once per cycle. The returned WaitGroup completes when all probes and reports are done.
