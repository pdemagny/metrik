# Security policy

## Supported versions

Security fixes land on `main` and ship in the next release. Only the latest release is supported.

## Reporting a vulnerability

Please do not open a public issue or pull request for a vulnerability. Report it privately through
[GitHub's private vulnerability reporting](https://github.com/pdemagny/metrik/security/advisories/new),
with the affected version or commit, the steps to reproduce, and the impact you expect.

Metrik is maintained on a best-effort basis: expect an acknowledgement within a week. Once a fix is
released, the advisory is published with credit to the reporter, unless you prefer otherwise.

## Scope

In scope: the `metrik` application, its Helm chart, the Pulumi program and the images published to
`ghcr.io/pdemagny`.

Out of scope: the Demo API's intentionally slow and failing endpoints, which exist to exercise
Metrik and are not meant to run in production.

## Verifying images

Every published image is signed keyless by the repository's `images` workflow and carries SBOM
attestations. See [Verify an image](../docs/metrik-deployment.md#verify-an-image).
