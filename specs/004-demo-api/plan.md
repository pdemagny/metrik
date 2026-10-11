# Implementation Plan: App-Agnostic Demo API

**Branch**: `004-demo-api` | **Date**: 2026-10-11 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/004-demo-api/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

The sample HTTP service in `apps/api` titles itself "Metrik API" and is otherwise called just
"api". It becomes the **Demo API**: its contract no longer mentions Metrik, and every identifier
(folder, Go module, command, chart, image, release, namespace) becomes `demo-api`. Everything
that refers to those identifiers moves with it: Skaffold, the Pulumi stacks, Metrik's shipped
values, the lint and task configuration, the README, the docs and the constitution.

The approach is a rename with `git mv` plus text edits; no behaviour changes. One existing
discrepancy is fixed on the way (the contract file names a schema differently from what the
service serves), and one test is added so the file and the served contract stay equal.

## Technical Context

**Language/Version**: Go 1.27.1 (pinned in `.mise.toml`)

**Primary Dependencies**: unchanged (`gin`, `fizz`, `tonic`, `zap` in the Demo API); no module added

**Storage**: N/A

**Testing**: `go test -race` through `mise run test`; `golangci-lint` through `mise run lint`; `helm lint` / `helm template` for the chart

**Target Platform**: Linux containers on Kubernetes (kind clusters `dev` and `prod`); local run with `air`

**Project Type**: monorepo of independent Go modules (two apps and the Pulumi program) plus a Hugo documentation site

**Performance Goals**: N/A (no runtime change)

**Constraints**: served routes, responses, port and versions unchanged; no file under the Demo API mentions Metrik; `specs/001` to `specs/003` untouched

**Scale/Scope**: 33 files: 20 moved (17 of them also edited), 13 edited in place; one new test; one constitution amendment

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle / rule | Assessment | Status |
| :--- | :--- | :--- |
| I. Simplicity First | No dependency, abstraction or setting added. The apps stay independent modules; the Demo API module imports nothing from this repository. `cmd/demo-api/` keeps the `cmd/<app>/` layout and still holds only `main`. | Pass |
| II. Configuration Over Code | Names and addresses change only in values files, stack files and `skaffold.yaml`. Nothing is hard-coded in application code or chart templates. | Pass |
| III. Observable by Default | Logging untouched. | Pass |
| IV. Resilient, Bounded Checking | Metrik's checking code untouched; only endpoint addresses in shipped values change. | Pass |
| V. Declarative, Reproducible Delivery | The Demo API still ships a chart under `deploy/chart`, is still built with `ko` and deployed by the Pulumi stacks. Security context and resources untouched. | Pass |
| OpenAPI spec updated with API changes | Title and descriptions change in code and in the file in the same commit. The existing schema-name mismatch is corrected and guarded by a test (research R7). | Pass |
| README stays accurate | README and docs updated in this feature (US3). | Pass |
| Tests and lint | One test added for the contract comparison; `mise run lint` and `mise run test` must pass in all three modules. | Pass |
| Literal paths in the constitution | The constitution names `apps/api` and `apps/api/api/openapi.json`. The rename makes that text stale, so it is amended to v1.2.3 (PATCH) through `/speckit-constitution` as part of this feature (FR-010, research R8). No principle is redefined. | Pass, with amendment |

No violation needs justification. **Post-design re-check**: the Phase 1 artifacts add nothing
beyond the above; the gate still passes.

## Project Structure

### Documentation (this feature)

```text
specs/004-demo-api/
├── plan.md              # This file
├── research.md          # Naming decisions, cleanup of old releases, contract drift finding
├── data-model.md        # Old-to-new name map and where each name is referenced
├── quickstart.md        # Validation steps per user story
├── contracts/
│   └── demo-api-identity.md   # Identity fields of the contract and its guarantees
├── checklists/
│   └── requirements.md
└── tasks.md             # Created by /speckit-tasks
```

### Source Code (repository root)

```text
apps/
├── demo-api/                        # was apps/api (git mv)
│   ├── .air.toml                    # build path -> ./cmd/demo-api
│   ├── go.mod                       # module demo-api
│   ├── api/openapi.json             # title, descriptions, schema name fix
│   ├── cmd/demo-api/main.go         # was cmd/api; import demo-api/internal/router
│   ├── configs/{dev,prod}-values.yaml   # image kind.local/demo-api, /ko-app/demo-api
│   ├── deploy/chart/demo-api/       # was deploy/chart/api; Chart name and helper names
│   └── internal/router/
│       ├── router.go                # title, description, group description
│       └── router_test.go           # + contract file equals served contract, no "metrik"
└── metrik/
    └── configs/{dev,prod}-values.yaml   # endpoints -> <stack>-demo-api.<stack>-demo-api

pulumi/Pulumi.{dev,prod}.yaml        # release, namespace, chart and values paths
skaffold.yaml                        # artifact, paths, image variables, release, namespace
.mise.toml                           # lint and test loops
.golangci.yaml                       # import allow-list: demo-api/internal
.gitignore                           # apps/demo-api/tmp/
README.md                            # "Demo API", paths, commands
docs/metrik-architecture.md          # table row, diagram label
docs/metrik-deployment.md            # build command, pod filter, address, upgrade note
docs/metrik-roadmap.md               # capitalisation
.specify/memory/constitution.md      # v1.2.3 via /speckit-constitution
```

**Structure Decision**: the repository layout is kept as is (independent modules under `apps/`,
each following golang-standards/project-layout). The only structural change is three folder
renames: `apps/api` to `apps/demo-api`, its `cmd/api` to `cmd/demo-api`, and its
`deploy/chart/api` to `deploy/chart/demo-api`. The inner `api/` folder that holds the contract
keeps its name.

## Delivery Order

The three user stories map to separable steps. Each leaves the repository in a working state.

1. **US1, contract** (in `apps/api`, before any move): change the title, description and group
   description in `router.go`; update `openapi.json` to match, including the schema name; add the
   test. After this step no file of the service mentions Metrik.
2. **US2, identifiers**: `git mv` the three folders, then edit every file listed in
   [data-model.md](data-model.md). This step cannot be split further without leaving references
   pointing at names that no longer exist, so it lands as one change. Verify with `mise run lint`,
   `mise run test`, `helm lint` and the search in quickstart section 3.
3. **Constitution**: amend to v1.2.3 through `/speckit-constitution`, as its own commit, right
   after step 2 so the governing document never names a folder that no longer exists.
4. **US3, documentation**: README and docs wording, paths and commands; the upgrade note in
   `docs/metrik-deployment.md`. Paths and commands in the docs that would break after step 2 are
   corrected in step 2's change.

Details that matter during step 2:

- The Go module rename changes one import (`main.go`) and one line of `.golangci.yaml`.
- The chart helpers are renamed consistently in `_helpers.tpl` and in every template that
  includes them (`deployment`, `service`, `serviceaccount`, `hpa`, `httproute`, `NOTES.txt`,
  `tests/test-connection`).
- Skaffold's image variables use `_` where the artifact name has `-` (research R4).
- Mentions of "api" that do not name the service are left alone (research R9).

## Complexity Tracking

No constitution violation to justify.
