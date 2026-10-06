# Metrik

Metrik is an automated health checker for web applications. It probes HTTP health endpoints at a
regular interval, measures response time and status, understands application-level health
indicators in the response body, and raises alerts when something degrades. It ships with a demo
API to monitor, Helm charts, and a Pulumi program to deploy everything to Kubernetes.

## Features

- **Concurrent checks**: probe any number of endpoints every cycle, bounded by a configurable
  concurrency limit so Metrik never amplifies an outage.
- **Layered configuration**: a YAML file, CLI flags or `METRIK_*` environment variables, with
  per-check overrides of thresholds, timeouts and retries. File changes are applied live, without
  a restart.
- **Health indicators**: beyond the HTTP status, Metrik parses the JSON body of the response and
  reports unhealthy application state, with its reason.
- **Resilient probing**: transient failures (network errors, timeouts, 5xx) are retried with
  exponential backoff, rate limits (`429`) honor `Retry-After`, and client errors fail fast.
- **Circuit breaker**: endpoints that keep failing are suspended, then probed once after a
  cooldown, to avoid hammering a service that is already down.
- **Structured logging and alerts**: every check emits a JSON log entry with name, endpoint,
  response time and outcome; slow responses warn, failures log an error and trigger the alert
  hook.
- **Safe by default**: TLS 1.2+, no redirect following, graceful shutdown, and containers running
  as non-root with a restricted security context.
- **Deployment included**: a Helm chart per application, images built with
  [ko](https://ko.build), [Pulumi](https://www.pulumi.com) stacks for `dev` and `prod`, and a
  [Skaffold](https://skaffold.dev) loop for in-cluster development.
- **Demo API**: a small service with live, slow and flaky readiness endpoints to practice on,
  documented with an OpenAPI spec (`apps/api/api/openapi.json`, served at `/openapi`).

## Getting started

Run Metrik locally against the demo API, with live reload.

### 1. Install the tools

With [mise](https://mise.jdx.dev):

```shell
mise install
```

This installs every tool pinned in [`.mise.toml`](.mise.toml), including the Spec Kit CLI used by
the [contributing guide](docs/metrik-contributing.md). Only `air` and `go` (plus a C compiler for
the race detector) are needed for this quickstart; the rest is for deployment and documentation.

### 2. Run the API and Metrik

[`air`](https://github.com/air-verse/air) rebuilds and restarts each application when its code
changes. In a terminal, start the demo API:

```shell
(cd apps/api/ && air)
```

In another terminal, start Metrik. It reads
[`apps/metrik/configs/metrik.yaml`](apps/metrik/configs/metrik.yaml), which probes the API on
`localhost:8080`:

```shell
(cd apps/metrik/ && air)
```

You should see a JSON log entry per check every 10 seconds, with slow responses, retries, failures
and alerts (the demo API is intentionally flaky). Edit `metrik.yaml` while it runs to see the
configuration reload.

### 3. Lint and test

```shell
mise run lint
mise run test   # runs with the race detector, which needs a C compiler
```

### Next steps

To deploy to Kubernetes, see the [deployment guide](docs/metrik-deployment.md).

## Documentation

| Guide                                           | Content                                                      |
| :---------------------------------------------- | :----------------------------------------------------------- |
| [Architecture](docs/metrik-architecture.md)     | Components, check lifecycle, circuit breaker and delivery diagrams. |
| [Configuration](docs/metrik-configuration.md)   | Every option with its default, flags, environment variables, log events. |
| [Deployment](docs/metrik-deployment.md)         | Deploy to Kubernetes with Pulumi, Helm or Skaffold.          |
| [Roadmap](docs/metrik-roadmap.md)               | What is planned next.                                        |
| [Contributing](docs/metrik-contributing.md)     | Conventions and the Spec Kit workflow.                       |

The same documentation is available as a website, built with [Hugo](https://gohugo.io):

```shell
mise run docs:serve   # live preview on http://localhost:1313/metrik/
mise run docs         # static build into public/
```

## References

- [logs](https://www.dash0.com/guides/logging-in-go-with-zap#triggering-actions-on-logs-with-hooks)
- [requests](https://www.digitalocean.com/community/tutorials/how-to-make-http-requests-in-go#best-practices-for-production-http-clients)
- [deamon](https://ieftimov.com/posts/four-steps-daemonize-your-golang-programs/#shut-down-on-sigtermsigint)
- [health](https://github.com/hellofresh/health-go/blob/master/health.go#L192)
- [api](https://mcorbin.fr/posts/2022-06-13-gin-golang/)
- [otel](https://opentelemetry.io/docs/concepts/instrumentation/code-based/)
