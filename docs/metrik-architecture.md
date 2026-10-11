---
title: Architecture
weight: 10
---

# Architecture

The repository is a small set of independent Go modules plus the tooling to deliver them. Metrik
is the health checker; the Demo API is a separate sample target that Metrik is pointed at, and it
knows nothing about Metrik.

| Path            | Role                                                                                 |
| :-------------- | :----------------------------------------------------------------------------------- |
| `apps/metrik`   | The health checker (CLI built with cobra, logging with zap)                          |
| `apps/demo-api` | The Demo API: a standalone HTTP service with live, slow and flaky readiness endpoints |
| `pulumi/`       | Pulumi (Go) program deploying both Helm charts to a cluster                          |
| `skaffold.yaml` | In-cluster development loop                                                          |

Inside `apps/metrik`, `cmd/metrik/` only wires commands, flags and configuration. The logic lives
in `internal/config` (settings, validation, per-check resolution), `internal/runner` (scheduling
and reporting), `internal/logger`, and the reusable `pkg/check` (probe with retries) and
`pkg/breaker` (circuit breaker).

## Runtime

```mermaid
flowchart LR
    subgraph Sources["Configuration"]
        File["metrik.yaml<br/>/config, ./configs, ."]
        Flags["CLI flags"]
        Env["METRIK_* env vars"]
    end

    Viper["viper<br/>(hot reload on file change)"]
    File --> Viper
    Flags --> Viper
    Env --> Viper

    Viper --> Run["metrik run"]
    Run --> Runner["runner.Run<br/>tick every runInterval"]
    Runner --> Cycle["startCycle<br/>resolve each check"]
    Cycle --> Busy{"previous probe<br/>still running?"}
    Busy -- "yes" --> Skip
    Busy -- "no" --> Gate{"breaker.Allow"}
    Gate -- "open" --> Skip["log: check skipped"]
    Gate -- "closed / trial" --> Limiter["limiter<br/>maxConcurrent slots"]
    Limiter --> Probe["check.Run<br/>GET + exponential backoff<br/>up to maxTries"]
    Probe <--> Target[("Monitored endpoint")]
    Probe --> Report["report<br/>classify the outcome"]
    Report --> Logs["zap JSON logs<br/>(stdout)"]
    Report -- "ERROR entries" --> Alert["alert hook<br/>(ALERT log entry)"]
    Report --> Record["breaker.Record"]
    Record --> Gate
```

Every cycle, each check is resolved against the global defaults, is skipped if its previous probe
is still running, passes through the circuit breaker, then runs in its own goroutine once it gets a slot from the concurrency limiter. The
outcome is logged at the right severity; error-level entries also go through the alert hook, and
the result feeds the breaker. Configuration reloads apply from the next cycle.

## Circuit breaker

```mermaid
stateDiagram-v2
    [*] --> Closed
    Closed --> Closed: probe succeeds (failures reset)
    Closed --> Open: failureThreshold consecutive failures
    Open --> Open: cooldown running (probes skipped)
    Open --> Trial: cooldown elapsed (one probe allowed)
    Trial --> Closed: trial succeeds
    Trial --> Open: trial fails (new cooldown)
```

`failureThreshold: 0` bypasses the breaker entirely. See
[Configuration](metrik-configuration.md#circuit-breaker) for the semantics.

## Delivery

```mermaid
flowchart LR
    Src["Go source"] -->|ko build| Img["Container image<br/>kind.local/metrik:0.1.0"]
    Img -->|loaded into| Kind[("kind cluster<br/>dev or prod")]
    Stack["Pulumi stack<br/>Pulumi.dev.yaml / Pulumi.prod.yaml"] -->|pulumi up| Release["Helm releases"]
    Chart["Helm charts<br/>apps/*/deploy/chart"] --> Release
    Values["configs/*-values.yaml"] --> Release
    Release --> Kind

    subgraph Kind
        direction TB
        NS["Namespace<br/>(pod security: restricted)"]
        CM["ConfigMap metrik.yaml"]
        Dep["Deployment metrik"]
        Api["Deployment demo-api"]
        CM -->|mounted at /config| Dep
        Dep -->|probes| Api
        NS --- CM
        NS --- Dep
        NS --- Api
    end
```

Skaffold shortcuts this path during development: it builds with ko and deploys the same charts to
the `kind-dev` cluster on every change.
