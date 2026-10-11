---

description: "Task list for turning the Metrik API into an app-agnostic Demo API"
---

# Tasks: App-Agnostic Demo API

**Input**: Design documents from `/specs/004-demo-api/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/demo-api-identity.md, quickstart.md

**Tests**: One test is included (T004). The plan adds it to keep the contract file equal to the served contract (FR-004, research R7). Nothing else is test-driven: this is a rename, and the existing suites are the safety net. They must stay green after every phase.

**Organization**: Tasks are grouped by user story (US1–US3 from spec.md).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: The user story the task belongs to

## Path Conventions

- Phases 1 to 3 work in `apps/api` (old name). From Phase 4 on, the same app is `apps/demo-api`.
- Run `mise run lint` and `mise run test` from the repository root. If `go` reports a toolchain version mismatch, prefix Go commands with `mise exec --`.
- Old-to-new names: [data-model.md](data-model.md). Contract values: [contracts/demo-api-identity.md](contracts/demo-api-identity.md).
- Do not rename uses of "api" that do not name the service: see [research.md, R9](research.md#r9-things-named-api-that-are-left-alone-fr-012).
- Do not touch `specs/001-*`, `specs/002-*` or `specs/003-*`.
- Use `git mv` for moves so history follows. Do not commit unless the maintainer asks.

---

## Phase 1: Setup (baseline)

**Purpose**: Prove the starting point is green.

- [X] T001 From the repository root, run `mise run lint` and `mise run test`. Both must report `0 issues.` and pass in `apps/api`, `apps/metrik` and `pulumi` before any change. If not, stop and report.

---

## Phase 2: Foundational

No blocking prerequisite: the three stories share no new code. Go straight to Phase 3.

---

## Phase 3: User Story 1 - The service presents itself as a Demo API (Priority: P1) 🎯 MVP

**Goal**: The contract is titled "Demo API", the service's own files never mention Metrik, and the contract file equals what the service serves.

**Independent Test**: [quickstart.md §1](quickstart.md) with `apps/api` in place of `apps/demo-api`: `grep -rniI 'metrik' apps/api` prints nothing, and the key-sorted diff between `/openapi` and `apps/api/api/openapi.json` is empty.

- [X] T002 [US1] In `apps/api/internal/router/router.go`, change the three identity strings and nothing else:
  - `openapi.Info.Title`: `"Demo API"`
  - `openapi.Info.Description`: `"Demo API with live, slow and flaky endpoints, meant as a target for health checks."`
  - `f.Group("/v1", "v1 api", "v1 metrik api.")` becomes `f.Group("/v1", "v1 api", "Demo API v1.")` (the group name `v1 api` stays)
- [X] T003 [US1] In `apps/api/api/openapi.json`, keeping the file's existing formatting:
  - `info.title`: `"Demo API"`
  - `info.description`: `"Demo API with live, slow and flaky endpoints, meant as a target for health checks."`
  - `tags[0].description`: `"Demo API v1."`
  - rename the schema `APIResponse` to `RouterAPIResponse` in `components.schemas` and in all four `$ref`s (`#/components/schemas/RouterAPIResponse`), which is the name the service already serves
- [X] T004 [US1] In `apps/api/internal/router/router_test.go`, extend the existing `openapi` subtest of `TestNewRouter` (keep its status assertion) so it also:
  - ~~fails if the served body contains "metrik" in any letter case~~ dropped during implementation: the assertion would put the word "metrik" in the service's own test file, which FR-002 forbids. The comparison below with the contract file, plus the search in T005, covers it.
  - decodes the served body and the file `../../api/openapi.json` (read with `os.ReadFile`) into `any` with `encoding/json`, and fails with a message telling the reader to update `api/openapi.json` if `reflect.DeepEqual` says they differ

  Standard library only. Run `(cd apps/api && go test -race -count=1 ./...)`: it must pass. If it fails on the comparison, fix `openapi.json` (T003), not the service.
- [X] T005 [US1] Verify the story: `grep -rniI 'metrik' apps/api` prints nothing; `mise run lint` and `mise run test` pass.

**Checkpoint**: The service no longer claims any relationship to Metrik. This can be delivered alone.

---

## Phase 4: User Story 2 - The service is named "demo" wherever it is identified (Priority: P2)

**Goal**: Folder, module, command, chart, image, release and namespace are all `demo-api`, and everything that refers to them still works.

**Independent Test**: [quickstart.md §2, §3 and §5](quickstart.md): lint and tests pass, the search for old names prints nothing, `helm template` renders resources named `dev-demo-api`.

**⚠️ T006 comes first. T007 to T018 then land together as one change**: in between, references point at names that do not exist.

- [X] T006 [US2] Move the folders, in this order, from the repository root:
  1. `git mv apps/api apps/demo-api`
  2. `git mv apps/demo-api/cmd/api apps/demo-api/cmd/demo-api`
  3. `git mv apps/demo-api/deploy/chart/api apps/demo-api/deploy/chart/demo-api`

  Leave `apps/demo-api/api/` (the contract folder) as is. If an untracked `apps/api/tmp/` is left behind, delete it.
- [X] T007 [US2] Rename the Go module: in `apps/demo-api/go.mod` set `module demo-api`; in `apps/demo-api/cmd/demo-api/main.go` change the import `"api/internal/router"` to `"demo-api/internal/router"`. Run `(cd apps/demo-api && go build ./... && go test -race -count=1 ./...)`.
- [X] T008 [P] [US2] In `apps/demo-api/.air.toml`, change the comment to `` `air` from apps/demo-api `` and the build command to `go build -o ./tmp/main ./cmd/demo-api`.
- [X] T009 [P] [US2] In `apps/demo-api/configs/dev-values.yaml` and `apps/demo-api/configs/prod-values.yaml`, set `image.repository: kind.local/demo-api` and `command: ["/ko-app/demo-api"]`. Leave tags, probes, security context and resources unchanged.
- [X] T010 [P] [US2] Rename the chart: in `apps/demo-api/deploy/chart/demo-api/Chart.yaml` set `name: demo-api` (leave `version` and `appVersion`). In `templates/_helpers.tpl` and in every template that includes a helper (`deployment.yaml`, `service.yaml`, `serviceaccount.yaml`, `hpa.yaml`, `httproute.yaml`, `NOTES.txt`, `tests/test-connection.yaml`), rename each helper `api.<x>` to `demo-api.<x>` for `name`, `fullname`, `chart`, `labels`, `selectorLabels` and `serviceAccountName`, in both `define` and `include`. Do not touch `apiVersion` fields or the `gateway-api` comments in `values.yaml`. Then run `helm lint apps/demo-api/deploy/chart/demo-api -f apps/demo-api/configs/dev-values.yaml` and `helm template dev-demo-api apps/demo-api/deploy/chart/demo-api -f apps/demo-api/configs/dev-values.yaml | grep -E '^  name:|image:'`: resources must be named `dev-demo-api` and the image `kind.local/demo-api:0.1.0`.
- [X] T011 [P] [US2] In `skaffold.yaml`, for the Demo API artifact and release:
  - artifact: `image: demo-api`, `context: apps/demo-api`, `ko.main: ./cmd/demo-api`
  - release: `name: dev-demo-api`, `namespace: dev-demo-api`, `chartPath: apps/demo-api/deploy/chart/demo-api`, values file `apps/demo-api/configs/dev-values.yaml`
  - value templates: `image.repository: "{{.IMAGE_REPO_demo_api}}"` and `image.tag: "{{.IMAGE_TAG_demo_api}}@{{.IMAGE_DIGEST_demo_api}}"` (underscore, not hyphen)

  Leave the Metrik artifact and release unchanged.
- [X] T012 [P] [US2] In `pulumi/Pulumi.dev.yaml` and `pulumi/Pulumi.prod.yaml`, change the first release to `name: demo-api`, `namespace: demo-api`, `chartPath: "../apps/demo-api/deploy/chart/demo-api/"` and `chartValuesFilePath: "../apps/demo-api/configs/dev-values.yaml"` (`prod-values.yaml` in the prod file). Leave the `metrik` release and the `metrik:releases` config key unchanged.
- [X] T013 [P] [US2] In `apps/metrik/configs/dev-values.yaml`, replace `dev-api.dev-api.svc.cluster.local` with `dev-demo-api.dev-demo-api.svc.cluster.local` in all four endpoints. In `apps/metrik/configs/prod-values.yaml`, replace `prod-api.prod-api.svc.cluster.local` with `prod-demo-api.prod-demo-api.svc.cluster.local` in all four endpoints. Change nothing else (same check names, paths, port and thresholds: FR-008).
- [X] T014 [P] [US2] In `.mise.toml`, change `apps/api` to `apps/demo-api` in the `for m in ...` line of both the `lint` and `test` tasks.
- [X] T015 [P] [US2] In `.golangci.yaml`, change the import allow-list entry `api/internal` to `demo-api/internal`.
- [X] T016 [P] [US2] In `.gitignore`, change `apps/api/tmp/` to `apps/demo-api/tmp/`.
- [X] T017 [P] [US2] Fix paths and commands in `README.md` so they work as written (wording comes in US3): `apps/api/api/openapi.json` becomes `apps/demo-api/api/openapi.json`; `(cd apps/api/ && air)` becomes `(cd apps/demo-api/ && air)`.
- [X] T018 [P] [US2] Fix paths, commands and addresses in the docs:
  - `docs/metrik-architecture.md`: table row `apps/api` becomes `apps/demo-api` (keep the table aligned); diagram label `Api["Deployment api"]` becomes `Api["Deployment demo-api"]`
  - `docs/metrik-deployment.md`: `(cd apps/api/ && ko build ./cmd/api ...)` becomes `(cd apps/demo-api/ && ko build ./cmd/demo-api --base-import-paths -t 0.1.0)`; the pod filter `grep -E 'api|metrik'` becomes `grep -E 'demo-api|metrik'`; the example endpoint `dev-api.dev-api.svc.cluster.local` becomes `dev-demo-api.dev-demo-api.svc.cluster.local`. Leave "Automount the API token." unchanged.
- [X] T019 [US2] Verify the story from the repository root:
  1. `mise run lint` and `mise run test` pass in `apps/demo-api`, `apps/metrik` and `pulumi`.
  2. Both searches of [quickstart.md §3](quickstart.md) print nothing, except the two lines of `.specify/memory/constitution.md`, which T020 fixes next.
  3. `grep -n 'metrik' apps/demo-api/go.mod` prints nothing (FR-003).
  4. `(cd apps/demo-api && air)` starts the service on port 8080 and `curl -s localhost:8080/v1/.well-known/live` answers `200`; stop it afterwards.
  5. Metrik's shipped settings point at the new address in both environments (FR-008), no cluster needed:
     - `helm template dev-metrik apps/metrik/deploy/chart/metrik -f apps/metrik/configs/dev-values.yaml | grep -c 'dev-demo-api.dev-demo-api.svc'` prints `4`
     - `helm template prod-metrik apps/metrik/deploy/chart/metrik -f apps/metrik/configs/prod-values.yaml | grep -c 'prod-demo-api.prod-demo-api.svc'` prints `4`
  6. `skaffold.yaml` still resolves: `skaffold render -p dev --digest-source=tag` succeeds and its output names the release resources `dev-demo-api` with an image ending in `demo-api`. If the command cannot run in this environment (for example it insists on a build or a cluster), run `skaffold diagnose -p dev` instead and say in the report which of the two ran. An error about an unknown `IMAGE_*` variable means the names in T011 are wrong: fix them there.

  Fix any leftover reference in the file that holds it, then re-run.
- [X] T020 [US2] Amend the constitution through `/speckit-constitution` (do not hand-edit outside that command): in `.specify/memory/constitution.md`, `apps/api` becomes `apps/demo-api` in Principle I, and `apps/api/api/openapi.json` becomes `apps/demo-api/api/openapi.json` in Technology & Deployment Constraints; version `1.2.3` (PATCH), Last Amended set to the day of the change. Remove the Sync Impact Report before commit. Then `grep -n 'apps/api' .specify/memory/constitution.md` prints nothing.

**Checkpoint**: The repository builds, lints, tests and renders its chart under the new names, and the constitution names the new paths.

---

## Phase 5: User Story 3 - Documentation tells the two apps apart consistently (Priority: P3)

**Goal**: Every mention of the service reads "Demo API", it is described as an independent target, and existing environments have an upgrade note.

**Independent Test**: [quickstart.md §6](quickstart.md): every mention is "Demo API" with a capital D, the docs site builds, and the deployment page has the upgrade note.

- [X] T021 [P] [US3] In `README.md`, make the name uniform: "a demo API to monitor" (intro) becomes "a Demo API to monitor"; "against the demo API" becomes "against the Demo API"; the heading "### 2. Run the API and Metrik" becomes "### 2. Run the Demo API and Metrik"; "start the demo API" becomes "start the Demo API"; "which probes the API on `localhost:8080`" becomes "which probes the Demo API on `localhost:8080`"; "(the demo API is intentionally flaky)" becomes "(the Demo API is intentionally flaky)". In the Features bullet, say it is a standalone service that knows nothing about Metrik. Leave the reference link labelled `api` near the end unchanged.
- [X] T022 [P] [US3] In `docs/metrik-architecture.md`, change the table role to "The Demo API: a standalone HTTP service with live, slow and flaky readiness endpoints, used as a sample target" (keep the table aligned), and check that the surrounding prose presents it as a separate target that Metrik probes, not as a Metrik component.
- [X] T023 [P] [US3] In `docs/metrik-roadmap.md`, change "an OpenAPI-described demo API" to "an OpenAPI-described Demo API".
- [X] T024 [US3] In `docs/metrik-deployment.md`, change "both the demo API and Metrik" to "both the Demo API and Metrik", and add a short section with the exact heading "## Upgrading from the `api` names", placed immediately before "## Cleanup" (the search in quickstart.md §3 skips everything between those two headings), containing, per [research.md R5](research.md#r5-what-happens-to-environments-deployed-under-the-old-name):
  - old and new names: release and namespace `<stack>-api` are now `<stack>-demo-api`; image `kind.local/api` is now `kind.local/demo-api` (rebuild it with the command from "Build and deploy")
  - new address: `http://<stack>-demo-api.<stack>-demo-api.svc.cluster.local:8080`, to be used in any custom Metrik settings that pointed at `<stack>-api.<stack>-api`
  - Pulumi: the next `pulumi up` creates the new release and namespace and deletes the old ones
  - Skaffold or manual installs: `helm uninstall dev-api -n dev-api` then `kubectl delete namespace dev-api`
- [X] T025 [US3] Verify the story: `grep -nE '[Dd]emo API|the API' README.md docs/*.md` shows only "Demo API" (capital D) for the service and no "the API" that refers to it; `mise run docs` builds the site without error.

**Checkpoint**: All three stories are done.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T026 Run [quickstart.md §4](quickstart.md): follow README section "Run the Demo API and Metrik" word for word; both apps start without editing a command, and Metrik logs results for its four checks.
- [X] T027 If the kind `dev` cluster is available, run the in-cluster part of [quickstart.md §5](quickstart.md): build both images, `pulumi up` the dev stack, and check one pod in `dev-demo-api`, one in `dev-metrik`, no `dev-api` namespace, and no connection or name-resolution error in Metrik's log. If no cluster is available, say so in the report instead of skipping silently. **Run by the maintainer after implementation.**
- [X] T028 Final pass from the repository root: `mise run lint`, `mise run test`, and `git status --short` showing only files listed in [data-model.md](data-model.md) plus `specs/004-demo-api/`. Report anything else to the maintainer.

---

## Dependencies & Execution Order

- **Phase 1** first.
- **US1 (Phase 3)**: depends on Phase 1 only. T002, T003 and T004 touch three different files but T004 only passes once T002 and T003 are done.
- **US2 (Phase 4)**: independent of US1 in principle, but do it after US1 so the moved files are already in their final wording. T006 → T007 → T008 to T018 (any order) → T019 → T020 (the constitution is amended as soon as the new paths exist, so it is never stale during US3).
- **US3 (Phase 5)**: after US2, because T017 and T018 edit the same files as T021, T022 and T024.
- **Polish (Phase 6)**: T026 to T028 last.

## Parallel Example: User Story 2

After T006 and T007, these touch different files and can be done together:

```text
T008 apps/demo-api/.air.toml
T009 apps/demo-api/configs/*-values.yaml
T010 apps/demo-api/deploy/chart/demo-api/**
T011 skaffold.yaml
T012 pulumi/Pulumi.*.yaml
T013 apps/metrik/configs/*-values.yaml
T014 .mise.toml
T015 .golangci.yaml
T016 .gitignore
T017 README.md
T018 docs/metrik-architecture.md, docs/metrik-deployment.md
```

## Parallel Example: User Story 3

```text
T021 README.md
T022 docs/metrik-architecture.md
T023 docs/metrik-roadmap.md
```

## Implementation Strategy

- **MVP**: Phases 1 and 3 (US1). Three files change, and the misleading "Metrik API" claim is gone.
- **Then** US2 as a single change, verified by T019 before going on.
- **Then** US3 and the polish phase. The constitution amendment (T020) closes US2, before US3 starts.

Suggested commits, if the maintainer asks for them (one logical change each, Conventional Commits, maintainer as sole author):

1. `refactor(api): present the service as a Demo API` (T002 to T004)
2. `refactor(demo-api): rename the api app to demo-api` (T006 to T018)
3. `docs(constitution): amend to v1.2.3 (demo-api paths)` (T020)
4. `docs: call the sample service Demo API and add an upgrade note` (T021 to T024)
5. `docs(specs): add the 004 demo-api feature` (`specs/004-demo-api/`)

## Notes

- No behaviour changes: same routes, responses, port and versions (FR-005).
- If T004's comparison fails after a later edit, the contract file is what gets fixed.
- `.specify/feature.json` already points at `specs/004-demo-api`.
