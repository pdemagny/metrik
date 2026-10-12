// Package config holds metrik's settings: the global defaults and the checks,
// with their validation and per-check resolution.
package config

import (
	"cmp"
	"fmt"
	"time"

	"metrik/pkg/check"
)

type (
	// Config is the decoded metrik.yaml (and its flag/env overrides).
	Config struct {
		Debug            bool           `yaml:"debug"`
		Verbose          bool           `yaml:"verbose"`
		RunInterval      time.Duration  `yaml:"runInterval"`
		MaxConcurrent    int            `yaml:"maxConcurrent"`
		RequestTimeout   time.Duration  `yaml:"requestTimeout"`
		AlertThreshold   time.Duration  `yaml:"alertThreshold"`
		MaxTries         uint           `yaml:"maxTries"`
		FailureThreshold int            `yaml:"failureThreshold"`
		Cooldown         time.Duration  `yaml:"cooldown"`
		Checks           []*check.Check `yaml:"checks"`
	}
)

// Validate rejects settings that have no meaning, such as a runInterval or a
// requestTimeout that is not positive or negative circuit breaker values, and
// a runInterval too short for a probe to use all its tries.
func (cfg Config) Validate() error {
	// A zero or negative interval never ticks, so no check would ever run.
	if cfg.RunInterval <= 0 {
		return fmt.Errorf("runInterval must be positive, got %s", cfg.RunInterval)
	}
	// A zero or negative timeout means no timeout to the HTTP client, so a probe could hang forever.
	if cfg.RequestTimeout <= 0 {
		return fmt.Errorf("requestTimeout must be positive, got %s", cfg.RequestTimeout)
	}
	if tries := max(cfg.MaxTries, 1); tooShort(cfg.RunInterval, tries, cfg.RequestTimeout) {
		return fmt.Errorf("runInterval (%s) must not be below maxTries x requestTimeout (%d x %s)",
			cfg.RunInterval, tries, cfg.RequestTimeout)
	}
	if cfg.FailureThreshold < 0 {
		return fmt.Errorf("failureThreshold must not be negative, got %d", cfg.FailureThreshold)
	}
	if cfg.Cooldown < 0 {
		return fmt.Errorf("cooldown must not be negative, got %s", cfg.Cooldown)
	}
	for _, c := range cfg.Checks {
		if c.FailureThreshold != nil && *c.FailureThreshold < 0 {
			return fmt.Errorf("check %q: failureThreshold must not be negative, got %d", c.Name, *c.FailureThreshold)
		}
		if c.Cooldown < 0 {
			return fmt.Errorf("check %q: cooldown must not be negative, got %s", c.Name, c.Cooldown)
		}
		if c.RequestTimeout != nil && *c.RequestTimeout <= 0 {
			return fmt.Errorf("check %q: requestTimeout must be positive, got %s", c.Name, *c.RequestTimeout)
		}
		if r := cfg.Resolve(c); tooShort(cfg.RunInterval, r.MaxTries, *r.RequestTimeout) {
			return fmt.Errorf("check %q: runInterval (%s) must not be below maxTries x requestTimeout (%d x %s)",
				c.Name, cfg.RunInterval, r.MaxTries, *r.RequestTimeout)
		}
	}
	return nil
}

// tooShort reports whether interval is below tries x timeout. It divides
// rather than multiplies, so a large tries value cannot overflow.
func tooShort(interval time.Duration, tries uint, timeout time.Duration) bool {
	if timeout <= 0 {
		return false
	}
	n := int64(interval / timeout)
	if n < 0 {
		return true
	}
	return uint64(n) < uint64(tries)
}

// Resolve returns a copy of c whose AlertThreshold, MaxTries and Cooldown fall
// back to the global value when unset (zero) on the check, and whose
// RequestTimeout and FailureThreshold fall back to the global value when nil.
func (cfg Config) Resolve(c *check.Check) *check.Check {
	r := *c
	r.AlertThreshold = cmp.Or(r.AlertThreshold, cfg.AlertThreshold)
	// Zero tries means unlimited retries to the backoff library; always keep them bounded.
	r.MaxTries = cmp.Or(r.MaxTries, cfg.MaxTries, 1)
	// Always fresh pointers, so the copy never aliases the input check.
	timeout := cfg.RequestTimeout
	if r.RequestTimeout != nil {
		timeout = *r.RequestTimeout
	}
	r.RequestTimeout = &timeout
	threshold := cfg.FailureThreshold
	if r.FailureThreshold != nil {
		threshold = *r.FailureThreshold
	}
	r.FailureThreshold = &threshold
	r.Cooldown = cmp.Or(r.Cooldown, cfg.Cooldown)
	return &r
}
