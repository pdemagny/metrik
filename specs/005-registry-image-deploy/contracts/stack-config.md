# Contract: Pulumi Stack Configuration

This is the operator-facing configuration of the `metrik` Pulumi project. Fields and validation
rules are in [data-model.md](../data-model.md).

## Local stack (`dev`): unchanged

```yaml
config:
  kubernetes:context: kind-dev
  metrik:releases:
    - name: metrik
      namespace: metrik
      chartPath: "../apps/metrik/deploy/chart/metrik/"
      chartValuesFilePath: "../apps/metrik/configs/dev-values.yaml"  # image: kind.local/metrik
      chartVersion: "0.1.0"
```

There is no `image` and no registry key, so the image comes from the values file and no pull
secret is used.

## Remote stack (`prod`): ghcr.io, mode a (created)

```yaml
config:
  kubernetes:context: <remote cluster context>
  metrik:registryServer: ghcr.io
  metrik:registryUsername: pdemagny
  metrik:registryPassword:
    secure: <ciphertext from `pulumi config set --secret metrik:registryPassword`>
  metrik:releases:
    - name: metrik
      namespace: metrik
      chartPath: "../apps/metrik/deploy/chart/metrik/"
      chartValuesFilePath: "../apps/metrik/configs/prod-values.yaml"
      chartVersion: "0.1.0"
      image:
        repository: ghcr.io/pdemagny/metrik
        tag: "0.1.0"
        # digest: sha256:...   # optional pin
```

## Other modes

```yaml
# Mode b (existing): a secret that already exists in every release namespace
metrik:imagePullSecret: my-registry-secret

# Mode c (none): nodes pull with cloud identity (GKE, EKS): set no registry key at all
```

## Guarantees

- **No image override without `image`.** A release without `image` takes its image from its
  values file. The stack's pull-credential mode still applies to it: in modes a and b, it gets
  `imagePullSecrets`.
- **The stack wins.** A release with `image` runs exactly `repository:tag`, or
  `repository:tag@digest`, whatever the values file says.
- **The password stays secret.** It is never in plain text in the stack file, state or output.
- **Invalid config changes nothing.** It fails at preview, with the rule and the release named.
- **Unpullable images fail fast.** A release whose pods can't become ready fails within 120 s,
  naming `<stack>-<release>`.
