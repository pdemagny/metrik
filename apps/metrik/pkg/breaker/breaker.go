// Package breaker suspends checks that keep failing and resumes them after a
// trial probe. See specs/002-endpoint-circuit-breaker/data-model.md for the
// state machine.
package breaker

import (
	"sync"
	"time"
)

// State is the circuit breaker state of one check.
type State string

const (
	Closed State = "closed"
	Open   State = "open"
	Trial  State = "trial"
)

type (
	circuit struct {
		state     State
		failures  int
		openUntil time.Time
		openedAt  time.Time
	}

	// Breaker holds the circuit of every check, keyed by an opaque identity.
	// It is safe for concurrent use.
	Breaker struct {
		mu sync.Mutex
		m  map[string]*circuit
	}
)

// New returns a Breaker with every circuit closed.
func New() *Breaker {
	return &Breaker{m: map[string]*circuit{}}
}

type (
	// Gate tells the caller whether to probe a check this cycle.
	Gate struct {
		Run, Trial bool
		State      State         // Set when Run is false.
		Remaining  time.Duration // Time left in the cool-down, when skipped.
	}

	// Kind is the kind of transition a Change reports.
	Kind int

	// Change is the state transition caused by recording a cycle.
	Change struct {
		Kind      Kind
		Failures  int           // Set on Opened.
		Suspended time.Duration // Set on Recovered.
	}
)

const (
	NoChange Kind = iota
	Opened
	Reopened
	Recovered
)

// get returns the circuit for key, creating it closed. b.mu must be held.
func (b *Breaker) get(key string) *circuit {
	c, ok := b.m[key]
	if !ok {
		c = &circuit{state: Closed}
		b.m[key] = c
	}
	return c
}

// Allow reports whether the check identified by key is probed at now. Once the
// cool-down of an Open circuit has elapsed, it moves the circuit to Trial and
// returns Gate{Run: true, Trial: true} exactly once.
func (b *Breaker) Allow(key string, now time.Time) Gate {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.get(key)

	switch {
	case c.state == Trial:
		return Gate{State: Trial}
	case c.state == Open && now.Before(c.openUntil):
		return Gate{State: Open, Remaining: c.openUntil.Sub(now)}
	case c.state == Open:
		c.state = Trial
		return Gate{Run: true, Trial: true}
	}
	return Gate{Run: true}
}

// Record applies the outcome of one cycle of the check identified by key.
// trial must be the Gate.Trial that Allow returned for that cycle. threshold
// must be positive: callers bypass the breaker entirely for a threshold of 0.
func (b *Breaker) Record(key string, trial, failed bool, threshold int, cooldown time.Duration, now time.Time) Change {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.get(key)

	if trial {
		if c.state != Trial { // Pruned by a reload while the trial was in flight.
			return Change{}
		}
		if failed {
			c.state, c.openUntil = Open, now.Add(cooldown)
			return Change{Kind: Reopened}
		}
		suspended := now.Sub(c.openedAt)
		*c = circuit{state: Closed}
		return Change{Kind: Recovered, Suspended: suspended}
	}
	// A probe started before the circuit opened: counting it would count that cycle twice.
	if c.state != Closed {
		return Change{}
	}
	if !failed {
		c.failures = 0
		return Change{}
	}
	c.failures++
	if c.failures < threshold {
		return Change{}
	}
	c.state, c.openUntil, c.openedAt = Open, now.Add(cooldown), now
	return Change{Kind: Opened, Failures: c.failures}
}

// Keep drops the circuit of every key not in keys, so removed checks and
// changed endpoints do not keep stale state.
func (b *Breaker) Keep(keys map[string]struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for key := range b.m {
		if _, ok := keys[key]; !ok {
			delete(b.m, key)
		}
	}
}
