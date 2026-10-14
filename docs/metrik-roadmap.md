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
| Faster CI checks         | Planned | Investigate and fix the slow lint and test jobs: they take 6 seconds locally but about 5 minutes in GitHub Actions. |

## Delivered

- Parallel checks bounded by `maxConcurrent`
- Retries with exponential backoff on transient failures
- Per-check overrides and hot configuration reload
- Circuit breaker for endpoints that keep failing
- Helm charts and Pulumi deployment, with an OpenAPI-described Demo API
- Signed multi-arch images published to ghcr.io, and deployment from a registry

Ideas and proposals are welcome: see [Contributing](metrik-contributing.md).
