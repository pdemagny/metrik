# Contract: Published Images and Workflow

## Triggers and outcomes

| Event | Lint + test | Build (amd64 + arm64) | Pushes | Tags created |
| :--- | :--- | :--- | :--- | :--- |
| Pull request (including from forks) | yes | yes | no | none |
| Push to `main` | yes (gate) | yes | yes | `sha-<7 hex>` |
| Push of tag `vX.Y.Z` | no (reuses the checked `main` build) | no | tag only | `X.Y.Z` on the `sha-<7 hex>` digest |
| Any other tag (`v1.2`, `v1.2.0-rc.1`, …) | no | no | no | none |

The workflow never creates `latest`, `main` or any other moving tag. Digest-named
`sha256-<digest>.sig` / `.att` entries, if they appear, are cosign attachments, not image tags.

## Image references

```text
ghcr.io/pdemagny/metrik:sha-<7 hex>      ghcr.io/pdemagny/demo-api:sha-<7 hex>
ghcr.io/pdemagny/metrik:X.Y.Z            ghcr.io/pdemagny/demo-api:X.Y.Z
```

The packages are private. Pulling requires `read:packages`.

## Verification

```shell
ID='https://github.com/pdemagny/metrik/.github/workflows/images.yaml@refs/heads/main'
ISSUER='https://token.actions.githubusercontent.com'

cosign verify --certificate-identity "$ID" --certificate-oidc-issuer "$ISSUER" \
  ghcr.io/pdemagny/metrik:0.1.0

cosign verify-attestation --type spdxjson \
  --certificate-identity "$ID" --certificate-oidc-issuer "$ISSUER" \
  ghcr.io/pdemagny/metrik:0.1.0          # index SBOM; platform digests are attested too
```

## Failure messages

- Release on a commit with no `main` images: `image ghcr.io/pdemagny/<app>:sha-<short> not found;
  wait for the main run of <sha> and re-run`. Nothing is tagged.
- Lint or test failure: the `build` (pull request) or `publish` (`main`) job is skipped, and
  nothing is pushed.
