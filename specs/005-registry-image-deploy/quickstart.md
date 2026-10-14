# Quickstart: Validating Registry Image Deployment

Commands run from the repository root. Image and stack details are in
[contracts/published-images.md](contracts/published-images.md) and
[contracts/stack-config.md](contracts/stack-config.md).

## Prerequisites

- Tools from `.mise.toml` (`mise install`), including the new `cosign` and `crane`.
- `pulumi login`, a remote Kubernetes cluster context, and a classic PAT with `read:packages`.
- For US3: Docker and kind.

## Static checks (every story)

```shell
mise run lint
mise run test        # includes the new table tests in pulumi/internal/stack
```

Expected: both pass in all three modules.

## US1: images are published

1. Open a pull request. Expected: `check` and both `build` jobs pass, and no new package version
   appears in ghcr.io.
2. Merge it. Expected: the `publish` jobs run, and both packages gain `sha-<7 hex>` within 20
   minutes (SC-002).
   ```shell
   crane manifest ghcr.io/pdemagny/metrik:sha-<short> | jq '.manifests[].platform'
   # → linux/amd64 and linux/arm64
   cosign verify --certificate-identity "$ID" --certificate-oidc-issuer "$ISSUER" \
     ghcr.io/pdemagny/metrik:sha-<short>
   cosign verify-attestation --type spdxjson --certificate-identity "$ID" \
     --certificate-oidc-issuer "$ISSUER" ghcr.io/pdemagny/metrik:sha-<short>
   ```
3. `git tag v0.1.0 && git push origin v0.1.0`. Expected: `crane digest …:0.1.0` equals
   `crane digest …:sha-<short>` for both apps.
4. Push `v0.1.1-rc.1`. Expected: no `release` job publishes anything.
5. Tag a commit that `main` never published, then push the tag. Expected: the release job fails
   with the "not found" message, and no tag is created.
6. `crane ls ghcr.io/pdemagny/metrik`. Expected: only `sha-*` and `X.Y.Z` tags, plus digest-named
   cosign attachments, if any. No `latest` and no `main`.

## US2: prod deploys from ghcr.io

```shell
cd pulumi
pulumi config set --stack prod --secret metrik:registryPassword "$GHCR_READ_PAT"
pulumi up --stack prod --config-file Pulumi.prod.yaml
kubectl get pods -n prod-metrik -o jsonpath='{.items[*].spec.containers[*].image}'
# → ghcr.io/pdemagny/metrik:0.1.0
```

Negative checks, with the original config restored after each:
- Set both `metrik:imagePullSecret` and the registry keys. Expected: preview fails, and nothing
  changes in the cluster.
- Remove `image.tag` from one release. Expected: preview fails, naming that release.
- Set `image.tag: "9.9.9"`. Expected: `pulumi up` fails within about 2 minutes, naming
  `prod-metrik`.
- Add a `digest`. Expected: the pod image is `…:0.1.0@sha256:…`.

## US3: dev on kind is unchanged

Without any registry login, follow the local section of `docs/metrik-deployment.md` on a fresh
`dev` kind cluster, then run `skaffold dev`. Expected: both apps run, and
`git diff main -- pulumi/Pulumi.dev.yaml apps/*/configs/dev-values.yaml skaffold.yaml` is empty.

## US4: docs

A reader who doesn't know the repository follows the remote walkthrough in
`docs/metrik-deployment.md` without help. Expected: it covers the PAT, stack keys, tag choice,
digest pinning, deploy, signature verification, and notes for GAR, ECR (12 h tokens) and Docker
Hub. `mise run docs` builds.
