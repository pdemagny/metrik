# Metrik

*Objective*:

Create an automated health check system for a web application that verifies the application's
availability and performance. The system should be capable of sending alerts when certain
thresholds are exceeded, suggesting potential issues with the service.

## Features

*Requirements*:

- ✅ Health Check Program: Write a program in Golang that performs HTTP GET requests to the
web application's health endpoint (e.g., /health or /status). The program should run at a
configurable interval (e.g., every 5 minutes).
- ✅ Metrics to Collect: Response time. HTTP status code. Optionally, parse the response body for
specific application health indicators (e.g., database connectivity status).
- ✅ Alerting Logic: If the response time exceeds a threshold (e.g., 3 seconds), log a warning. If the
HTTP status code is not 200, log an error. If any optional health indicators are in an error state,
log an error.
- ✅ Logging and Notification: Implement logging for all health checks, including timestamp,
endpoint, response time, and status. When an error is logged, trigger a simple alert mechanism.
This could be a mock function that simulates sending an email or posting a message to a Slack
channel.
- ✅ Configuration: Use a configuration file or environment variables to manage settings such as
the health endpoint URL, check interval, and response time threshold.
- ✅ Documentation: Provide a README file that explains how to set up, configure, and run the
health check system. Include any dependencies that need to be installed
- ✅ Create a helm chart to package your code.
- ✅ Deploy your code in a kubernetes cluster (local or remote using a cloud provider, though local
might be easier for testing) using the helm chart created before using an IaC solution such as
Terraform or Pulumi.

*Bonus Points*:

- ℹ️ Containerize the health check system using Docker -(compose).
- ✅ Implement a retry mechanism for transient failures.
- ✅ Make the program able to check multiple endpoints in parallel, if the system needs to scale.
- ✅ OpenAPI spec with your solution.
- ℹ️ Whatever else you would like to add to the solution, code or deployment wise.

## Roadmap

- Use OpenTelemetry to measure endpoints health, and send signals data to a backend such as Prometheus.

## Setup

### Tools

#### Asdf

If you are an `asdf` user, run this command to install all the tools:

```shell
while IFS= read -r line; do
    asdf plugin add "$(echo "${line}" | awk -F' ' '{print $1}')"
    asdf install "$(echo "${line}" | awk -F' ' '{print $1}')" "$(echo "${line}" | awk -F' ' '{print $2}')"
    asdf set "$(echo "${line}" | awk -F' ' '{print $1}')" "$(echo "${line}" | awk -F' ' '{print $2}')"
done <.tool-versions
```

#### Mise

If you are a `mise` user, run this command to install all the tools:

```shell
mise install
```

#### Manual

You can also install tools manually, using their official instructions for your platform:

| Link                                                                                                   | Required |
| :----------------------------------------------------------------------------------------------------- | :------: |
| [air](https://github.com/air-verse/air?tab=readme-ov-file#installation)                                |    ✅     |
| [dive](https://github.com/wagoodman/dive?tab=readme-ov-file#installation)                              |    ❌     |
| [golang](https://go.dev/doc/install)                                                                   |    ✅     |
| [golangci-lint](https://golangci-lint.run/docs/welcome/install/local/)                                 |    ✅     |
| [helm](https://helm.sh/docs/intro/install)                                                             |    ✅     |
| [k9s](https://github.com/derailed/k9s?tab=readme-ov-file#installation)                                 |    ✅     |
| [ko](https://github.com/ko-build/ko?tab=readme-ov-file#install-ko-and-get-started)                     |    ✅     |
| [kubectl](https://kubernetes.io/fr/docs/tasks/tools/install-kubectl/)                                  |    ✅     |
| [kubectx](https://github.com/ahmetb/kubectx?tab=readme-ov-file#installation)                           |    ✅     |
| [lazydocker](https://github.com/jesseduffield/lazydocker)                                              |    ❌     |
| [minikube](https://minikube.sigs.k8s.io/docs/start/?arch=%2Flinux%2Fx86-64%2Fstable%2Fbinary+download) |    ✅     |
| [pulumi](https://www.pulumi.com/docs/get-started/download-install/)                                    |    ✅     |
| [skaffold](https://skaffold.dev/docs/install/#standalone-binary)                                       |    ✅     |

### Clusters

Create two local Kubernetes clusters, one for development, one for production.

> [!WARNING]
> This is only for demonstration purposes, never use a minikube cluster in production.

Feel free to adjust the settings of these commands, including the resources and driver.

> [!IMPORTANT]
> If you do choose the Docker driver, then you need to have Docker installed on your machine ([Instructions](https://docs.docker.com/engine/install/)).

```shell
for mk in mk1 mk2; do
    minikube start -p ${mk} --kubernetes-version 1.37.1 --cpus 2 --memory 4GB --disk-size 10GB --driver docker --wait=all --wait-timeout=5m --preload=true && \
    minikube addons enable metrics-server -p ${mk}
done
```

### Pulumi

> [!IMPORTANT]
> This step is optional if you already have a Pulumi organization where you can create projects and deploy stacks.

You need to have a Pulumi Cloud account and be authenticated.

- Go to the [Pulumi Cloud](https://app.pulumi.com/signup) page and create an account, there is a Free Tier for individuals.
- Login to your new Pulumi account in your terminal:

  ```shell
  pulumi login
  ```

## Usage

> [!IMPORTANT]
> Throughout all this **Usage** guide, you don't have to leave the `metrik/` directory.

### Configure

Metrik reads `metrik.yaml` from `/config`, `./configs` or the current directory. See
[`apps/metrik/configs/metrik.yaml`](apps/metrik/configs/metrik.yaml) for a documented example.
Global settings can also be set with a flag or a `METRIK_`-prefixed environment variable. Checks
can override `alertThreshold`, `maxTries`, `requestTimeout`, `failureThreshold` and `cooldown`.

#### Circuit breaker

A check whose probes fail `failureThreshold` cycles in a row is skipped for `cooldown`, then
probed once. If that trial succeeds, the check is probed normally again. If it fails, the check is
skipped for another `cooldown`. Every failed probe still sends an alert; skipped cycles don't.

| Setting | Flag | Environment variable | Default |
|---|---|---|---|
| `failureThreshold` | `--failureThreshold` | `METRIK_FAILURETHRESHOLD` | `3` |
| `cooldown` | `--cooldown` | `METRIK_COOLDOWN` | `60s` |

Set `failureThreshold: 0` on a check (or globally) to probe it every cycle, without the circuit
breaker. Negative values are rejected at startup and on config reload.

### Develop

#### Local

For fast iteration, you can use `air` to get live reload for the two applications:

In a terminal, run:

```shell
(cd apps/api/ && air)
```

In another terminal, run:

```shell
(cd apps/metrik/ && air)
```

#### Lint and test

Lint and test every Go module (tests run with the race detector, which needs a C compiler):

```shell
mise run lint
mise run test
```

#### In-cluster

For fast iteration and condition closer to production, you can use `skaffold` to automate the develop+deploy loop:

```shell
kubectx mk1
skaffold dev
```

Once you are done developing, just press ctrl^C to stop the loop and cleanup deployment artifacts.

### Deploy

> [!IMPORTANT]
> Normally we would build images in Continuous Integration and store them in a registry, but for this demo we will build images directly into the minikube clusters.

#### Dev

```shell
# Point to the dev cluster
kubectx mk1
eval $(minikube docker-env -p mk1)
# Build images and store them in the dev cluster
(cd apps/api/ && ko build . --base-import-paths . -L --platform=linux/arm64 -t 0.1.0)
(cd apps/metrik/ && ko build . --base-import-paths . -L --platform=linux/arm64 -t 0.1.0)
# Deploy the charts in the dev cluster
(cd pulumi/ && pulumi up --stack dev --config-file Pulumi.dev.yaml)
```

#### Prod

```shell
# Point to the prod cluster
kubectx mk2
eval $(minikube docker-env -p mk2)
# Build images and store them in the prod cluster
(cd apps/api/ && ko build . --base-import-paths . -L --platform=linux/arm64 -t 0.1.0)
(cd apps/metrik/ && ko build . --base-import-paths . -L --platform=linux/arm64 -t 0.1.0)
# Deploy the charts in the prod cluster
(cd pulumi/ && pulumi up --stack prod --config-file Pulumi.prod.yaml)
```

### Cleanup

```shell
# Pulumi
export PULUMI_ORG=<NAME_OF_YOUR_PULUMI_ORG>
for stack in dev prod; do
    (cd pulumi/ && pulumi destroy --stack ${stack} && \
    pulumi stack rm --force --preserve-config ${PULUMI_ORG:?PULUMI_ORG must not be empty}/metrik/${stack})
done
pulumi logout

# Minikube
for mk in mk1 mk2; do
    minikube delete -p ${mk} && \
    eval $(minikube docker-env -p ${mk} --unset)
done
```

## References

- [logs](https://www.dash0.com/guides/logging-in-go-with-zap#triggering-actions-on-logs-with-hooks)
- [requests](https://www.digitalocean.com/community/tutorials/how-to-make-http-requests-in-go#best-practices-for-production-http-clients)
- [deamon](https://ieftimov.com/posts/four-steps-daemonize-your-golang-programs/#shut-down-on-sigtermsigint)
- [health](https://github.com/hellofresh/health-go/blob/master/health.go#L192)
- [api](https://mcorbin.fr/posts/2022-06-13-gin-golang/)
- [otel](https://opentelemetry.io/docs/concepts/instrumentation/code-based/)
