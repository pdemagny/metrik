---
title: Roadmap
weight: 40
---

# Roadmap

What is planned next. Items are not ordered and carry no dates.

| Feature                  | Status  | Description                                                                                   |
| :----------------------- | :------ | :-------------------------------------------------------------------------------------------- |
| OpenTelemetry metrics    | Planned | Record check results (availability, response time, status code) as OpenTelemetry signals and export them to a backend such as Prometheus. |
| Notification channels    | Planned | Replace the log-based alert hook (`ALERT` entries) with real notifications, such as email or Slack messages. |

## Delivered

- Parallel checks bounded by `maxConcurrent`
- Retries with exponential backoff on transient failures
- Per-check overrides and hot configuration reload
- Circuit breaker for endpoints that keep failing
- Helm charts and Pulumi deployment, with an OpenAPI-described Demo API
- Signed multi-arch images published to ghcr.io, and deployment from a registry
- Faster CI checks: parallel lint and test jobs, only on changed modules, with Go caches

Ideas and proposals are welcome: see [Contributing](metrik-contributing.md).
