# Quickstart: Validate the Demo API Rename

Run everything from the repository root unless noted otherwise. Names come from
[data-model.md](data-model.md); contract values from
[contracts/demo-api-identity.md](contracts/demo-api-identity.md).

## 1. The service calls itself a Demo API (US1, FR-001, FR-002, FR-004, SC-001)

```shell
grep -rniI 'metrik' apps/demo-api
(cd apps/demo-api && air) &   # or: go run ./cmd/demo-api
curl -s localhost:8080/openapi | jq '.info, .tags'
diff <(curl -s localhost:8080/openapi | jq -S .) <(jq -S . apps/demo-api/api/openapi.json)
```

Expected:
- `grep` prints nothing.
- `info.title` is `Demo API`; the tag description is `Demo API v1.`
- `diff` prints nothing.

## 2. The service stands alone and behaves as before (FR-003, FR-005, SC-006)

```shell
grep -n 'metrik' apps/demo-api/go.mod
mise run lint
mise run test
for p in .well-known/live slow .well-known/ready; do
  curl -s -o /dev/null -w "%{http_code} $p\n" "localhost:8080/v1/$p"
done
```

Expected:
- `grep` prints nothing.
- `0 issues.` for `apps/demo-api`, `apps/metrik` and `pulumi`; every test passes, including the
  new comparison of the contract file with the served contract.
- `200` for `live`, `200` for `slow` (after up to 4 seconds), `200` for `ready` (its body carries
  status `503` about half the time, as before).

## 3. No old name is left (US2, FR-006, FR-007, FR-012, SC-002)

```shell
ls apps
old='apps/api|cmd/api|chart/api|kind\.local/api|ko-app/api|(dev|prod)-api|IMAGE_[A-Z]+_api|"api\.|"api/internal|- api/internal|name: api$|namespace: api$|image: api$'
git grep -nE "$old" -- . ':!specs/00[1-3]-*' ':!specs/004-demo-api' ':!docs/metrik-deployment.md'
awk '/^## Upgrading from/{skip=1} /^## Cleanup/{skip=0} !skip' docs/metrik-deployment.md | grep -nE "$old"
```

Expected: `ls` shows `demo-api` and `metrik`; `git grep` and `grep` print nothing.

The second search covers the deployment page without its upgrade note, the one place that names
the old identifiers on purpose (FR-011).

## 4. Local run as the README describes it (US2 scenario 4, SC-004)

Follow README section "Run the Demo API and Metrik" word for word from a clean checkout.

Expected: both start without editing any command, and Metrik logs results for its four checks.

## 5. In-cluster (US2 scenarios 2 and 3, FR-008, SC-005)

```shell
helm lint apps/demo-api/deploy/chart/demo-api -f apps/demo-api/configs/dev-values.yaml
helm template dev-demo-api apps/demo-api/deploy/chart/demo-api -f apps/demo-api/configs/dev-values.yaml | grep -E '^  name:|image:'
```

Expected: lint passes; resources are named `dev-demo-api` and the image is
`kind.local/demo-api:0.1.0`.

Without a cluster, check that Metrik's shipped settings point at the new address and that the
Skaffold configuration resolves:

```shell
helm template dev-metrik apps/metrik/deploy/chart/metrik -f apps/metrik/configs/dev-values.yaml | grep -c 'dev-demo-api.dev-demo-api.svc'
helm template prod-metrik apps/metrik/deploy/chart/metrik -f apps/metrik/configs/prod-values.yaml | grep -c 'prod-demo-api.prod-demo-api.svc'
skaffold render -p dev --digest-source=tag | grep -E 'name: dev-demo-api|image:'
```

Expected: `4` and `4`; the render succeeds and shows resources named `dev-demo-api` with an image
ending in `demo-api`. If `skaffold render` cannot run here, `skaffold diagnose -p dev` must at
least succeed.

Then deploy the dev stack as in `docs/metrik-deployment.md` and run:

```shell
kubectl get pods --all-namespaces | grep -E 'demo-api|metrik'
kubectl get namespace dev-api
kubectl logs -n dev-metrik deploy/dev-metrik --tail 50
```

Expected:
- One running pod in `dev-demo-api` and one in `dev-metrik`.
- `dev-api` is not found (Pulumi removed it during the same `pulumi up`).
- Metrik's log shows the same mix as before: liveness passing, slow checks warning or timing out
  now and then, readiness failing about half the time. No connection or name-resolution errors.

## 6. Documentation (US3, FR-009, FR-011, SC-003)

```shell
grep -nE '[Dd]emo API|the API' README.md docs/*.md
mise run docs
```

Expected:
- Every mention of the service reads "Demo API" (capital D); no "the API" refers to it.
- The site builds; the architecture diagram shows `Deployment demo-api` as a separate box probed
  by Metrik; the deployment page has the upgrade note (old and new names, new address, cleanup
  commands).

## 7. Constitution (FR-010)

```shell
grep -n 'apps/api' .specify/memory/constitution.md
```

Expected: nothing; the version line reads `1.2.3`.
