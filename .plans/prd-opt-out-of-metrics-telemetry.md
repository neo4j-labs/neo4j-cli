# PRD: Opt-out of metrics telemetry (CLI-72)

## Overview

`neo4j-cli` emits Mixpanel events (`STARTUP`, `HELP`, `AURA`, `QUERY`, `SKILL`, `COMMAND`) on every invocation, identified by a hashed machine ID. Both `README.md` and the npm `distribution/npm/cli/README.md` already advertise the opt-out command `neo4j-cli config set telemetry false --rw`, but it errors with `invalid config key: "telemetry"` because the key isn't in `ValidConfigKeys` and `analytics.(*Analytics).Disable()` is never called from production code. This PRD wires the advertised opt-out up end-to-end and adds the `DO_NOT_TRACK=1` environment variable as an additional disable lever (consoledonottrack.com, de facto std).

Linear: https://linear.app/neo4j/issue/CLI-72/opt-out-of-metrics-telemetry

## Goals

- Make the documented command `neo4j-cli config set telemetry false --rw` work and persist the choice across invocations.
- Honor `DO_NOT_TRACK=1` (literal `"1"`) as a runtime override that disables telemetry without writing to the config file.
- Keep the change surgical: no new packages, no public-API changes to `analytics.Service`, no bundle drift.
- Cover the new behavior with unit tests next to the touched files.

## Non-Goals

- Adding a `--no-telemetry` CLI flag or `NEO4J_CLI_NO_TELEMETRY` env var.
- Honoring `DO_NOT_TRACK` values other than literal `"1"` (e.g. `"true"`, `"yes"`, any-non-empty).
- Reworking the analytics package (`common/analytics/`) internals — `Analytics.Disable()` is the existing entry point and stays as-is.
- Surfacing telemetry status in CLI output (e.g. a startup banner). `config get telemetry` and `config list` already render the value.
- Migrating already-stored Mixpanel data or providing a "delete my data" command.
- Adding any new top-level command (no skill bundle command-tree changes expected).

## Requirements

### Functional Requirements

- REQ-F-001: `telemetry` is a valid global config key. `neo4j-cli config set telemetry false --rw` succeeds and persists `telemetry: false` to `~/.config/neo4j/cli/config.json` (or platform equivalent).
- REQ-F-002: `neo4j-cli config set telemetry true --rw` succeeds and persists `telemetry: true`.
- REQ-F-003: `neo4j-cli config set telemetry <anything-else> --rw` returns the usage error `invalid value for 'telemetry': <val> (valid values: true, false)` and does NOT modify the config file.
- REQ-F-004: `neo4j-cli config set telemetry false` (no `--rw`) returns the existing write-gate error `this command writes; pass --rw to allow it`.
- REQ-F-005: `neo4j-cli config get telemetry` returns `true` (default) or the persisted boolean.
- REQ-F-006: `neo4j-cli config list` includes a `telemetry` row/key in `default`, `json`, and `table` formats. `--format json` emits `"telemetry": true` by default.
- REQ-F-007: When the persisted `telemetry` value is `false`, the analytics service is disabled before any event is emitted (including the `STARTUP` event that fires in `main.go` immediately after `clicfg.NewConfig`). No Mixpanel HTTP traffic is generated for that process.
- REQ-F-008: When `DO_NOT_TRACK=1` is set in the process env, the analytics service is disabled regardless of the persisted `telemetry` value. Values other than `"1"` (e.g. `""`, `"0"`, `"true"`, `"yes"`) do NOT trigger the disable path.
- REQ-F-009: Default behavior is unchanged: a fresh install with no `telemetry` key and no `DO_NOT_TRACK` env var keeps emitting events as today.
- REQ-F-010: A user-facing changelog entry of kind `Minor` is recorded via `changie` describing the opt-out (config key + env var).

### Non-Functional Requirements

- REQ-NF-001: Backwards-compatible — existing config files (no `telemetry` key) MUST continue working with telemetry enabled, no migration step.
- REQ-NF-002: The decision logic (config value + env var → disable?) is extracted into a small unexported helper in `common/clicfg/` so it can be unit-tested without spinning up the full `Config`.
- REQ-NF-003: No new external dependencies. Use only `viper`, `os`, and existing `clierr`/`clicfg` primitives.
- REQ-NF-004: All three local gates pass: `make fmt-check`, `make lint`, `make test`. CI gates (`make generate-check`, license-check) also stay green.
- REQ-NF-005: Storage format mirrors the existing `format` key — stored as a string `"true"`/`"false"` via `sjson.Set`; read back with `viper.GetBool` (cast auto-coerces). No change to `Set(key, value string)` signature.
- REQ-NF-006: Telemetry disable MUST happen synchronously inside `clicfg.NewConfig`, before the function returns the `*Config`, so the `STARTUP` event in `neo4j-cli/main.go` is already gated when emitted.
- REQ-NF-007: README updates ship in the same PR. Both `README.md` and `distribution/npm/cli/README.md` mention the `DO_NOT_TRACK=1` lever in addition to the existing `config set telemetry false --rw` example.

## Technical Considerations

### Files touched

- `common/clicfg/clicfg.go`
  - L137 — extend `globalConfig.ValidConfigKeys` to `[]string{"format", "telemetry"}`. Cascades into `validGetArgs`/`validSetArgs` (tab completion) and `Printable()` automatically.
  - L262 `setDefaultValues` — add `Viper.SetDefault("telemetry", true)`.
  - L433 `GlobalConfig.Set` — add a bool-value gate analogous to the `format` block:
    ```go
    if key == "telemetry" {
        if value != "true" && value != "false" {
            return clierr.NewUsageError("invalid value for 'telemetry': %s (valid values: true, false)", value)
        }
    }
    ```
  - L132 `NewConfig` — immediately after `events := analytics.NewAnalytics(...)`, gate via the new helper:
    ```go
    if shouldDisableTelemetry(Viper, os.Getenv) {
        events.Disable()
    }
    ```
  - New unexported helper (same file or a sibling, e.g. `telemetry.go` in `common/clicfg/`):
    ```go
    func shouldDisableTelemetry(v *viper.Viper, getenv func(string) string) bool {
        if !v.GetBool("telemetry") { return true }
        if getenv("DO_NOT_TRACK") == "1" { return true }
        return false
    }
    ```

- `neo4j-cli/internal/subcommands/config/set_test.go` — add table-driven rows:
  - `config set --rw telemetry false` → asserts `telemetry` written as `"false"`.
  - `config set --rw telemetry true` → asserts `telemetry` written as `"true"`.
  - `config set --rw telemetry maybe` → asserts usage error.
  - `config set telemetry false` (no `--rw`) → existing write-gate error.

- `neo4j-cli/internal/subcommands/config/list_test.go` — first two cases hard-code the exact JSON output of `config list`. Both must gain `"telemetry": true,` so the assertion still byte-matches. Table-format case uses `wantContains` and likely needs no edit; verify on the way.

- `common/clicfg/clicfg_test.go` (or a new `telemetry_test.go`) — table-driven `TestShouldDisableTelemetry`:
  - default (telemetry=true, no env) → `false`.
  - telemetry=false, no env → `true`.
  - telemetry=true, `DO_NOT_TRACK=1` → `true`.
  - telemetry=true, `DO_NOT_TRACK=0` → `false`.
  - telemetry=true, `DO_NOT_TRACK=""` → `false`.
  - telemetry=true, `DO_NOT_TRACK=true` → `false` (literal `"1"` only).

- `README.md:178` — keep the existing example; add one sentence after the code block: `Set DO_NOT_TRACK=1 to disable telemetry without writing config.`

- `distribution/npm/cli/README.md:71` — mirror the same one-sentence addition.

- `.changes/unreleased/neo4j-cli-Minor-…yaml` — via `changie new --projects neo4j-cli --kind Minor --body "Add telemetry opt-out via 'config set telemetry false' and the DO_NOT_TRACK=1 environment variable"`.

### Architecture / integration points

- The analytics worker goroutine in `analytics.(*Analytics).NewAnalyticsWithClient` starts before `events.Disable()` is callable. That's fine: `Disable()` flips a flag (`a.disabled = true`); `EmitEvent` checks the flag before sending to the channel (`common/analytics/analytics.go:134`). The worker stays alive but idle until `Flush()` runs in `main.go`. No data race because `Disable()` is called synchronously from `NewConfig` before the `*Config` is returned and before any other goroutine can touch `events`.
- `viper.GetBool` cast-coerces string-stored `"true"`/`"false"` correctly; no special-casing needed at read time.
- `Printable()` iterates `ValidConfigKeys` to render `config list`, so adding `"telemetry"` automatically surfaces it in all three formats.
- Bundle regen: `neo4j-cli/internal/skill/bundle/references/config.md` is a generic command-syntax page that does NOT enumerate `ValidConfigKeys`, so `make generate-check` is expected to stay clean. Run as a gate, not a fix step.

### Potential gotchas

- The list_test.go cases use exact-match JSON `wantOut` strings — adding a key without updating those will fail with a noisy diff. Update both cases.
- `t.Setenv("DO_NOT_TRACK", "")` is required to scope the env var per test; never rely on the ambient env.
- Don't accidentally bind `telemetry` to a Viper env (e.g. via `BindEnv`) — keep `DO_NOT_TRACK` handling explicit in the helper so the precedence rule ("env wins over config") is testable and reviewable.

## Acceptance Criteria

- [ ] `bin/neo4j-cli config set telemetry false --rw` exits 0 and writes `telemetry` to `config.json`.
- [ ] `bin/neo4j-cli config set telemetry true --rw` exits 0 and overwrites the value to `true`.
- [ ] `bin/neo4j-cli config set telemetry maybe --rw` exits non-zero with the documented usage error and does not modify `config.json`.
- [ ] `bin/neo4j-cli config set telemetry false` (no `--rw`) exits non-zero with the existing write-gate error.
- [ ] `bin/neo4j-cli config get telemetry` returns `true` on a fresh install and the persisted value after a write.
- [ ] `bin/neo4j-cli config list --format json` output includes `"telemetry"`.
- [ ] With `telemetry=false` persisted, an end-to-end smoke run (any subcommand or `--help`) generates ZERO Mixpanel HTTP requests. Verified by pointing `aura.base-url`/Mixpanel endpoint at a netcat listener or by injecting an `HTTPClient` mock in a test that exercises `NewConfig` → emit → flush.
- [ ] With `telemetry=true` persisted but `DO_NOT_TRACK=1` set in env, same: zero Mixpanel HTTP requests.
- [ ] With `telemetry=true` and `DO_NOT_TRACK` unset (or set to a value other than `"1"`), telemetry continues to fire as today.
- [ ] `TestShouldDisableTelemetry` covers all six rows enumerated above and passes.
- [ ] `TestConfigSet` covers the four new rows and passes.
- [ ] `TestConfigList` first two exact-match JSON cases include `"telemetry": true` and still pass.
- [ ] `make fmt-check && make lint && make test` is green.
- [ ] `make generate-check` is green (no bundle drift).
- [ ] A `changie` entry of kind `Minor` lives under `.changes/unreleased/` describing the feature in user-facing terms.
- [ ] `README.md` and `distribution/npm/cli/README.md` mention `DO_NOT_TRACK=1` alongside the existing `config set telemetry false --rw` example.

## Out of Scope

- New CLI flag for telemetry (`--no-telemetry`).
- Alternative env-var names (`NEO4J_CLI_NO_TELEMETRY`, `NO_TELEMETRY`, etc.).
- Broader `DO_NOT_TRACK` semantics (truthy/non-empty handling).
- Changes to `common/analytics/` internals, the Mixpanel SDK wrapper, or the event taxonomy.
- A new "telemetry status" subcommand or banner output.
- Bundle command-tree additions; reference page edits only if `make generate-check` flags drift.
- Backfill for users whose data is already in Mixpanel.

## Open Questions

None. The three open items from the planning phase were resolved before this PRD was generated:

1. `DO_NOT_TRACK` semantics → literal `"1"` only.
2. `NEO4J_CLI_NO_TELEMETRY` env var → out of scope.
3. `config list --format table` test expectations → verify during implementation (no pre-emptive edit).
