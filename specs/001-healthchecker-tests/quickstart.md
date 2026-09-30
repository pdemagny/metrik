# Quickstart: validating the healthchecker test suite

## Prerequisites

- Tools from `.mise.toml` (`mise install`). Run lint through `mise exec --`, because a golangci-lint v1 binary may shadow v2 on PATH.
- No network, cluster or running `apps/api` is needed.

## Run

```sh
cd apps/metrik
go test -race -count=1 ./...             # all tests; expect PASS, 3 SKIP (KD-1..3)
go test -race -count=1 -v ./... | grep -E 'SKIP|KNOWN DEFECT'   # list pinned defects
go test -cover ./pkg/check ./cmd         # SC-002: check ≥ 90 %, cmd logic ≥ 80 % (see note)
go test -race -count=20 ./...            # SC-004: no flakes
mise exec -- golangci-lint run ./...     # no findings beyond the baseline
```

## Expected outcomes

| Validation | Proves | Spec |
|------------|--------|------|
| `pkg/check` tests pass, total time < 10 s | every row of the verdict table in [data-model.md](./data-model.md) | US1, FR-001..004 |
| `TestResolve*` pass | override/fallback matrix | US2, FR-005 |
| `TestExampleConfigLoads` passes | `configs/metrik.yaml` decodes (3 checks, globals as documented) | FR-006 |
| `TestReport*` pass | severity table + alert hook only on ERROR | US3.1–4, FR-007 |
| `TestStartCycleBound` passes | peak in-flight ≤ `maxConcurrent`, every check probed once | US3.5, FR-008 |
| 3 tests SKIP with `KNOWN DEFECT KD-n` | divergences stay visible | FR-013, SC-006 |
| Wall time of `go test ./...` < 30 s | | SC-003 |

Coverage note: `cmd` coverage is measured over `resolve`, `report` and `startCycle`. `Execute` and the `RunE` tick loop are excluded (spec Assumptions), so read the per-function numbers with `go test -coverprofile=c.out ./cmd && go tool cover -func=c.out`.

## Mutation sanity check (SC-005)

Temporarily make one of these changes, confirm at least one test fails, then revert:

1. In `pkg/check/main.go`, replace `backoff.Permanent(errors.New("client error"))` with `errors.New("client error")`. The 4xx single-attempt test should fail.
2. In `startCycle`, size the limiter at `MaxConcurrent+1`. `TestStartCycleBound` should fail.
