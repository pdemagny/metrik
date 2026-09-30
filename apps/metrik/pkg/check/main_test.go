package check

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

var okPayload = APIResponse{Message: "ok", Status: http.StatusOK, Reason: "all dependencies up"}

// newServer starts a test server that counts hits and passes the 1-based hit number to h.
func newServer(t *testing.T, h func(w http.ResponseWriter, r *http.Request, hit int32)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	hits := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h(w, r, hits.Add(1))
	}))
	t.Cleanup(srv.Close)
	return srv, hits
}

func writeOK(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(okPayload); err != nil {
		t.Errorf("encode payload: %v", err)
	}
}

func writeText(t *testing.T, w http.ResponseWriter, code int, body string) {
	t.Helper()
	w.WriteHeader(code)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write body: %v", err)
	}
}

// probe runs c with a no-op logger and a client that, like the one built by
// the run command, uses the given timeout and does not follow redirects.
func probe(t *testing.T, c *Check, timeout time.Duration) (*Result, error) {
	t.Helper()
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return c.Run(t.Context(), zap.NewNop(), client)
}

func TestRunVerdicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		handler  func(t *testing.T, w http.ResponseWriter, r *http.Request)
		maxTries uint
		hits     int32
		wantErr  string // "" means no error, "*" means any error.
		code     int
		body     *APIResponse
	}{
		{
			name: "healthy payload",
			handler: func(t *testing.T, w http.ResponseWriter, _ *http.Request) {
				t.Helper()
				writeOK(t, w)
			},
			maxTries: 3, hits: 1, code: http.StatusOK, body: &okPayload,
		},
		{
			name: "client error is not retried",
			handler: func(t *testing.T, w http.ResponseWriter, _ *http.Request) {
				t.Helper()
				writeText(t, w, http.StatusNotFound, "not found")
			},
			maxTries: 3, hits: 1, wantErr: "client error", code: http.StatusNotFound,
		},
		{
			name: "too many requests without Retry-After is a client error",
			handler: func(t *testing.T, w http.ResponseWriter, _ *http.Request) {
				t.Helper()
				writeText(t, w, http.StatusTooManyRequests, "slow down")
			},
			maxTries: 3, hits: 1, wantErr: "client error", code: http.StatusTooManyRequests,
		},
		{
			name: "unreadable payload is retried",
			handler: func(t *testing.T, w http.ResponseWriter, _ *http.Request) {
				t.Helper()
				writeText(t, w, http.StatusOK, "not json")
			},
			maxTries: 2, hits: 2, wantErr: "*", code: http.StatusOK,
		},
		{
			name: "empty payload is retried",
			handler: func(t *testing.T, w http.ResponseWriter, _ *http.Request) {
				t.Helper()
				writeText(t, w, http.StatusOK, "")
			},
			maxTries: 2, hits: 2, wantErr: "*", code: http.StatusOK,
		},
		{
			name: "redirect is not followed",
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				t.Helper()
				http.Redirect(w, r, "/elsewhere", http.StatusFound)
			},
			maxTries: 2, hits: 2, wantErr: "*", code: http.StatusFound,
		},
		{
			name: "single try makes exactly one attempt",
			handler: func(t *testing.T, w http.ResponseWriter, _ *http.Request) {
				t.Helper()
				writeText(t, w, http.StatusInternalServerError, "not json")
			},
			maxTries: 1, hits: 1, wantErr: "*", code: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, hits := newServer(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
				tt.handler(t, w, r)
			})

			res, err := probe(t, NewCheck("test", srv.URL, time.Second, tt.maxTries), time.Second)

			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr == "*" && err == nil:
				t.Fatal("expected an error, got nil")
			case tt.wantErr != "" && tt.wantErr != "*" && (err == nil || err.Error() != tt.wantErr):
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
			if got := hits.Load(); got != tt.hits {
				t.Errorf("attempts = %d, want %d", got, tt.hits)
			}
			if res.Code != tt.code {
				t.Errorf("code = %d, want %d", res.Code, tt.code)
			}
			if tt.body != nil && (res.Body == nil || *res.Body != *tt.body) {
				t.Errorf("body = %+v, want %+v", res.Body, tt.body)
			}
			if err == nil && res.Duration <= 0 {
				t.Errorf("duration = %s, want > 0", res.Duration)
			}
		})
	}
}

func TestRunConnectionRefused(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	core, logs := observer.New(zap.DebugLevel)

	res, err := NewCheck("test", url, time.Second, 2).Run(t.Context(), zap.New(core), &http.Client{Timeout: time.Second})

	if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("error = %v, want connection refused", err)
	}
	if res.Code != 0 {
		t.Errorf("code = %d, want 0", res.Code)
	}
	// A refused dial never reaches a server, so attempts are counted from the per-attempt log.
	if got := logs.FilterMessage("connection was refused").Len(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestRunTimeout(t *testing.T) {
	t.Parallel()
	srv, hits := newServer(t, func(_ http.ResponseWriter, r *http.Request, _ int32) {
		<-r.Context().Done()
	})

	_, err := probe(t, NewCheck("test", srv.URL, time.Second, 2), 50*time.Millisecond)

	var ne net.Error
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("error = %v, want a timeout", err)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestRunRecoversBeforeLimit(t *testing.T) {
	t.Parallel()
	srv, hits := newServer(t, func(w http.ResponseWriter, _ *http.Request, hit int32) {
		if hit < 3 {
			writeText(t, w, http.StatusServiceUnavailable, "not json")
			return
		}
		writeOK(t, w)
	})

	res, err := probe(t, NewCheck("test", srv.URL, time.Second, 3), time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
	if res.Code != http.StatusOK || res.Body == nil || res.Body.Status != http.StatusOK {
		t.Errorf("result = %+v, want code 200 and status 200", res)
	}
}

func TestRunRetryAfter(t *testing.T) {
	t.Parallel()
	var (
		mu    sync.Mutex
		times []time.Time
	)
	srv, hits := newServer(t, func(w http.ResponseWriter, _ *http.Request, hit int32) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		if hit == 1 {
			w.Header().Set("Retry-After", "1")
			writeText(t, w, http.StatusTooManyRequests, "slow down")
			return
		}
		writeOK(t, w)
	})

	_, err := probe(t, NewCheck("test", srv.URL, time.Second, 3), time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if gap := times[1].Sub(times[0]); gap < time.Second {
		t.Errorf("gap between attempts = %s, want >= 1s", gap)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	srv, hits := newServer(t, func(w http.ResponseWriter, _ *http.Request, _ int32) {
		cancel()
		writeText(t, w, http.StatusOK, "not json")
	})

	start := time.Now()
	_, err := NewCheck("test", srv.URL, time.Second, 10).Run(ctx, zap.NewNop(), &http.Client{Timeout: time.Second})
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
	// The first randomized backoff is at least 250ms; stopping sooner proves no wait happened.
	if elapsed >= 250*time.Millisecond {
		t.Errorf("Run took %s after cancel, want < 250ms", elapsed)
	}
}

func TestRunIsolatedBetweenCalls(t *testing.T) {
	t.Parallel()
	srv, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request, hit int32) {
		if hit == 1 {
			writeOK(t, w)
			return
		}
		writeText(t, w, http.StatusNotFound, "not found")
	})
	c := NewCheck("test", srv.URL, time.Second, 1)

	first, err := probe(t, c, time.Second)
	if err != nil {
		t.Fatalf("first run: unexpected error: %v", err)
	}
	second, err := probe(t, c, time.Second)
	if err == nil {
		t.Fatal("second run: expected an error, got nil")
	}

	if second.Code != http.StatusNotFound || second.Body != nil {
		t.Errorf("second result = %+v, want code 404 and no body", second)
	}
	if first.Code != http.StatusOK || first.Body == nil || *first.Body != okPayload {
		t.Errorf("first result changed: %+v", first)
	}
}

func TestRunServerErrorIsRetried(t *testing.T) {
	t.Parallel()
	srv, hits := newServer(t, func(w http.ResponseWriter, _ *http.Request, _ int32) {
		writeText(t, w, http.StatusServiceUnavailable, `{"message":"down","status":503}`)
	})

	_, err := probe(t, NewCheck("test", srv.URL, time.Second, 3), time.Second)

	if err == nil {
		t.Error("expected an error, got nil")
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}
