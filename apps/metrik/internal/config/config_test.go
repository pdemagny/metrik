package config

import (
	"strings"
	"testing"
	"time"

	"metrik/pkg/check"

	"github.com/spf13/viper"
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

	t.Run("circuit breaker settings", func(t *testing.T) {
		t.Parallel()
		settings := Config{FailureThreshold: 3, Cooldown: time.Minute}
		zero, five := 0, 5

		tests := []struct {
			name      string
			check     check.Check
			threshold int
			cooldown  time.Duration
		}{
			{name: "inherited", check: check.Check{}, threshold: 3, cooldown: time.Minute},
			{name: "disabled", check: check.Check{FailureThreshold: &zero}, threshold: 0, cooldown: time.Minute},
			{name: "threshold override", check: check.Check{FailureThreshold: &five}, threshold: 5, cooldown: time.Minute},
			{name: "cooldown override", check: check.Check{Cooldown: 2 * time.Minute}, threshold: 3, cooldown: 2 * time.Minute},
		}
		for _, tt := range tests {
			in := tt.check
			got := settings.Resolve(&in)
			if got.FailureThreshold == nil || *got.FailureThreshold != tt.threshold {
				t.Errorf("%s: failureThreshold = %v, want %d", tt.name, got.FailureThreshold, tt.threshold)
			}
			if in.FailureThreshold != nil && got.FailureThreshold == in.FailureThreshold {
				t.Errorf("%s: resolved failureThreshold aliases the input pointer", tt.name)
			}
			if got.Cooldown != tt.cooldown {
				t.Errorf("%s: cooldown = %s, want %s", tt.name, got.Cooldown, tt.cooldown)
			}
		}
	})

	t.Run("zero max tries everywhere stays bounded", func(t *testing.T) {
		t.Parallel()
		got := Config{}.Resolve(&check.Check{})
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

			got := global.Resolve(&in)

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
	v.SetConfigFile("../../configs/metrik.yaml")
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
	if err := cfg.Validate(); err != nil {
		t.Errorf("validate = %v, want nil", err)
	}
	if cfg.RunInterval != 15*time.Second || cfg.MaxConcurrent != 10 || cfg.RequestTimeout != 5*time.Second ||
		cfg.AlertThreshold != 3*time.Second || cfg.MaxTries != 3 || cfg.FailureThreshold != 3 || cfg.Cooldown != time.Minute {
		t.Errorf("globals = %+v, want runInterval 15s, maxConcurrent 10, requestTimeout 5s, alertThreshold 3s, maxTries 3, "+
			"failureThreshold 3, cooldown 1m", cfg)
	}

	want := []check.Check{
		{Name: "liveness ok", Endpoint: "http://localhost:8080/v1/.well-known/live"},
		{Name: "liveness slow timeout", Endpoint: "http://localhost:8080/v1/slow", RequestTimeout: time.Second},
		{Name: "liveness slow warn", Endpoint: "http://localhost:8080/v1/slow", AlertThreshold: 3 * time.Second},
		{Name: "readiness flaky", Endpoint: "http://localhost:8080/v1/.well-known/ready"},
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

func TestValidate(t *testing.T) {
	t.Parallel()
	negative := -1

	tests := []struct {
		name string
		cfg  Config
		want []string // Substrings of the error; nil means no error.
	}{
		{name: "zeros and unset are valid", cfg: Config{Checks: []*check.Check{{Name: "x"}}}},
		{
			name: "negative global threshold",
			cfg:  Config{FailureThreshold: -1},
			want: []string{"failureThreshold", "must not be negative"},
		},
		{
			name: "negative global cooldown",
			cfg:  Config{Cooldown: -5 * time.Second},
			want: []string{"cooldown", "must not be negative"},
		},
		{
			name: "negative check threshold",
			cfg:  Config{Checks: []*check.Check{{Name: "x", FailureThreshold: &negative}}},
			want: []string{`check "x"`, "failureThreshold", "must not be negative"},
		},
		{
			name: "negative check cooldown",
			cfg:  Config{Checks: []*check.Check{{Name: "x", Cooldown: -5 * time.Second}}},
			want: []string{`check "x"`, "cooldown", "must not be negative"},
		},
		{
			name: "run interval below tries x timeout",
			cfg:  Config{RunInterval: 10 * time.Second, MaxTries: 3, RequestTimeout: 5 * time.Second},
			want: []string{"runInterval (10s)", "maxTries x requestTimeout (3 x 5s)"},
		},
		{
			name: "run interval equal to tries x timeout",
			cfg: Config{
				RunInterval: 15 * time.Second, MaxTries: 3, RequestTimeout: 5 * time.Second,
				Checks: []*check.Check{{Name: "x"}},
			},
		},
		{
			name: "zero tries count as one",
			cfg: Config{
				RunInterval: 5 * time.Second, RequestTimeout: 5 * time.Second,
				Checks: []*check.Check{{Name: "x"}},
			},
		},
		{
			name: "zero tries, run interval below timeout",
			cfg:  Config{RunInterval: 4 * time.Second, RequestTimeout: 5 * time.Second},
			want: []string{"runInterval (4s)", "(1 x 5s)"},
		},
		{
			name: "check timeout override breaks the rule",
			cfg: Config{
				RunInterval: 15 * time.Second, MaxTries: 3, RequestTimeout: 5 * time.Second,
				Checks: []*check.Check{{Name: "x", RequestTimeout: 6 * time.Second}},
			},
			want: []string{`check "x"`, "runInterval (15s)", "(3 x 6s)"},
		},
		{
			name: "check tries override breaks the rule",
			cfg: Config{
				RunInterval: 15 * time.Second, MaxTries: 3, RequestTimeout: 5 * time.Second,
				Checks: []*check.Check{{Name: "x", MaxTries: 4}},
			},
			want: []string{`check "x"`, "runInterval (15s)", "(4 x 5s)"},
		},
		{
			name: "check overrides within the rule",
			cfg: Config{
				RunInterval: 15 * time.Second, MaxTries: 3, RequestTimeout: 5 * time.Second,
				Checks: []*check.Check{{Name: "x", MaxTries: 5, RequestTimeout: 3 * time.Second}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.want == nil {
				if err != nil {
					t.Fatalf("validate = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validate = nil, want an error containing %q", tt.want)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("validate = %q, want it to contain %q", err, w)
				}
			}
		})
	}
}
