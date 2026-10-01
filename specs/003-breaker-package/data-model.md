# Data Model: Circuit Breaker as a Standalone Package

No data changes. The breaker's states, fields, transitions, identity, pruning and ignored-event
rules are exactly those in
[feature 002's data model](../002-endpoint-circuit-breaker/data-model.md). This document only
maps them to their new exported names.

## Entities

| Entity (002) | Where it lives now | Exported as |
|---|---|---|
| Check circuit state | `pkg/breaker` | unexported `circuit`; its state is exposed as `breaker.State` |
| States `closed` / `open` / `trial` | `pkg/breaker` | `breaker.Closed` / `breaker.Open` / `breaker.Trial` (string values unchanged, because they appear in logs) |
| Per-cycle decision | `pkg/breaker` | `breaker.Gate{Run, Trial, State, Remaining}` |
| Transition result | `pkg/breaker` | `breaker.Change{Kind, Failures, Suspended}`, where `Kind` is one of `NoChange`, `Opened`, `Reopened`, `Recovered` |
| Check identity (name + endpoint) | `pkg/check` | `(*check.Check).ID()`, an opaque string to the breaker |
| Circuit breaker settings | unchanged: `check.Check` / `cmd.Config` | `FailureThreshold`, `Cooldown` |

## Dependency rule

```text
cmd ──▶ pkg/breaker      (stdlib only: sync, time)
cmd ──▶ pkg/check ──▶ backoff, zap   (unchanged)
pkg/breaker ─╳─▶ cmd, pkg/check, zap, viper, cobra
```
