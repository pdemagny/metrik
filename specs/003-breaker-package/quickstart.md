# Quickstart: Validate the Breaker Package Move

Run everything from the repository root unless noted otherwise.

## 1. The breaker stands alone (US1, SC-002, FR-002)

```shell
(cd apps/metrik && go test -race -count=1 ./pkg/breaker/)
(cd apps/metrik && go list -deps ./pkg/breaker | grep -E '^metrik/|zap|viper|cobra')
```

Expected:
- The tests pass, and they cover every transition in [the 002 data model](../002-endpoint-circuit-breaker/data-model.md): opens at the threshold, success resets, trial once, recovery, reopen, stale results ignored, pruning.
- The `go list` line prints only `metrik/pkg/breaker` itself: no `metrik/cmd`, no `metrik/pkg/check`, no zap, viper or cobra.

## 2. Nothing breaker-related is left in `cmd` (SC-004)

```shell
ls apps/metrik/cmd/breaker*.go 2>/dev/null
grep -nE 'circuitState|type circuits?\b|func newCircuits|func circuitKey' apps/metrik/cmd/*.go
```

Expected: no output from either command.

## 3. Operators see no difference (US2, SC-001, SC-003, SC-005)

```shell
mise run lint
mise run test
```

Expected: `0 issues.` in every module, and every test passes.

Then replay [feature 002's quickstart §2](../002-endpoint-circuit-breaker/quickstart.md) (outage, suspension, failed trial, recovery, breaker disabled, negative cool-down on reload).

Expected: the same sequence of entries, levels and fields as in [002's log-events contract](../002-endpoint-circuit-breaker/contracts/log-events.md), and the same number of `ALERT` entries.

## 4. No new dependency (SC-005)

```shell
git diff --stat -- apps/metrik/go.mod apps/metrik/go.sum
```

Expected: no changes.

## 5. Gaps recorded (US3, SC-006)

```shell
grep -nE '^func|^type|^var' apps/metrik/cmd/*.go | grep -v _test
```

Expected: everything listed is either CLI code (`Execute`, `NewRunCmd`, `NewVersionCmd`, the root command and its variables) or appears in [plan.md → Complexity Tracking](plan.md#complexity-tracking).
