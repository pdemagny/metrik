# Contract: Configuration

How operators control the breaker. This extends the existing `metrik.yaml` / flag / environment surface without changing any existing key.

## Global settings

| YAML key | Flag | Environment variable | Default |
|---|---|---|---|
| `failureThreshold` | `--failureThreshold` | `METRIK_FAILURETHRESHOLD` | `3` |
| `cooldown` | `--cooldown` | `METRIK_COOLDOWN` | `60s` |

Precedence is the same as for the existing settings (viper): flag > environment > config file > flag default.

## Per-check settings

```yaml
checks:
  - name: "readiness flaky"
    endpoint: "http://api:8080/v1/.well-known/ready"
    failureThreshold: 5   # optional; unset = global, 0 = breaker off for this check
    cooldown: "2m"        # optional; unset or 0 = global
```

As with the other per-check settings, these can only be set in the config file, not through flags or environment variables.

## Validation

Startup and reload fail with a fatal log entry if any of these values is negative:

| Input | Error message contains |
|---|---|
| `failureThreshold: -1` (global) | `failureThreshold` and `must not be negative` |
| `cooldown: "-5s"` (global) | `cooldown` and `must not be negative` |
| check `x` with `failureThreshold: -1` | `check "x"`, `failureThreshold` and `must not be negative` |
| check `x` with `cooldown: "-5s"` | `check "x"`, `cooldown` and `must not be negative` |

## Compatibility

- Existing configs keep working unchanged. They get the breaker with the defaults, which is on by default (spec FR-012).
- `failureThreshold: 0` globally restores exactly the current behaviour (SC-006).
