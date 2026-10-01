// Package config holds metrik's settings: the global defaults and the checks,
// with their validation and per-check resolution.
package config

import (
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

// Validate rejects settings that have no meaning, such as negative circuit
// breaker values.
func (cfg Config) Validate() error {
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
	}
	return nil
}

// Resolve returns a copy of c whose AlertThreshold, MaxTries, RequestTimeout and
// Cooldown fall back to the global value when unset (zero) on the check, and
// whose FailureThreshold falls back to the global value when nil.
func (cfg Config) Resolve(c *check.Check) *check.Check {
	r := *c
	if r.AlertThreshold == 0 {
		r.AlertThreshold = cfg.AlertThreshold
	}
	if r.MaxTries == 0 {
		r.MaxTries = cfg.MaxTries
	}
	// Zero tries means unlimited retries to the backoff library; always keep them bounded.
	if r.MaxTries == 0 {
		r.MaxTries = 1
	}
	if r.RequestTimeout == 0 {
		r.RequestTimeout = cfg.RequestTimeout
	}
	// Always a fresh pointer, so the copy never aliases the input check.
	threshold := cfg.FailureThreshold
	if r.FailureThreshold != nil {
		threshold = *r.FailureThreshold
	}
	r.FailureThreshold = &threshold
	if r.Cooldown == 0 {
		r.Cooldown = cfg.Cooldown
	}
	return &r
}
