# PRD: Unify skill/query output into shared output infrastructure

## Overview

This branch refactored output configuration so that `cfg.Global.Output()` / `cfg.Global.BindOutput()` are the canonical output accessors, and `RegisterOutputFlag` at each binary root registers and binds a single persistent `--output` flag. However, the two new command groups added on this branch — `common/skill` and `neo4j-cli/query` — were not integrated into that uniform path. They each define their own local `--output` persistent flag (shadowing the root's), read output via `cfg.Aura.Output()` instead of `cfg.Global.Output()`, and the `query` package's TTY auto-detection logic remains local rather than shared.

This PRD covers completing the integration: moving TTY auto-detection into shared infrastructure so it applies to all commands across both binaries, then wiring `skill` and `query` into that shared path.

## Goals

- All commands across both binaries resolve their effective output mode (`json` or `table`) through a single shared code path.
- TTY auto-detection (piped/redirected stdout → `json`, interactive TTY → `table` when `--output` is unset) applies uniformly to every command, including `aura instance list`, `skill list`, and `query`.
- `skill` and `query` no longer shadow the root's persistent `--output` flag with their own duplicate definitions.
- `skill` leaf commands read via `cfg.Global.Output()` instead of `cfg.Aura.Output()`.
- `skill` and `query` commands use `common/output.PrintBodyMap` for rendering instead of ad-hoc JSON/table logic.
- Test fixtures use the current config JSON format (`{"output":"..."}` at the root) rather than the old `{"aura":{"output":"..."}}` format.

## Non-Goals

- Changing `SkillsScope` or `QueryScope` to produce different logic in `NewConfig` — they remain semantic labels only.
- Adding config `get`/`set` commands for `skill` or `query` — those packages don't expose user-facing config commands.
- Changing the `--output` flag's allowed values or help text.

## Requirements

### Functional Requirements

- REQ-F-001: Add a shared `ResolveOutput(cmd *cobra.Command, cfg *clicfg.Config) string` function in `common/output`. When the effective output value is `"default"` or `""`, it checks whether `cmd.OutOrStdout()` is a TTY: returns `"table"` if yes, `"json"` if no. Any explicitly set value (`"json"`, `"table"`) passes through unchanged.
- REQ-F-002: Expose a package-level test seam `var StdoutIsTerminal = func(w io.Writer) bool { ... }` in `common/output` so tests in dependent packages can control TTY detection without hitting the real terminal.
- REQ-F-003: Update `common/output.PrintBodyMap` to route the `"default"` case through `ResolveOutput` (currently `"default"` always produces table output regardless of whether stdout is a TTY).
- REQ-F-004: Remove the `--output` persistent flag definition from `common/skill/skill.go`. The root command's `RegisterOutputFlag` already registers a persistent `--output` that cascades to all subcommands including `skill`.
- REQ-F-005: Introduce `ResponseData` result types for each `skill` leaf command (`installResults`, `listResults`, `checkResults`) implementing both `AsArray() []map[string]any` (for table rows) and `MarshalJSON()` (for JSON array output). Wire `common/skill/install.go`, `list.go`, and `check.go` to call `common/output.PrintBodyMap(cmd, cfg, data, fields)` instead of ad-hoc JSON/table logic and `cfg.Aura.Output()`.
- REQ-F-006: Remove the `--output` persistent flag definition from `neo4j-cli/query/query.go`. Same reasoning as REQ-F-004.
- REQ-F-007: Introduce a `queryResult` type in `neo4j-cli/query/output.go` implementing `ResponseData`. `AsArray()` returns rows as `[]map[string]any` keyed by column name (used by `PrintBodyMap` for table rendering). `MarshalJSON()` returns the existing custom envelope `{columns, rows, truncated, arrays_truncated}` (used by `PrintBodyMap` for JSON rendering). Replace `renderRows` and `printJSONRows`/`printTableRows` with a single `common/output.PrintBodyMap` call.
- REQ-F-008: Update `neo4j-cli/query/output.go` to replace the local `resolveOutput()` function and local `stdoutIsTerminal` seam with a call to `common/output.ResolveOutput()` (now called indirectly via `PrintBodyMap`).
- REQ-F-009: Remove the now-unused local `stdoutIsTerminal` seam from `neo4j-cli/query/output.go` and its seed in `neo4j-cli/query/testseam_test.go`.

### Non-Functional Requirements

- REQ-NF-001: All existing tests pass after the changes — no previously-passing test may be left red.
- REQ-NF-002: `make fmt-check` and `make test` both pass as final gates.
- REQ-NF-003: The `common/output.StdoutIsTerminal` seam must be restored between tests via `t.Cleanup` to avoid cross-test pollution.

## Technical Considerations

### Using `PrintBodyMap` with custom JSON shapes

`PrintBodyMap` calls `json.MarshalIndent(values, ...)` on the `ResponseData` value. If the underlying type implements `MarshalJSON()`, that method is used — so a type can satisfy `ResponseData` while still emitting a non-array JSON shape. This is already the established pattern in the codebase (`PrintableConfigData.MarshalJSON()` in `clicfg.go` emits a flat map rather than an array).

For `skill`: the result types are slices of structs, so `MarshalJSON()` can just delegate to the default slice marshaling (an array of objects) — this matches the current `printJSON` output exactly.

For `query`: `queryResult.MarshalJSON()` emits `{columns, rows, truncated, arrays_truncated}`, preserving the existing consumer-facing JSON schema. `AsArray()` returns `[]map[string]any` rows keyed by column name so `PrintBodyMap`'s `printTable` path works. The `fields []string` passed to `PrintBodyMap` is the dynamic column list from the query result.

### Flag shadowing bug (the core problem)

Cobra's `mergePersistentFlags` builds a merged FlagSet for each command by walking the parent chain. When a child command defines a persistent flag with the same name as a parent's, the child's definition wins (the parent's is silently skipped as a duplicate). This means `skill`'s and `query`'s local `--output` persistent flag currently shadow the root's `--output` even when the user specifies `--output` at the root level — the value the user passed is never seen by `cfg.Global`. Removing the local definitions fixes the shadowing.

### Placing `ResolveOutput` in `common/output`

`query/output.go` already has a correct implementation of TTY-aware output resolution. It should be lifted to `common/output` as the canonical version. The test seam (`StdoutIsTerminal`) must be package-level in `common/output` so that both query and skill tests can override it.

### Impact on existing aura command tests

Tests that configure `output = "default"` (or `""`) and assert **table** output will fail after REQ-F-003, because with a non-TTY test buffer, `ResolveOutput` will return `"json"`. Identify these tests and update them to either:
- Set `output = "table"` explicitly in the config fixture, or
- Override `common/output.StdoutIsTerminal` to return `true` for the test.

From AGENTS.md: "Test helpers default to `"output": "json"` at the JSON root" — the majority of aura tests use explicit json and are unaffected. The fix is narrow.

### Test fixture config format

Test fixtures in `common/skill/cmd_helpers_test.go`, `neo4j-cli/query/output_test.go`, `neo4j-cli/query/run_test.go`, and `neo4j-cli/query/connect_test.go` use `{"aura":{"output":"..."}}` (the pre-refactor key). After REQ-F-005 and REQ-F-007 migrate callers to `cfg.Global.Output()`, viper reads from the top-level `"output"` key. Update these fixtures to `{"output":"..."}`.

### `stdoutIsTerminal` in `query/testseam_test.go`

`testseam_test.go` currently seeds `stdoutIsTerminal` (package-level in `query`) in `TestMain`. After REQ-F-008 removes that seam, the seed in `TestMain` should be removed. Tests that need to control TTY detection should override `common/output.StdoutIsTerminal` with a `withStdoutTTY(t, val)` helper using `t.Cleanup` for restoration.

## Acceptance Criteria

- [ ] `common/output.ResolveOutput(cmd, cfg)` exists and auto-detects TTY when output is `""` / `"default"`.
- [ ] `common/output.StdoutIsTerminal` is a package-level var seam, production-initialized to the `term.IsTerminal` check.
- [ ] `common/output.PrintBodyMap` uses `ResolveOutput` for its default-mode branch.
- [ ] `common/skill/skill.go` has no `--output` flag definition.
- [ ] `common/skill/install.go`, `list.go`, `check.go` use `common/output.PrintBodyMap` with `ResponseData` result types; no ad-hoc JSON/table rendering remains.
- [ ] `neo4j-cli/query/query.go` has no `--output` flag definition.
- [ ] `neo4j-cli/query/output.go` has a `queryResult` type implementing `ResponseData` with `AsArray()` (rows as column-keyed maps) and `MarshalJSON()` (custom envelope). `renderRows` delegates to `common/output.PrintBodyMap`.
- [ ] `neo4j-cli/query/output.go` has no local `resolveOutput` or `stdoutIsTerminal`.
- [ ] `neo4j-cli/query/testseam_test.go` no longer seeds `stdoutIsTerminal`.
- [ ] All test fixtures use `{"output":"..."}` at the JSON root, not `{"aura":{"output":"..."}}`.
- [ ] `make test` passes with no failing tests.
- [ ] `make fmt-check` reports no unformatted files.

## Out of Scope

- Adding `neo4j-cli query config get/set` or `neo4j-cli skill config get/set` commands.
- Changing `SkillsScope` / `QueryScope` to produce distinct `NewConfig` behaviour.
- Implementing the commented-out `aura.output` → `output` migration in `clicfg.go`.

## Open Questions

- None — all scope and design questions resolved in the PRD session.
