package breaker_test

import (
	"testing"
	"time"

	"metrik/pkg/breaker"
)

const (
	testThreshold = 3
	testCooldown  = 60 * time.Second
)

var t0 = time.Unix(0, 0)

// at returns t0 shifted by d, to keep the tables readable.
func at(d time.Duration) time.Time { return t0.Add(d) }

func TestOpens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		results []bool // failed, one per cycle, recorded at t0, t0+1s, ...
		allowAt time.Duration
		want    breaker.Gate
	}{
		{
			name:    "below threshold stays closed",
			results: []bool{true, true},
			allowAt: 10 * time.Second,
			want:    breaker.Gate{Run: true},
		},
		{
			name:    "success resets the count",
			results: []bool{true, true, false, true, true},
			allowAt: 10 * time.Second,
			want:    breaker.Gate{Run: true},
		},
		{
			name:    "threshold reached opens",
			results: []bool{true, true, true},
			allowAt: 2*time.Second + 10*time.Second, // 10s after the third failure.
			want:    breaker.Gate{State: breaker.Open, Remaining: 50 * time.Second},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cs := breaker.New()
			for i, failed := range tt.results {
				cs.Record("k", false, failed, testThreshold, testCooldown, at(time.Duration(i)*time.Second))
			}
			if got := cs.Allow("k", at(tt.allowAt)); got != tt.want {
				t.Errorf("Allow = %+v, want %+v", got, tt.want)
			}
		})
	}

	t.Run("opening one key leaves the others closed", func(t *testing.T) {
		t.Parallel()
		cs := breaker.New()
		for range testThreshold {
			cs.Record("a", false, true, testThreshold, testCooldown, t0)
		}
		if got := cs.Allow("a", t0); got.Run {
			t.Errorf("Allow(a) = %+v, want no run", got)
		}
		if got := cs.Allow("b", t0); got != (breaker.Gate{Run: true}) {
			t.Errorf("Allow(b) = %+v, want run", got)
		}
	})

	t.Run("reaching the threshold reports the change", func(t *testing.T) {
		t.Parallel()
		cs := breaker.New()
		var got breaker.Change
		for range testThreshold {
			got = cs.Record("k", false, true, testThreshold, testCooldown, t0)
		}
		if got != (breaker.Change{Kind: breaker.Opened, Failures: testThreshold}) {
			t.Errorf("Record = %+v, want opened with %d failures", got, testThreshold)
		}
	})
}

func TestKeep(t *testing.T) {
	t.Parallel()
	cs := breaker.New()
	open := func(key string) {
		for range testThreshold {
			cs.Record(key, false, true, testThreshold, testCooldown, t0)
		}
	}
	open("removed")
	open("kept")
	open("check\x00http://old")

	cs.Keep(map[string]struct{}{"kept": {}, "check\x00http://new": {}})

	if got := cs.Allow("removed", t0); got != (breaker.Gate{Run: true}) {
		t.Errorf("Allow(removed) = %+v, want a fresh closed circuit", got)
	}
	if got := cs.Allow("check\x00http://new", t0); got != (breaker.Gate{Run: true}) {
		t.Errorf("Allow(changed endpoint) = %+v, want a fresh closed circuit", got)
	}
	if got := cs.Allow("kept", t0); got.State != breaker.Open {
		t.Errorf("Allow(kept) = %+v, want still open", got)
	}
}

func TestTrial(t *testing.T) {
	t.Parallel()
	// openAtT0 returns circuits whose "k" circuit opened at t0.
	openAtT0 := func() *breaker.Breaker {
		cs := breaker.New()
		for range testThreshold {
			cs.Record("k", false, true, testThreshold, testCooldown, t0)
		}
		return cs
	}

	t.Run("cool-down elapsed allows exactly one trial", func(t *testing.T) {
		t.Parallel()
		cs := openAtT0()
		if got := cs.Allow("k", at(testCooldown)); got != (breaker.Gate{Run: true, Trial: true}) {
			t.Fatalf("first Allow = %+v, want a trial", got)
		}
		if got := cs.Allow("k", at(testCooldown+time.Second)); got != (breaker.Gate{State: breaker.Trial}) {
			t.Errorf("Allow during trial = %+v, want skipped in trial state", got)
		}
	})

	t.Run("successful trial closes", func(t *testing.T) {
		t.Parallel()
		cs := openAtT0()
		cs.Allow("k", at(testCooldown))
		got := cs.Record("k", true, false, testThreshold, testCooldown, at(61*time.Second))
		if got != (breaker.Change{Kind: breaker.Recovered, Suspended: 61 * time.Second}) {
			t.Errorf("Record = %+v, want recovered after 61s", got)
		}
		if g := cs.Allow("k", at(62*time.Second)); g != (breaker.Gate{Run: true}) {
			t.Errorf("Allow after recovery = %+v, want run", g)
		}
		// The failure count was reset: threshold-1 failures keep it closed.
		for range testThreshold - 1 {
			cs.Record("k", false, true, testThreshold, testCooldown, at(63*time.Second))
		}
		if g := cs.Allow("k", at(64*time.Second)); g != (breaker.Gate{Run: true}) {
			t.Errorf("Allow after %d new failures = %+v, want run", testThreshold-1, g)
		}
	})

	t.Run("failed trial reopens for a full cool-down", func(t *testing.T) {
		t.Parallel()
		cs := openAtT0()
		cs.Allow("k", at(testCooldown))
		got := cs.Record("k", true, true, testThreshold, testCooldown, at(61*time.Second))
		if got != (breaker.Change{Kind: breaker.Reopened}) {
			t.Errorf("Record = %+v, want reopened", got)
		}
		if g := cs.Allow("k", at(120*time.Second)); g != (breaker.Gate{State: breaker.Open, Remaining: time.Second}) {
			t.Errorf("Allow at t0+120s = %+v, want open with 1s remaining (until t0+121s)", g)
		}
	})

	t.Run("trial result after the circuit was pruned is ignored", func(t *testing.T) {
		t.Parallel()
		cs := openAtT0()
		cs.Allow("k", at(testCooldown))
		cs.Keep(nil)
		if got := cs.Record("k", true, false, testThreshold, testCooldown, at(61*time.Second)); got != (breaker.Change{}) {
			t.Errorf("Record = %+v, want no change", got)
		}
	})

	t.Run("stale non-trial result while open is ignored", func(t *testing.T) {
		t.Parallel()
		cs := openAtT0()
		if got := cs.Record("k", false, false, testThreshold, testCooldown, at(time.Second)); got != (breaker.Change{}) {
			t.Errorf("Record = %+v, want no change", got)
		}
		if g := cs.Allow("k", at(2*time.Second)); g.State != breaker.Open {
			t.Errorf("Allow = %+v, want still open", g)
		}
	})
}
