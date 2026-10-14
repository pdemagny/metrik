# Data Model: Registry Image Deployment

There is no runtime data. The model is the Pulumi stack configuration and the images published
in the registry.

## Stack configuration

### Release (existing, extended)

One entry of `metrik:releases`.

| Field | Type | Required | Notes |
| :--- | :--- | :--- | :--- |
| `name` | string | yes | Existing |
| `namespace` | string | yes | Existing. Deployed as `<stack>-<namespace>` |
| `chartPath` | string | yes | Existing |
| `chartValuesFilePath` | string | yes | Existing |
| `chartVersion` | string | yes | Existing |
| `image` | Image | no | **New.** When absent, the values file sets the image (the `dev` path) |

### Image (new)

| Field | Type | Required | Notes |
| :--- | :--- | :--- | :--- |
| `repository` | string | yes | e.g. `ghcr.io/pdemagny/metrik`, with no tag or digest |
| `tag` | string | yes | e.g. `0.1.0`, `sha-1a2b3c4` |
| `digest` | string | no | `sha256:<64 hex>`. When set, the image reference is `repository:tag@digest` |

Effective reference passed to the chart: `image.repository = repository`, and `image.tag = tag`,
or `tag + "@" + digest` when a digest is set.

### Pull credentials (new, per stack)

| Key | Type | Notes |
| :--- | :--- | :--- |
| `metrik:registryServer` | string | e.g. `ghcr.io`, `https://index.docker.io/v1/` |
| `metrik:registryUsername` | string | |
| `metrik:registryPassword` | secret string | Set with `pulumi config set --secret` |
| `metrik:imagePullSecret` | string | Name of an existing secret in every release namespace |

Derived **mode**:
- **a (created)**: server, username and password are all set. A Secret named
  `registry-credentials` of type `kubernetes.io/dockerconfigjson` is created in each release
  namespace and referenced by the release.
- **b (existing)**: `imagePullSecret` is set. The release references that name.
- **c (none)**: none of the keys is set. No reference is added.

### Validation rules

The program checks these before registering any resource, so the deploy fails at preview.

| # | Rule | Error mentions |
| :--- | :--- | :--- |
| V1 | `image.repository` must be non-empty when `image` is present | release name |
| V2 | `image.tag` must be non-empty when `image` is present | release name |
| V3 | `image.repository` must contain no `@`, and no `:` after its last `/` | release name |
| V4 | `image.digest`, if set, must match `^sha256:[0-9a-f]{64}$` | release name |
| V5 | server, username and password must be all set or all unset | which keys are missing |
| V6 | mode a (created) and mode b (existing) must not both be set | both modes |

A digest set without a repository fails V1. This matches the spec edge case "partial image
config".

## Published image

| Attribute | Value |
| :--- | :--- |
| Name | `ghcr.io/<owner>/metrik`, `ghcr.io/<owner>/demo-api` |
| Platforms | `linux/amd64`, `linux/arm64` (OCI index) |
| Tags | `sha-<7 hex>` (from `main`), `X.Y.Z` (release, same digest) |
| Signature | Keyless cosign, identity `…/.github/workflows/images.yaml@refs/heads/main` |
| SBOM | SPDX JSON, signed in-toto attestation on the index digest and on each platform digest |
| Retention | Kept forever (clarification 3) |

### Lifecycle

```text
pull request ──► built (both platforms), discarded
push to main ──► published as sha-<short>, SBOM attested, signed
tag vX.Y.Z   ──► existing sha-<short> verified, then tagged X.Y.Z (same digest)
                 (sha-<short> missing → release run fails, nothing tagged)
other tags   ──► nothing
```
