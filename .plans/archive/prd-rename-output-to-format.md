# PRD: Rename --output flag to --format with -f shorthand

## Overview

Rename the CLI's `--output` flag to `--format` (with `-f` shorthand) across both binaries (`neo4j-cli` and `aura-cli`). The internal config key stored on disk also renames from `"output"` to `"format"`. The commented-out migration block in `clicfg.go` is updated to reflect the new key names (`output` → `format`, `aura.output` → `format`) but remains commented out — it is preserved as reference for a future stable-release upgrade path, following the same convention as the existing comment.

## Goals

- Users run `neo4j-cli instance list --format json` (or `-f json`) instead of `--output json`.
- `config get format` / `config set format json` replace `config get output` / `config set output json`.
- The migration block in `clicfg.go` is kept commented out but updated to reference the correct new key names, preserving it as a ready reference for a future stable-release upgrade path.
- All internal Go identifiers (`ValidOutputValues`, `Output()`, `BindOutput()`) are renamed to match.

## Non-Goals

- Keeping `--output` as a deprecated alias — it is removed entirely.
- Changing the allowed values (`default`, `json`, `table`) or their behavior.
- Renaming the `--output` concept in any way other than the flag name and config key.

## Requirements

### Functional Requirements

- REQ-F-001: Rename the flag from `--output` to `--format` with shorthand `-f` in `common/flags/flags.go` (`RegisterOutputFlag`). Update the flag lookup from `"output"` to `"format"` in the `PersistentPreRunE` validation and `BindOutput` call.
- REQ-F-002: Rename the viper key from `"output"` to `"format"` everywhere in `common/clicfg/clicfg.go`: `SetDefault`, `GetString`, `BindPFlag`, `ValidConfigKeys`, the `"output"` special-case guards in `AuraConfig.Get()` and `GlobalConfig.Set()`, and the `ResolveConfigKey` docstring.
- REQ-F-003: Rename `GlobalConfig.Output()` → `GlobalConfig.Format()` and `GlobalConfig.BindOutput()` → `GlobalConfig.BindFormat()`. Update all callers (`common/output/output.go::ResolveOutput`, `common/flags/flags.go`, `neo4j-cli/aura/internal/subcommands/import/job/get.go`, `neo4j-cli/aura/internal/subcommands/tenant/get.go`).
- REQ-F-004: Rename `ValidOutputValues` → `ValidFormatValues` in `clicfg.go`. Update all references: `common/flags/flags.go` (flag description, validation loop, error message), `common/clicfg/clicfg.go` (Set validation, error message), and any other callers.
- REQ-F-005: Update the commented-out migration block in `NewConfig` (`clicfg.go`) to reference the correct new keys — changing the destination from `"output"` to `"format"` and adding a second clause for `"output"` → `"format"` alongside the existing `"aura.output"` clause. The block must remain commented out at the end of this work. The surrounding NOTE comment should be updated to reflect the new key names. Do not restore the `gjson` import (keep it absent, as the block stays commented out).
- REQ-F-006: Update all test files that pass `"--output"` as a cobra argument to pass `"--format"` (or `"-f"`). Update all test config fixtures that set the `"output"` viper key to use `"format"` (e.g., `{"output":"json"}` → `{"format":"json"}`).
- REQ-F-007: Update all Go source comments and string literals that refer to the flag by name (`--output`, `"output"` as a config key description) in non-generated files. Key files: `AGENTS.md`, `CONTRIBUTING.md`, `README.md`, `docs/usageGuide/`, `.agents/architecture.md`, `.agents/repo-layout.md`.
- REQ-F-008: Regenerate skill bundles with `make generate` after all code changes are complete, so `SKILL.md` and `references/*.md` in both `neo4j-cli/internal/skill/bundle/` and `neo4j-cli/aura/internal/skill/bundle/` reflect `--format` / `-f`.
- REQ-F-009: Update `neo4j-cli/internal/skill/additions.md` and `neo4j-cli/aura/internal/skill/additions.md` if they mention `--output` directly (these feed the generated bundle).

### Non-Functional Requirements

- REQ-NF-001: `make test` passes with no failures after all changes.
- REQ-NF-002: `make fmt-check` passes (no gofmt drift).
- REQ-NF-003: `make generate-check` passes — committed bundle files must match what `go generate` produces from the updated cobra tree.
- REQ-NF-004: The migration block in `clicfg.go` must remain commented out — it must not be executed at runtime.

## Technical Considerations

### Flag name vs viper key coupling

`flags.RegisterOutputFlag` registers the pflag as `"format"` and then calls `cfg.Global.BindFormat(cmd.Flags().Lookup("format"))`, which calls `viper.BindPFlag("format", flag)`. Both the cobra flag name and the viper key rename together — they must stay in sync. The existing `BindPFlag` mechanism handles the `--format` value being surfaced as `cfg.Global.Format()`.

### Migration block — stays commented out

The migration block in `clicfg.go` is not executed at runtime — it is preserved as commented-out reference code for a future stable-release upgrade path, following the same convention as the current comment. The update to this block is editorial only: change the destination key from `"output"` to `"format"` and add a second clause for `"output"` → `"format"`. Do not restore the `gjson` import since the block is not compiled. The surrounding NOTE comment should be updated to describe the new key names.

### AuraConfig.Get() special-case

`AuraConfig.Get()` has a special-case at line 264-265 that routes the `"output"` key to the global viper instance instead of the aura-prefixed viper. This guard must change from `key == "output"` to `key == "format"` in step with the viper key rename.

### `ResolveConfigKey` rejection of `"aura.format"`

`ResolveConfigKey` currently rejects `"aura.output"` (a global-only key addressed with the aura prefix). The rejection comment and guard must update to reference `"aura.format"`.

### Skill bundle regeneration

Skill bundles are generated by `go generate ./...` from the live cobra tree. After the flag rename, `make generate` must be run to update the committed `.md` files. `make generate-check` in CI will catch any drift. The `additions.md` files are hand-authored — check them for `--output` mentions and update manually before running `make generate`.

### Test blast radius

62 `--output` occurrences across 26 test files, plus config fixtures using `{"output":"..."}` at the JSON root (introduced by the unify-skill-query-output feature). All must change to `--format` / `{"format":"..."}`.

## Acceptance Criteria

- [ ] `--format` (and `-f`) are accepted by all commands in both binaries; `--output` is rejected as unknown flag.
- [ ] `config get format` returns the stored value; `config set format json` persists it.
- [ ] The migration block in `clicfg.go` is commented out but updated: destination key is `"format"`, both `"aura.output"` and `"output"` clauses are present, and the NOTE comment references the new key names.
- [ ] `ValidFormatValues` is defined in `clicfg.go`; `ValidOutputValues` does not exist.
- [ ] `GlobalConfig.Format()` and `GlobalConfig.BindFormat()` exist; `Output()` and `BindOutput()` do not.
- [ ] `make test` exits 0.
- [ ] `make fmt-check` exits 0.
- [ ] `make generate-check` exits 0 (bundles match generated output).
- [ ] README, CONTRIBUTING.md, AGENTS.md, and usage guides reference `--format` / `-f`, not `--output`.

## Out of Scope

- Keeping `--output` as a deprecated alias.
- Changing `ValidFormatValues` entries (`default`, `json`, `table`).
- Renaming anything in the HTTP API or JSON response bodies (only the CLI flag and config key change).

## Open Questions

- None — scope and migration approach confirmed.
