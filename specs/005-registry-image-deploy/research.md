# Research: Registry Image Deployment

Each section records a decision, the reason for it, and the alternatives that were rejected.

## R1. One workflow file, three jobs

**Decision**: one workflow, `.github/workflows/images.yaml`, triggered by `pull_request`, by `push`
to `main`, and by `push` of tags matching `v[0-9]+.[0-9]+.[0-9]+`.

| Job | Runs on | Does |
| :--- | :--- | :--- |
| `check` | pull requests, `main` | `mise run lint`, `mise run test` |
| `build` (matrix `metrik`, `demo-api`, needs `check`) | pull requests | Multi-arch `ko build --push=false`, read-only token |
| `publish` (matrix `metrik`, `demo-api`, needs `check`) | `main` | Multi-arch `ko build` + push, SBOM attestations, signature |
| `release` (matrix `metrik`, `demo-api`) | `v*` tags | Validate the tag, check that `sha-<short>` exists and is signed, add tag `X.Y.Z` |

**Rationale**: a single file shows the whole image lifecycle in one place. `needs: check` enforces
FR-005, and the release job never rebuilds (clarification 1).

**Alternatives**: separate `ci.yaml` and `release.yaml` files (more files, and the `check` gate
would be duplicated or lost on release). GoReleaser (a new tool and config format, and it rebuilds
on tag).

## R2. Tool installation in CI goes through mise

**Decision**: CI installs only the tools it needs from `.mise.toml` with `jdx/mise-action`
(`install_args`: `go golangci-lint ko cosign crane`). `cosign` and `crane` are added to
`.mise.toml` and pinned to their current stable releases. Third-party actions are pinned by full
commit SHA.

**Rationale**: constitution V makes `.mise.toml` the single source of tool installation, so local
and CI versions match. Installing only a subset avoids downloading hugo, pulumi, kind, etc. on
every run. Pinning by SHA protects the workflow, which holds a `packages: write` token, from a
moved action tag.

**Alternatives**: `sigstore/cosign-installer` and `ko-build/setup-ko` actions (a second source of
tool versions, which violates V).

## R3. Image names and tags

**Decision**: `KO_DOCKER_REPO=ghcr.io/<owner, lowercased>` with `--base-import-paths`, which gives
`ghcr.io/pdemagny/metrik` and `ghcr.io/pdemagny/demo-api`. `main` builds pass
`--tags sha-${GITHUB_SHA::7}`. Releases add `X.Y.Z` with `crane tag`.

**Rationale**: these are the same image names the local flow produces under `kind.local`, so only
the registry prefix changes. Passing `--tags` replaces ko's default `latest` tag, which is how
FR-004 is met. A fixed 7-character short SHA can be computed the same way by the release job from
the tagged commit.

**Alternatives**: `--bare` (both apps would share one name). The full 40-character SHA (unwieldy
in stack files). `git rev-parse --short` (its length varies between clones).

**Accepted limitation**: ko's default base image (`cgr.dev/chainguard/static`) is not pinned by
digest. Re-running a `main` build for the same commit can therefore produce a different digest.
The spec allows this, because the tag still points at a build of the same commit, and the
`X.Y.Z` tag keeps the digest it was promoted from. Pinning `defaultBaseImage` in a `.ko.yaml` is
left as a later improvement.

## R4. Multi-arch and pull-request builds

**Decision**: `--platform=linux/amd64,linux/arm64` on every build. Pull requests use
`--push=false`, which builds both platforms and publishes nothing, with no registry login.

**Rationale**: ko cross-compiles pure Go (`CGO_ENABLED=0` in images) with no QEMU. `--push=false`
proves the image builds on fork PRs, which have no write token (spec edge case).

**Alternatives**: `ko build --local` (cannot load a multi-platform index into the Docker daemon).

## R5. SBOM as a signed attestation

**Decision**: ko writes SPDX SBOMs locally (`--sbom=spdx --sbom-dir`), and `cosign attest --type
spdxjson` attaches each one as a keyless, signed in-toto attestation to the matching per-platform
digest, **and** the index SBOM to the index digest. With the index attested,
`cosign verify-attestation <repo>:<tag>` works on the tag itself (FR-008). **Confirmed in T002** (ko 0.19.1, local
throwaway registry): `--sbom=spdx --sbom-dir <dir>` writes `<app>-index.spdx.json` and
`<app>-linux-<arch>.spdx.json`, **and** pushes an unsigned SBOM for each digest under the tag
`sha256-<digest>.sbom`. `--sbom=none` disables both, so the unsigned copies are accepted as an
extra. They are digest-named attachments, like cosign's, not image tags. Mapping: the index file
goes to the index digest from ko's output. `linux-<arch>` goes to the digest of the
`.manifests[]` entry whose `platform.architecture` is `<arch>` in `crane manifest <index>`.

**Rationale**: FR-008 requires an SBOM that can be verified with public tooling. An attestation is
signed by the same workflow identity as the image and checked with `cosign verify-attestation`.
Cosign has deprecated attaching unsigned SBOMs under `.sbom` tags.

**Alternatives**: ko's default attached SBOM (unsigned, uses a deprecated storage scheme). Syft
(a second SBOM generator, when ko already has one).

## R6. Keyless signing

**Decision**: on `main`, `cosign sign --yes --recursive <repo>@<index digest>` signs the index and
each platform manifest using the job's GitHub OIDC token (`id-token: write`). The expected
identity is `https://github.com/pdemagny/metrik/.github/workflows/images.yaml@refs/heads/main`
with issuer `https://token.actions.githubusercontent.com`.

**Rationale**: there are no keys to store or rotate. Signatures and attestations are bound to the
digest, so the `X.Y.Z` tag added at release reuses them unchanged, which keeps them valid
(clarification 1).

**Note on tags**: cosign stores signatures and attestations either as OCI referrers or, on
registries without the referrers API, under digest-named tags (`sha256-<digest>.sig`/`.att`).
These are attachments addressed by digest, not image tags. The docs state this so they are not
mistaken for the moving tags SC-003 forbids.

**Alternatives**: a cosign key pair kept in repository secrets (needs secret handling and
rotation). GitHub's `actions/attest-build-provenance` (verified with the `gh` CLI rather than
registry-agnostic tooling, and it is provenance, not an image signature).

## R7. Release promotion

**Decision**: for each app, the `release` job:
1. Checks that the tag matches `^v[0-9]+\.[0-9]+\.[0-9]+$`, as a guard on top of the trigger
   filter. Otherwise it exits successfully with nothing published.
2. Resolves `<repo>:sha-<short>` with `crane digest`. If it is missing, the job fails with
   "image <repo>:sha-<short> not found; wait for the main run of <sha> and re-run".
3. Runs `cosign verify` on that digest against the identity from R6.
4. Runs `crane tag <repo>@<digest> X.Y.Z`.

**Rationale**: this meets FR-003 and its clarifications: same digest, no rebuild, a clear failure
when the commit was never published, and safe to re-run. Only commits pushed to `main` get
`sha-` images, so a release tag on an unmerged commit fails by construction.

**Alternatives**: `docker buildx imagetools create` (Docker isn't managed by mise). `oras tag` (a
second registry tool, when crane shares ko's library).

## R8. Registry authentication in CI

**Decision**: the built-in `GITHUB_TOKEN`. Permissions are `contents: read` at workflow level, and
`packages: write` plus `id-token: write` on the `publish` job, and `packages: write` alone on the
`release` job, which verifies but never signs. The pull-request `build` job keeps `contents: read`
only, so code from a branch never runs next to a write token. Login is
`ko login ghcr.io --username "${{ github.actor }}" --password-stdin`, which writes the Docker
config file that crane and cosign also read.

**Rationale**: FR-010, with no long-lived token. Packages created from a workflow default to
private and linked to the repository. The docs note a one-time check of the package visibility
(FR-009).

**Alternatives**: a PAT in repository secrets (long-lived, forbidden by FR-010).

## R9. Stack configuration shape

**Decision**: add an optional `image` object (`repository`, `tag`, `digest`) to each entry of
`metrik:releases`, plus four flat stack keys: `metrik:registryServer`, `metrik:registryUsername`,
`metrik:registryPassword` (set with `--secret`), and `metrik:imagePullSecret`. See
[contracts/stack-config.md](contracts/stack-config.md).

**Rationale**: flat keys let the password be read with `config.TrySecret`, which returns a secret
`pulumi.StringOutput` that stays encrypted in state and masked in output (FR-014). A secret nested
inside the object would need `TrySecretObject` and an untyped apply. Credentials are per stack
(spec assumption), and images are per release.

**Alternatives**: a `metrik:registry` object (awkward secret handling). Image settings at stack
level (each app has its own repository).

## R10. Applying the config: chart values override and pull secret

**Decision**:
- When a release has `image`, the program passes `helm.ReleaseArgs.Values`
  `{image: {repository, tag: "<tag>" or "<tag>@<digest>"}}`. These values take precedence over
  the values file. Both charts already render `repository:tag`, and `tag@digest` makes that a
  valid digest-pinned reference (the same trick `skaffold.yaml` already uses). No chart changes.
- **Mode a** (server, username and password set): one `kubernetes.io/dockerconfigjson` Secret
  named `registry-credentials` per release namespace. The JSON is built from the secret output in
  an `ApplyT`, so it stays secret. Values gain `imagePullSecrets: [{name: registry-credentials}]`.
- **Mode b** (`imagePullSecret` set): values gain `imagePullSecrets: [{name: <that name>}]`, and
  nothing is created.
- **Mode c** (neither): no `imagePullSecrets` override, so the values file applies.

**Rationale**: FR-011 to FR-013 and FR-016. A release without `image` gets no override, which
keeps `dev` byte-for-byte as before (FR-012, FR-019).

**Alternatives**: patching the values files per environment (does not meet "only stack
configuration changes" in SC-005). Attaching the pull secret to the ServiceAccount (a hidden
link, where the chart already exposes `imagePullSecrets`).

## R11. Validation before any change

**Decision**: a pure Go function in `pulumi/internal/stack` validates the loaded config before
any resource is registered. Pulumi aborts the preview when `pulumi.Run` returns an error. Rules
are in [data-model.md](data-model.md#validation-rules). Table-driven tests cover each rule.

**Rationale**: FR-015. The checks run during preview, so nothing reaches the cluster.

## R12. Failing within 2 minutes

**Decision**: every `helm.Release` gets `Timeout: 120` (seconds), with `SkipAwait` left at its
default of `false`. A pod stuck in `ImagePullBackOff` keeps the Deployment unready, so the
release fails after 120 s with the Pulumi resource name (`<stack>-<release>`) in the error.

**Rationale**: FR-017 and clarification 4. This is a fixed delivery constraint from the spec, not
an operational setting of the checker, so it is a named constant rather than new config (YAGNI).
It also applies to `dev`, where images are already on the nodes and pods start within seconds.

**Alternatives**: a per-stack timeout key (no requirement for it). `Atomic: true` (a rollback hides
the failed state the operator needs to inspect).

## R13. The `prod` stack and its values files

**Decision**:
- `Pulumi.prod.yaml`: `kubernetes:context` is the maintainer's remote cluster context, set during
  implementation. `metrik:registryServer: ghcr.io`, `metrik:registryUsername: pdemagny`, an
  encrypted `metrik:registryPassword` (a classic PAT with `read:packages`), and per release an
  `image` with `repository: ghcr.io/pdemagny/<app>` and `tag: "0.1.0"`.
- `apps/*/configs/prod-values.yaml`: drop the `image.repository: kind.local/...` and `image.tag`
  lines, since the stack provides them. Leaving `kind.local` in a prod file would be misleading
  (deferred clarification item).
- A first release `v0.1.0` is tagged once the workflow has published a `main` build (spec
  assumption).

**Rationale**: FR-018. GitHub Packages only accepts classic PATs for registry pulls.

## R14. Registry notes for the docs

| Registry | Repository format | Credential (mode a) | Prefer |
| :--- | :--- | :--- | :--- |
| ghcr.io | `ghcr.io/<owner>/<app>` | username + classic PAT `read:packages` | public package, or mode a |
| Docker Hub | `docker.io/<user>/<app>`, server `https://index.docker.io/v1/` | username + access token (read-only scope) | mode a |
| Google Artifact Registry | `<region>-docker.pkg.dev/<project>/<repo>/<app>` | `_json_key` + service-account JSON key | GKE node or Workload Identity service account with Artifact Registry Reader → mode c |
| Amazon ECR | `<account>.dkr.ecr.<region>.amazonaws.com/<app>` | `AWS` + `aws ecr get-login-password`, **expires after 12 h** | EKS node role with `AmazonEC2ContainerRegistryReadOnly` → mode c |

Pushing to another registry is the same `ko build` with a different `KO_DOCKER_REPO`.

## R15. Constitution text

**Decision**: amend the Technology & Deployment Constraints line "kind clusters `dev` and `prod`
for local demos" to describe `dev` on kind with side-loaded images and `prod` on a remote cluster
pulling signed images from ghcr.io. Also mention the GitHub Actions image workflow and the new
mise tools. This is a MINOR bump (1.2.3 → 1.3.0), because it materially expands delivery
guidance, made through `/speckit-constitution` as part of this feature.
