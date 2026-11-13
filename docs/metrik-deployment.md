---
title: Deployment
weight: 30
---

# Deployment

Metrik ships as a container image (built with [ko](https://ko.build)) and a Helm chart, deployed
by a Pulumi program. The repository has two stacks:

- `dev` runs on a local [kind](https://kind.sigs.k8s.io) cluster. Images are built on your machine
  and loaded straight onto its nodes, so no registry is involved.
- `prod` runs on a remote Kubernetes cluster and pulls signed images published to ghcr.io by the
  repository's `images` workflow.

The image source is stack configuration only. The same charts and values files work with any
registry. All commands run from the repository root.

## Prerequisites

- The tools of the [getting started](../README.md#getting-started) section (`mise install`).
- A Pulumi Cloud account (free tier available) and `pulumi login`. Skip this if you already have
  an organization where you can create projects and stacks.
- For `dev`: Docker (kind runs its nodes as containers).
- For `prod`: a kubeconfig context for the remote cluster, and read access to the registry.

## Local deployment (dev on kind)

> [!WARNING]
> kind is for demonstration only, never use it in production.

### 1. Create the cluster

```shell
kind create cluster --name dev --image kindest/node:v1.37.0 --wait 5m
```

### 2. Build and deploy

The stack deploys both the Demo API and Metrik, so build both images. `KO_DOCKER_REPO=kind.local`
tells ko to load them onto the kind nodes instead of pushing them.

```shell
kubectx kind-dev
export KO_DOCKER_REPO=kind.local KIND_CLUSTER_NAME=dev
(cd apps/demo-api/ && ko build ./cmd/demo-api --base-import-paths -t 0.1.0)
(cd apps/metrik/ && ko build ./cmd/metrik --base-import-paths -t 0.1.0)
(cd pulumi/ && pulumi up --stack dev --config-file Pulumi.dev.yaml)
```

Pulumi creates one namespace per release (`<stack>-<namespace>`, with the `restricted` pod
security level enforced) and one Helm release per entry of the stack's `metrik:releases`:

```yaml
config:
  kubernetes:context: kind-dev
  metrik:releases:
    - name: metrik
      namespace: metrik
      chartPath: "../apps/metrik/deploy/chart/metrik/"
      chartValuesFilePath: "../apps/metrik/configs/dev-values.yaml"
      chartVersion: "0.1.0"
      # image: ...   # optional, see "Remote deployment"; dev takes the image from its values file
```

Check the result and read the logs:

```shell
kubectl get pods --all-namespaces | grep -E 'demo-api|metrik'
kubectl logs -n dev-metrik deploy/dev-metrik --follow
```

## Remote deployment (prod from a registry)

`prod` deploys released images from the repository's public ghcr.io packages. See
[Published images](#published-images) for how they are built and tagged.

### 1. Get a read credential (private packages only)

The ghcr.io packages are public: pulling them needs no credential, so leave out the
`metrik:registry*` keys below and skip this step ([mode c](#pull-credential-modes)).

Images in private packages, for example published from a private fork, need a GitHub personal
access token **(classic)** with only the `read:packages` scope. GitHub Packages does not accept
fine-grained tokens for pulling images.

### 2. Configure the stack

`pulumi/Pulumi.prod.yaml` holds everything about where images come from:

```yaml
config:
  kubernetes:context: <your remote cluster context>
  metrik:registryServer: ghcr.io           # private packages only
  metrik:registryUsername: <your GitHub user>  # private packages only
  metrik:releases:
    - name: metrik
      namespace: metrik
      chartPath: "../apps/metrik/deploy/chart/metrik/"
      chartValuesFilePath: "../apps/metrik/configs/prod-values.yaml"
      chartVersion: "0.1.0"
      image:
        repository: ghcr.io/pdemagny/metrik
        tag: "0.1.0"
        # digest: sha256:...   # optional: pin the exact image
```

For private packages, store the token encrypted in the stack, never in plain text:

```shell
(cd pulumi/ && pulumi config set --stack prod --secret metrik:registryPassword "<token>")
```

`image` overrides the image of the values file. `tag` is a release (`X.Y.Z`) or a `main` build
(`sha-<7 hex>`). Adding `digest` pins the exact image even if a tag were ever moved. Find it with:

```shell
crane digest ghcr.io/pdemagny/metrik:0.1.0
```

### 3. Deploy

```shell
(cd pulumi/ && pulumi up --stack prod --config-file Pulumi.prod.yaml)
kubectl get pods -n prod-metrik -o jsonpath='{.items[*].spec.containers[*].image}'
```

### Pull-credential modes

A stack uses one of three modes. The mode applies to every release in the stack.

| Mode | Stack keys | Effect |
| :--- | :--- | :--- |
| a (created) | `metrik:registryServer`, `metrik:registryUsername`, `metrik:registryPassword` (secret) | Pulumi creates a `registry-credentials` pull secret in each release namespace and references it |
| b (existing) | `metrik:imagePullSecret: <name>` | Releases reference a pull secret that already exists in each namespace |
| c (none) | none of the above | No pull secret: public images, or nodes that pull with their own cloud identity |

### When it fails

- **Invalid configuration** fails at `pulumi preview`, before anything changes in the cluster,
  naming the release or keys. Examples: an `image` without `tag`, a `repository` that contains a
  tag or digest, a malformed `digest`, only some of the three credential keys, or both modes a
  and b.
- **Images that cannot be pulled**, because of a wrong tag or bad credentials, fail the release
  after 2 minutes with the release name (`prod-metrik`) in the error. Inspect the pod events with
  `kubectl describe pod -n prod-metrik`.

## Published images

The `images` workflow (`.github/workflows/images.yaml`) builds both images with ko for
`linux/amd64` and `linux/arm64`:

| Event | Lint + test | Build | Push | Tags |
| :--- | :--- | :--- | :--- | :--- |
| Pull request | yes | yes | no | none |
| Push to `main` | yes, must pass | yes | yes | `sha-<7 hex>` |
| Tag `vX.Y.Z` | no (reuses the `main` build) | no | tag only | `X.Y.Z`, same digest as `sha-<7 hex>` |
| Any other tag, including `vX.Y.Z-rc.N` | no | no | no | none |

- There is **no `latest` tag**, and no other tag that moves. To try a release candidate, pin its
  commit's `sha-<7 hex>`.
- A release tag only works on a commit that `main` already published. Otherwise the release job
  fails with `image … not found`. Re-run it after the `main` run finishes.
- Image tags and chart versions are independent. Stacks that pull from a registry always set the
  image tag.
- All images are kept, so any `sha-<7 hex>` stays deployable.
- Tags named `sha256-<digest>.sig`, `.att` or `.sbom` are signatures, attestations and SBOMs
  attached to an image, not image tags.
- The packages are public. A new package starts private: after its first publish, set its
  visibility once in the package's settings on GitHub.

### Verify an image

Every image is signed keyless by the workflow, and carries signed SPDX SBOM attestations (on the
multi-platform index and on each platform image):

```shell
ID='https://github.com/pdemagny/metrik/.github/workflows/images.yaml@refs/heads/main'
ISSUER='https://token.actions.githubusercontent.com'
cosign verify --certificate-identity "$ID" --certificate-oidc-issuer "$ISSUER" \
  ghcr.io/pdemagny/metrik:0.1.0
cosign verify-attestation --type spdxjson \
  --certificate-identity "$ID" --certificate-oidc-issuer "$ISSUER" \
  ghcr.io/pdemagny/metrik:0.1.0
```

## Other registries

The stack does not depend on ghcr.io. To use another registry, push the images there with ko,
then change only `image.repository` and the credential keys. For example:

```shell
export KO_DOCKER_REPO=<registry prefix below>
(cd apps/metrik/ && ko build ./cmd/metrik --base-import-paths --platform=linux/amd64,linux/arm64 -t 0.1.0)
```

Signing is up to you there: the workflow only signs what it publishes to ghcr.io.

### Docker Hub

- **Repository**: `docker.io/<user>/metrik`, so `KO_DOCKER_REPO=docker.io/<user>`.
- **Mode a**: `registryServer: https://index.docker.io/v1/`, your username, and a read-only
  [access token](https://docs.docker.com/security/for-developers/access-tokens/) as the password.
  The token lasts until you revoke it.

### Google Artifact Registry

- **Repository**: `<region>-docker.pkg.dev/<project>/<repository>/metrik`.
- **Preferred, mode c**: on GKE, give the node service account (or the Workload Identity of the
  namespace) the *Artifact Registry Reader* role. No pull secret is needed.
- **Mode a**: `registryServer: <region>-docker.pkg.dev`, `registryUsername: _json_key`, and a
  service-account JSON key as the password. The key lasts until it is deleted, so rotate it.

### Amazon ECR

- **Repository**: `<account>.dkr.ecr.<region>.amazonaws.com/metrik`.
- **Preferred, mode c**: on EKS, attach `AmazonEC2ContainerRegistryReadOnly` to the node role.
  No pull secret is needed.
- **Mode a only for short tests**: `registryUsername: AWS`, and the password from
  `aws ecr get-login-password`. **It expires after 12 hours**, after which new pods fail to pull.
  Nothing refreshes it automatically.

## The Helm chart

The chart is in `apps/metrik/deploy/chart/metrik`. It renders a Deployment, a ServiceAccount and,
optionally, a ConfigMap holding `metrik.yaml`. Mount that ConfigMap at `/config`, where Metrik
looks for its configuration (see [Configuration](metrik-configuration.md)).

### Values

| Value                    | Default        | Description                                                 |
| :----------------------- | :------------- | :---------------------------------------------------------- |
| `replicaCount`           | `1`            | Number of pods. Each replica runs every check.              |
| `image.repository`       | _empty_        | Image repository (required).                                |
| `image.tag`              | chart appVersion | Image tag.                                                |
| `image.pullPolicy`       | `IfNotPresent` | Image pull policy.                                          |
| `imagePullSecrets`       | `[]`           | Registry credentials.                                       |
| `command`                | `[]`           | Container command, for ko images `["/ko-app/metrik", "run"]`. |
| `configmap.create`       | `false`        | Create the ConfigMap from `configmap.data`.                 |
| `configmap.name`         | _empty_        | ConfigMap name; defaults to the release full name.          |
| `configmap.data`         | `null`         | Map of file name to content, e.g. `metrik.yaml`.            |
| `serviceAccount.create`  | `true`         | Create a ServiceAccount.                                    |
| `serviceAccount.automount` | `true`       | Automount the API token.                                    |
| `serviceAccount.annotations` / `.name` | `{}` / _empty_ | Annotations and name override.                |
| `podAnnotations`, `podLabels` | `{}`      | Extra pod metadata.                                         |
| `podSecurityContext`, `securityContext` | `{}` | Pod and container security contexts.               |
| `resources`              | `{}`           | Requests and limits.                                        |
| `livenessProbe`, `readinessProbe` | `{}`  | Kubernetes probes (Metrik exposes no port, leave unset).    |
| `volumes`, `volumeMounts` | `[]`          | Extra volumes, normally the ConfigMap mounted at `/config`. |
| `nodeSelector`, `tolerations`, `affinity` | `{}` / `[]` / `{}` | Scheduling.                      |
| `nameOverride`, `fullnameOverride` | _empty_ | Name overrides.                                          |

### Example values

This is `apps/metrik/configs/dev-values.yaml` (checks shortened): the ConfigMap is created by the
chart, mounted read-only, and the container is locked down.

```yaml
image:
  repository: kind.local/metrik
  tag: 0.1.0
command: ["/ko-app/metrik", "run"]
configmap:
  create: true
  data:
    metrik.yaml: |
      runInterval: "15s"
      maxConcurrent: 10
      checks:
        - name: "liveness ok"
          endpoint: "http://dev-demo-api.dev-demo-api.svc.cluster.local:8080/v1/.well-known/live"
volumes:
  - name: metrik-config
    configMap:
      name: dev-metrik # the release name, here <stack>-<release>
volumeMounts:
  - name: metrik-config
    mountPath: "/config"
    readOnly: true
securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
  readOnlyRootFilesystem: true
  runAsNonRoot: true
  runAsUser: 1000
  seccompProfile:
    type: RuntimeDefault
resources:
  requests: { cpu: 5m, memory: 32M }
  limits: { memory: 32M }
```

> [!NOTE]
> The mounted ConfigMap name must match the one the chart creates: the release name, or
> `configmap.name` when set.

### Using the chart without Pulumi

On an existing cluster with an image in a registry, you can install the chart directly:

```shell
helm install metrik apps/metrik/deploy/chart/metrik \
  --namespace monitoring --create-namespace \
  --values my-values.yaml \
  --set image.repository=registry.example.com/metrik \
  --set image.tag=0.1.0
```

Point the `checks` of `my-values.yaml` at your own endpoints. To change checks later, update the
values and run `helm upgrade`; Metrik reloads the mounted file without a restart (the kubelet
refreshes ConfigMap mounts after a short delay).

## Inner loop with Skaffold

For fast iteration closer to production, Skaffold builds with ko and deploys both charts to the
`dev` cluster on every change:

```shell
kubectx kind-dev
skaffold dev
```

Press `Ctrl+C` to stop; Skaffold removes what it deployed.

## Upgrading from the `api` names

The Demo API used to be deployed as `api`. If an environment predates the rename:

- **Names**: the release and namespace `<stack>-api` are now `<stack>-demo-api`, and the image
  `kind.local/api` is now `kind.local/demo-api`. Rebuild the image with the command from
  [Build and deploy](#2-build-and-deploy).
- **Address**: the Demo API now answers at
  `http://<stack>-demo-api.<stack>-demo-api.svc.cluster.local:8080`. Update any Metrik settings of
  your own that still point at `<stack>-api.<stack>-api`.
- **Pulumi**: the next `pulumi up` creates the new release and namespace and deletes the old ones.
- **Skaffold or manual installs**: remove what is left by hand:

  ```shell
  helm uninstall dev-api -n dev-api
  kubectl delete namespace dev-api
  ```

## Cleanup

```shell
export PULUMI_ORG=<NAME_OF_YOUR_PULUMI_ORG>
for stack in dev prod; do
    (cd pulumi/ && pulumi destroy --stack "${stack}" && \
    pulumi stack rm --force --preserve-config "${PULUMI_ORG:?PULUMI_ORG must not be empty}/metrik/${stack}")
done
pulumi logout

kind delete cluster --name dev
```
