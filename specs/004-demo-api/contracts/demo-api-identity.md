# Contract: How the Demo API Identifies Itself

The service's HTTP contract is the OpenAPI document at `apps/demo-api/api/openapi.json`, also
served at `GET /openapi`. This feature changes only its identity fields and corrects one schema
name in the file. Routes, parameters, responses and status codes are untouched.

## Identity fields (changed)

| Field                 | Before           | After                                                                                 |
| :-------------------- | :--------------- | :------------------------------------------------------------------------------------ |
| `info.title`          | `Metrik API`     | `Demo API`                                                                            |
| `info.description`    | `Metrik API.`    | `Demo API with live, slow and flaky endpoints, meant as a target for health checks.`  |
| `tags[0].description` | `v1 metrik api.` | `Demo API v1.`                                                                        |

## Unchanged

| Field                         | Value                                                        |
| :---------------------------- | :----------------------------------------------------------- |
| `info.version`                | `0.1.0`                                                      |
| `tags[0].name`, operation tags | `v1 api`                                                    |
| Paths                         | `/v1/.well-known/live`, `/v1/.well-known/ready`, `/v1/slow`  |
| Operation ids                 | `getLive`, `getReady`, `getSlow`                             |
| Responses                     | `200` on all three; `503` also on `/v1/.well-known/ready`    |
| Response body                 | `message` (required), `status` (required), `reason`          |

## File correction (research R7)

The served document names the response schema `RouterAPIResponse`. The file is corrected to use
that name in `components.schemas` and in its four `$ref`s. The served document does not change.

## Guarantees

1. The served document and the file are equal once both are parsed (key order and whitespace do
   not matter). A test in the router package enforces this.
2. Neither contains "metrik" in any letter case.
3. In-cluster, the service answers at
   `http://<stack>-demo-api.<stack>-demo-api.svc.cluster.local:8080`, where `<stack>` is `dev` or
   `prod`. Locally it answers at `http://localhost:8080`, as before.
