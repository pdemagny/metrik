# Research: App-Agnostic Demo API

No `NEEDS CLARIFICATION` remained in the Technical Context. The decisions below settle the naming
and the knock-on effects found while inventorying the repository.

## R1. Identifier form

- **Decision**: `demo-api` everywhere an identifier is needed; "Demo API" in prose and titles. The
  full old-to-new map is in [data-model.md](data-model.md).
- **Rationale**: one spelling for the folder, module, command, chart, image, release and namespace
  means a single search finds every reference. Hyphens are valid in all of these (Go module path,
  Helm chart name, Kubernetes names, image names).
- **Alternatives considered**: `demoapi` (no hyphen) reads worse and differs from the display
  name; `demo` alone drops the word the docs already use; keeping `api` for the Go module while
  renaming the folder leaves a second name to remember.

## R2. Command folder and binary name

- **Decision**: `apps/demo-api/cmd/demo-api/`, so the binary `ko` builds is `/ko-app/demo-api` and
  the image built with `--base-import-paths` is `kind.local/demo-api`.
- **Rationale**: the constitution fixes the layout as `cmd/<app>/`. `ko` names the binary after
  the main package's folder and, with `--base-import-paths`, names the image after the last
  element of the import path; both follow from the folder name with no extra setting.
- **Alternatives considered**: keeping `cmd/api` avoids touching `command:` in the values files
  but breaks the `cmd/<app>/` rule and leaves an image called `api`.

## R3. In-cluster address

- **Decision**: release `demo-api` in namespace `demo-api` in both Pulumi stacks, giving
  `http://<stack>-demo-api.<stack>-demo-api.svc.cluster.local:8080`; Skaffold release and
  namespace `dev-demo-api`.
- **Rationale**: the chart's `fullname` helper returns the release name when it already contains
  the chart name. `dev-demo-api` contains `demo-api`, so the Service is named `dev-demo-api`,
  exactly as `dev-api` works today. Pulumi prefixes release and namespace with the stack name.
- **Alternatives considered**: keeping the `api` release and namespace would leave the address
  unchanged and remove the upgrade note, but FR-006 asks for the release and namespace to carry
  the demo name.

## R4. Skaffold image variables

- **Decision**: artifact `image: demo-api`; the Helm value templates become
  `{{.IMAGE_REPO_demo_api}}`, `{{.IMAGE_TAG_demo_api}}` and `{{.IMAGE_DIGEST_demo_api}}`.
- **Rationale**: Skaffold derives these variable names from the artifact name and replaces
  characters that are not valid in a template identifier (here `-`) with `_`.
- **Alternatives considered**: none; an unreplaced `-` does not parse as a Go template field.

## R5. What happens to environments deployed under the old name

- **Decision**: no migration code. Document the cleanup in `docs/metrik-deployment.md`.
- **Rationale**: environments are local kind clusters.
  - Pulumi: the release and namespace resource names change, so the next `pulumi up` creates the
    new ones and deletes the old ones in the same run. Nothing is left behind.
  - Skaffold: `skaffold dev` removes its releases on exit, so normally nothing is left. If a
    `dev-api` release survived, it is removed by hand with `helm uninstall dev-api -n dev-api`
    and `kubectl delete namespace dev-api`.
  - Operators with their own Metrik settings must replace `<stack>-api.<stack>-api` with
    `<stack>-demo-api.<stack>-demo-api` in their endpoints.
- **Note on the spec**: the spec's first edge case says the old release "stays behind until
  removed". That is true for Skaffold and for manual installs; with Pulumi it is removed
  automatically. The upgrade note states both.
- **Alternatives considered**: Pulumi `aliases` to rename in place. Rejected: a Helm release and a
  namespace cannot be renamed in place anyway, and the clusters are disposable.

## R6. Contract wording

- **Decision**:
  - title: `Demo API`
  - description: `Demo API with live, slow and flaky endpoints, meant as a target for health checks.`
  - route group: name `v1 api` (unchanged), description `Demo API v1.`
- **Rationale**: satisfies FR-001 and SC-007 (a reader knows what it is for without hearing about
  Metrik) while changing as little of the contract as possible. The group name is also the tag on
  every operation; leaving it avoids touching each route.
- **Alternatives considered**: renaming the tag to `v1` is tidier but changes every operation's
  tag for no gain.

## R7. The repository contract file already differs from the served contract

- **Finding**: comparing the document served at `/openapi` with `apps/api/api/openapi.json`
  (both key-sorted) shows one difference, unrelated to naming: the served schema is called
  `RouterAPIResponse`, the file calls it `APIResponse` (the definition and its four references).
  Everything else matches.
- **Decision**: correct the file to say `RouterAPIResponse`, and add a test that compares the
  file with the served document so they cannot drift again.
- **Rationale**: FR-004 requires the two to match and FR-005 forbids changing what the service
  serves, so the file is the side that moves. The constitution already requires the file to be
  updated with any route or payload change; a test makes that mechanical. The test needs only the
  standard library.
- **Alternatives considered**: overriding the schema name in code so the service serves
  `APIResponse`. Rejected for this feature: it changes the served contract, which FR-005 rules
  out. It can be done later as its own change if the leaked package prefix is unwanted.

## R8. Constitution wording

- **Decision**: amend the constitution to v1.2.3 (PATCH): `apps/api` becomes `apps/demo-api` in
  Principle I, and `apps/api/api/openapi.json` becomes `apps/demo-api/api/openapi.json` in
  Technology & Deployment Constraints. Done through `/speckit-constitution`, committed on its own
  as `docs(constitution): ...`.
- **Rationale**: FR-010. Governance requires amendments to go through that command; a path
  correction is a clarification, so PATCH.
- **Alternatives considered**: leaving the constitution as is would make it name a folder that no
  longer exists.

## R9. Things named "api" that are left alone (FR-012)

- The `api/` folder inside the app (contract location, fixed by the project layout).
- The Go type `APIResponse` and test fixtures in `apps/metrik` that use `"api"` as an arbitrary
  check name or host.
- Kubernetes `apiVersion` fields, "gateway-api" comments in the chart values, and "Automount the
  API token" in the deployment page.
- The README reference link labelled `api` (it points to an article about the web framework).
- Check names in example settings (`"api liveness"`, `"api slow"` in the configuration page):
  they are sample labels chosen by an operator, not the service's name.
- `specs/001` to `specs/003`.
