# Metrik Constitution

## Core Principles

### I. Simplicity First

- Solutions MUST use the Go standard library before adding a third-party dependency; each new
  module in a `go.mod` MUST be justified in the change description.
- Code MUST NOT introduce abstractions, interfaces, or configuration knobs for requirements that
  do not exist yet (YAGNI).
- Each app (`apps/demo-api`, `apps/metrik`) and the IaC (`pulumi/`) remain independent Go modules;
  shared code across them MUST NOT be introduced without an amendment-level justification.
- Each app's `cmd/<app>/` directory (golang-standards/project-layout) MUST contain only `main` and
  the commands themselves: command definitions, flag and configuration wiring, and run start-up and
  shutdown. All other logic (probing, scheduling, reporting, alerting, circuit breaking, settings
  resolution and validation, HTTP routing) MUST live in `internal/` when it is specific to the app,
  or in `pkg/` when it is reusable.
- Moving logic out of `cmd/<app>/` into a library package is not a speculative abstraction. Such a
  package is allowed even when `cmd/<app>/` is its only caller. Library packages MUST NOT import
  anything under `cmd/`.

Rationale: Metrik is a small health-check system; every added layer costs more in review and
maintenance than it returns at this scale. The `cmd`/library split is not an extra layer but the
project's fixed layout: it keeps logic testable without the CLI and easy to find.

### II. Configuration Over Code

- Every operational setting (endpoints, intervals, timeouts, thresholds, retries, concurrency,
  log verbosity) MUST be configurable through the YAML config file or environment variables,
  never hard-coded.
- Per-check settings MAY override global defaults; defaults MUST be documented in the example
  config (`apps/metrik/configs/metrik.yaml`).
- Environment-specific values (dev/prod) MUST live in `configs/*-values.yaml` or the matching
  Pulumi stack file, not in application code or chart templates.

Rationale: operators must be able to retarget and tune checks without rebuilding images.

### III. Observable by Default

- Every health check MUST emit a structured log entry containing at least timestamp, check name,
  endpoint, response time, status code, and outcome.
- Severity MUST be consistent: threshold breaches log at WARN; non-2xx status, failed health
  indicators, and exhausted retries log at ERROR and trigger the alert mechanism.
- Logging MUST go through the shared structured logger (`zap`); `fmt.Print*` to stdout/stderr
  MUST NOT be used for runtime diagnostics.

Rationale: a monitoring tool that cannot itself be observed provides false confidence.

### IV. Resilient, Bounded Checking

- Every outbound request MUST have a timeout and propagate a `context.Context`.
- Retries MUST be bounded (`maxTries`) and applied only to transient failures (network errors,
  timeouts, 5xx); client errors (4xx) MUST NOT be retried.
- Parallel checks MUST be bounded by `maxConcurrent`; unbounded goroutine fan-out is forbidden.
- Processes MUST shut down gracefully on SIGINT/SIGTERM, letting in-flight checks finish or be
  cancelled via context.

Rationale: a health checker must never amplify an outage or leak resources while observing one.

### V. Declarative, Reproducible Delivery

- Each deployable app MUST ship a Helm chart under `deploy/chart` and be deployed through Pulumi
  stacks (`dev`, `prod`); manual `kubectl apply`/`helm install` MUST NOT be the deployment path.
- Images MUST be built with `ko`; containers MUST run as non-root with a restrictive
  `securityContext` and explicit resource requests/limits.
- Tool versions MUST be pinned in `.mise.toml`, the single source of tool installation. CI
  MUST install its tools through mise from `.mise.toml` as well, never through separate setup
  actions.

Rationale: dev and prod must be reproducible from the repository alone.

## Technology & Deployment Constraints

- Language: Go (version pinned in `.mise.toml`) for apps and IaC.
- Repository layout: each app follows golang-standards/project-layout: `cmd/<app>/` (entry point
  and commands), `internal/` (app-specific packages), `pkg/` (reusable packages), `api/` (API
  contracts), `configs/` (configuration), and `deploy/` (Helm chart).
- CLI: `cobra`; logging: `zap`; HTTP API contract: OpenAPI spec at `apps/demo-api/api/openapi.json`,
  which MUST be updated in the same change as any API route or payload change.
- Packaging: `ko` images, Helm charts; orchestration: Kubernetes; IaC: Pulumi (Go).
- Environments: the `dev` stack runs on a local kind cluster with images loaded onto its nodes;
  the `prod` stack runs on a remote Kubernetes cluster and pulls images from a registry
  (ghcr.io). The registry, image and pull credentials MUST be set only in the Pulumi stack
  configuration, never in chart templates or application code.
- Image publishing: the GitHub Actions workflow `.github/workflows/images.yaml` lints, tests,
  builds with `ko`, and publishes multi-arch, keyless-signed images with SBOM attestations to
  ghcr.io. Tags MUST be immutable: `sha-<short>` from `main`, `X.Y.Z` promoted from that build on
  release, and never `latest`.
- Inner dev loop: `air` for local live reload, `skaffold dev` for in-cluster iteration.
- The README MUST stay accurate for setup, configuration, and run instructions; a change that
  alters any of these MUST update the README in the same change.

## Development Workflow & Quality Gates

- `golangci-lint` (config `.golangci.yaml`) MUST pass with no new findings before a change is
  considered done.
- New or changed logic in `pkg/` and `internal/` MUST be covered by Go tests (table-driven where
  inputs vary); `go test ./...` MUST pass in every touched module.
- Commits MUST follow Conventional Commits (`type(scope): short message`), contain one logical
  change each, and be authored solely by the maintainer (no co-author trailers).
- Agents MUST NOT commit or push without an explicit request; work stops for maintainer review.
- Only files related to the current task are staged; unrelated working-tree changes are left
  untouched and reported.

## Governance

- This constitution supersedes conflicting practices; `AGENTS.md` provides runtime guidance for
  agents and MUST NOT contradict it.
- Amendments are made via `/speckit-constitution`, reviewed by the maintainer, and committed as
  `docs(constitution): ...` with the Sync Impact Report removed before commit.
- Versioning follows SemVer: MAJOR for removing or redefining a principle, MINOR for adding a
  principle/section or materially expanding guidance, PATCH for clarifications and wording.
- Every plan (`/speckit-plan`) MUST include a Constitution Check; any violation MUST be recorded
  with justification in the plan's Complexity Tracking, or the design changed.

**Version**: 1.3.0 | **Ratified**: 2026-09-30 | **Last Amended**: 2026-10-14
