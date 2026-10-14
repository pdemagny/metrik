---

description: "Task list for Registry Image Deployment"
---

# Tasks: Registry Image Deployment

**Input**: Design documents from `/specs/005-registry-image-deploy/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: The constitution requires Go tests for new logic in `internal/` (Development Workflow &
Quality Gates), so `pulumi/internal/stack` gets table-driven tests written before its
implementation. The workflow and docs are validated by the runs in [quickstart.md](quickstart.md).

**Organization**: Tasks are grouped by user story so each can be implemented and tested on its own.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1–US4)

## Path Conventions

Repository root: `/home/pierre/Projects/metrik`. Go modules: `apps/metrik`, `apps/demo-api` and
`pulumi`. CI lives in `.github/workflows/`, and docs in `docs/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Tools that both the workflow and local verification need.

- [X] T001 Add `cosign = "3.1.3"` (aqua:sigstore/cosign) and `crane = "0.22.1"` (aqua:google/go-containerregistry) to the `[tools]` table of `.mise.toml`, keeping alphabetical order. Run `mise install cosign crane` and check that `cosign version` and `crane version` work.
- [X] T002 Find out how ko 0.19.1 SBOMs behave with `--push`. Build a throwaway image to a scratch registry (e.g. `KO_DOCKER_REPO=ttl.sh/metrik-sbom-test ko build ./cmd/metrik --base-import-paths --sbom=spdx --sbom-dir "$SCRATCH"` in `apps/metrik`), then check whether an unsigned `.sbom` was also uploaded (`crane ls ttl.sh/metrik-sbom-test`) and which file in `$SCRATCH` maps to which platform digest. Record the chosen flags and the file-to-digest mapping in research.md R5 of `specs/005-registry-image-deploy/research.md`. If ko can't write SBOMs without uploading them, accept the extra unsigned SBOM and say so in R5.

---

## Phase 2: Foundational (Blocking Prerequisites)

None. US1 (CI) and US2 (Pulumi) touch disjoint files. Only the **end-to-end** validation of US2
needs a released image from US1 (see Dependencies).

---

## Phase 3: User Story 1 - Images are published automatically from the repository (Priority: P1) 🎯 MVP

**Goal**: on pull requests, run lint, tests and a multi-arch build with no push. On `main`, push
signed `sha-<7 hex>` images with SBOM attestations to private ghcr.io. On `vX.Y.Z`, add the
`X.Y.Z` tag to the same digest. Never create `latest` or `main` tags.

**Independent Test**: quickstart.md, "US1: images are published", steps 1–6.

### Implementation for User Story 1

- [X] T003 [US1] Create `.github/workflows/images.yaml` with:
  - `name: images`.
  - Triggers: `pull_request`, `push` on `branches: [main]` and `tags: ['v[0-9]+.[0-9]+.[0-9]+']`.
  - Top-level `permissions: contents: read`.
  - Workflow `env`: `KO_DOCKER_REPO: ghcr.io/pdemagny` (lowercase owner, research R3).
- [X] T004 [US1] Add the `check` job to `.github/workflows/images.yaml` (research R1, R2):
  - `if: github.ref_type != 'tag'`, `runs-on: ubuntu-latest`.
  - Steps: `actions/checkout`, then `jdx/mise-action` with `install_args: "go golangci-lint"`.
  - Then run `mise run lint` and `mise run test`. The test task needs CGO and a C compiler for
    `-race`; ubuntu runners ship gcc.
- [X] T005 [US1] Add the `build` job to `.github/workflows/images.yaml`, for pull requests only
  (research R4, R8):
  - `needs: check`, `if: github.event_name == 'pull_request'`.
  - `strategy.matrix.app: [metrik, demo-api]`, `fail-fast: false`.
  - Job permissions `contents: read` **only**, so branch code never runs next to a write token.
  - Install `go ko` through `jdx/mise-action`.
  - Run in `apps/${{ matrix.app }}`
    `ko build ./cmd/${{ matrix.app }} --base-import-paths --platform=linux/amd64,linux/arm64 --push=false`,
    with no login.
- [X] T006 [US1] Add the `publish` job to `.github/workflows/images.yaml` (research R1, R8):
  - `needs: check`, `if: github.event_name == 'push' && github.ref == 'refs/heads/main'`.
  - Same matrix as `build`.
  - Job permissions `contents: read`, `packages: write`, `id-token: write`.
  - Install `go ko cosign crane` through `jdx/mise-action`.
  - Steps:
  1. `echo "${{ secrets.GITHUB_TOKEN }}" | ko login ghcr.io --username "${{ github.actor }}" --password-stdin`.
  2. `ref=$(ko build ./cmd/${{ matrix.app }} --base-import-paths --platform=linux/amd64,linux/arm64 --tags "sha-${GITHUB_SHA::7}" <SBOM flags from T002>)`.
     `--tags` replaces ko's default `latest` (FR-004). Keep `$ref` (`repo@sha256:…`) as a step
     output.
- [X] T007 [US1] In the `publish` job in `.github/workflows/images.yaml`, add keyless SBOM
  attestations using the SBOM files and digest mapping from T002:
  - one per platform image:
    `cosign attest --yes --type spdxjson --predicate <file> <repo>@<platform digest>`, with the
    platform digests from `crane manifest "$ref"`;
  - one for the index SBOM ko writes, attested to the index digest (`$ref`), so
    `cosign verify-attestation <repo>:<tag>` works on the tag itself.

  (research R5, FR-008)
- [X] T008 [US1] In the `publish` job in `.github/workflows/images.yaml`, sign
  with `cosign sign --yes --recursive "$ref"`. This signs the index and each platform manifest
  (research R6, FR-008).
- [X] T009 [US1] Add the `release` job to `.github/workflows/images.yaml` (research R7, FR-003):
  - `if: github.ref_type == 'tag'`, matrix `app: [metrik, demo-api]`, permissions
    `contents: read`, `packages: write`.
  - Install `cosign crane` through mise, then log in as in T006.
  - Steps:
    1. Guard: if `${GITHUB_REF_NAME}` doesn't match `^v[0-9]+\.[0-9]+\.[0-9]+$`, print "not a
       release tag, nothing to publish" and `exit 0`.
    2. `version=${GITHUB_REF_NAME#v}`, `short=${GITHUB_SHA::7}`,
       `repo=ghcr.io/pdemagny/${{ matrix.app }}`.
    3. `digest=$(crane digest "$repo:sha-$short")`. If that fails, print
       `image $repo:sha-$short not found; wait for the main run of $GITHUB_SHA and re-run` and
       `exit 1`.
    4. `cosign verify --certificate-identity 'https://github.com/pdemagny/metrik/.github/workflows/images.yaml@refs/heads/main' --certificate-oidc-issuer https://token.actions.githubusercontent.com "$repo@$digest"`.
    5. `crane tag "$repo@$digest" "$version"`.
- [X] T010 [US1] Pin every `uses:` in `.github/workflows/images.yaml` to a full commit SHA with a
  trailing `# vX.Y.Z` comment (`actions/checkout`, `jdx/mise-action`). Set
  `jdx/mise-action` `cache: true` so tools aren't downloaded on every run (research R2, SC-002).
- [ ] T011 [US1] Maintainer, manual: open a pull request with T001–T010 and confirm that `check`
  and both `build` jobs pass and nothing is pushed. After the merge, confirm that
  `ghcr.io/pdemagny/metrik` and `ghcr.io/pdemagny/demo-api` have `sha-<7 hex>` tags for both
  platforms, valid signatures and SBOM attestations (`cosign verify-attestation` on the tag and on
  one platform digest), and **private** visibility in the package settings (FR-009). Note the
  `publish` run's duration from the merge, which must be under 20 minutes (SC-002) (quickstart
  US1 steps 1–2).
- [ ] T012 [US1] Maintainer, manual: tag the published commit with
  `git tag v0.1.0 && git push origin v0.1.0`. Confirm that `crane digest …:0.1.0` equals
  `…:sha-<short>` for both apps. Note the `release` run's duration, which must be under 20 minutes (SC-002). Push
  `v0.1.1-rc.1` and confirm nothing is published, then delete that tag. Confirm `crane ls` lists no `latest` or `main` (quickstart US1 steps 3–6).

**Checkpoint**: images are published, signed and promoted. US1 is deliverable on its own.

---

## Phase 4: User Story 2 - The prod stack deploys to a remote cluster from a registry (Priority: P2)

**Goal**:
- Each release can set its image (repository, tag, optional digest) in the stack.
- Three pull-credential modes are supported.
- Invalid config fails at preview.
- An unpullable image fails within 120 s.
- `prod` targets a remote cluster and pins ghcr.io `0.1.0`.

**Independent Test**: quickstart.md "US2: prod deploys from ghcr.io", including the negative
checks.

### Tests for User Story 2 (constitution-required, write first and see them fail)

- [X] T013 [P] [US2] Create `pulumi/internal/stack/config_test.go` with table-driven tests for
  `Validate` and the helpers from data-model.md:
  - **V1**: `image.repository` must be non-empty when `image` is present.
  - **V2**: `image.tag` must be non-empty when `image` is present.
  - **V3**: `image.repository` must contain no `@`, and no `:` after its last `/`. Test that
    `localhost:5000/metrik` is valid and that `ghcr.io/x/metrik:1.0` and `ghcr.io/x/metrik@sha256:…`
    are rejected.
  - **V4**: `image.digest`, if set, must match `^sha256:[0-9a-f]{64}$`.
  - **V5**: server, username and password must be all set or all unset. Each partial combination
    is rejected, and the error names the missing keys.
  - **V6**: mode a (created) and mode b (existing) must not both be set.
  - A release without `image` is valid. Errors name the release.
  - `ImageTag` returns `tag`, or `tag@digest` when a digest is set.
  - `Values` returns nil for no image and no pull secret. With an image only, it returns
    `{"image": {"repository", "tag"}}`, plus `"imagePullSecrets": [{"name": …}]` when a pull
    secret name is given.
  - `DockerConfigJSON("ghcr.io","u","p")` returns
    `{"auths":{"ghcr.io":{"username":"u","password":"p","auth":"dTpw"}}}`.

### Implementation for User Story 2

- [X] T014 [US2] Create `pulumi/internal/stack/config.go` (package `stack`) to make T013 pass:
  - Types: `Image{Repository, Tag, Digest string}`. `Release` (existing fields from
    `pulumi/main.go` plus `Image *Image` with tag `pulumi:"image"`; check how `config.TryObject`
    decodes nested camelCase keys and match it). `Registry{Server, Username, Password,
    ImagePullSecret string}` with `Mode()` returning `ModeCreated` (a), `ModeExisting` (b) or
    `ModeNone` (c).
  - `func Validate(releases []Release, reg Registry) error` implementing V1–V6 with errors that
    name the release or keys.
  - `func ImageTag(img Image) string`.
  - `func Values(img *Image, pullSecret string) map[string]any`.
  - `func DockerConfigJSON(server, username, password string) (string, error)`, using
    `encoding/json` and `encoding/base64` from the standard library.
  - A constant `CreatedPullSecretName = "registry-credentials"`.
- [X] T015 [P] [US2] In `pulumi/pkg/helm/release/release.go`, add `Values pulumi.Map` and
  `Timeout int` to `resourceArgs`, add the options `WithValues(pulumi.Map)` and
  `WithTimeout(int)`, and set `Values` and `Timeout: pulumi.IntPtr(...)` (only when non-zero) on
  `helm.ReleaseArgs` in `Create()`.
- [X] T016 [US2] Update `pulumi/main.go`:
  1. Replace the local `Release` type with `stack.Release`.
  2. Read `metrik:registryServer`, `metrik:registryUsername` and `metrik:imagePullSecret` with
     `metrikConfig.Get`. Read `metrik:registryPassword` with
     `password, err := metrikConfig.TrySecret("registryPassword")`, and treat `err == nil` as
     "password set". Never call `Get` on the secret key, which warns about reading a secret as
     plain text.
  3. Call `stack.Validate` before creating the provider, and return its error.
  4. In mode a (created), create a `corev1.Secret` named `stack.CreatedPullSecretName` in each release
     namespace: type `kubernetes.io/dockerconfigjson`, `StringData` `.dockerconfigjson` from
     `password.ApplyT(... stack.DockerConfigJSON ...)`, so it stays secret.
  5. Pass `release.WithValues(...)` built from `stack.Values(rel.Image, pullSecretName)`, where
     the pull secret name is the created name (mode a), the existing name (mode b), or `""`
     (mode c). This applies to every release, including those without `image` (FR-012). Add a dependency on the
     Secret when one is created.
  6. Pass `release.WithTimeout(releaseTimeoutSeconds)` with
     `const releaseTimeoutSeconds = 120` (FR-017, research R12).
- [X] T017 [P] [US2] Remove the `image.repository: kind.local/metrik` and `image.tag` lines from
  `apps/metrik/configs/prod-values.yaml`, and the `kind.local/demo-api` ones from
  `apps/demo-api/configs/prod-values.yaml`. Keep `command` and everything else (research R13).
  Leave the `dev-values.yaml` files untouched.
- [X] T018 [US2] Update `pulumi/Pulumi.prod.yaml` per contracts/stack-config.md:
  - Ask the maintainer for the remote `kubernetes:context` value.
  - Add `metrik:registryServer: ghcr.io` and `metrik:registryUsername: pdemagny`.
  - Add `image: {repository: ghcr.io/pdemagny/demo-api, tag: "0.1.0"}` and
    `image: {repository: ghcr.io/pdemagny/metrik, tag: "0.1.0"}` to the two releases.
  - Don't write the password. The maintainer sets it with
    `pulumi config set --stack prod --secret metrik:registryPassword` (classic PAT,
    `read:packages`).
- [X] T019 [US2] From `pulumi/`, run `go build ./... && go test ./...`. Then run
  `pulumi preview --stack prod --config-file Pulumi.prod.yaml` with the negative configs from
  quickstart US2 (both modes, missing tag) and confirm each fails at preview, naming the rule and
  release.
- [ ] T020 [US2] Maintainer, manual (needs T012): run
  `pulumi up --stack prod --config-file Pulumi.prod.yaml`. Confirm the pods run
  `ghcr.io/pdemagny/{metrik,demo-api}:0.1.0`, that a nonexistent tag fails within about 2 minutes
  naming `prod-metrik`, and that adding a `digest` makes the pod image `…:0.1.0@sha256:…`
  (quickstart US2).

**Checkpoint**: a remote cluster runs released images from ghcr.io, configured only by the stack.

---

## Phase 5: User Story 3 - Local kind development is unchanged (Priority: P2)

**Goal**: `dev` on kind behaves exactly as before.

**Independent Test**: quickstart.md "US3: dev on kind is unchanged".

- [ ] T021 [US3] Confirm that
  `git diff main -- pulumi/Pulumi.dev.yaml apps/metrik/configs/dev-values.yaml apps/demo-api/configs/dev-values.yaml skaffold.yaml apps/*/deploy/chart`
  is empty. Then run `pulumi preview --stack dev --config-file Pulumi.dev.yaml` in `pulumi/`
  against an existing `dev` stack. The only expected diff is the new `timeout: 120` on both
  releases: no Secret, no `values`.
- [ ] T022 [US3] Without any registry login, follow the local section of
  `docs/metrik-deployment.md` (after T023) on a fresh `dev` kind cluster, then run `skaffold dev`.
  Both apps must run (SC-001).

**Checkpoint**: the local workflow is unaffected.

---

## Phase 6: User Story 4 - Operators learn how to deploy from any registry (Priority: P3)

**Goal**: the deployment docs cover local `dev`, remote `prod` from ghcr.io, per-registry notes,
tags and verification.

**Independent Test**: quickstart.md "US4: docs".

- [X] T023 [US4] Restructure `docs/metrik-deployment.md`:
  - The intro states that `dev` runs on kind with images loaded onto the nodes, and that `prod`
    runs on a remote cluster with signed images from ghcr.io. Keep the kind warning for `dev`.
  - "1. Create the clusters" and "2. Build and deploy" become a **Local (dev on kind)** section
    that creates only the `dev` cluster.
  - Remove "For prod, repeat with kind-prod…".
  - Update the stack example so it notes the optional `image` key.
  - Update **Cleanup**: destroy both stacks, but delete only the `dev` kind cluster.
- [X] T024 [US4] Add a **Remote (prod from a registry)** section to `docs/metrik-deployment.md`
  (FR-020):
  1. Create a classic PAT with `read:packages` (fine-grained tokens can't pull from ghcr.io).
  2. Set `kubernetes:context`, `metrik:registryServer` and `metrik:registryUsername`, and
     `pulumi config set --secret metrik:registryPassword`.
  3. Set `image.repository` and `image.tag` per release, and optionally a `digest` (find it with
     `crane digest ghcr.io/pdemagny/metrik:0.1.0`).
  4. Run `pulumi up --stack prod`.
  5. Explain the three pull-credential modes, a (created), b (existing) and c (none), with the
     YAML from contracts/stack-config.md, and that a mode applies to every release in the
     stack.
  6. Explain failures: a preview validation error, and an unpullable image failing after 120 s.
- [X] T025 [US4] Add a **Published images** section to `docs/metrik-deployment.md` (FR-022,
  FR-008):
  - The triggers → tags table from contracts/published-images.md.
  - "There is no `latest` tag", and what the digest-named cosign attachments are.
  - The `cosign verify` and `cosign verify-attestation` commands with the exact identity and
    issuer.
  - The one-time private-visibility check.
  - Tags and chart versions are independent.
  - To test a release candidate, pin its `sha-<short>`.
- [X] T026 [US4] Add an **Other registries** section to `docs/metrik-deployment.md` (FR-021), based
  on the research R14 table. Give one short subsection each for Google Artifact Registry, Amazon
  ECR (tokens expire after 12 hours, node role recommended → mode c (none)) and Docker Hub, each with the
  repository format, the credential for mode a, the node-identity alternative, and the
  `KO_DOCKER_REPO` to push there.
- [X] T027 [P] [US4] Review `README.md`, especially lines 26, 79 and 87 (deployment mentions and
  the docs table). Update them so they mention registry deployment to remote clusters alongside
  kind (FR-023, constitution README rule).

**Checkpoint**: an operator can deploy from any supported registry using only the docs.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T028 Amend `.specify/memory/constitution.md` through `/speckit-constitution` to 1.3.0
  (MINOR, research R15):
  - Technology & Deployment Constraints: kind cluster `dev` for local demos with side-loaded
    images. `prod` on a remote cluster pulling signed ghcr.io images. A GitHub Actions workflow
    builds and publishes images.
  - Principle V tool list: mention that CI installs tools through mise.
- [X] T029 Run `mise run lint`, `mise run test` and `mise run docs` from the repository root. All
  must pass with no new findings.
- [ ] T030 Check SC-005: in a scratch copy of the `prod` stack config, point it at another
  registry (e.g. Docker Hub: `metrik:registryServer: https://index.docker.io/v1/`, and
  `image.repository: docker.io/<user>/metrik`). Confirm that `pulumi preview` succeeds and
  `git diff -- apps pulumi/*.go pulumi/internal pulumi/pkg` is empty. Don't commit the scratch
  config. Then go through `specs/005-registry-image-deploy/quickstart.md` end to end. Tick the
  remaining items and note any deviation in `specs/005-registry-image-deploy/research.md`.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (T001–T002)**: T001 blocks the US1 tool installs. T002 blocks T006 and T007.
- **Foundational**: none.
- **US1 (T003–T012)**: T003 → T004 → T005 (`build`) → T006 (`publish`) → T007/T008 → T009
  (`release`) → T010 → T011 → T012. All are the same file, so they are sequential.
- **US2 (T013–T020)**: T013 → T014 → T016. T015 and T017 run in parallel with T013/T014. Then
  T018 → T019. **T020 needs T012** (a released `0.1.0`).
- **US3 (T021–T022)**: T021 needs T015/T016 (the timeout diff). T022 needs T023.
- **US4 (T023–T027)**: can start any time. Its content depends on decisions already made in the
  contracts.
- **Polish (T028–T030)**: after all stories.

### User Story Dependencies

- **US1**: independent. It is the MVP.
- **US2**: the code is independent of US1. End-to-end validation (T020) needs US1's first release.
- **US3**: a regression guard on US2's Pulumi changes.
- **US4**: documents US1 and US2. It can be written in parallel and finalized after T020.

### Parallel Opportunities

- T001 ∥ T002 once T001 is installed. T013 ∥ T015 ∥ T017 ∥ T027.
- US1 (workflow file) and US2 (Pulumi files) can be developed in parallel by different people.

---

## Parallel Example: User Story 2

```bash
Task: "Write table-driven tests in pulumi/internal/stack/config_test.go"          # T013
Task: "Add WithValues/WithTimeout to pulumi/pkg/helm/release/release.go"         # T015
Task: "Drop kind.local image lines from apps/*/configs/prod-values.yaml"         # T017
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. T001–T002 (Setup).
2. T003–T010: the workflow.
3. **Stop and validate** with T011–T012: images are published, signed and promoted, which is
   already useful to anyone who can pull them.

### Incremental Delivery

1. US1 → published images (MVP).
2. US2 + US3 → remote prod from the stack, with dev unchanged.
3. US4 → docs.
4. Polish → constitution amendment and full checks.

### Suggested Commits

One logical change per commit, Conventional Commits, made only when the maintainer asks:
1. `build(mise): pin cosign and crane`
2. `ci(images): build, sign and publish images to ghcr.io`
3. `feat(pulumi): deploy images from a registry with pull credentials`
4. `chore(configs): let the prod stack set the image`
5. `feat(pulumi): point the prod stack at a remote cluster and ghcr.io`
6. `docs(deployment): document registry deployments`
7. `docs(constitution): ...`
