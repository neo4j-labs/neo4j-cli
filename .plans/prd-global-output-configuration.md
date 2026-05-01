# PRD: Global Output Configuration

## Overview

Today the `--output` flag and its config-backed default (`aura.output`) only apply to a subset of `neo4j aura` commands. Commands like `neo4j aura config list`, `neo4j aura config get`, and all `neo4j credential` commands ignore the output setting. There is also no way to set the output default outside of the aura-specific config.

This feature moves output configuration to a top-level viper key (`output`), removes `output` from the aura config, adds `neo4j config get/set/list` commands, and makes every command in the CLI respect the configured default.

## Goals

- Provide a single, CLI-wide default output format setting
- Add `neo4j config get <key>`, `neo4j config set <key> <value>`, and `neo4j config list` commands
- Ensure every command in every binary that produces output respects the `--output` flag and configured default
- Remove `output` from the aura-specific config (`aura.output` and `neo4j aura config set/get output`)

## Non-Goals

- Adding any top-level config keys beyond `output` in this iteration
- Changing the rendering format of credential commands (they will bind the flag correctly but rendering improvements are a follow-up)
- Supporting per-product output overrides (e.g., an aura-specific output that overrides the global one)
- Manual per-command output flag registration or binding — individual resource commands must not need to implement output logic themselves

## Requirements

### Functional Requirements

- REQ-F-001: `neo4j config get output` returns the currently configured output value (`default`, `json`, or `table`)
- REQ-F-002: `neo4j config set output <value>` persists the value to config.json at the top-level `output` key; invalid values return an error
- REQ-F-003: `neo4j config list` prints all top-level config keys and their current values as JSON
- REQ-F-004: `neo4j config get <invalid-key>` and `neo4j config set <invalid-key> <value>` return a usage error
- REQ-F-005: `output` is removed from aura `ValidConfigKeys`; `neo4j aura config set output <value>` returns "invalid config key specified: output"
- REQ-F-006: The `--output` flag on every resource command continues to override the config for that invocation only
- REQ-F-007: All `neo4j aura` resource commands resolve output via the top-level `output` key when the flag is not explicitly passed
- REQ-F-008: `neo4j aura config list` and `neo4j aura config get` respect the top-level output config for their rendered output
- REQ-F-009: `neo4j credential` subcommands bind the `--output` flag and respect the top-level output config
- REQ-F-012: `neo4j config list` and `neo4j config get` also respect the `--output` flag, rendering as JSON or table accordingly
- REQ-F-010: Users who previously set `aura.output` in config.json are silently migrated on first run: the value is moved to the top-level `output` key and the `aura.output` key is deleted
- REQ-F-011: Valid output values remain: `default`, `json`, `table` (unchanged)

### Non-Functional Requirements

- REQ-NF-001: The top-level config is stored in the same `config.json` file as the aura config (no new files)
- REQ-NF-002: The `neo4j config` command structure is extensible — adding new top-level keys in future requires only adding them to `ValidConfigKeys` in `GlobalConfig`
- REQ-NF-003: `make test` passes with no regressions
- REQ-NF-004: Output flag registration, validation, and viper binding must be centralized — individual resource-group commands (instance, tenant, etc.) must not contain any output flag logic

## Technical Considerations

### Config storage

The top-level output is stored at the root of `config.json` as `"output": "<value>"` (not under the `"aura"` subtree). Viper reads it via key `"output"` with default `"default"`.

```json
{
  "output": "json",
  "aura": {
    "auth-url": "...",
    "base-url": "...",
    "default-tenant": "..."
  }
}
```

### New `GlobalConfig` struct

Add a `GlobalConfig` struct to `common/clicfg/clicfg.go` backed by the same viper instance as `AuraConfig`. Add `Global *GlobalConfig` field to `Config`. Methods mirror `AuraConfig`'s config methods but operate on the top-level viper key:

- `Output() string` — reads `viper.GetString("output")`
- `BindOutput(flag *pflag.Flag)` — binds pflag to viper key `"output"`
- `Get(key string) interface{}` — reads `viper.Get(key)`
- `Set(key, value string)` — writes top-level JSON key via `sjson.Set(data, key, value)`
- `IsValidConfigKey(key string) bool`
- `Print(cmd *cobra.Command)` — JSON-encodes all valid keys

### Output method migration

Remove `Output()` and `BindOutput()` from `AuraConfig`. All callers change from `cfg.Aura.Output()` / `cfg.Aura.BindOutput()` to `cfg.Global.Output()` / `cfg.Global.BindOutput()`.

### Centralized flag registration at binary entry points via `EnableTraverseRunHooks`

Rather than each resource-group command registering and binding `--output` in its own `PersistentPreRunE`, the flag is lifted all the way to the root of each binary:

- `--output` is registered as a `PersistentFlag` on the root command of each binary — the `neo4j` root in `neo4j-cli/main.go` and the standalone root in `neo4j-cli/aura/cmd/main.go`
- `PersistentPreRunE` is added at the same root level: validates the flag value and calls `cfg.Global.BindOutput()`
- `cobra.EnableTraverseRunHooks = true` is set at each entry point so that the root hook always runs even when a child defines its own `PersistentPreRunE` (resource-group hooks for base-url/auth-url still run alongside the root hook)
- Resource-group `PersistentPreRunE` functions are stripped of all output logic; they only bind base-url and auth-url

Placing the flag at the binary root rather than at the `aura` intermediate level means every command in both binaries is covered with no exceptions — including `neo4j credential`, `neo4j config`, `neo4j aura config`, and any future top-level commands. No manual per-command wiring is needed.

### New command package

Create `neo4j-cli/internal/subcommands/config/` (new directory) with `config.go`, `get.go`, `set.go`, `list.go`. Register in `neo4j-cli/main.go` with `cmd.AddCommand(config.NewCmd(cfg))`.

### Silent config migration

In `NewConfig`, after reading the config file: if `viper.IsSet("aura.output")` and `!viper.IsSet("output")`, move the value to the top-level key via `GlobalConfig.Set` and delete `aura.output` from the file via `sjson.Delete`.

## Acceptance Criteria

- [ ] `neo4j config get output` returns `"default"` with an empty config
- [ ] `neo4j config set output json` writes `"output": "json"` at the root of config.json
- [ ] `neo4j config set output invalid` returns an error
- [ ] `neo4j config set unknown-key value` returns an error
- [ ] `neo4j config list` returns `{"output": "default"}` (or current value)
- [ ] `neo4j aura config set output json` returns "invalid config key specified: output"
- [ ] `neo4j aura config list` no longer shows `output`
- [ ] `neo4j aura instance list` respects `neo4j config set output json` without `--output` flag
- [ ] `neo4j aura config list` (the config command itself) respects the global output setting
- [ ] `neo4j credential list --output json` overrides the config for that invocation
- [ ] A config.json with `"aura": {"output": "json"}` is silently migrated on next run
- [ ] `make test` passes
- [ ] No resource-group command (instance, tenant, etc.) contains any `--output` flag registration or `BindOutput` call
- [ ] `neo4j config list --output table` renders config keys as a table
- [ ] `neo4j config get output --output json` renders as JSON

## Out of Scope

- Per-product output overrides (aura-specific output config that takes precedence over global)
- Adding other top-level config keys (auth-url, base-url promotion)
- Changing credential command rendering (table vs JSON formatting)

## Open Questions

None — all design decisions resolved via Q&A.

---

## Implementation Steps

### Step 1 — `common/clicfg/clicfg.go`
- Add `GlobalConfig` struct with `viper`, `fs`, `ValidConfigKeys []string`
- Add methods: `Output()`, `BindOutput()`, `Get()`, `Set()`, `IsValidConfigKey()`, `Print()`
- Add `Global *GlobalConfig` to `Config` struct
- Change `setDefaultValues`: `Viper.SetDefault("output", "default")` instead of `"aura.output"`
- Remove `"output"` from `AuraConfig.ValidConfigKeys` (line 79)
- Remove `Output()` and `BindOutput()` from `AuraConfig` (lines 207–215)
- Add silent migration block in `NewConfig`
- Populate `cfg.Global` with the shared viper instance

### Step 2 — `neo4j-cli/aura/internal/output/output.go`
- Change `cfg.Aura.Output()` → `cfg.Global.Output()` (line 20)

### Step 3 — Binary entry points (centralized output hook)
Add `--output` PersistentFlag and `PersistentPreRunE` to the root command at each entry point. The hook validates the flag value against `ValidOutputValues` and calls `cfg.Global.BindOutput(cmd.Flags().Lookup("output"))`.

**`neo4j-cli/main.go` `NewCmd()`**: add to the `neo4j` root — covers `neo4j aura *`, `neo4j credential *`, `neo4j config *`, and all future top-level commands.

**`neo4j-cli/aura/cmd/main.go` `main()`**: add to the standalone aura-cli root returned by `aura.NewStandaloneCmd(cfg)` — covers all `aura-cli` commands including `credential`.

Consider extracting a shared `registerOutputFlag(cmd *cobra.Command, cfg *clicfg.Config)` helper (e.g., in a new `neo4j-cli/aura/internal/flags/` package) to avoid duplicating the validation logic between the two entry points.

### Step 4 — 8 resource-group `PersistentPreRunE` hooks (strip output logic)
In each of the following, remove the `--output` PersistentFlag registration and the output validation + `BindOutput` call. Keep only base-url and auth-url binding:
- `subcommands/instance/instance.go`
- `subcommands/tenant/tenant.go`
- `subcommands/customermanagedkey/customermanagedkey.go`
- `subcommands/graphanalytics/graphanalytics.go`
- `subcommands/graphanalytics/session/session.go`
- `subcommands/import/import.go`
- `subcommands/dataapi/data_api.go`
- `subcommands/deployment/deployment.go`

### Step 5 — Direct `cfg.Aura.Output()` call sites
Change to `cfg.Global.Output()`:
- `subcommands/import/job/get.go` (2 calls)
- `subcommands/tenant/get.go` (1 call)


### Step 6 — New `neo4j config` command package
Create `neo4j-cli/internal/subcommands/config/`:
- `config.go` — `NewCmd(cfg)`, assembles Get/Set/List subcommands
- `get.go` — `neo4j config get <key>`; validates key, prints `cfg.Global.Get(key)`
- `set.go` — `neo4j config set <key> <value>`; validates key + value, calls `cfg.Global.Set()`
- `list.go` — `neo4j config list`; calls `cfg.Global.Print(cmd)`

Mirror the structure of `neo4j-cli/aura/internal/subcommands/config/`.

### Step 7 — `neo4j-cli/main.go` and `neo4j-cli/aura/cmd/main.go`
- In `neo4j-cli/main.go` `NewCmd()`: call `registerOutputFlag(cmd, cfg)` and add `cmd.AddCommand(config.NewCmd(cfg))`; set `cobra.EnableTraverseRunHooks = true` in `main()` before `cmd.Execute()`
- In `neo4j-cli/aura/cmd/main.go`: call `registerOutputFlag(cmd, cfg)` on the standalone root; set `cobra.EnableTraverseRunHooks = true` before `cmd.Execute()`

### Step 8 — Test helper default fixture
`neo4j-cli/aura/internal/test/testutils/auratesthelper.go` — move `"output": "json"` from inside `"aura": {...}` to the top level of the embedded config JSON.

### Step 9 — Test files using `"aura.output"` key
Update `helper.SetConfigValue("aura.output", ...)` → `helper.SetConfigValue("output", ...)` in:
- `subcommands/instance/get_test.go`
- `subcommands/deployment/` (multiple test files)

### Step 10 — Aura config tests
- `subcommands/config/list_test.go` — drop `"output"` from expected JSON
- `subcommands/config/get_test.go` — update `config get output` test to expect an error
- `subcommands/config/set_test.go` — update expected error message for `config set output invalid`

### Step 11 — New `neo4j config` command tests
Create `neo4j-cli/internal/subcommands/config/config_test.go` covering all acceptance criteria above.

### Step 12 — `common/clicfg/clicfg_test.go`
Update config fixture: move `"output"` to top-level JSON.

## Verification

```sh
# Build
make build

# Manual smoke test
./bin/neo4j-cli config list
./bin/neo4j-cli config set output json
./bin/neo4j-cli config get output   # should return "json"
./bin/neo4j-cli aura config list    # should NOT show output
./bin/neo4j-cli aura config set output json  # should error

# Run all tests
make test
```
