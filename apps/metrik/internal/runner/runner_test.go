package runner

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"metrik/internal/config"
	"metrik/pkg/breaker"
	"metrik/pkg/check"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func newObserved(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.DebugLevel)
	return zap.New(core), logs
}

// newAlertRecorder returns an alert hook that counts error-level entries, like notifyOnCritical.
func newAlertRecorder(t *testing.T) (func(zapcore.Entry) error, *atomic.Int32) {
	t.Helper()
	alerts := &atomic.Int32{}
	return func(e zapcore.Entry) error {
		if e.Level >= zapcore.ErrorLevel {
			alerts.Add(1)
		}
		return nil
	}, alerts
}

type logLine struct {
	msg   string
	level zapcore.Level
}

func TestReport(t *testing.T) {
	t.Parallel()
	healthy := &check.APIResponse{Message: "ok", Status: http.StatusOK}

	tests := []struct {
		name   string
		res    *check.Result
		err    error
		lines  []logLine
		fields map[string]any // Checked on the first entry.
		alerts int32
		failed bool
	}{
		{
			name:   "fast and healthy",
			res:    &check.Result{Code: http.StatusOK, Duration: 10 * time.Millisecond, Body: healthy},
			lines:  []logLine{{"check completed", zapcore.InfoLevel}},
			fields: map[string]any{"code": int64(http.StatusOK)},
		},
		{
			name:  "slow but healthy",
			res:   &check.Result{Code: http.StatusOK, Duration: 2 * time.Second, Body: healthy},
			lines: []logLine{{"slow response", zapcore.WarnLevel}, {"check completed", zapcore.InfoLevel}},
		},
		{
			name: "application reports failure",
			res: &check.Result{Code: http.StatusOK, Duration: 10 * time.Millisecond, Body: &check.APIResponse{
				Status: http.StatusServiceUnavailable, Message: "down", Reason: "db",
			}},
			lines:  []logLine{{"application error", zapcore.ErrorLevel}},
			fields: map[string]any{"status": int64(http.StatusServiceUnavailable), "message": "down", "reason": "db"},
			alerts: 1,
			failed: true,
		},
		{
			name:   "probe failed",
			res:    &check.Result{Code: http.StatusInternalServerError},
			err:    errors.New("boom"),
			lines:  []logLine{{"check failed", zapcore.ErrorLevel}},
			fields: map[string]any{"error": "boom", "code": int64(http.StatusInternalServerError)},
			alerts: 1,
			failed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logger, logs := newObserved(t)
			alert, alerts := newAlertRecorder(t)

			if failed := report(logger, alert, tt.res, tt.err, time.Second); failed != tt.failed {
				t.Errorf("failed = %t, want %t", failed, tt.failed)
			}

			entries := logs.All()
			if len(entries) != len(tt.lines) {
				t.Fatalf("got %d log entries, want %d: %v", len(entries), len(tt.lines), entries)
			}
			for i, want := range tt.lines {
				if entries[i].Message != want.msg || entries[i].Level != want.level {
					t.Errorf("entry %d = %q at %s, want %q at %s", i, entries[i].Message, entries[i].Level, want.msg, want.level)
				}
			}
			ctx := entries[0].ContextMap()
			for k, want := range tt.fields {
				if ctx[k] != want {
					t.Errorf("field %q = %v (%T), want %v (%T)", k, ctx[k], ctx[k], want, want)
				}
			}
			if got := alerts.Load(); got != tt.alerts {
				t.Errorf("alerts = %d, want %d", got, tt.alerts)
			}
		})
	}
}

// slowServer answers every request with a healthy payload after 50ms of
// simulated work, tracking the peak number of requests in flight and hits per path.
type slowServer struct {
	*httptest.Server
	inFlight, peak atomic.Int32
	mu             sync.Mutex
	paths          map[string]int
}

func newSlowServer(t *testing.T) *slowServer {
	t.Helper()
	s := &slowServer{paths: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := s.inFlight.Add(1)
		for {
			p := s.peak.Load()
			if n <= p || s.peak.CompareAndSwap(p, n) {
				break
			}
		}
		s.mu.Lock()
		s.paths[r.URL.Path]++
		s.mu.Unlock()
		time.Sleep(50 * time.Millisecond) // Simulated work, so probes overlap.
		s.inFlight.Add(-1)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(check.APIResponse{Message: "ok", Status: http.StatusOK}); err != nil {
			t.Errorf("encode payload: %v", err)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *slowServer) hits() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.paths)
}

func checksFor(url string, names ...string) []*check.Check {
	checks := make([]*check.Check, 0, len(names))
	for _, name := range names {
		checks = append(checks, &check.Check{Name: name, Endpoint: url + "/" + name})
	}
	return checks
}

func TestStartCycleBound(t *testing.T) {
	t.Parallel()
	const maxConcurrent = 2
	srv := newSlowServer(t)
	cfg := config.Config{
		MaxConcurrent: maxConcurrent, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second, FailureThreshold: 3,
		Checks: checksFor(srv.URL, "c0", "c1", "c2", "c3", "c4", "c5"),
	}
	logger, logs := newObserved(t)
	alert, alerts := newAlertRecorder(t)

	startCycle(t.Context(), logger, cfg, make(chan bool, maxConcurrent), breaker.New(), &sync.Map{}, alert).Wait()

	if got := srv.peak.Load(); got > maxConcurrent {
		t.Errorf("peak in-flight probes = %d, want <= %d", got, maxConcurrent)
	}
	paths := srv.hits()
	if len(paths) != len(cfg.Checks) {
		t.Errorf("probed %d distinct checks, want %d: %v", len(paths), len(cfg.Checks), paths)
	}
	for p, n := range paths {
		if n != 1 {
			t.Errorf("%s probed %d times, want 1", p, n)
		}
	}
	if got := logs.FilterMessage("check completed").Len(); got != len(cfg.Checks) {
		t.Errorf("completed checks = %d, want %d", got, len(cfg.Checks))
	}
	if got := alerts.Load(); got != 0 {
		t.Errorf("alerts = %d, want 0", got)
	}
}

func TestStartCyclesShareBound(t *testing.T) {
	t.Parallel()
	const maxConcurrent = 2
	srv := newSlowServer(t)
	cfg := config.Config{
		MaxConcurrent: maxConcurrent, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second, FailureThreshold: 3,
		Checks: checksFor(srv.URL, "a0", "a1", "a2", "a3"),
	}
	other := cfg
	other.Checks = checksFor(srv.URL, "b0", "b1", "b2", "b3")
	logger, _ := newObserved(t)
	alert, _ := newAlertRecorder(t)
	limiterCh := make(chan bool, maxConcurrent)

	// The second cycle starts while the first one still has probes in flight.
	circuits, inFlight := breaker.New(), &sync.Map{}
	first := startCycle(t.Context(), logger, cfg, limiterCh, circuits, inFlight, alert)
	second := startCycle(t.Context(), logger, other, limiterCh, circuits, inFlight, alert)
	first.Wait()
	second.Wait()

	if got := srv.peak.Load(); got > maxConcurrent {
		t.Errorf("peak in-flight probes across cycles = %d, want <= %d", got, maxConcurrent)
	}
	paths := srv.hits()
	if len(paths) != len(cfg.Checks)+len(other.Checks) {
		t.Errorf("probed %d distinct checks, want %d: %v", len(paths), len(cfg.Checks)+len(other.Checks), paths)
	}
	for p, n := range paths {
		if n != 1 {
			t.Errorf("%s probed %d times, want 1", p, n)
		}
	}
}

func TestStartCycleSkipsInFlight(t *testing.T) {
	t.Parallel()
	srv := newSlowServer(t)
	cfg := config.Config{
		MaxConcurrent: 2, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second, FailureThreshold: 3,
		Checks: checksFor(srv.URL, "a0", "a1"),
	}
	logger, logs := newObserved(t)
	alert, _ := newAlertRecorder(t)
	limiterCh := make(chan bool, cfg.MaxConcurrent)
	circuits, inFlight := breaker.New(), &sync.Map{}

	// The second cycle starts while the probes of the first one are still running.
	first := startCycle(t.Context(), logger, cfg, limiterCh, circuits, inFlight, alert)
	second := startCycle(t.Context(), logger, cfg, limiterCh, circuits, inFlight, alert)
	first.Wait()
	second.Wait()

	for p, n := range srv.hits() {
		if n != 1 {
			t.Errorf("%s probed %d times, want 1 (second cycle skipped)", p, n)
		}
	}
	skipped := logs.FilterMessage("check skipped").FilterLevelExact(zapcore.WarnLevel).All()
	if len(skipped) != len(cfg.Checks) {
		t.Fatalf("check skipped entries = %d, want %d", len(skipped), len(cfg.Checks))
	}
	for _, e := range skipped {
		if ctx := e.ContextMap(); ctx["reason"] != "previous probe still running" || ctx["circuit"] != nil {
			t.Errorf("check skipped fields = %v, want only the in-flight reason", ctx)
		}
	}

	// Finished probes release their check for the next cycle.
	startCycle(t.Context(), logger, cfg, limiterCh, circuits, inFlight, alert).Wait()
	for p, n := range srv.hits() {
		if n != 2 {
			t.Errorf("%s probed %d times, want 2 (released after the first probe)", p, n)
		}
	}
	if got := logs.FilterMessage("check skipped").Len(); got != len(cfg.Checks) {
		t.Errorf("check skipped entries = %d, want still %d", got, len(cfg.Checks))
	}
}

func TestStartCycleFailedProbeReleases(t *testing.T) {
	t.Parallel()
	srv, hits := newHitServer(t, func(string) bool { return true })
	disabled := 0
	cfg := config.Config{
		MaxConcurrent: 1, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second,
		Checks: []*check.Check{{Name: "down", Endpoint: srv.URL + "/down", FailureThreshold: &disabled}},
	}
	logger, logs := newObserved(t)
	alert, _ := newAlertRecorder(t)
	limiterCh := make(chan bool, cfg.MaxConcurrent)
	circuits, inFlight := breaker.New(), &sync.Map{}

	for range 3 {
		startCycle(t.Context(), logger, cfg, limiterCh, circuits, inFlight, alert).Wait()
	}

	if got := hits("/down"); got != 3 {
		t.Errorf("hits = %d, want 3 (a failed probe releases its check)", got)
	}
	if got := logs.FilterMessage("check skipped").Len(); got != 0 {
		t.Errorf("check skipped entries = %d, want 0", got)
	}
}

func TestStartCycleInFlightSkipKeepsTrial(t *testing.T) {
	t.Parallel()
	var healthy atomic.Bool
	srv, hits := newHitServer(t, func(string) bool { return !healthy.Load() })
	cfg := config.Config{
		MaxConcurrent: 1, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second,
		FailureThreshold: 1, Cooldown: 30 * time.Millisecond,
		Checks: checksFor(srv.URL, "flaky"),
	}
	logger, logs := newObserved(t)
	alert, _ := newAlertRecorder(t)
	limiterCh := make(chan bool, cfg.MaxConcurrent)
	circuits, inFlight := breaker.New(), &sync.Map{}
	cycle := func() { startCycle(t.Context(), logger, cfg, limiterCh, circuits, inFlight, alert).Wait() }

	cycle() // Fails, opens.
	time.Sleep(40 * time.Millisecond)
	healthy.Store(true)

	// The cool-down has elapsed, but an earlier probe of the check is still running.
	key := cfg.Checks[0].ID()
	inFlight.Store(key, struct{}{})
	cycle()
	if got := hits("/flaky"); got != 1 {
		t.Fatalf("hits = %d, want 1 (skipped while in flight)", got)
	}
	inFlight.Delete(key)

	cycle() // The trial probe is still available, and closes the circuit.
	if got := hits("/flaky"); got != 2 {
		t.Errorf("hits = %d, want 2 (trial probe ran)", got)
	}
	if got := logs.FilterMessage("circuit closed").Len(); got != 1 {
		t.Errorf("circuit closed entries = %d, want 1", got)
	}
}

func TestStartCycleAlerts(t *testing.T) {
	t.Parallel()
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	cfg := config.Config{
		MaxConcurrent: 1, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second, FailureThreshold: 3,
		Checks: []*check.Check{{Name: "down", Endpoint: closed.URL}},
	}
	logger, logs := newObserved(t)
	alert, alerts := newAlertRecorder(t)

	startCycle(t.Context(), logger, cfg, make(chan bool, 1), breaker.New(), &sync.Map{}, alert).Wait()

	failed := logs.FilterMessage("check failed").FilterLevelExact(zapcore.ErrorLevel).All()
	if len(failed) != 1 {
		t.Fatalf("check failed entries = %d, want 1", len(failed))
	}
	if name := failed[0].ContextMap()["name"]; name != "down" {
		t.Errorf("name field = %v, want %q", name, "down")
	}
	if got := alerts.Load(); got != 1 {
		t.Errorf("alerts = %d, want 1", got)
	}
}

func TestNotifyOnCritical(t *testing.T) {
	t.Parallel()
	logger, logs := newObserved(t)
	notify := notifyOnCritical(logger)

	for _, level := range []zapcore.Level{zapcore.InfoLevel, zapcore.WarnLevel, zapcore.ErrorLevel} {
		if err := notify(zapcore.Entry{Level: level, Message: level.String() + " entry"}); err != nil {
			t.Fatalf("notify(%s): %v", level, err)
		}
	}

	alerts := logs.FilterMessage("ALERT").All()
	if len(alerts) != 1 {
		t.Fatalf("ALERT entries = %d, want 1 (error level only)", len(alerts))
	}
	if got := alerts[0].ContextMap()["alert"]; got != "error entry" {
		t.Errorf("alert field = %v, want %q", got, "error entry")
	}
}

func TestReportNullBody(t *testing.T) {
	t.Parallel()
	logger, logs := newObserved(t)
	alert, alerts := newAlertRecorder(t)

	if !report(logger, alert, &check.Result{Code: http.StatusOK}, nil, time.Second) {
		t.Error("failed = false, want true")
	}

	if got := logs.FilterLevelExact(zapcore.ErrorLevel).Len(); got != 1 {
		t.Errorf("error entries = %d, want 1", got)
	}
	if got := alerts.Load(); got != 1 {
		t.Errorf("alerts = %d, want 1", got)
	}
}

func TestReportNilResult(t *testing.T) {
	t.Parallel()
	logger, logs := newObserved(t)
	alert, alerts := newAlertRecorder(t)

	if !report(logger, alert, nil, errors.New("bad url"), time.Second) {
		t.Error("failed = false, want true")
	}

	if got := logs.FilterMessage("check failed").FilterLevelExact(zapcore.ErrorLevel).Len(); got != 1 {
		t.Errorf("check failed entries = %d, want 1", got)
	}
	if got := alerts.Load(); got != 1 {
		t.Errorf("alerts = %d, want 1", got)
	}
}

// newHitServer answers 500 on paths for which fail reports true and a healthy
// payload otherwise. The returned function reports the hits on a path so far.
func newHitServer(t *testing.T, fail func(path string) bool) (srv *httptest.Server, hits func(path string) int) {
	t.Helper()
	var mu sync.Mutex
	counts := map[string]int{}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		if fail(r.URL.Path) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(check.APIResponse{Message: "ok", Status: http.StatusOK}); err != nil {
			t.Errorf("encode payload: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	hits = func(path string) int {
		mu.Lock()
		defer mu.Unlock()
		return counts[path]
	}
	return srv, hits
}

func TestStartCycleSkipsOpenCircuit(t *testing.T) {
	t.Parallel()
	srv, hits := newHitServer(t, func(path string) bool { return path == "/bad" })
	cfg := config.Config{
		MaxConcurrent: 2, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second,
		FailureThreshold: 2, Cooldown: time.Hour,
		Checks: checksFor(srv.URL, "bad", "good"),
	}
	logger, logs := newObserved(t)
	alert, _ := newAlertRecorder(t)
	limiterCh := make(chan bool, cfg.MaxConcurrent)
	circuits := breaker.New()

	for range 3 {
		startCycle(t.Context(), logger, cfg, limiterCh, circuits, &sync.Map{}, alert).Wait()
	}

	if got := hits("/bad"); got != 2 {
		t.Errorf("/bad hits = %d, want 2 (skipped once the circuit opened)", got)
	}
	if got := hits("/good"); got != 3 {
		t.Errorf("/good hits = %d, want 3 (unaffected)", got)
	}
	skipped := logs.FilterMessage("check skipped").FilterLevelExact(zapcore.WarnLevel).All()
	if len(skipped) != 1 {
		t.Fatalf("check skipped entries = %d, want 1", len(skipped))
	}
	if ctx := skipped[0].ContextMap(); ctx["name"] != "bad" || ctx["circuit"] != "open" {
		t.Errorf("check skipped fields = %v, want name=bad circuit=open", ctx)
	}
}

func TestStartCycleSkipUsesNoSlot(t *testing.T) {
	t.Parallel()
	srv, _ := newHitServer(t, func(string) bool { return true })
	cfg := config.Config{
		MaxConcurrent: 1, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second,
		FailureThreshold: 1, Cooldown: time.Hour,
		Checks: checksFor(srv.URL, "down"),
	}
	logger, _ := newObserved(t)
	alert, _ := newAlertRecorder(t)
	limiterCh := make(chan bool, cfg.MaxConcurrent)
	circuits := breaker.New()
	startCycle(t.Context(), logger, cfg, limiterCh, circuits, &sync.Map{}, alert).Wait() // Opens the circuit.

	for len(limiterCh) < cap(limiterCh) {
		limiterCh <- true
	}
	done := make(chan struct{})
	go func() {
		startCycle(t.Context(), logger, cfg, limiterCh, circuits, &sync.Map{}, alert).Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("startCycle blocked on a full limiter for a suspended check")
	}
}

func TestStartCycleTrialRecovers(t *testing.T) {
	t.Parallel()
	var healthy atomic.Bool
	srv, hits := newHitServer(t, func(string) bool { return !healthy.Load() })
	cfg := config.Config{
		MaxConcurrent: 1, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second,
		FailureThreshold: 1, Cooldown: 30 * time.Millisecond,
		Checks: checksFor(srv.URL, "flaky"),
	}
	logger, _ := newObserved(t)
	alert, _ := newAlertRecorder(t)
	limiterCh := make(chan bool, cfg.MaxConcurrent)
	circuits := breaker.New()
	cycle := func(wantHits int, step string) {
		t.Helper()
		startCycle(t.Context(), logger, cfg, limiterCh, circuits, &sync.Map{}, alert).Wait()
		if got := hits("/flaky"); got != wantHits {
			t.Fatalf("after %s: hits = %d, want %d", step, got, wantHits)
		}
	}

	cycle(1, "first failure (opens)")
	cycle(1, "cycle during cool-down (skipped)")
	time.Sleep(40 * time.Millisecond)
	cycle(2, "failed trial (reopens)")
	time.Sleep(40 * time.Millisecond)
	healthy.Store(true)
	cycle(3, "successful trial (closes)")
	cycle(4, "normal cycle")
}

func TestStartCycleCircuitLogs(t *testing.T) {
	t.Parallel()
	var healthy atomic.Bool
	srv, _ := newHitServer(t, func(string) bool { return !healthy.Load() })
	cfg := config.Config{
		MaxConcurrent: 1, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second,
		FailureThreshold: 2, Cooldown: 30 * time.Millisecond,
		Checks: checksFor(srv.URL, "api"),
	}
	logger, logs := newObserved(t)
	alert, alerts := newAlertRecorder(t)
	limiterCh := make(chan bool, cfg.MaxConcurrent)
	circuits := breaker.New()
	cycle := func() { startCycle(t.Context(), logger, cfg, limiterCh, circuits, &sync.Map{}, alert).Wait() }

	cycle() // Fails.
	cycle() // Fails, opens.
	cycle() // Skipped.
	time.Sleep(40 * time.Millisecond)
	cycle() // Failed trial, reopens.
	time.Sleep(40 * time.Millisecond)
	healthy.Store(true)
	cycle() // Successful trial, closes.

	tests := []struct {
		msg    string
		level  zapcore.Level
		fields map[string]any
	}{
		{"circuit opened", zapcore.ErrorLevel, map[string]any{
			"circuit": "open", "failures": int64(2), "cooldown": 30 * time.Millisecond,
		}},
		{"check skipped", zapcore.WarnLevel, map[string]any{"circuit": "open"}},
		{"circuit reopened", zapcore.ErrorLevel, map[string]any{"circuit": "open", "cooldown": 30 * time.Millisecond}},
		{"circuit closed", zapcore.InfoLevel, map[string]any{"circuit": "closed"}},
	}
	for _, tt := range tests {
		entries := logs.FilterMessage(tt.msg).All()
		if len(entries) != 1 {
			t.Errorf("%q entries = %d, want 1", tt.msg, len(entries))
			continue
		}
		if entries[0].Level != tt.level {
			t.Errorf("%q level = %s, want %s", tt.msg, entries[0].Level, tt.level)
		}
		ctx := entries[0].ContextMap()
		if ctx["name"] != "api" || ctx["endpoint"] != srv.URL+"/api" {
			t.Errorf("%q name/endpoint = %v/%v, want api/%s", tt.msg, ctx["name"], ctx["endpoint"], srv.URL+"/api")
		}
		for k, want := range tt.fields {
			if ctx[k] != want {
				t.Errorf("%q field %q = %v (%T), want %v (%T)", tt.msg, k, ctx[k], ctx[k], want, want)
			}
		}
	}
	for _, e := range logs.FilterMessage("circuit closed").All() {
		if suspended, ok := e.ContextMap()["suspended"].(time.Duration); !ok || suspended <= 0 {
			t.Errorf("circuit closed suspended = %v, want a positive duration", e.ContextMap()["suspended"])
		}
	}
	// Only the three failed probes alert; circuit transitions never do.
	if got := alerts.Load(); got != 3 {
		t.Errorf("alerts = %d, want 3", got)
	}
}

func TestStartCycleBreakerDisabled(t *testing.T) {
	t.Parallel()
	srv, hits := newHitServer(t, func(string) bool { return true })
	disabled := 0
	cfg := config.Config{
		MaxConcurrent: 1, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second,
		FailureThreshold: 3, Cooldown: time.Hour,
		Checks: []*check.Check{{Name: "always", Endpoint: srv.URL + "/always", FailureThreshold: &disabled}},
	}
	logger, logs := newObserved(t)
	alert, alerts := newAlertRecorder(t)
	limiterCh := make(chan bool, cfg.MaxConcurrent)
	circuits := breaker.New()

	for range 5 {
		startCycle(t.Context(), logger, cfg, limiterCh, circuits, &sync.Map{}, alert).Wait()
	}

	if got := hits("/always"); got != 5 {
		t.Errorf("hits = %d, want 5 (probed every cycle)", got)
	}
	if got := alerts.Load(); got != 5 {
		t.Errorf("alerts = %d, want 5", got)
	}
	for _, e := range logs.All() {
		if strings.HasPrefix(e.Message, "circuit") || e.Message == "check skipped" {
			t.Errorf("unexpected breaker entry %q", e.Message)
		}
	}
}
