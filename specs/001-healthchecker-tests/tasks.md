---

description: "Task list for the healthchecker automated tests"
---

# Tasks: Healthchecker Automated Tests

**Input**: Design documents from `specs/001-healthchecker-tests/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/test-seams.md, quickstart.md

**Tests**: this feature *is* a test suite, so the test tasks are the implementation. The only production changes are the behavior-preserving extractions in `apps/metrik/cmd/run.go` ([contracts/test-seams.md](./contracts/test-seams.md)) and one lint-config line.

**Organization**: tasks are grouped by user story, so each story can be implemented and tested on its own.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)

## Conventions for every task

- Paths are relative to the repository root. Go commands run from `apps/metrik`, and lint runs as `mise exec -- golangci-lint run --fix=false ./...` (a golangci-lint v1 on PATH rejects the v2 config; `.golangci.yaml` sets `fix: true`, which would otherwise rewrite unrelated code).
- Tests live **in the same package** as the code (`package check`, `package cmd`), not `_test` packages.
- Lint rules that bite tests: **thelper** (helpers take `t *testing.T` first and call `t.Helper()` first); **tparallel** (if a subtest calls `t.Parallel()`, its parent must too); **godot** (comments end with `.`); **depguard** (imports only from stdlib, `go.uber.org/zap/...`, `github.com/spf13/viper`, `github.com/cenkalti/backoff/v5`, `metrik/...` after T001); **errcheck** with `check-blank` (no `_ =` on errors; check `w.Write`/`json.Encoder.Encode` errors or use `t.Errorf`); **noctx**.
- Use `zap.NewNop()` when logs are not asserted; use `go.uber.org/zap/zaptest/observer` when they are.
- Known-defect tests begin with `t.Skip("KNOWN DEFECT KD-n: <one-line reason>")`, and their body asserts the **intended** behavior.
- Lint baseline before this feature (commit 9ccb748): **13 findings** (depguard 5, gocritic 4, revive 4). Success means no finding that isn't in the baseline.
- Do not commit. Stop for maintainer review (AGENTS.md).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: unblock `cmd` test imports under the strict depguard allow-list.

- [X] T001 In `.golangci.yaml`, under `linters.settings.depguard.rules.main.allow`, add two entries right after `- metrik/cmd`: `- metrik/pkg` and `- metrik/internal`. Verify with `cd apps/metrik && mise exec -- golangci-lint run ./...` that the depguard findings on `metrik/pkg/check`, `metrik/internal/context` and `metrik/internal/logger` are gone and no new finding appears (the `github.com/fsnotify/fsnotify` depguard finding remains; out of scope).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: nothing blocks *all* stories. US1 needs no setup; US2 and US3 need T001 only. The `run.go` extractions are placed in the story that first needs them.

**Checkpoint**: after T001, all story phases may start.

---

## Phase 3: User Story 1 - Verify the outcome of a single health probe (Priority: P1) 🎯 MVP

**Goal**: every probe verdict in the [data-model.md](./data-model.md) verdict table is pinned by a test against `Check.Run`, including attempt counts, retry timing and cancellation.

**Independent Test**: `cd apps/metrik && go test -race -count=1 -cover ./pkg/check` → PASS, 1 SKIP (KD-1), coverage ≥ 90 %, wall time < 10 s. No `cmd` changes needed.

All US1 tasks edit `apps/metrik/pkg/check/main_test.go` (new file, `package check`) and must run in order. Every top-level test and every subtest calls `t.Parallel()` (research R2: real 500 ms backoff, kept cheap by parallelism and `maxTries ≤ 3`).

- [X] T002 [P] [US1] Create `apps/metrik/pkg/check/main_test.go` with shared helpers: (a) `newServer(t *testing.T, h func(w http.ResponseWriter, r *http.Request, hit int32)) (*httptest.Server, *atomic.Int32)` that wraps `h`, increments the counter at the start of each request (passing the 1-based hit number), and registers `t.Cleanup(srv.Close)`; (b) `writePayload(t *testing.T, w http.ResponseWriter, code int, p APIResponse)` that sets `Content-Type: application/json`, calls `WriteHeader(code)` and JSON-encodes `p`, reporting encode errors with `t.Errorf`; (c) `run(t *testing.T, ctx context.Context, url string, maxTries uint, timeout time.Duration) (*Result, error)` that builds `NewCheck("test", url, time.Second, maxTries)` and calls `Run(ctx, zap.NewNop(), &http.Client{Timeout: timeout})`. Every helper calls `t.Helper()` first.
- [X] T003 [US1] Add table-driven `TestRunVerdicts` in `apps/metrik/pkg/check/main_test.go`. Each case gives: handler, `maxTries`, expected hit count, expected `err == nil`, expected `Result.Code`, and expected `Result.Body` (nil or value). Cases:
  1. 200 + `{"message":"ok","status":200}`, maxTries 3 → 1 hit, nil error, Code 200, Body `{Message:"ok", Status:200}`.
  2. 404 plain text, maxTries 3 → 1 hit, error with message exactly `"client error"`, Code 404.
  3. 429 **without** `Retry-After`, maxTries 3 → 1 hit, `"client error"`, Code 429.
  4. 200 with body `not json`, maxTries 2 → 2 hits, non-nil error, Code 200.
  5. 200 with empty body, maxTries 2 → 2 hits, non-nil error, Code 200.
  6. 302 via `http.Redirect(w, r, "/elsewhere", http.StatusFound)`, maxTries 2 → 2 hits (redirects are not followed), non-nil error, Code 302.
  7. 500 with body `not json`, maxTries **1** → exactly 1 hit, non-nil error.
- [X] T004 [US1] Add `TestRunConnectionRefused` in `apps/metrik/pkg/check/main_test.go`: start `httptest.NewServer`, save its `URL`, `Close()` it immediately, then call `run` with maxTries 2 and a 1 s timeout. Assert: non-nil error, `errors.Is(err, syscall.ECONNREFUSED)`, and `res.Code == 0`.
- [X] T005 [US1] Add `TestRunTimeout` in `apps/metrik/pkg/check/main_test.go`: the handler blocks on `<-r.Context().Done()`; call `run` with maxTries 2 and a client timeout of 50 ms. Assert: 2 hits, non-nil error, and `var ne net.Error; errors.As(err, &ne) && ne.Timeout()`.
- [X] T006 [US1] Add `TestRunRecoversBeforeLimit` in `apps/metrik/pkg/check/main_test.go`: hits 1–2 answer 503 with body `not json`, hit 3 answers 200 + `{"message":"ok","status":200}`; maxTries 3. Assert: 3 hits, nil error, Code 200, `Body.Status == 200`.
- [X] T007 [US1] Add `TestRunRetryAfter` in `apps/metrik/pkg/check/main_test.go`: hit 1 answers 429 with header `Retry-After: 1`, hit 2 answers 200 + valid payload; record `time.Now()` per hit under a mutex. maxTries 3. Assert: 2 hits, nil error, and gap between hit 1 and hit 2 `>= 1*time.Second`.
- [X] T008 [US1] Add `TestRunStopsOnCancel` in `apps/metrik/pkg/check/main_test.go`: create `ctx, cancel := context.WithCancel(t.Context())`; the handler calls `cancel()` on hit 1 and answers 200 with body `not json` (a retryable decode error). maxTries 10. Assert: `errors.Is(err, context.Canceled)`, exactly 1 hit, and `Run` returned in under 250 ms (below the minimum randomized first backoff), measured with `time.Since`.
- [X] T009 [US1] Add `TestRunIsolatedBetweenCalls` in `apps/metrik/pkg/check/main_test.go`: one `Check`, server answers hit 1 with 200 + valid payload and hit 2 with 404. Call `Run` twice on the **same** `*Check` with maxTries 1. Assert the second result has Code 404 and `Body == nil` (no leak from the first call), and the first result is unchanged (Code 200).
- [X] T010 [US1] Add `TestRunServerErrorIsRetried` in `apps/metrik/pkg/check/main_test.go`, starting with `t.Skip("KNOWN DEFECT KD-1: 5xx with a decodable payload is accepted without retry (constitution IV)")`. Body: handler answers 503 + `{"message":"down","status":503}` every time; maxTries 3. Assert 3 hits and a non-nil error.

**Checkpoint**: `go test -race -count=1 -cover ./pkg/check` passes with 1 skip and ≥ 90 % coverage. US1 is deliverable alone.

---

## Phase 4: User Story 2 - Verify per-check settings override global defaults (Priority: P2)

**Goal**: the override/fallback rule and the example config are pinned by tests.

**Independent Test**: `cd apps/metrik && go test -race -count=1 -run 'TestResolve|TestExampleConfigLoads' ./cmd` → PASS; `go build ./...` still succeeds.

- [X] T011 [US2] In `apps/metrik/cmd/run.go`, extract `func (cfg Config) resolve(c *check.Check) *check.Check` per [contracts/test-seams.md](./contracts/test-seams.md). It returns a **new** `*check.Check` with `Name` and `Endpoint` copied, and `AlertThreshold`, `MaxTries` and `RequestTimeout` each taken from `c` if non-zero, else from `cfg`. It must not mutate `c`. In `RunE`, replace the three if/else blocks and the `check.NewCheck(...)` call (current lines 72–94) with `chk := runConfig.resolve(c)`, `client := createClient(chk.RequestTimeout)`, and use `chk` for `Run` and `chk.AlertThreshold` where `alertThreshold` was used. Renaming the local `check` to `chk` also removes the package-name shadowing. Behavior must be identical. Verify with `go build ./... && go vet ./...`.
- [X] T012 [US2] Create `apps/metrik/cmd/run_test.go` (`package cmd`) with table-driven `TestResolve`. Global `Config{AlertThreshold: 3*time.Second, MaxTries: 3, RequestTimeout: 5*time.Second}`. Cases: no override → (3s, 3, 5s); only `AlertThreshold: time.Second` → (1s, 3, 5s); only `MaxTries: 5` → (3s, 5, 5s); only `RequestTimeout: time.Second` → (3s, 3, 1s); all three overridden → check values. For each case, assert `Name`/`Endpoint` are copied, the result is a different pointer, and the input check is unchanged afterwards. Use `t.Parallel()` at both levels.
- [X] T013 [US2] Add `TestExampleConfigLoads` in `apps/metrik/cmd/run_test.go`: `v := viper.New(); v.SetConfigFile("../configs/metrik.yaml")`, `ReadInConfig()`, then `Unmarshal(&cfg)` into `Config`, and fail on any error. Assert: `Verbose == true`, `Debug == false`, `RunInterval == 10*time.Second`, `MaxConcurrent == 10`, `RequestTimeout == 5*time.Second`, `AlertThreshold == 3*time.Second`, `MaxTries == 3`, and `len(Checks) == 3`. The checks are: `[0]` name `"liveness slow warn"`, endpoint `"http://localhost:8080/v1/slow"`, `AlertThreshold == 3*time.Second`; `[1]` name `"liveness not found"`, endpoint `"http://localhost:8080/v1/.well-known/notfound"`; `[2]` name `"liveness connection refused"`, endpoint `""`. Put a comment on `[2]`: `// Endpoint is empty because the example config half-comments this check out.` Do **not** use the package-level `rootConfig`.

**Checkpoint**: US2 tests pass. The `run.go` change is behavior-preserving (build and vet are clean, and the diff only moves code).

---

## Phase 5: User Story 3 - Verify the run's classification, alerting and concurrency bound (Priority: P3)

**Goal**: severity, alert routing and the `maxConcurrent` invariant are pinned by tests, and KD-2/KD-3 are pinned as skips.

**Independent Test**: `cd apps/metrik && go test -race -count=1 -run 'TestReport|TestStartCycle' ./cmd` → PASS, 2 SKIP.

**Depends on**: T011 (`startCycle` calls `resolve`).

- [X] T014 [US3] In `apps/metrik/cmd/run.go`, extract `func report(logger *zap.Logger, alert func(zapcore.Entry) error, res *check.Result, err error, alertThreshold time.Duration)` from the goroutine body (from `alertLogger := zap.New(...)` through the `check completed` log). Keep messages, fields, levels and order byte-identical, and build the alert logger as `zap.New(logger.Core(), zap.Hooks(alert))`. The goroutine now calls `report(childlogger, notifyOnCritical, results, err, chk.AlertThreshold)` right after `<-limiterCh`. Verify with `go build ./... && go vet ./...`.
- [X] T015 [US3] In `apps/metrik/cmd/run.go`, extract `func startCycle(ctx context.Context, logger *zap.Logger, cfg Config, alert func(zapcore.Entry) error) *sync.WaitGroup` containing the whole body of the `case <-time.Tick(...)` branch: `wg := &sync.WaitGroup{}`, `limiterCh := make(chan bool, cfg.MaxConcurrent)`, `defer close(limiterCh)`, and the per-check loop (limiter send **before** `wg.Add(1)` and the goroutine; limiter receive right after `chk.Run`). The goroutine calls `report(..., alert, ...)`. **Drop** the no-op `go func(){ wg.Wait() }()` and `return wg`. The tick branch becomes `startCycle(ctx, logger, runConfig, notifyOnCritical)`, ignoring the return value, which preserves today's timing (research R5). This also removes the baseline gocritic `deferInLoop` finding. Verify with `go build ./... && go vet ./...`.
- [X] T016 [US3] In `apps/metrik/cmd/run_test.go`, add helpers: `newObserved(t *testing.T) (*zap.Logger, *observer.ObservedLogs)` using `observer.New(zap.DebugLevel)`, and `newAlertRecorder(t *testing.T) (func(zapcore.Entry) error, *atomic.Int32)`. The recorder func counts only entries with `Level >= zapcore.ErrorLevel` (mirroring `notifyOnCritical`) and returns nil. Both call `t.Helper()` first.
- [X] T017 [US3] Add table-driven `TestReport` in `apps/metrik/cmd/run_test.go`, with `alertThreshold = time.Second` for all cases. For each case, assert the exact ordered list of `(Message, Level)` from `logs.All()`, the key fields via `ContextMap()`, and the alert count. Cases:
  1. `res{Code:200, Duration:10ms, Body:&{Status:200}}`, err nil → `[("check completed", Info)]`, 0 alerts.
  2. Same but `Duration: 2*time.Second` → `[("slow response", Warn), ("check completed", Info)]`, 0 alerts.
  3. `Body:&{Status:503, Message:"down", Reason:"db"}` → `[("application error", Error)]` with fields `status=503`, `message="down"`, `reason="db"`, 1 alert.
  4. `res{Code:500}`, `err=errors.New("boom")` → `[("check failed", Error)]` with field `error="boom"`, 1 alert.
- [X] T018 [US3] Add `TestStartCycleBound` in `apps/metrik/cmd/run_test.go`: an `httptest` server whose handler increments an `atomic.Int32` in-flight counter, updates a max-seen value (CAS loop), records the hit path under a mutex, sleeps 50 ms, decrements, and answers 200 + `{"message":"ok","status":200}`. Use `Config{MaxConcurrent: 2, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second}` with 6 checks named `c0`..`c5` and endpoints `srv.URL+"/c0"`..`"/c5"`. Call `startCycle(t.Context(), logger, cfg, alert).Wait()`. Assert: max in-flight `<= 2`; each path hit exactly once (6 total); 6 `"check completed"` entries; 0 alerts.
- [X] T019 [US3] Add `TestReportNullBody` in `apps/metrik/cmd/run_test.go`, starting with `t.Skip("KNOWN DEFECT KD-2: a JSON null payload leaves Result.Body nil and report panics (constitution IV)")`. Body: call `report` with `res{Code:200, Body:nil}` and nil err. Assert no panic and exactly one Error-level entry and 1 alert.
- [X] T020 [US3] Add `TestReportNilResult` in `apps/metrik/cmd/run_test.go`, starting with `t.Skip("KNOWN DEFECT KD-3: an unpreparable request returns a nil Result and report panics (constitution IV)")`. Body: call `report` with `res == nil` and `err = errors.New("bad url")`. Assert no panic, one `"check failed"` Error entry, and 1 alert.

**Checkpoint**: `go test -race -count=1 ./cmd` passes with 2 skips; all three stories pass together.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: prove the success criteria and quality gates hold across the whole suite.

- [X] T021 Run `gofmt -l apps/metrik` (expect no output) and `cd apps/metrik && mise exec -- golangci-lint run ./...`. Compare against the 13-finding baseline: the only allowed findings are baseline ones not already cleared by T001/T015. Fix any new finding in `apps/metrik/pkg/check/main_test.go`, `apps/metrik/cmd/run_test.go` or `apps/metrik/cmd/run.go`.
- [X] T022 Run every command in [quickstart.md](./quickstart.md) from `apps/metrik`: `go test -race -count=1 ./...` (PASS, 3 SKIP, wall time < 30 s, SC-003); `go test -race -count=20 ./...` (no failures, SC-004); `go test -coverprofile=c.out ./pkg/check ./cmd && go tool cover -func=c.out` (`pkg/check` ≥ 90 %; `resolve`/`report`/`startCycle` ≥ 80 %, SC-002). Delete `c.out` afterwards.
- [X] T023 Mutation check (SC-005), reverting each change right after: (1) in `apps/metrik/pkg/check/main.go`, replace `backoff.Permanent(errors.New("client error"))` with `errors.New("client error")` → `TestRunVerdicts` case 2 must fail; (2) in `apps/metrik/cmd/run.go`, change the limiter capacity to `cfg.MaxConcurrent+1` → `TestStartCycleBound` must fail. Confirm with `git diff --stat` that both files are back to their post-T015 state.
- [X] T024 Update the known-defects table (R7) in `specs/001-healthchecker-tests/research.md` with each defect's test name: KD-1 → `TestRunServerErrorIsRetried`, KD-2 → `TestReportNullBody`, KD-3 → `TestReportNilResult`. Then include the KD list and the R7 observations in the final report to the maintainer (SC-006). Do not commit.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (T001)**: no dependencies. Needed by US2/US3 (the `cmd` test imports `metrik/pkg/check`); **not** needed by US1.
- **Foundational**: empty.
- **US1 (T002–T010)**: no dependencies; can start immediately, in parallel with T001.
- **US2 (T011–T013)**: after T001.
- **US3 (T014–T020)**: after T001 **and T011**. T014 → T015 → T016 → T017..T020.
- **Polish (T021–T024)**: after all desired stories.

### User Story Dependencies

- **US1 (P1)**: fully independent (separate package and file).
- **US2 (P2)**: independent of US1.
- **US3 (P3)**: needs US2's T011 refactor because `startCycle` calls `resolve`. Its tests don't depend on US2's tests.

### Within Each User Story

- Refactor task (if any) → build/vet → helpers → test cases → known-defect skip.
- Tasks in the same file are sequential. `run.go` edits are T011 → T014 → T015; `run_test.go` edits are T012 → T013 → T016..T020.

### Parallel Opportunities

- T001 (`.golangci.yaml`) ∥ T002–T010 (`pkg/check/main_test.go`).
- The whole of US1 ∥ the whole of US2 (different packages and files).
- US3 overlaps US1 once T011 is done.

---

## Parallel Example

```text
# Two workers after the start:
Worker A: T002 → T003 → … → T010   (apps/metrik/pkg/check/main_test.go)
Worker B: T001 → T011 → T012 → T013 → T014 → … → T020   (.golangci.yaml, apps/metrik/cmd/*)
Then: T021 → T022 → T023 → T024
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. T002–T010 (no setup needed).
2. **Stop and validate**: `go test -race -count=1 -cover ./pkg/check`, which pins the core probe logic with zero production changes.

### Incremental Delivery

1. US1 → review → (maintainer commit: `test(check): cover probe verdicts and retry rules`)
2. T001 → (`chore(lint): allow metrik internal packages in depguard`)
3. T011, T014, T015 → (`refactor(metrik): extract run cycle helpers for testability`)
4. US2 + US3 tests → (`test(metrik): cover settings resolution, classification and concurrency`)
5. Polish.

These commits only happen when the maintainer asks for them.

---

## Notes

- [P] = different file, no dependency on an incomplete task.
- Never fix KD-1..3 in this feature. They stay as skips and are reported (spec Assumptions).
- If a test needs `time.Sleep` to "wait for" something, redesign it with a channel or counter instead (FR-010 no flaky timing). The only allowed sleeps are the handler's 50 ms in T018, which simulates work, and nothing else.

## Phase 7: Convergence

- [X] T025 CRITICAL: In `apps/metrik/pkg/check/main.go` `Run`, treat any 5xx response as a retryable error (return an error before decoding, keeping `r.Code`/`r.Duration` set) so it is retried up to `MaxTries`; remove the `t.Skip` from `TestRunServerErrorIsRetried` in `apps/metrik/pkg/check/main_test.go` and make it pass; update `TestRunRecoversBeforeLimit` only if its expectations change per Constitution IV (contradicts)
- [X] T026 CRITICAL: In `apps/metrik/cmd/run.go` `report`, log a nil `results.Body` (JSON `null` payload) as an ERROR `application error` (with alert) instead of dereferencing it; remove the `t.Skip` from `TestReportNullBody` in `apps/metrik/cmd/run_test.go` and make it pass per Constitution IV (contradicts)
- [X] T027 CRITICAL: In `apps/metrik/cmd/run.go` `report`, handle a nil `results` when `err != nil` (log `check failed` with the error, no `code`/`duration` fields, alert fired) so an unpreparable request cannot panic; remove the `t.Skip` from `TestReportNilResult` in `apps/metrik/cmd/run_test.go` and make it pass per Constitution IV (contradicts)
- [X] T028 CRITICAL: Guarantee bounded retries when the resolved `MaxTries` is 0 (global and per-check unset): in `apps/metrik/cmd/run.go` `resolve`, fall back to at least 1 attempt; add a `TestResolve` case in `apps/metrik/cmd/run_test.go` with global `MaxTries: 0` asserting `MaxTries >= 1`; document the rule in `apps/metrik/configs/metrik.yaml` comments per Constitution IV (contradicts)
- [X] T029 CRITICAL: Make the `maxConcurrent` bound hold across overlapping cycles in `apps/metrik/cmd/run.go` (share one limiter channel across ticks, created once in `RunE` and passed to `startCycle`, or skip a tick while the previous cycle is still running); add a test in `apps/metrik/cmd/run_test.go` that starts two cycles back-to-back against slow endpoints and asserts peak in-flight `<= MaxConcurrent`; update `specs/001-healthchecker-tests/contracts/test-seams.md` only if the `startCycle` signature changes per Constitution IV (contradicts)
- [X] T030 CRITICAL: Replace `fmt.Printf` in `notifyOnCritical` (`apps/metrik/cmd/run.go`) with output through the shared zap logger (e.g. a dedicated `alert` logger field or a structured `ALERT` entry), keeping the hook signature `func(zapcore.Entry) error` per Constitution III (contradicts)
- [X] T031 In `TestRunConnectionRefused` (`apps/metrik/pkg/check/main_test.go`), pass a `zaptest/observer` logger instead of `zap.NewNop()` and assert exactly 2 `"connection was refused"` entries for `maxTries` 2, proving N attempts per US1/AC4 (partial)
- [X] T032 Extend `TestStartCycleBound` or add `TestStartCycleAlerts` in `apps/metrik/cmd/run_test.go` with one check pointing at a closed server (connection refused, `MaxTries: 1`) and assert one `"check failed"` ERROR entry and exactly 1 alert recorded through the `alert` argument per US3 Independent Test / FR-007 (partial)
- [X] T033 Run `CGO_ENABLED=1 go test -race -count=20 ./...` from `apps/metrik` on a machine with a C compiler and record the result in the final report per SC-004 (partial)
- [X] T034 In `TestRunVerdicts` healthy case (`apps/metrik/pkg/check/main_test.go`), serve a payload with a non-empty `reason` and assert it is decoded, and assert `res.Duration > 0` per US1/AC1 (partial)
