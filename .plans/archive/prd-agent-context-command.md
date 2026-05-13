# PRD: `neo4j-cli agent-context` command (CLI-83)

## Overview

Audit finding B2 (blocker) in `agent-cli-auditor.md` §7.2: `neo4j-cli` ships a SKILL.md bundle (Layer 3) but no Layer 2 — a structured, machine-readable view of the CLI's shape. An agent that has never used the CLI must trial-and-error its way through commands, flag types, enum values, and exit codes; that burns tokens and produces brittle invocations.

Linear: https://linear.app/neo4j/issue/CLI-83/b2-no-agent-context-command (parent CLI-71)

Fix: add a top-level `neo4j-cli agent-context` command that emits a stable JSON envelope describing the full command tree (commands, flags, types, descriptions), the closed enum of exit codes, the closed enum of error codes, the list of supported output formats, and the canonical async-flag name. The command **reflects the live cobra tree at runtime** — there is no embedded JSON file, no `go generate` step, no parallel artifact to keep in sync. Drift-free by construction. The small surface that *can* drift (exit codes, error codes, async-flag name, schema version) is hand-coded in one source file and locked by tests.

Plan reference: `/Users/oskarhane/.claude/plans/time-to-tackle-https-linear-app-neo4j-is-frolicking-kitten.md` (working notes from the planning Q&A session that resolved the open design decisions).

## Goals

- `neo4j-cli agent-context` (with `--format json`) emits a single JSON envelope containing `schema_version`, `cli_version`, `binary`, `commands` (full recursive tree), `exit_codes`, `error_codes`, `output_formats`, and `async_flag`. The envelope shape matches the issue body and `agent-cli-auditor.md` §7.2.
- The `commands` tree is reflected from the live cobra tree at every invocation — adding a new subcommand, flag, or alias auto-surfaces with no regen step.
- The command honours the existing root `--format` flag: `table` on TTY (degraded flat command-list view), `json` on redirect, `toon` available. JSON is the canonical machine view.
- `output_formats` is sourced live from `clicfg.ValidFormatValues` so it cannot drift from the format-flag validator.
- `cli_version` is sourced from `app.Version` (ldflag-injected at release time; `"dev"` in local builds).
- A new section in `AGENTS.md` ("Agent Context Notes") tells contributors where the hand-coded constants live (`build.go`) and when to bump `schema_version`.
- The auditor source file (`agent-cli-auditor.md`) the user dropped at repo root for this session is `.gitignore`d, not committed.

## Non-Goals

- Build-time generation / embedding a static JSON artifact. The runtime reflection IS the auto-generation; an embedded file would add a regen pipeline without buying anything.
- Renaming `--await` to `--wait` (auditor's canonical). v1 emits `"async_flag": "--await"` (honest to the actual flag in this repo). Renaming is a separate audit item.
- Expanding `exit_codes` to the auditor's full §4.1 set (`2: usage_error`, `3: not_found`, `4: permission_denied`, …). Current `main.go` only emits `0` and `1`; v1 documents only what the binary actually returns. Wiring richer exit codes is a separate audit item.
- Mapping `clierr.NewUsageError` / `NewUpstreamError` / `NewFatalError` to distinct exit codes. v1 documents the three error-code names; exit-code mapping is a separate audit item.
- Per-command enum-value introspection (e.g. extracting `[default,json,table,toon]` from the `--format` flag automatically). v1 emits only the static `output_formats` envelope key. Per-flag enum extraction (cobra's `pflag.Value.Type()` returns `string` for custom enum values) is a follow-up.
- Adding `available_profiles`, `feedback_endpoint_configured`, `deliver_schemes` from auditor §7.2 — those features don't exist in this CLI today. Emitting them would be aspirational, not honest.
- Including hidden cobra commands or hidden flags in the JSON (consistent with the existing skill-bundle renderer at `common/skill/render/render.go:265`).
- MCP wrapper, schema-first codegen, or any other long-game architecture from auditor §16.

## Requirements

### Functional Requirements

- REQ-F-001: `bin/neo4j-cli agent-context --format json` writes a single JSON object to stdout containing exactly these top-level keys: `schema_version` (int), `cli_version` (string), `binary` (string, value `"neo4j-cli"`), `commands` (object), `exit_codes` (object, string-keyed string values), `error_codes` (object, string-keyed string values), `output_formats` (array of strings), `async_flag` (string).
- REQ-F-002: `schema_version` is the integer `1` for this initial release. Constant defined in `neo4j-cli/internal/subcommands/agentcontext/build.go` as `schemaVersion = 1`. Bumping is required on any breaking JSON-shape change (renaming a top-level key, changing a field's type, dropping a documented code).
- REQ-F-003: `cli_version` reflects `app.Version` at invocation time (ldflag-injected semver; `"dev"` in unflagged local builds). The package receives the version via `agentcontext.NewCmd(cfg, version)` rather than importing `app` directly (avoids an import cycle).
- REQ-F-004: `commands` is a recursive object keyed by each visible subcommand's first-`Use`-token (lowercased). Each value is a `Command` object with these fields: `use` (string), `short` (string), `long` (string), `example` (string), `aliases` ([]string), `hidden` (bool, always `false` in emitted output since hidden commands are skipped — kept for forward compatibility), `deprecated` (string, empty if not deprecated), `flags` ([]Flag, sorted by name), `subcommands` (recursive map).
- REQ-F-005: Each `Flag` object has fields: `name` (string, long-flag name without `--`), `shorthand` (string, single letter or empty), `type` (string, the value of `pflag.Flag.Value.Type()`), `default` (string, value of `pflag.Flag.DefValue`), `description` (string, value of `pflag.Flag.Usage`), `inherited` (bool, `true` if the flag was injected via `cmd.InheritedFlags()` from a parent rather than declared on the command itself).
- REQ-F-006: Flag iteration mirrors `common/skill/render/render.go:281-293` — `fs.VisitAll`, skip `Hidden`, then sort by `Name` ascending. Both `LocalFlags()` and `InheritedFlags()` are walked; inherited flags carry `"inherited": true` so an agent inspecting a single subcommand sees the full effective flag set without walking parents.
- REQ-F-007: Recursion descends into every command for which `cobra.Command.IsAvailableCommand()` returns true (cobra's own visibility predicate; excludes hidden commands and the auto-generated `help` command). Caps depth at 10 (defensive bound; no command tree in this repo exceeds 4).
- REQ-F-008: `exit_codes` is the literal object `{"0": "success", "1": "general error"}`. Constant defined in `build.go` as `exitCodes map[string]string`. Documents only what the binary actually emits today.
- REQ-F-009: `error_codes` is the literal object `{"usage_error": "<desc>", "upstream_error": "<desc>", "fatal_error": "<desc>"}`, mirroring the three constructors in `common/clierr/error.go:9-21`. Descriptions: `usage_error` = `"invalid flag, missing argument, or other input rejection"`, `upstream_error` = `"transient API failure; retry may succeed"`, `fatal_error` = `"unrecoverable internal failure"`.
- REQ-F-010: `output_formats` equals `clicfg.ValidFormatValues[:]` (the slice literal `["default", "json", "table", "toon"]` at `common/clicfg/clicfg.go:37`). Sourced live, not duplicated.
- REQ-F-011: `async_flag` is the literal string `"--await"` (the canonical async flag name in this repo; constant in `build.go`).
- REQ-F-012: The `commands` tree includes the `agent-context` command itself (self-documenting). The walker has no special-case to exclude its own entry.
- REQ-F-013: Format dispatch honours the existing root `--format` flag registered at `neo4j-cli/app/app.go:38` via `flags.RegisterOutputFlag(cmd, cfg)`. Behaviour:
  - `--format json` (and the default when stdout is not a TTY): `json.NewEncoder(out).Encode(ctx)` — single-line compact JSON.
  - `--format toon`: same envelope rendered via the project's existing toon path.
  - `--format table` (default on TTY): degraded human view — a flat table with columns `path | aliases | short`, one row per visible command in `cmd.CommandPath()` lexicographic order, plus a footer block printing `cli_version`, `schema_version`, and `async_flag`.
- REQ-F-014: The leaf command is registered under `neo4j-cli/internal/subcommands/agentcontext/` following the repo's one-file-per-leaf cobra layout (per AGENTS.md "Cobra Command Layout"). Files:
  - `agentcontext.go` — `func NewCmd(cfg *clicfg.Config, version string) *cobra.Command` with `Short`, `Long`, `Example` (flush-left per AGENTS.md "Cobra Help / Skill Bundle Rendering Notes"), and the `RunE` that dispatches on format.
  - `build.go` — pure functions (no cobra side effects), exporting `BuildContext(root *cobra.Command, cliVersion string) Context` and the `Context` / `Command` / `Flag` types with snake_case `json:` tags. Holds the closed-set constants (`schemaVersion`, `exitCodes`, `errorCodes`, `asyncFlag`).
  - `agentcontext_test.go` — end-to-end tests that invoke `app.NewCmd(cfg)` + `cmd.SetArgs([]string{"agent-context", "--format", "json"})`.
  - `build_test.go` — table-driven walker tests against a synthetic mini cobra tree (hermetic, no full app construction).
- REQ-F-015: Mount the command in `neo4j-cli/app/app.go` by adding one line: `cmd.AddCommand(agentcontext.NewCmd(cfg, Version))`. `Version` is the existing `var Version = "dev"` at `neo4j-cli/app/app.go:27`.
- REQ-F-016: `Long` description on the leaf command explains the v1 schema honestly (no aspirational fields) and links to the source-of-truth note in `AGENTS.md`. The `Example` field shows three realistic invocations per auditor §7.1: `neo4j-cli agent-context`, `neo4j-cli agent-context --format json | jq '.commands | keys'`, `neo4j-cli agent-context --format json | jq -e '.commands.aura.subcommands.instance.subcommands.list.flags'`.
- REQ-F-017: New section in `AGENTS.md` titled `## Agent Context Notes`, inserted after the existing "Cobra Help / Skill Bundle Rendering Notes" section. Content (verbatim):
  - "`neo4j-cli agent-context` emits the full CLI shape as JSON for AI-agent discovery (Layer 2 per `agent-cli-auditor.md` §7.2). Reflected from the live cobra tree at runtime — no static artifact to keep in sync."
  - "Adding a new command/flag automatically surfaces in the next `agent-context` invocation. No regen step, no `make generate-check` involvement for the JSON itself. (Skill-bundle `references/<cmd>.md` still needs `go generate` per the existing rules.)"
  - "Hand-coded constants live in `neo4j-cli/internal/subcommands/agentcontext/build.go`: `schemaVersion`, `exitCodes`, `errorCodes`, `asyncFlag`. Update these when adding a new error category, exit code, or async-flag convention."
  - "`output_formats` is sourced from `clicfg.ValidFormatValues`; do NOT duplicate the list in agent-context."
  - "Bump `schemaVersion` on breaking JSON-shape changes (rename a top-level key, change a field type, drop a documented code)."
  - "Tests in `agentcontext_test.go` lock the envelope shape, output-format parity, and tree coverage. Adding a new top-level command will trip the coverage test until the JSON includes it — the failure message tells you what's missing."
- REQ-F-018: Add `/agent-cli-auditor.md` to `/Users/oskarhane/Development/neo4j-cli/.gitignore` so the auditor skill file the user left in repo root for this session does not get committed.
- REQ-F-019: A `Minor`-kind changelog entry is added under `.changes/unreleased/` via `changie new --projects neo4j-cli --kind Minor --body "Add neo4j-cli agent-context command emitting the full CLI shape as JSON for AI-agent discovery (CLI-83)"` (or hand-authored YAML per AGENTS.md "Changie Notes").
- REQ-F-020: Skill bundle regenerated via `go generate ./neo4j-cli/internal/skill/...`. Expected outputs: a new `neo4j-cli/internal/skill/bundle/references/agent-context.md` file and an updated `neo4j-cli/internal/skill/bundle/SKILL.md` table-of-contents row. Both files are committed in the same PR.

### Non-Functional Requirements

- REQ-NF-001: No new external dependencies. Uses only `encoding/json`, `github.com/spf13/cobra`, `github.com/spf13/pflag` (already in go.mod), and the existing `common/clicfg` / project output helpers.
- REQ-NF-002: Cross-OS: tests run on linux/windows/macos. No OS-specific code paths. Deterministic JSON ordering via sorted flag slice (REQ-F-006) and Go's stable JSON encoding of `map[string]...` (sorts keys alphabetically).
- REQ-NF-003: Local gates green: `make test`, `make fmt-check`, `make lint`. CI gates green: `make generate-check` (which expects the regenerated `references/agent-context.md`), `make license-check` (Neo4j copyright header on every new `.go` file).
- REQ-NF-004: `TestGenerator_RoundTrip` (the existing bundle-drift gate in `make test`) passes after `go generate ./neo4j-cli/internal/skill/...`.
- REQ-NF-005: Performance: a single `bin/neo4j-cli agent-context --format json` invocation completes in <100ms on a developer laptop (cold start). No I/O beyond stdout/stderr. The walker is a pure tree traversal over an in-memory cobra tree.
- REQ-NF-006: Output size: emitted JSON is targeted at <50 KB compact (well under the auditor §8.3 default 25k-token cap). If a future grow-out exceeds the budget we'll revisit; for now, document the size in REQ-V-005 below.

### Verification Requirements

- REQ-V-001: `TestAgentContext_Envelope` (in `agentcontext_test.go`): invokes the cobra command via `app.NewCmd(cfg).SetArgs([]string{"agent-context", "--format", "json"})`, decodes stdout into a `Context` struct, asserts every top-level field has the right type and a non-zero value (`schema_version == 1`, `cli_version != ""`, `binary == "neo4j-cli"`, `len(commands) > 0`, `len(exit_codes) == 2`, `len(error_codes) == 3`, `output_formats` matches `clicfg.ValidFormatValues[:]`, `async_flag == "--await"`).
- REQ-V-002: `TestAgentContext_OutputFormatsParity` (in `agentcontext_test.go`): asserts the emitted `output_formats` array is byte-equal to `clicfg.ValidFormatValues[:]` — catches drift if someone edits one but not the other.
- REQ-V-003: `TestAgentContext_TreeCoverage` (in `agentcontext_test.go`): walks `app.NewCmd(cfg)` independently in the test, collects every visible command's full path (`cmd.CommandPath()`), then walks the emitted JSON's `commands` tree and collects the same set; asserts the two sets are equal. Failure message names the missing/extra paths.
- REQ-V-004: `TestAgentContext_FormatRoundTrip` (in `agentcontext_test.go`): table-driven over `{"json", "toon", "table"}`. For each format, invoke the command, assert stdout is non-empty and the command exits without error. For `json` and `toon`, additionally assert that re-parsing yields the same `Context` (toon path uses the existing project toon marshaller's symmetrical decode if available; if not, skip the decode round-trip but still assert the envelope keys are present in the text).
- REQ-V-005: `TestBuildContext_Walker` (in `build_test.go`): table-driven against a synthetic cobra tree built inline (`&cobra.Command{Use: "root"}` with a couple of nested subcommands and a mix of local + persistent flags). Asserts: flags sorted alphabetically; hidden flags skipped; hidden subcommands skipped; persistent flags from parent appear on child with `"inherited": true`; aliases captured; `Deprecated` field surfaces in JSON.
- REQ-V-006: `TestAgentContext_HelpExamples` (light): assert that the leaf command's `Example` field is non-empty and flush-left (no leading two-space indent) per the bundle-renderer convention in AGENTS.md.
- REQ-V-007: Manual smoke (acceptance criteria below): `bin/neo4j-cli agent-context --format json | jq -e '.commands.aura.subcommands.instance.subcommands.list.flags'` exits 0 and prints a non-empty flags array, demonstrating ≥3 levels of recursion.

## Technical Considerations

### Files touched

- **New**: `neo4j-cli/internal/subcommands/agentcontext/agentcontext.go` — parent file with `NewCmd(cfg *clicfg.Config, version string) *cobra.Command`. Body ≤80 lines per the layout rule. Wires `Short`, `Long`, `Example`, and `RunE`. The `RunE` reads `cfg.Aura.Output()` (or the canonical accessor for `--format`), builds the context via `BuildContext`, and dispatches to the right renderer.
- **New**: `neo4j-cli/internal/subcommands/agentcontext/build.go` — pure walker. Exports `Context`, `Command`, `Flag` types and `BuildContext(root *cobra.Command, cliVersion string) Context`. Holds package-private constants `schemaVersion`, `exitCodes`, `errorCodes`, `asyncFlag`.
- **New**: `neo4j-cli/internal/subcommands/agentcontext/agentcontext_test.go` — REQ-V-001 through REQ-V-004, REQ-V-006.
- **New**: `neo4j-cli/internal/subcommands/agentcontext/build_test.go` — REQ-V-005 (hermetic walker tests on a synthetic tree).
- **Edit**: `neo4j-cli/app/app.go` — add import `"github.com/neo4j/cli/neo4j-cli/internal/subcommands/agentcontext"` and one line `cmd.AddCommand(agentcontext.NewCmd(cfg, Version))` adjacent to the existing `cmd.AddCommand(update.NewCmd(...))` block at lines 62-66.
- **Edit**: `AGENTS.md` — new section "Agent Context Notes" per REQ-F-017. Insert after the existing "Cobra Help / Skill Bundle Rendering Notes" subsection.
- **Edit**: `.gitignore` — append `/agent-cli-auditor.md` per REQ-F-018.
- **New** (regenerated): `neo4j-cli/internal/skill/bundle/references/agent-context.md` — auto-produced by `go generate ./neo4j-cli/internal/skill/...`.
- **Edit** (regenerated): `neo4j-cli/internal/skill/bundle/SKILL.md` — table-of-contents row for `agent-context` appended by the bundle renderer.
- **New**: `.changes/unreleased/neo4j-cli-Minor-<ts>.yaml` per REQ-F-019.

### Walker design

The fresh walker in `build.go` is intentionally not factored out of `common/skill/render/render.go` (which couples flag iteration to markdown rendering). Mirror the iteration pattern but emit struct fields:

```go
func walkCommand(cmd *cobra.Command) Command {
    out := Command{
        Use:        firstToken(cmd.Use),
        Short:      cmd.Short,
        Long:       cmd.Long,
        Example:    cmd.Example,
        Aliases:    append([]string{}, cmd.Aliases...),
        Deprecated: cmd.Deprecated,
        Flags:      collectFlags(cmd),
        Subcommands: map[string]Command{},
    }
    for _, sub := range cmd.Commands() {
        if !sub.IsAvailableCommand() { continue }
        out.Subcommands[firstToken(sub.Use)] = walkCommand(sub)
    }
    return out
}

func collectFlags(cmd *cobra.Command) []Flag {
    seen := map[string]bool{}
    var rows []Flag
    cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
        if f.Hidden { return }
        rows = append(rows, mkFlag(f, false))
        seen[f.Name] = true
    })
    cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) {
        if f.Hidden || seen[f.Name] { return }
        rows = append(rows, mkFlag(f, true))
    })
    sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
    return rows
}
```

### Format dispatch

The `RunE` reads `cfg.Aura.Output()` (the existing format accessor wired via `flags.RegisterOutputFlag` at `app.go:38`) and switches:

```go
switch cfg.Aura.Output() {
case "json", "default" /* when piped */:
    return json.NewEncoder(cmd.OutOrStdout()).Encode(ctx)
case "toon":
    return renderToon(cmd.OutOrStdout(), ctx)   // existing toon path
case "table":
    return renderTable(cmd.OutOrStdout(), ctx)  // flat command-list table
}
```

`renderTable` is local to this package — it flattens the recursive `commands` tree into rows sorted by `cmd.CommandPath()`. The aura subsystem's existing table helpers (`output.PrintBodyMap` / `NewSingleValueResponseData`) assume a flat key-value map per AGENTS.md "Credentials Storage Notes" line 218; they don't fit our nested envelope, so we render directly.

`default` resolves to `json` when stdout is not a TTY (existing root `--format` machinery handles this).

### Schema versioning

`schema_version = 1` for v1. Bump rules (documented in AGENTS.md Agent Context Notes):

- **Bump** on: renaming a top-level envelope key; changing a field's Go type; removing a documented exit/error code; changing the recursion structure of `commands`.
- **No bump** on: adding a new top-level envelope key; adding fields to `Command` / `Flag`; adding entries to `exit_codes` / `error_codes` / `output_formats`; new commands or flags surfacing automatically via reflection.

### Why no embedded JSON

A build-time generated JSON (analogous to `internal/skill/bundle/SKILL.md`) was considered and rejected:

- Skill bundles need to be embedded because they ship as files to agent install locations outside the CLI; an agent reads them at skill install time.
- `agent-context` output is consumed at CLI invocation time by an agent that already has the CLI running. Runtime reflection produces the same bytes with zero embed pipeline.
- The bundle pipeline exists because rendering Markdown from a cobra tree is slow enough (~100ms) that pre-rendering it makes sense for read-many use cases. JSON encoding is sub-millisecond and runs once per invocation.
- Runtime reflection makes drift **structurally impossible** for the recursive `commands` tree — the largest surface — eliminating the need for a drift-detection test on that surface. The remaining hand-coded constants are tiny and locked by REQ-V-001.

### Source-of-truth gates

- `clicfg.ValidFormatValues` (`common/clicfg/clicfg.go:37`) is the single source for the format enum. Both the format-flag validator and `agent-context` read from it. REQ-V-002 locks the parity.
- `clierr` constructors (`common/clierr/error.go:9-21`) are the source for error categories. `errorCodes` in `build.go` is a hand-curated map that mirrors them; no programmatic linkage today (the constructors don't expose code names). Future work: extend `clierr` to carry a code constant, and have `build.go` reflect over a registered set.
- `app.Version` (`neo4j-cli/app/app.go:27`) is the source for `cli_version`. `NewCmd` receives it as a parameter rather than importing `app` directly — keeps `agentcontext` independently buildable and avoids an import cycle.

### Bundle regeneration impact

- Per AGENTS.md "Makefile Notes": adding any new command to the neo4j-cli command tree requires `go generate ./neo4j-cli/internal/skill/...` to refresh `bundle/references/`, otherwise `TestGenerator_RoundTrip` fails.
- The renderer (`common/skill/render/render.go`) walks `cmd.Commands()` and produces one reference file per top-level subcommand. The new `agent-context` command will produce `references/agent-context.md` automatically — no code changes to the renderer.

### Out-of-tree auditor file

The user dropped `/Users/oskarhane/Development/neo4j-cli/agent-cli-auditor.md` at repo root as Layer 2 context for this session. Explicit instruction: do not commit it. The `.gitignore` entry per REQ-F-018 prevents accidental staging.

## Acceptance Criteria

- [ ] `bin/neo4j-cli agent-context --help` shows a `Long` description, three flush-left `Examples:` lines, and lists no surprising flags beyond `--format` / `--rw` (inherited from root).
- [ ] `bin/neo4j-cli agent-context --format json` exits 0 and emits a single compact JSON line containing all eight top-level envelope keys (`schema_version`, `cli_version`, `binary`, `commands`, `exit_codes`, `error_codes`, `output_formats`, `async_flag`).
- [ ] `bin/neo4j-cli agent-context --format json | jq '.schema_version'` returns `1`.
- [ ] `bin/neo4j-cli agent-context --format json | jq '.output_formats'` returns `["default", "json", "table", "toon"]` byte-equal to `clicfg.ValidFormatValues`.
- [ ] `bin/neo4j-cli agent-context --format json | jq '.async_flag'` returns `"--await"`.
- [ ] `bin/neo4j-cli agent-context --format json | jq -e '.commands."agent-context"'` exits 0 (self-documenting).
- [ ] `bin/neo4j-cli agent-context --format json | jq -e '.commands.aura.subcommands.instance.subcommands.list.flags'` exits 0 and returns a non-empty array (≥3 levels of recursion verified).
- [ ] `bin/neo4j-cli agent-context --format toon` exits 0 and produces non-empty toon output containing the same envelope keys.
- [ ] `bin/neo4j-cli agent-context` from a TTY produces a flat table with rows for every visible command path.
- [ ] `bin/neo4j-cli agent-context` (no args, piped to a file) defaults to JSON via the existing root `--format` TTY-detection.
- [ ] `TestAgentContext_Envelope`, `TestAgentContext_OutputFormatsParity`, `TestAgentContext_TreeCoverage`, `TestAgentContext_FormatRoundTrip`, `TestBuildContext_Walker`, `TestAgentContext_HelpExamples` all exist and pass.
- [ ] `TestGenerator_RoundTrip` passes after `go generate ./neo4j-cli/internal/skill/...` adds `references/agent-context.md`.
- [ ] `make test`, `make fmt-check`, `make lint`, `make generate-check`, `make license-check` all green.
- [ ] `AGENTS.md` contains the new `## Agent Context Notes` section per REQ-F-017.
- [ ] `.gitignore` contains `/agent-cli-auditor.md`.
- [ ] A `Minor` changelog entry exists under `.changes/unreleased/`.
- [ ] `git status` shows `agent-cli-auditor.md` as untracked-and-ignored (no longer in the `??` output).

## Out of Scope

- Build-time JSON generation / embedded artifact (alternative rejected; see Technical Considerations).
- Renaming `--await` → `--wait`.
- Expanding exit codes beyond `0` / `1`.
- Mapping `clierr` error categories to distinct exit codes.
- Per-flag enum-value introspection (e.g. cobra `pflag.Value` custom-type metadata).
- Aspirational §7.2 fields: `available_profiles`, `feedback_endpoint_configured`, `deliver_schemes`.
- A `feedback` subcommand (auditor §11.2).
- Tee-on-failure infrastructure (auditor §5.1).
- Schema-first codegen migration (auditor §16).
- MCP wrapper of the CLI.
- Including hidden cobra commands / hidden flags.
- Markdown / human-readable `--help` rewrite beyond the new leaf command's own help text.

## Open Questions

- Branch name: `oskar/cli-83-agent-context-command` (default — matches Linear's `gitBranchName` `cli-83-b2-no-agent-context-command` shortened, plus the user's `oskar/` prefix convention).
