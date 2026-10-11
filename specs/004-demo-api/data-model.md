# Data Model: App-Agnostic Demo API

The feature stores no data. Its "model" is the set of names the service goes by, and where each
one is referenced. Every row must be changed in the same change set (FR-007).

## Name map

| What                         | Old                                 | New                                           |
| :--------------------------- | :---------------------------------- | :-------------------------------------------- |
| Display name                 | Metrik API / demo API / the API     | Demo API                                      |
| App folder                   | `apps/api`                          | `apps/demo-api`                               |
| Command folder               | `apps/api/cmd/api`                  | `apps/demo-api/cmd/demo-api`                  |
| Go module                    | `api`                               | `demo-api`                                    |
| Contract file                | `apps/api/api/openapi.json`         | `apps/demo-api/api/openapi.json`              |
| Chart folder                 | `apps/api/deploy/chart/api`         | `apps/demo-api/deploy/chart/demo-api`         |
| Chart name                   | `api`                               | `demo-api`                                    |
| Chart template helpers       | `api.name`, `api.fullname`, ...     | `demo-api.name`, `demo-api.fullname`, ...     |
| Image                        | `kind.local/api`                    | `kind.local/demo-api`                         |
| Container command            | `/ko-app/api`                       | `/ko-app/demo-api`                            |
| Pulumi release and namespace | `api` / `api`                       | `demo-api` / `demo-api`                       |
| Deployed release, namespace  | `<stack>-api`                       | `<stack>-demo-api`                            |
| Skaffold artifact            | `api`                               | `demo-api`                                    |
| Skaffold image variables     | `IMAGE_{REPO,TAG,DIGEST}_api`       | `IMAGE_{REPO,TAG,DIGEST}_demo_api`            |
| In-cluster address           | `<stack>-api.<stack>-api.svc...`    | `<stack>-demo-api.<stack>-demo-api.svc...`    |

Unchanged: port `8080`, routes under `/v1`, `/openapi`, chart `version` and `appVersion`, the
contract's `version` (`0.1.0`), the route group/tag name `v1 api`.

## Where each name is referenced

| File                                                   | Names it holds                                             |
| :----------------------------------------------------- | :--------------------------------------------------------- |
| `apps/demo-api/go.mod`                                 | module                                                     |
| `apps/demo-api/cmd/demo-api/main.go`                   | module (import of `internal/router`)                       |
| `apps/demo-api/internal/router/router.go`              | display name (title, description, group description)       |
| `apps/demo-api/api/openapi.json`                       | display name; schema name fix (research R7)                |
| `apps/demo-api/.air.toml`                              | app folder (comment), command folder                       |
| `apps/demo-api/configs/{dev,prod}-values.yaml`         | image, container command                                   |
| `apps/demo-api/deploy/chart/demo-api/Chart.yaml`       | chart name                                                 |
| `apps/demo-api/deploy/chart/demo-api/templates/**`     | chart template helpers                                     |
| `apps/metrik/configs/{dev,prod}-values.yaml`           | in-cluster address (4 endpoints each)                      |
| `skaffold.yaml`                                        | artifact, folders, image variables, release, namespace     |
| `pulumi/Pulumi.{dev,prod}.yaml`                        | release, namespace, chart folder, values file path         |
| `.mise.toml`                                           | app folder (`lint` and `test` loops)                       |
| `.golangci.yaml`                                       | module (`api/internal` in the import allow-list)           |
| `.gitignore`                                           | app folder (`apps/api/tmp/`)                               |
| `README.md`                                            | display name, app folder, contract file                    |
| `docs/metrik-architecture.md`                          | display name, app folder, diagram label                    |
| `docs/metrik-deployment.md`                            | display name, folders, pod filter, address, upgrade note   |
| `docs/metrik-roadmap.md`                               | display name                                               |
| `.specify/memory/constitution.md`                      | app folder, contract file (via `/speckit-constitution`)    |

## Validation rules

- No file under `apps/demo-api/` contains "metrik" in any letter case (FR-002, SC-001).
- `apps/demo-api/go.mod` requires no module from this repository (FR-003).
- Outside `specs/001` to `specs/003`, no old value from the name map remains where it names the
  service (SC-002). Exclusions are listed in [research.md](research.md), R9.
- The contract file equals the served contract (FR-004).
