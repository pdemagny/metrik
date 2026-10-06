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
| Registry-based delivery  | Planned | Build and publish images in continuous integration instead of loading them into kind, and deploy from the registry. |

## Delivered

- Parallel checks bounded by `maxConcurrent`
- Retries with exponential backoff on transient failures
- Per-check overrides and hot configuration reload
- Circuit breaker for endpoints that keep failing
- Helm charts and Pulumi deployment, with an OpenAPI-described demo API

Ideas and proposals are welcome: see [Contributing](metrik-contributing.md).
