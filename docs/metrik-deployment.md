---
title: Deployment
weight: 30
---

# Deployment

Metrik ships as a container image (built with [ko](https://ko.build)) and a Helm chart, deployed
by a Pulumi program. The repository demonstrates this on two local [kind](https://kind.sigs.k8s.io)
clusters, `dev` and `prod`.

> [!WARNING]
> kind is for demonstration only, never use it in production.

> [!IMPORTANT]
> Normally images are built in CI and pushed to a registry. This guide builds them straight into
> the kind clusters. All commands run from the repository root.

## Prerequisites

- The tools of the [getting started](../README.md#getting-started) section, plus Docker (kind runs its
  nodes as containers).
- A Pulumi Cloud account (free tier available) and `pulumi login`. Skip this if you already have
  an organization where you can create projects and stacks.

## 1. Create the clusters

```shell
for cluster in dev prod; do
    kind create cluster --name "${cluster}" --image kindest/node:v1.37.0 --wait 5m
done
```

## 2. Build and deploy

The stack deploys both the demo API and Metrik, so build both images.

```shell
# Dev
kubectx kind-dev
export KO_DOCKER_REPO=kind.local KIND_CLUSTER_NAME=dev
(cd apps/demo-api/ && ko build ./cmd/demo-api --base-import-paths -t 0.1.0)
(cd apps/metrik/ && ko build ./cmd/metrik --base-import-paths -t 0.1.0)
(cd pulumi/ && pulumi up --stack dev --config-file Pulumi.dev.yaml)
```

For prod, repeat with `kind-prod`, `KIND_CLUSTER_NAME=prod`, `--stack prod` and
`Pulumi.prod.yaml`.

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
```

Check the result and read the logs:

```shell
kubectl get pods --all-namespaces | grep -E 'demo-api|metrik'
kubectl logs -n dev-metrik deploy/dev-metrik --follow
```

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
      runInterval: "10s"
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

## Cleanup

```shell
export PULUMI_ORG=<NAME_OF_YOUR_PULUMI_ORG>
for stack in dev prod; do
    (cd pulumi/ && pulumi destroy --stack "${stack}" && \
    pulumi stack rm --force --preserve-config "${PULUMI_ORG:?PULUMI_ORG must not be empty}/metrik/${stack}")
done
pulumi logout

for cluster in dev prod; do
    kind delete cluster --name "${cluster}"
done
```
