# Implementation Plan: Registry Image Deployment

**Branch**: `005-registry-image-deploy` | **Date**: 2026-10-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/005-registry-image-deploy/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

Add a GitHub Actions workflow, `.github/workflows/images.yaml`, with four jobs:
- **`check`** (pull requests and `main`): lint and test.
- **`build`** (pull requests): a multi-arch ko build that pushes nothing, with a read-only token.
- **`publish`** (`main`): push `sha-<7 hex>` images of metrik and demo-api to private ghcr.io,
  and sign them keyless with cosign, along with SPDX SBOM attestations on the index and on each
  platform image.
- **`release`** (`vX.Y.Z` tags): verify those exact `main` images and add the `X.Y.Z` tag with crane, with no
  rebuild.

On the deploy side, the Pulumi program gains:
- an optional per-release `image` (`repository`, `tag`, optional `digest`), passed as chart value
  overrides;
- three stack-level pull-credential modes: create a dockerconfigjson secret from encrypted config,
  reference an existing secret, or none;
- validation before any change;
- a 120 s release timeout.

`dev` keeps loading images into kind and its files don't change. `prod` moves to a remote cluster
pinned to `0.1.0` from ghcr.io. The charts don't change. The docs gain a remote walkthrough and
per-registry notes.

## Technical Context

**Language/Version**: Go 1.27.1 (pinned in `.mise.toml`) for the apps and the Pulumi program. The
workflow is GitHub Actions YAML.

**Primary Dependencies**: no Go module added. Pulumi uses the existing
`pulumi-kubernetes/sdk/v4` (`helm/v3.Release` `Values`/`Timeout`, `core/v1.Secret`). New CLI
tools, pinned in `.mise.toml`: `cosign`, `crane`. CI uses `jdx/mise-action` and
`actions/checkout`, both pinned by SHA.

**Storage**: N/A. Images live in ghcr.io, and config lives in Pulumi stack files and state.

**Testing**: `mise run test`, which adds table-driven tests for `pulumi/internal/stack`
(validation, image reference, dockerconfigjson). `mise run lint`. The workflow is validated by
the runs in [quickstart.md](quickstart.md).

**Target Platform**: GitHub-hosted `ubuntu-latest` runners. Images are linux/amd64 and
linux/arm64. Clusters are kind `dev` and any remote Kubernetes for `prod`.

**Project Type**: monorepo of independent Go modules (two apps and the Pulumi program), a Hugo
docs site, and now CI.

**Performance Goals**: images available within 20 min of a merge or tag (SC-002). An unpullable
release fails within 2 min (FR-017).

**Constraints**:
- No `latest` or other moving tags.
- No long-lived push token.
- No changes to charts, `dev` files or `skaffold.yaml`.
- Registry choice lives only in stack config.

**Scale/Scope**:
- 1 new workflow and 1 new Go package with tests.
- Edits to `pulumi/main.go`, `release.go`, `Pulumi.prod.yaml`, two prod values files,
  `.mise.toml`, the deployment docs, the README (if needed) and the constitution.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle / rule | Assessment | Status |
| :--- | :--- | :--- |
| I. Simplicity First | No Go module added. One small package, `pulumi/internal/stack`, holds pure config logic so it can be tested. There is no new abstraction beyond the three spec-required modes. The timeout is a constant, not a setting. The modules stay independent. | Pass |
| II. Configuration Over Code | Registry, image and credentials are stack config. The prod image lines move out of the values file into the stack file, an environment file the constitution allows. Nothing is hard-coded in charts or app code. | Pass |
| III. Observable by Default | The apps are untouched. | Pass |
| IV. Resilient, Bounded Checking | The checker is untouched. | Pass |
| V. Declarative, Reproducible Delivery | Images are still built with ko. Deploys still go through Pulumi, and the pull secret is created by Pulumi, not `kubectl`. `cosign` and `crane` are pinned in `.mise.toml`, and CI installs through mise. Security context and resources are unchanged. | Pass |
| Technology constraints, "kind clusters `dev` and `prod`" | Becomes stale when `prod` goes remote. MINOR amendment to 1.3.0 through `/speckit-constitution` in this feature (research R15). | Pass, with amendment |
| README accurate | Deployment docs rewritten (US4). README checked and updated where it describes deployment. | Pass |
| Tests and lint | New logic in `internal/` has table tests. `mise run lint` and `mise run test` gate CI itself. | Pass |
| Commit and agent rules | Unchanged. No commit without an explicit request. | Pass |

No violation needs justification. **Post-design re-check**: the design artifacts (stack config
contract, image contract, data model) add no dependency, chart change or manual deploy step. The
gate still passes.

## Project Structure

### Documentation (this feature)

```text
specs/005-registry-image-deploy/
├── plan.md                      # This file
├── research.md                  # R1–R15: workflow, tools, tags, signing, promotion, stack config
├── data-model.md                # Stack config entities, validation rules, image lifecycle
├── quickstart.md                # Validation per user story
├── contracts/
│   ├── stack-config.md          # Operator-facing Pulumi config: dev, prod, modes, guarantees
│   └── published-images.md      # Triggers → tags, references, verification, failure messages
├── checklists/
│   └── requirements.md
└── tasks.md                     # Created by /speckit-tasks
```

### Source Code (repository root)

```text
.github/workflows/
└── images.yaml                  # NEW: check → build (PR) / publish (main); release (tags)

.mise.toml                       # + cosign, crane

pulumi/
├── main.go                      # load + validate config, create pull secrets, pass values/timeout
├── Pulumi.prod.yaml             # remote context, ghcr credentials, image per release
├── Pulumi.dev.yaml              # unchanged
├── internal/stack/              # NEW
│   ├── config.go                # Release, Image, Registry, Validate, ImageTag, DockerConfigJSON, Values
│   └── config_test.go           # table-driven tests for V1–V6 and helpers
└── pkg/helm/release/release.go  # + WithValues, WithTimeout options

apps/metrik/configs/prod-values.yaml    # drop image.repository/tag (stack provides)
apps/demo-api/configs/prod-values.yaml  # same
apps/*/configs/dev-values.yaml          # unchanged
apps/*/deploy/chart/                    # unchanged
skaffold.yaml                           # unchanged

docs/metrik-deployment.md        # local (dev/kind) + remote (prod/ghcr) + registry notes + verify
README.md                        # deployment mentions, if affected
.specify/memory/constitution.md  # 1.3.0 amendment
```

**Structure Decision**: all deploy logic stays in the existing `pulumi/` module. The new
`internal/stack` package keeps config parsing and validation testable without a Pulumi engine,
following the constitution's rule that logic lives outside the entry point. CI is one workflow
file, and there is no reusable-workflow layering.

## Complexity Tracking

No constitution violations to justify.
