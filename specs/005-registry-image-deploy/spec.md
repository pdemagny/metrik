# Feature Specification: Registry Image Deployment

**Feature Branch**: `005-registry-image-deploy`

**Created**: 2026-10-14

**Status**: Draft

**Input**: User description: "metrik should support deploying to non-local clusters by pulling images from an image registry. it should not break the current behavior to work locally by loading images in kind. it should be registry agnostic (GAR, ERC, ghcr, Hub ...). we will configure a Github Actions workflow that builds images with ko and pushes images to ghcr.io in this repository. we will then document how metrik can use these images for non-local clusters"

## Clarifications

### Session 2026-10-14

- Q: When a release tag vX.Y.Z is pushed, should the workflow rebuild the images or promote the sha-<short> images already built from that commit on main? → A: Promote the existing images: add the `X.Y.Z` tag to the already-tested, already-signed `sha-<short>` images, with no rebuild.
- Q: Should pre-release git tags such as v1.2.0-rc.1 publish images too? → A: No. Only exact `vX.Y.Z` tags publish. Release candidates are tested by pinning their commit's `sha-<short>` tag.
- Q: Every merge to main adds a new sha-<short> image to the private ghcr.io packages. Should old ones be cleaned up? → A: No. All images are kept, and cleanup is out of scope.
- Q: When pods can't pull their image, how quickly should the prod deployment give up and report failure? → A: Within 2 minutes.

## User Scenarios & Testing *(mandatory)*

Today Metrik and the Demo API can only be deployed to local kind clusters. Their images are built on
the operator's machine and loaded directly onto the kind nodes, never pushed to a registry. The
values files hardcode that local image location, and both stacks (`dev` and `prod`) point at kind
contexts. A cluster that isn't kind can't run either app, because there is nowhere to pull the
images from.

This feature adds a second way to get images: the repository publishes them to a container
registry, and a stack can deploy from any registry. The `dev` stack keeps the local kind workflow
exactly as it is. The `prod` stack moves to a real remote cluster and pulls released images from
the repository's ghcr.io packages.

This feature has three kinds of users:
- **The maintainer**, who merges changes and cuts releases and expects images to be published
  automatically, safely and reproducibly.
- **Operators**, who deploy Metrik to a cluster of their choice from a registry of their choice.
- **Local developers**, who iterate on the `dev` kind cluster and must not notice any change.

### User Story 1 - Images are published automatically from the repository (Priority: P1)

When the maintainer merges to `main`, Metrik and Demo API images are built, checked and pushed to
the repository's private ghcr.io packages under a tag that identifies the commit. When the
maintainer pushes a release tag `vX.Y.Z` on a commit already published from `main`, those exact
images also get the tag `X.Y.Z`, with no rebuild. On a pull
request, the same checks and builds run, but nothing is published. Each published image runs on
both amd64 and arm64 nodes, carries a software bill of materials, and is signed, so anyone can
verify that the repository built it.

**Why this priority**: a remote cluster can do nothing until images exist in a registry. This is
the foundation for every other story.

**Independent Test**: merge a change to `main` and push a release tag. Confirm that both images
appear in ghcr.io under the commit tag and the release tag, for both architectures, with an SBOM
and a valid signature, and that no mutable tag exists. Open a pull request and confirm that the
checks and builds run and nothing is published.

**Acceptance Scenarios**:

1. **Given** a commit merged to `main`, **When** the publish run finishes, **Then** both images
   exist in ghcr.io tagged `sha-<short commit hash>`.
2. **Given** a git tag `vX.Y.Z` pushed on a commit already published from `main`, **When** the
   release run finishes, **Then** both images are tagged `X.Y.Z` with the same digests as their
   `sha-<short>` images.
3. **Given** a git tag `vX.Y.Z` pushed on a commit that was never published from `main`, **When**
   the release run executes, **Then** it fails with a message naming the missing images, and
   nothing is tagged.
4. **Given** any published image, **When** it is inspected, **Then** it provides linux/amd64 and
   linux/arm64 variants, has an attached SBOM, and its signature verifies against this
   repository's workflow identity.
5. **Given** lint or tests fail on a commit, **When** the run executes, **Then** no image is
   published for that commit.
6. **Given** a pull request, including one from a fork, **When** its run executes, **Then** lint,
   tests and image builds run, the result is reported on the pull request, and nothing is pushed.
7. **Given** the registry after any number of runs, **When** its tags are listed, **Then** no
   `latest`, `main` or other moving tag exists.

---

### User Story 2 - The prod stack deploys to a remote cluster from a registry (Priority: P2)

An operator deploys the `prod` stack to a real, non-kind cluster. The stack configuration says, for
each release, which image repository and tag to run (optionally pinned to a digest), and how the
cluster gets pull credentials. Changing registry (ghcr.io, Docker Hub, Google Artifact Registry,
Amazon ECR) means changing configuration values only.

**Why this priority**: this is the outcome the feature exists for, but it depends on published
images (Story 1).

**Independent Test**: with images published to ghcr.io, run the `prod` deployment against a remote
cluster with pull credentials in the stack configuration. Confirm that both apps run with exactly
the configured image and that no manual cluster command was needed.

**Acceptance Scenarios**:

1. **Given** a `prod` stack whose config names a ghcr.io repository, a released tag and registry
   credentials stored as secret config, **When** it is deployed, **Then** each release namespace
   receives a pull credential, and both apps run the configured image.
2. **Given** a stack whose config names an existing pull-secret instead of credentials, **When**
   it is deployed, **Then** the pods reference that secret and nothing new is created.
3. **Given** a stack with no pull credentials configured (for example, a cluster whose nodes are
   allowed to pull through cloud identity), **When** it is deployed, **Then** pods are created
   with no pull secret.
4. **Given** a stack whose image config includes a digest, **When** it is deployed, **Then** the
   pods run exactly that digest, even if the tag were ever moved.
5. **Given** the same stack retargeted to another registry by changing only repository and
   credential values, **When** it is deployed, **Then** it succeeds with no change to code, chart
   or values files.

---

### User Story 3 - Local kind development is unchanged (Priority: P2)

A developer creates the `dev` kind cluster, builds images straight onto its nodes, and deploys the
`dev` stack with the same commands as before. Nothing needs to be pushed and no registry
credentials are needed. The `skaffold dev` inner loop keeps working too.

**Why this priority**: it shares P2 with Story 2 because breaking the everyday local workflow is
unacceptable, whatever value the remote path adds.

**Independent Test**: on a machine with no registry login, follow the local section of the
deployment docs end to end against a fresh `dev` kind cluster, and run `skaffold dev`. Both apps
run.

**Acceptance Scenarios**:

1. **Given** a fresh `dev` kind cluster and no registry credentials, **When** the documented local
   commands are run, **Then** both apps deploy and run with images loaded onto the kind nodes.
2. **Given** the `dev` stack's configuration, **When** it is compared with the one before this
   feature, **Then** it needs no new required setting, and the only change in what it deploys
   is the release timeout (FR-017), which applies to every release.
3. **Given** the `dev` kind cluster, **When** `skaffold dev` runs, **Then** it builds, deploys and
   live-updates both apps as before.

---

### User Story 4 - Operators learn how to deploy from any registry (Priority: P3)

An operator reads the deployment docs. They find a complete walkthrough for deploying from the
repository's ghcr.io images to a remote cluster, and short notes for each major registry: how
repository names are written, how credentials are obtained, and the pitfalls. They also learn how
to verify an image's signature before deploying it.

**Why this priority**: the capability exists without the docs, but few operators could use it
without guidance, especially for registries other than ghcr.io.

**Independent Test**: someone who knows Kubernetes but not this repository follows the ghcr.io
walkthrough and deploys to a remote cluster without asking the maintainer anything.

**Acceptance Scenarios**:

1. **Given** the deployment docs, **When** an operator follows the remote walkthrough, **Then** it
   covers obtaining a ghcr.io read credential, setting the stack's image and credential config,
   choosing a tag, optionally pinning a digest, deploying, and verifying the image signature.
2. **Given** the docs, **When** an operator targets Google Artifact Registry, Amazon ECR or Docker
   Hub, **Then** a section per registry explains the repository format, the credential to use,
   and its lifetime, including that ECR tokens expire after 12 hours and node identity is the
   recommended alternative.
3. **Given** the docs, **When** an operator looks up the image tags, **Then** they learn which tags
   exist (`sha-<short>` and `X.Y.Z`) and that there is no `latest`.

---

### Edge Cases

- **Configured image tag does not exist** in the registry, or credentials are wrong: within 2
  minutes, the deployment reports a failure naming the release. It must not finish as successful
  while pods can't pull, and it must not wait longer.
- **Both credentials and an existing secret name** set for the same stack: the configuration is
  rejected before anything changes in the cluster, because the intent is ambiguous.
- **Partial image config** (a repository with no tag, or a digest with no repository): rejected
  before anything changes in the cluster.
- **Image config absent on a release**: the release uses its values file unchanged. This is how
  `dev` keeps working.
- **Git tag that isn't exactly `vX.Y.Z`** (e.g. `v1.2`, `release-1`, or a pre-release like
  `v1.2.0-rc.1`): nothing is published for it. To test a release candidate, pin its commit's
  `sha-<short>` tag.
- **Re-running a publish for the same commit or release tag**: it may republish the same tag, but
  tags must never point at the build of a different commit.
- **Release tag pushed before the commit's `main` images are published** (e.g. tagging right
  after merge, while that run is still going): the release run fails, and re-running it after the
  `main` run finishes succeeds.
- **Pull request from a fork**, which has no access to repository secrets: the run still lints,
  tests and builds, and it never needs publish credentials.
- **Expired short-lived credentials** (ECR): new pods fail to pull. The docs say so and point to
  node identity. Refreshing credentials automatically is out of scope.
- **A remote cluster on arm64 nodes**: it pulls the arm64 variant with no configuration.

## Requirements *(mandatory)*

### Functional Requirements

**Publishing**

- **FR-001**: The repository MUST provide an automated workflow that builds the Metrik and Demo API
  images with ko and publishes them to this repository's ghcr.io packages.
- **FR-002**: A push to `main` MUST publish both images tagged `sha-<short commit hash>`.
- **FR-003**: A pushed git tag matching `vX.Y.Z` MUST add the tag `X.Y.Z` (no `v` prefix) to the
  existing `sha-<short>` images of that commit, without rebuilding. The release digests MUST be
  identical to the `main` digests, and their signatures and SBOMs MUST stay valid. If the commit
  has no published `sha-<short>` images, the run MUST fail and tag nothing. Other tag formats,
  including SemVer pre-releases (`vX.Y.Z-<pre>`) and build metadata, MUST NOT publish anything.
- **FR-004**: The workflow MUST NOT publish `latest`, `main` or any other tag that moves between
  builds.
- **FR-005**: Lint and tests for every module MUST pass before any image is built and published
  from `main`. Release runs reuse those already-checked images. A failure
  MUST prevent publishing for that run.
- **FR-006**: On pull requests, the workflow MUST run lint, tests and image builds and MUST NOT
  push anything.
- **FR-007**: Every published image MUST support linux/amd64 and linux/arm64.
- **FR-008**: Every published image MUST have a signed SBOM attestation and a keyless signature
  tied to this repository's workflow identity. This applies to the multi-platform index and to
  each platform image. Both MUST be verifiable with public tooling against the image tag itself.
- **FR-009**: The ghcr.io packages MUST be private.
- **FR-010**: Publishing MUST use only the workflow's own short-lived repository credentials. No
  long-lived personal token is stored for pushing.

**Deploying**

- **FR-011**: A stack MUST be able to set, per release, the image repository and tag, and
  optionally a digest. When set, these MUST override the values file.
- **FR-012**: A release with no image config in its stack MUST take its image from its values
  file, as today. The 2-minute failure limit (FR-017) applies to every release. A stack's
  pull-credential mode applies to every release in it, including those without image config.
- **FR-013**: A stack MUST support three pull-credential modes, at most one at a time:
  (a) registry server, username and password/token stored as secret stack config, from which
  the deployment creates a pull secret in each release namespace;
  (b) the name of a pull secret that already exists in the cluster;
  (c) none.
- **FR-014**: In mode (a), the credential MUST stay encrypted in stack configuration and MUST NOT
  appear in plain text in the repository, logs or deployment output.
- **FR-015**: The deployment MUST reject, before changing the cluster, a stack configuration that
  sets both credential modes (a) and (b), or that sets an incomplete image reference.
- **FR-016**: Stacks MUST be registry-agnostic. Switching between ghcr.io, Docker Hub, Google
  Artifact Registry and Amazon ECR MUST require changing only stack configuration values.
- **FR-017**: A deployment whose images cannot be pulled MUST end in a reported failure naming
  the release, within 2 minutes of that release starting.
- **FR-018**: The `prod` stack MUST target a remote, non-kind cluster and deploy a released `X.Y.Z`
  image of both apps from ghcr.io, using pull-credential mode (a).
- **FR-019**: The `dev` stack, its values files and `skaffold dev` MUST keep working against the
  local `dev` kind cluster with images loaded onto the nodes, with no registry and no credentials.

**Documentation**

- **FR-020**: The deployment docs MUST keep the local kind walkthrough for `dev` and add a remote
  walkthrough for `prod` using the ghcr.io images: read credential, stack config, tag choice,
  optional digest pinning, deploy and signature verification.
- **FR-021**: The docs MUST include a short section each for Google Artifact Registry, Amazon ECR
  and Docker Hub, covering repository format, credential type and lifetime, and node-identity
  alternatives.
- **FR-022**: The docs MUST list the published tag scheme and state that there is no `latest` tag.
- **FR-023**: The README MUST be updated where setup or deployment instructions change.

### Key Entities

- **Published image**: a multi-architecture Metrik or Demo API image in ghcr.io, identified by its
  digest. It is built once from `main` and tagged `sha-<short>`. A release adds an `X.Y.Z` tag to
  the same digest. It has an SBOM and a signature attached.
- **Release image config**: per release in a stack. It holds repository, tag and an optional
  digest. When absent, the values file applies.
- **Pull-credential config**: per stack. It is one of: secret registry credentials, an existing
  secret name, or none. It applies to every release namespace in the stack.
- **Stack**: `dev` (local kind, side-loaded images, no registry) or `prod` (remote cluster, ghcr.io
  images, created pull secret).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The documented local `dev` workflow works end to end on a fresh kind cluster with
  zero changes to its commands and no registry login.
- **SC-002**: Within 20 minutes of a merge to `main` or a release tag, both images are available in
  the registry for both architectures.
- **SC-003**: 100% of published images have a verifiable signature and an SBOM. Zero `latest` or
  other moving tags exist in the registry.
- **SC-004**: An operator new to the repository deploys `prod` to a fresh remote cluster in under
  30 minutes, following only the docs and running no manual cluster commands.
- **SC-005**: Moving a stack to a different registry changes zero lines of code, chart templates
  or values files. Only stack configuration changes.
- **SC-006**: A change that breaks lint, tests or an image build is flagged on its pull request
  100% of the time, and nothing is ever published from a pull request.

## Assumptions

- A first release tag (e.g. `v0.1.0`) is cut once the workflow exists, so that `prod` has a
  released image to pin.
- The maintainer has a remote Kubernetes cluster for `prod` and its kubeconfig context. The spec
  doesn't depend on which provider runs it. The cluster runs a Kubernetes version that supports
  the `restricted` pod security level the stacks already enforce.
- Read access to the private ghcr.io packages uses a token with package read permission that the
  operator creates. Creating and rotating it is documented but not automated.
- Image tags and chart versions stay independent. Chart `version`/`appVersion` are still bumped by
  hand, and stacks that pull from a registry always set the image tag explicitly.
- Pull-credential config applies to every release namespace in a stack. Per-release credentials
  are not needed, since both apps come from the same registry.
- Verifying signatures at deploy time (admission policy) is out of scope. The docs show manual
  verification only.
- The workflow publishes images only. It doesn't deploy to any cluster.
- Automatic refresh of short-lived registry credentials (e.g. ECR) is out of scope.
- Every published image is kept forever, so any `sha-<short>` stays deployable. Cleaning up the
  registry is out of scope.
- No extra example stack is committed. `dev` (kind) and `prod` (remote) together show both paths.
