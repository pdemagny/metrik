package cmd

import (
	"encoding/json"
	"errors"
	"maps"
	"metrik/pkg/check"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestResolve(t *testing.T) {
	t.Parallel()
	global := Config{AlertThreshold: 3 * time.Second, MaxTries: 3, RequestTimeout: 5 * time.Second}

	tests := []struct {
		name           string
		check          check.Check
		alertThreshold time.Duration
		maxTries       uint
		requestTimeout time.Duration
	}{
		{
			name:           "no override",
			check:          check.Check{},
			alertThreshold: 3 * time.Second, maxTries: 3, requestTimeout: 5 * time.Second,
		},
		{
			name:           "alert threshold only",
			check:          check.Check{AlertThreshold: time.Second},
			alertThreshold: time.Second, maxTries: 3, requestTimeout: 5 * time.Second,
		},
		{
			name:           "max tries only",
			check:          check.Check{MaxTries: 5},
			alertThreshold: 3 * time.Second, maxTries: 5, requestTimeout: 5 * time.Second,
		},
		{
			name:           "request timeout only",
			check:          check.Check{RequestTimeout: time.Second},
			alertThreshold: 3 * time.Second, maxTries: 3, requestTimeout: time.Second,
		},
		{
			name:           "all overridden",
			check:          check.Check{AlertThreshold: time.Second, MaxTries: 5, RequestTimeout: 2 * time.Second},
			alertThreshold: time.Second, maxTries: 5, requestTimeout: 2 * time.Second,
		},
	}

	t.Run("zero max tries everywhere stays bounded", func(t *testing.T) {
		t.Parallel()
		got := Config{}.resolve(&check.Check{})
		if got.MaxTries != 1 {
			t.Errorf("maxTries = %d, want 1", got.MaxTries)
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := tt.check
			in.Name, in.Endpoint = "api", "http://example.test/live"
			before := in

			got := global.resolve(&in)

			if got == &in {
				t.Fatal("resolve returned the input check, want a copy")
			}
			if in != before {
				t.Errorf("input check mutated: %+v, want %+v", in, before)
			}
			if got.Name != in.Name || got.Endpoint != in.Endpoint {
				t.Errorf("name/endpoint = %q/%q, want %q/%q", got.Name, got.Endpoint, in.Name, in.Endpoint)
			}
			if got.AlertThreshold != tt.alertThreshold {
				t.Errorf("alertThreshold = %s, want %s", got.AlertThreshold, tt.alertThreshold)
			}
			if got.MaxTries != tt.maxTries {
				t.Errorf("maxTries = %d, want %d", got.MaxTries, tt.maxTries)
			}
			if got.RequestTimeout != tt.requestTimeout {
				t.Errorf("requestTimeout = %s, want %s", got.RequestTimeout, tt.requestTimeout)
			}
		})
	}
}

func TestExampleConfigLoads(t *testing.T) {
	t.Parallel()
	v := viper.New()
	v.SetConfigFile("../configs/metrik.yaml")
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}

	if !cfg.Verbose || cfg.Debug {
		t.Errorf("verbose/debug = %t/%t, want true/false", cfg.Verbose, cfg.Debug)
	}
	if cfg.RunInterval != 10*time.Second || cfg.MaxConcurrent != 10 || cfg.RequestTimeout != 5*time.Second ||
		cfg.AlertThreshold != 3*time.Second || cfg.MaxTries != 3 {
		t.Errorf("globals = %+v, want runInterval 10s, maxConcurrent 10, requestTimeout 5s, alertThreshold 3s, maxTries 3", cfg)
	}

	want := []check.Check{
		{Name: "liveness slow warn", Endpoint: "http://localhost:8080/v1/slow", AlertThreshold: 3 * time.Second},
		{Name: "liveness not found", Endpoint: "http://localhost:8080/v1/.well-known/notfound"},
		// Endpoint is empty because the example config half-comments this check out.
		{Name: "liveness connection refused"},
	}
	if len(cfg.Checks) != len(want) {
		t.Fatalf("got %d checks, want %d", len(cfg.Checks), len(want))
	}
	for i, w := range want {
		if *cfg.Checks[i] != w {
			t.Errorf("checks[%d] = %+v, want %+v", i, *cfg.Checks[i], w)
		}
	}
}

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
		},
		{
			name:   "probe failed",
			res:    &check.Result{Code: http.StatusInternalServerError},
			err:    errors.New("boom"),
			lines:  []logLine{{"check failed", zapcore.ErrorLevel}},
			fields: map[string]any{"error": "boom", "code": int64(http.StatusInternalServerError)},
			alerts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logger, logs := newObserved(t)
			alert, alerts := newAlertRecorder(t)

			report(logger, alert, tt.res, tt.err, time.Second)

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
	cfg := Config{
		MaxConcurrent: maxConcurrent, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second,
		Checks: checksFor(srv.URL, "c0", "c1", "c2", "c3", "c4", "c5"),
	}
	logger, logs := newObserved(t)
	alert, alerts := newAlertRecorder(t)

	startCycle(t.Context(), logger, cfg, alert).Wait()

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

func TestStartCycleAlerts(t *testing.T) {
	t.Parallel()
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	cfg := Config{
		MaxConcurrent: 1, MaxTries: 1, RequestTimeout: time.Second, AlertThreshold: time.Second,
		Checks: []*check.Check{{Name: "down", Endpoint: closed.URL}},
	}
	logger, logs := newObserved(t)
	alert, alerts := newAlertRecorder(t)

	startCycle(t.Context(), logger, cfg, alert).Wait()

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

func TestReportNullBody(t *testing.T) {
	t.Parallel()
	logger, logs := newObserved(t)
	alert, alerts := newAlertRecorder(t)

	report(logger, alert, &check.Result{Code: http.StatusOK}, nil, time.Second)

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

	report(logger, alert, nil, errors.New("bad url"), time.Second)

	if got := logs.FilterMessage("check failed").FilterLevelExact(zapcore.ErrorLevel).Len(); got != 1 {
		t.Errorf("check failed entries = %d, want 1", got)
	}
	if got := alerts.Load(); got != 1 {
		t.Errorf("alerts = %d, want 1", got)
	}
}
