# PRD: Global Output Configuration

## Overview

Today the `--output` flag and its config-backed default (`aura.output`) only apply to a subset of `neo4j aura` commands. Commands like `neo4j aura config list`, `neo4j aura config get`, and all `neo4j credential` commands ignore the output setting. There is also no way to set the output default outside of the aura-specific config.

This feature moves output configuration to a top-level viper key (`output`), removes `output` from the aura config, adds `neo4j config get/set/list` commands, and makes every command in the CLI respect the configured default. It also restructures the aura config commands to mirror the credential pattern: `neo4j aura config` is removed and replaced by `neo4j config aura`, while the standalone `aura-cli config` command becomes a flat interface over both global and aura-scoped keys.

## Goals

- Provide a single, CLI-wide default output format setting
- Add `neo4j config get <key>`, `neo4j config set <key> <value>`, and `neo4j config list` commands for global config
- Add `neo4j config aura get/set/list` commands for aura-scoped config, replacing `neo4j aura config`
- Ensure every command in every binary that produces output respects the `--output` flag and configured default
- Remove `output` from the aura-specific config (`aura.output` and any aura config set/get output path)
- Provide a flat `aura-cli config` interface in standalone mode covering both global and aura-scoped keys, following the same pattern as credentials

## Non-Goals

- Adding any top-level config keys beyond `output` in this iteration
- Supporting per-product output overrides (e.g., an aura-specific output that overrides the global one)
- Manual per-command output flag registration or binding — individual resource commands must not need to implement output logic themselves

## Requirements

### Functional Requirements

- REQ-F-001: `neo4j config get output` returns the currently configured output value (`default`, `json`, or `table`)
- REQ-F-002: `neo4j config set output <value>` persists the value to config.json at the top-level `output` key; invalid values return an error
- REQ-F-003: `neo4j config list` prints all top-level config keys and their current values as JSON
- REQ-F-004: `neo4j config get <invalid-key>` and `neo4j config set <invalid-key> <value>` return a usage error
- REQ-F-005: `output` is removed from aura `ValidConfigKeys`; `neo4j config aura set output <value>` returns "invalid config key specified: output"
- REQ-F-006: The `--output` flag on every resource command continues to override the config for that invocation only
- REQ-F-007: All `neo4j aura` resource commands resolve output via the top-level `output` key when the flag is not explicitly passed
- REQ-F-008: `neo4j config aura list` and `neo4j config aura get` respect the top-level output config for their rendered output
- REQ-F-009: `neo4j credential` subcommands bind the `--output` flag and respect the top-level output config
- REQ-F-012: `neo4j config list` and `neo4j config get` also respect the `--output` flag, rendering as JSON or table accordingly
- REQ-F-010: The one-time `aura.output` → `output` migration block in `NewConfig` is commented out (not deleted), with a comment explaining it is untested code reserved for a future stable-release upgrade path and must not run in this experimental release
- REQ-F-011: Valid output values remain: `default`, `json`, `table` (unchanged)
- REQ-F-013: When running `aura-cli` standalone, `aura-cli config` is a flat command that handles both global (no `.` in the value) and aura-scoped (`aura.` viper prefix) config keys; routing is determined by which `ValidConfigKeys` list the key belongs to
- REQ-F-014: When running `aura-cli` standalone, `aura-cli config set output <value>` persists to the global `output` key; `aura-cli config set auth-url <value>` persists to the aura-scoped `aura.auth-url` key; invalid keys or values return errors
- REQ-F-015: When running `aura-cli` standalone, `aura-cli config list` shows all keys from both global and aura config domains
- REQ-F-016: `neo4j aura config` is removed; aura config is accessible at `neo4j config aura get/set/list`, following the same pattern as `neo4j credential` (moved from `neo4j aura credential`)
- REQ-F-017: `neo4j config aura get <key>`, `neo4j config aura set <key> <value>`, and `neo4j config aura list` behave identically to the former `neo4j aura config` commands (same keys, same validation, same output rendering)
- REQ-F-018: All config commands (`neo4j config get/list`, `neo4j config aura get/list`, `aura-cli config get/list`) use `common/output.PrintBodyMap` with a `ConfigData` value for rendering; duplicate private JSON/table helpers are removed from each config package; the `default` output mode renders as a table (consistent with all other commands)
- REQ-F-019: `neo4j credential list`, `neo4j credential get`, `aura-cli credential list`, and `aura-cli credential get` respect the `--output` flag: `--output table` renders a structured table with named columns (same pattern as `neo4j aura instance list`), `--output json` renders JSON; the default output format is respected when no flag is passed
- REQ-F-020: Credential rendering uses `common/output.PrintBodyMap` (via the `aura/internal/output` adapter) with the appropriate field list, rather than calling `cfg.Credentials.Aura.Print` directly

### Non-Functional Requirements

- REQ-NF-001: The top-level config is stored in the same `config.json` file as the aura config (no new files)
- REQ-NF-002: The `neo4j config` command structure is extensible — adding new top-level keys in future requires only adding them to `ValidConfigKeys` in `GlobalConfig`
- REQ-NF-003: `make test` passes with no regressions
- REQ-NF-004: Output flag registration, validation, and viper binding must be centralized — individual resource-group commands (instance, tenant, etc.) must not contain any output flag logic
- REQ-NF-005: The `ResponseData` interface and the core rendering logic are extracted from `neo4j-cli/aura/internal/output/output.go` and `neo4j-cli/aura/internal/api/response.go` into a new `common/output` package, making them available to all sub-CLIs; `aura/internal/output/output.go` becomes a thin adapter; the extraction and its design rationale are documented in AGENTS.md
- REQ-NF-006: New tests use table-driven style (`for _, tc := range []struct{...}{...}`) and are split into per-command files (`get_test.go`, `set_test.go`, `list_test.go`) matching the rest of the repo's convention; AGENTS.md is updated with this naming convention
- REQ-NF-007: No config or credential subcommand package defines its own JSON/table rendering logic; all rendering goes through `common/output.PrintBodyMap`

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

### Silent config migration (disabled for experimental release)

The migration block in `NewConfig` is commented out rather than deleted. This code would move `aura.output` → `output` on first run for users upgrading from an old config. It must not run in this experimental release because users may switch between the stable and experimental CLIs; running the migration in the experimental version would break the stable version's `aura.output` config. The block is kept as commented-out reference code for when the stable release is ready to ship this change.

### Config command restructuring (credential pattern)

The aura config commands follow the same pattern used for credentials: they are lifted out of `neo4j aura` and made accessible at the root `neo4j config` level. Concretely:

- `config.NewCmd(cfg)` (the aura config package) is no longer added in `aura.NewCmd`. It is instead added as an `aura` subcommand of the `neo4j-cli/internal/subcommands/config` package, so the full path becomes `neo4j config aura`.
- `neo4j-cli/main.go` registers the top-level `config.NewCmd(cfg)` (global config) which now also includes an `aura` subcommand wiring `neo4j config aura`.
- `neo4j aura config` no longer exists; existing users must migrate to `neo4j config aura`.

Export a `NewAuraSubCmd(cfg)` function from the aura config package (or re-use `NewCmd`) and register it under the global config command.

### Standalone aura-cli flat config

The standalone `aura-cli config` command must expose both global and aura-scoped keys in a single flat interface — no `aura-cli config aura` subcommand. This mirrors how `aura-cli credential` works: credentials are flat at the root, not nested.

Recommended approach: create a `NewStandaloneConfigCmd(cfg)` that builds a merged `config` command whose `get`/`set`/`list` subcommands are aware of all keys from `cfg.Aura.ValidConfigKeys` and `cfg.Global.ValidConfigKeys`. Routing logic:
- For `set`/`get`: if key is in `cfg.Global.ValidConfigKeys` → call `cfg.Global` methods; if in `cfg.Aura.ValidConfigKeys` → call `cfg.Aura` methods; otherwise return "invalid config key" error.
- For `list`: render all keys from both domains.

`NewStandaloneCmd` replaces the current `config.NewCmd(cfg)` reference with `NewStandaloneConfigCmd(cfg)`. Global and aura keys are currently disjoint, so routing is unambiguous. Adding new keys to either domain requires only updating the relevant `ValidConfigKeys` slice.

### ConfigData type in common/output

Add a `ConfigData` type to `common/output/output.go` that satisfies `ResponseData` and can be passed directly to the existing `PrintBodyMap`:

```go
type ConfigData []ConfigEntry

type ConfigEntry struct {
    Key   string
    Value interface{}
}

// AsArray returns rows suitable for table rendering: [{"key": k, "value": v}, ...]
func (c ConfigData) AsArray() []map[string]any { ... }

// GetSingleOrError returns the single entry as a map or an error if len != 1
func (c ConfigData) GetSingleOrError() (map[string]any, error) { ... }

// MarshalJSON produces a flat map {"key1": v1, "key2": v2, ...} so that
// json.MarshalIndent in PrintBodyMap emits the expected config JSON shape
func (c ConfigData) MarshalJSON() ([]byte, error) { ... }
```

Config commands pass `ConfigData` to `PrintBodyMap` with fields `["key", "value"]`:

```go
// config list
data := output.ConfigData{{Key: "output", Value: cfg.Global.Get("output")}, ...}
output.PrintBodyMap(cmd, cfg, data, []string{"key", "value"})

// config get
data := output.ConfigData{{Key: key, Value: value}}
output.PrintBodyMap(cmd, cfg, data, []string{"key", "value"})
```

The `default` output mode is handled by `PrintBodyMap`'s `"table", "default"` case — config commands render as a table by default, consistent with all other commands.

The following private rendering functions must be deleted entirely (not refactored):
- `printAuraConfigTable` — `aura/internal/subcommands/config/list.go`
- `printConfigKeyValueAsJSON` — `aura/internal/subcommands/config/list.go`
- `printConfigKeyValueAsTable` — `aura/internal/subcommands/config/list.go`
- `printGlobalConfigTable` — `neo4j-cli/internal/subcommands/config/list.go`
- `printConfigKeyValueAsJSON` — `neo4j-cli/internal/subcommands/config/list.go`
- `printConfigKeyValueAsTable` — `neo4j-cli/internal/subcommands/config/list.go`
- `standaloneConfigPrintKeyValueAsJSON` — `aura/standalone_config.go`
- `standaloneConfigPrintKeyValueAsTable` — `aura/standalone_config.go`
- `standaloneConfigPrintAllJSON` — `aura/standalone_config.go`
- `standaloneConfigPrintAllTable` — `aura/standalone_config.go`

The now-redundant `PrintAuraConfig` method on `AuraConfig` and `Print` method on `GlobalConfig` are also deleted.

### Credential rendering update

Credential list/get commands currently call `cfg.Credentials.Aura.Print` which always outputs JSON. They should instead:

1. Convert the credential data to a type satisfying `common/output.ResponseData` (implement `AsArray()` and `GetSingleOrError()`)
2. Call `output.PrintBodyMap` with the appropriate field list (e.g., `["name", "client-id"]`)

This ensures `--output table` produces a structured table matching the pattern of `neo4j aura instance list`.

### output.go extraction to common

`ResponseData` (defined in `neo4j-cli/aura/internal/api/response.go`) is a generic interface over `[]map[string]any` with no aura-specific dependencies — it belongs in `common`. The rendering functions in `neo4j-cli/aura/internal/output/output.go` (`PrintBodyMap`, `printTable`, `getNestedField`) also have no aura-specific dependencies beyond the `ResponseData` interface.

Extraction plan:
1. Create `common/output/output.go` with the `ResponseData` interface (`AsArray() []map[string]any` and `GetSingleOrError() (map[string]any, error)`) and all rendering functions (`PrintBodyMap`, `printTable`, `getNestedField`), accepting `common/output.ResponseData`.
2. `api/response.go` retains `ListResponseData`, `SingleValueResponseData`, and `ParseBody` — they already satisfy the interface, so no changes needed beyond the import path update.
3. `aura/internal/output/output.go` becomes a thin adapter: it keeps `PrintBody` (which calls `api.ParseBody` and delegates to `common/output.PrintBodyMap`) and re-exports `PrintBodyMap` as a pass-through so callers don't need import changes.
4. All callers continue importing `aura/internal/output` — no changes required at call sites.
5. Document the package design (interface in common, parse logic stays in api, thin adapter in aura) in AGENTS.md.

## Acceptance Criteria

- [ ] `neo4j config get output` returns `"default"` with an empty config
- [ ] `neo4j config set output json` writes `"output": "json"` at the root of config.json
- [ ] `neo4j config set output invalid` returns an error
- [ ] `neo4j config set unknown-key value` returns an error
- [ ] `neo4j config list` returns `{"output": "default"}` (or current value)
- [ ] `neo4j config aura set output json` returns "invalid config key specified: output"
- [ ] `neo4j config aura list` no longer shows `output`
- [ ] `neo4j config aura get auth-url` returns the configured auth-url (same behaviour as former `neo4j aura config get`)
- [ ] `neo4j aura instance list` respects `neo4j config set output json` without `--output` flag
- [ ] `neo4j config aura list` respects the global output setting for its own rendered output
- [ ] `neo4j aura config` no longer exists (unknown command error)
- [ ] `neo4j credential list --output json` overrides the config for that invocation
- [ ] The `aura.output` migration block in `NewConfig` is commented out with a clear note explaining it must not run in this experimental release
- [ ] `aura-cli config set output json` (standalone) writes `"output": "json"` at the JSON root
- [ ] `aura-cli config set auth-url <url>` (standalone) writes the value under `aura.auth-url`
- [ ] `aura-cli config get output` (standalone) returns the current global output value
- [ ] `aura-cli config list` (standalone) includes both global keys (`output`) and aura keys (`auth-url`, `base-url`, `default-tenant`)
- [ ] `neo4j config list --output table` renders a key/value table via `common/output.PrintBodyMap` with `ConfigData`
- [ ] `neo4j config get output --output json` renders `{"output": "<value>"}` via `common/output.PrintBodyMap` with `ConfigData`
- [ ] `neo4j config list` (default mode) renders as a table, consistent with all other commands
- [ ] `neo4j config aura list` and `neo4j config aura get` use `ConfigData` + `PrintBodyMap`
- [ ] `aura-cli config list` and `aura-cli config get` use `ConfigData` + `PrintBodyMap`
- [ ] `neo4j credential list --output table` renders a structured table with named columns (not a key/value table)
- [ ] `neo4j credential list --output json` renders JSON
- [ ] `neo4j credential list` (no flag) respects the configured output default
- [ ] `aura-cli credential list --output table` renders a structured table
- [ ] No config or credential package contains its own JSON/table rendering logic after this change
- [ ] New `neo4j config` tests use table-driven style and are split into `get_test.go`, `set_test.go`, `list_test.go`
- [ ] `common/output` package exists with `ResponseData` interface and rendering functions
- [ ] `aura/internal/output/output.go` is a thin adapter — only `PrintBody` logic remains there
- [ ] `api/response.go` types (`ListResponseData`, `SingleValueResponseData`) satisfy `common/output.ResponseData`
- [ ] All existing callers of `output.PrintBodyMap` continue to work without import changes
- [ ] AGENTS.md documents the per-command test file convention and the common/output package design
- [ ] `make test` passes
- [ ] No resource-group command (instance, tenant, etc.) contains any `--output` flag registration or `BindOutput` call
- [ ] `neo4j config list --output table` renders config keys as a table
- [ ] `neo4j config get output --output json` renders as JSON

## Out of Scope

- Per-product output overrides (aura-specific output config that takes precedence over global)
- Adding other top-level config keys (auth-url, base-url promotion)

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

# Global config smoke test
./bin/neo4j-cli config list
./bin/neo4j-cli config set output json
./bin/neo4j-cli config get output         # should return "json"

# Aura config (new path)
./bin/neo4j-cli config aura list          # should NOT show output
./bin/neo4j-cli config aura get auth-url  # should return configured value
./bin/neo4j-cli config aura set output json  # should error

# Old path should be gone
./bin/neo4j-cli aura config list          # should error: unknown command

# Standalone flat config
./bin/aura-cli config list                # should show both output and aura keys
./bin/aura-cli config set output table
./bin/aura-cli config set auth-url https://example.com
./bin/aura-cli config get output          # should return "table"

# Run all tests
make test
```
