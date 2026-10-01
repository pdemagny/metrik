# Quickstart: Validate the Endpoint Circuit Breaker

Run everything from the repository root.

## 1. Automated checks

```shell
mise run lint
mise run test
```

Expected:
- Lint reports `0 issues.` for every module.
- The tests pass, including the breaker tests in `apps/metrik/pkg/breaker` and the `startCycle` tests in `apps/metrik/cmd`. They cover:
  - the state transitions in [data-model.md](data-model.md)
  - the validation table in [contracts/configuration.md](contracts/configuration.md)
  - the log and alert table in [contracts/log-events.md](contracts/log-events.md)

## 2. Local outage and recovery

Prerequisites: `mise install` has been run, and port 8080 is free.

1. Write a scratch config with a short cool-down. In `/tmp/metrik-cb/metrik.yaml`:

   ```yaml
   runInterval: "2s"
   maxConcurrent: 2
   requestTimeout: "1s"
   alertThreshold: "3s"
   maxTries: 1
   failureThreshold: 2
   cooldown: "6s"
   checks:
     - name: "live"
       endpoint: "http://localhost:8080/v1/.well-known/live"
   ```

2. Start the checker without the API running:

   ```shell
   (cd apps/metrik && go build -o /tmp/metrik-cb/metrik .)
   (cd /tmp/metrik-cb && ./metrik run)   # picks up ./metrik.yaml
   ```

   Expected, in order:
   - 2 × `check failed` (each followed by an `ALERT`), then `circuit opened` with `failures: 2` and `cooldown: "6s"`.
   - 2 × `check skipped` with decreasing `remaining`, and no `ALERT`.
   - A trial probe: `check failed` + `ALERT`, then `circuit reopened`.

3. In another terminal, start the API: `(cd apps/api && go run .)`

   Expected, within one cool-down plus one interval (≤ 8s): `check completed`, then `circuit closed` with a `suspended` duration. After that, a normal `check completed` every 2s.

4. Disable the breaker with `METRIK_FAILURETHRESHOLD=0` on the command from step 2, then stop the API.

   Expected: a `check failed` + `ALERT` every 2s, with no circuit entries. This is today's behaviour (SC-006).

5. Edit the scratch config to set `cooldown: "-1s"` while the checker runs.

   Expected: a fatal entry naming `cooldown` and `must not be negative`, and the process exits.

## 3. Demo configurations

```shell
grep -n "notfound\|8081" apps/metrik/configs/*.yaml
```

Expected: no matches. The always-failing demo checks are gone from the example, dev and prod configs. `TestExampleConfigLoads` passes against the trimmed example.
