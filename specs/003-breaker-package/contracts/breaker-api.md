# Contract: `metrik/pkg/breaker` API

This is the library interface that `cmd` (and future callers) depend on. Its behaviour is that of
[feature 002's data model](../../002-endpoint-circuit-breaker/data-model.md).
Operator-facing contracts (configuration, log events) are unchanged. See
[002 contracts](../../002-endpoint-circuit-breaker/contracts/).

```go
package breaker // imports: sync, time only

type State string

const (
	Closed State = "closed"
	Open   State = "open"
	Trial  State = "trial"
)

// Gate tells the caller whether to probe a check this cycle.
type Gate struct {
	Run, Trial bool
	State      State         // Set when Run is false.
	Remaining  time.Duration // Time left in the cool-down, when skipped.
}

type Kind int

const (
	NoChange Kind = iota
	Opened
	Reopened
	Recovered
)

// Change is the state transition caused by recording a cycle.
type Change struct {
	Kind      Kind
	Failures  int           // Set on Opened.
	Suspended time.Duration // Set on Recovered.
}

// Breaker holds the circuit of every check, keyed by an opaque identity.
// It is safe for concurrent use.
type Breaker struct{ /* unexported */ }

func New() *Breaker

// Allow reports whether the check identified by key is probed at now.
// From Open, once the cool-down has elapsed, it moves the circuit to Trial
// and returns Gate{Run: true, Trial: true} exactly once.
func (b *Breaker) Allow(key string, now time.Time) Gate

// Record applies the outcome of one cycle. trial MUST be the Gate.Trial
// returned by Allow for that cycle. threshold MUST be > 0: callers bypass
// the breaker entirely for a threshold of 0.
func (b *Breaker) Record(key string, trial, failed bool, threshold int, cooldown time.Duration, now time.Time) Change

// Keep drops the circuit of every key not in keys.
func (b *Breaker) Keep(keys map[string]struct{})
```

## Caller obligations (`cmd/startCycle`)

1. Call `Keep` once per cycle with the IDs of the checks whose threshold is > 0.
2. For each check with a threshold > 0, call `Allow` before taking a concurrency slot. Skip the check when `!Gate.Run`, and log `check skipped` with `circuit = string(Gate.State)`.
3. After the probe, call `Record` with the same `Gate.Trial`, and log `Change` per [log-events](../../002-endpoint-circuit-breaker/contracts/log-events.md).
4. Use `(*check.Check).ID()` as the key.

## `metrik/pkg/check` addition

```go
// ID identifies a check by name and endpoint, so a changed endpoint is a new check.
func (c *Check) ID() string // Name + "\x00" + Endpoint
```
