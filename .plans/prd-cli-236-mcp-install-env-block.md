# PRD: Align `mcp install` config-mode with the `.mcpb` bundle (CLI-236)

## Overview

`neo4j-cli mcp install` and `neo4j-cli mcp bundle` both claim to install the same MCP
connector into Claude Desktop, but they produce materially different results. The
difference is not cosmetic: **in config mode every MCP capability gate is permanently
dead, with no way for the user to turn it on.**

Verified on the developer machine (both artifacts present on disk):

**Bundle path** (`mcp bundle`, or `mcp install --bundle`) — Desktop registers a real
extension:

- `~/Library/Application Support/Claude/Claude Extensions/local.mcpb.neo4j.neo4j-cli/`
- `~/…/Claude Extensions Settings/local.mcpb.neo4j.neo4j-cli.json` holding `userConfig`
  with all 7 fields (`allow_writes`, `allow_aura`, `allow_credential_write`,
  `neo4j_cli_path`, `neo4j_credential`, `allowed_databases`, `result_row_limit`)
- Shown in Settings → Extensions as **"Neo4j CLI"** with a full config screen

**Config path** (`mcp install`, the default) — a bare entry in
`claude_desktop_config.json` (`common/skill/mcpconfig.go:71-76`):

```json
"neo4j-cli": { "command": "/Users/…/neo4j-cli", "args": ["mcp", "serve"] }
```

No `env` block at all. Shown under Settings → Developer as the raw key, no screen.

### Why the missing `env` block is a functional break

1. **All three capability gates are permanently dead.** `resolveGates`
   (`mcp/serve.go:117-135`) consults `NEO4J_CLI_MCP_ALLOW_WRITES` / `_ALLOW_AURA` /
   `_ALLOW_CREDENTIAL_WRITE` **only** when `NEO4J_CLI_MCP_MANIFEST=1` is present
   (`serve.go:161`). Desktop spawns `mcp serve` with no flags, and config mode sets no
   env — so `manifestMarker` is false, every gate resolves false, and
   `neo4j_cli_run_write` refuses **every** write forever while Aura and credential
   commands stay unreachable. A config-mode user has no lever at all.
2. **`NEO4J_CLI_FLAG_MCP_SERVER=1` is not set**, so the spawned server depends on
   `flag.mcp-server` happening to be persisted in the user's config file
   (`common/clicfg/flags.go:45`). The manifest sets it explicitly
   (`mcp/manifest.go:159`); config mode should too.
3. **`install.go`'s `Long` already documents a fallback that does not exist**: it
   promises install "falls back to config write on other platforms when the open command
   is unavailable" (`install.go:49-51`), but `runInstallBundle` returns a fatal error on
   `openFile` failure (`install.go:162-164`).

### Decision (confirmed with user)

Keep the direct config write as the **default** for `mcp install`; fix its env block so
the capability surface matches the bundle.

**Honest scope limit — accepted, not a gap to close later.** The display name
("Neo4j CLI") and the settings screen are `.mcpb` manifest features. A `mcpServers` entry
in `claude_desktop_config.json` cannot have either — Desktop renders the raw key on a
different screen entirely. The *identifier* is already identical (`neo4j-cli`) in both
paths (`mcpconfig.go:76`, `manifest.go:141`). This work aligns everything that **can** be
aligned — capabilities, env, feature flag, truthful reporting — and states the remaining
irreducible difference in help text instead of silently misleading.

### Correction to the source plan (verified, supersedes it)

The plan specified keying the `--rw`-implies-all-gates rule off `cmd.Flags().Changed("rw")`,
on the premise that `EnforceWriteGate` auto-*sets* `--rw` in an interactive terminal.
**It does not.** `RequireWriteAccess` (`common/flags/flags.go:118-143`) only *waives* the
requirement by returning `nil`; nothing mutates the flag (a repo-wide grep for `Set("rw"`
outside tests returns nothing). The flag's usage string "Auto-applied in interactive
terminals" describes permission, not mutation.

`Changed` and value therefore agree everywhere **except** an explicit `--rw=false`, where
`Changed`=true but value=false — and `Changed` would enable all three gates, the exact
opposite of what was typed. **This PRD specifies the flag value.**

## Goals

- A user who installs via `mcp install` can actually use write / Aura / credential
  capabilities in Claude Desktop, at a granularity they choose.
- The env var names have exactly one definition in the repo; the config-mode and
  manifest forms cannot drift apart.
- `mcp install`'s documented open-failure fallback becomes real, and the reported
  install method reflects what actually happened.
- Users with an existing gate-dead entry get a signal (`mcp check` → `drift`) and a
  remediation that already exists (`mcp install` refresh).
- `mcp install --help` tells the truth about what each method yields, including the
  difference config mode cannot close.

## Non-Goals

- Making `mcp install` default to `--bundle`. Rejected: the direct config write stays the
  default.
- Giving the config-mode entry a display name or settings screen. Impossible for a
  `mcpServers` entry — see the scope limit above.
- Changing the `.mcpb` manifest's contents. The refactor to shared constants must produce
  **byte-identical** manifest output.
- Changing `mcp serve`'s gate semantics, the `NEO4J_CLI_MCP_MANIFEST` marker design, or
  the security reasoning documented at `serve.go:142-159`.
- Adding MCP support to any additional agent. `claude-desktop` remains the only catalog
  entry with `MCPConfig` set (`common/skill/agents.go:63-69`).
- Wiring `neo4j_credential`, `allowed_databases` or `result_row_limit` into the runtime.
  They are declared in the manifest's `user_config` but have no env counterpart today;
  that asymmetry is pre-existing and out of scope.

## Requirements

### Functional Requirements

**Shared env definition**

- REQ-F-001: A new file `common/skill/mcpenv.go` defines the five MCP env var names as
  exported constants: `EnvMCPFeatureFlag` (`NEO4J_CLI_FLAG_MCP_SERVER`),
  `EnvMCPManifest` (`NEO4J_CLI_MCP_MANIFEST`), `EnvMCPAllowWrites`
  (`NEO4J_CLI_MCP_ALLOW_WRITES`), `EnvMCPAllowAura` (`NEO4J_CLI_MCP_ALLOW_AURA`),
  `EnvMCPAllowCredentialWrite` (`NEO4J_CLI_MCP_ALLOW_CREDENTIAL_WRITE`).
- REQ-F-002: The file exports `type MCPGates struct{ AllowWrites, AllowAura,
  AllowCredentialWrite bool }` and `func MCPServerEnv(g MCPGates) map[string]string`.
- REQ-F-003: `MCPServerEnv` always emits **all five** keys. `EnvMCPFeatureFlag` and
  `EnvMCPManifest` are always `"1"`; the three gates are the literal strings `"true"` or
  `"false"` per the corresponding `MCPGates` field.
- REQ-F-004: `mcp/serve.go` drops its five local consts (`serve.go:59-64`) and references
  the `common/skill` constants. `envBool` / `resolveGates` behaviour is unchanged.
- REQ-F-005: `mcp/manifest.go:158-164` builds its `Env` map keys from the same constants,
  retaining `${user_config.*}` template values. Only the values differ between forms.

**Config-mode env block**

- REQ-F-006: `MCPConfigEntry` (`mcpconfig.go:15`, currently declared but unused) gains
  `Env map[string]string` with json tag `env,omitempty`.
- REQ-F-007: `InstallMCPConfig` takes an `MCPGates` parameter and writes
  `"env": MCPServerEnv(gates)` into the server entry.
- REQ-F-008: The surgical merge, atomic temp-file+rename, file-mode preservation and the
  `"neo4j-cli"` server key are unchanged. Unrelated top-level keys and sibling
  `mcpServers` entries continue to survive.
- REQ-F-009: `RemoveMCPConfig` is unchanged — it deletes the whole entry, env included.

**Install gate flags**

- REQ-F-010: `mcp install` gains three boolean flags, all defaulting false:
  `--allow-writes`, `--allow-aura`, `--allow-credential-write`. The latter two reuse the
  existing constants `server.AllowAuraFlag` / `server.AllowCredentialWriteFlag`
  (`server/allow.go:35-36`) rather than new literals.
- REQ-F-011: Gate resolution — if **any** of the three `--allow-*` flags is explicitly
  passed (`cmd.Flags().Changed`), the three literal flag values are used as-is.
  Otherwise all three take the **value** of `--rw` (`ParseBool` of
  `cmd.Flag("rw").Value.String()`), not its `Changed` state. Resulting matrix:

  | invocation | writes | aura | cred |
  |---|---|---|---|
  | `--rw` | ✓ | ✓ | ✓ |
  | `--rw=false` | ✗ | ✗ | ✗ |
  | interactive, no `--rw` in argv | ✗ | ✗ | ✗ |
  | `--rw --allow-writes` | ✓ | ✗ | ✗ |
  | `--rw --allow-aura --allow-writes` | ✓ | ✓ | ✗ |
  | `--allow-aura` (no `--rw`, interactive) | ✗ | ✓ | ✗ |

- REQ-F-012: The resolved gates thread through `runInstallCmd` → `runInstallOne` →
  `runInstallConfig` → `InstallMCPConfig`.
- REQ-F-013: In `--bundle` mode the three flags have no effect (the manifest wires the
  Desktop toggles instead). Each flag's usage string says so.

**Truthful fallback and reporting**

- REQ-F-014: `runInstallOne` returns the method actually used alongside its error, so
  `method` is observed rather than derived from `useBundle`. The
  `method := "config"; if useBundle { method = "mcpb" }` derivation is removed from both
  `runInstallCmd` (`install.go:108-111`) and `runInstallAndRender` (`install.go:128-131`).
- REQ-F-015: When `openFile` fails in `runInstallBundle`, install falls back to the config
  write with the same gates, reports `method` as `config`, and surfaces the generated
  `.mcpb` path so the user can open it manually. If the fallback also fails, the original
  fatal error is returned.

**Drift detection**

- REQ-F-016: `mcp check` reports `drift` when an installed entry's env block lacks
  `NEO4J_CLI_MCP_MANIFEST`, in addition to the existing command-path comparison. The
  existing remediation message ("run `neo4j-cli mcp install` to refresh") and the non-zero
  exit on drift are unchanged.
- REQ-F-017: A read helper (e.g. `readMCPConfigEnv`) is added alongside
  `readMCPCommandPath` (`mcpconfig.go:52`), reusing `readMCPConfigServerEntry`. An absent,
  unparseable or inaccessible file continues to read as "not installed", not an error.

**Documentation**

- REQ-F-018: `install.go`'s `Long` documents: the `--rw` implication rule and per-flag
  override; each of the three flags and the capability it unlocks; that `--rw` carries two
  meanings here (permission to modify your config file, and by implication the server's
  capability set); and what each method yields — config mode writes
  `mcpServers."neo4j-cli"` with a hand-editable env block, while `--bundle` registers a
  Desktop extension named "Neo4j CLI" whose settings screen exposes the same gates as
  toggles.
- REQ-F-019: The `Example` block stays flush-left with ≥2 invocations, a `# comment` per
  invocation, the `neo4j-cli` prefix, and `--rw` on writes — including one example of the
  subset form. Gate: `TestAllLeafCommands_HaveExamples`.
- REQ-F-020: One `.changes/unreleased/` entry with `kind: Patch`, describing only the
  observable effect. Verify the claim against the asserted env map before writing it.

### Non-Functional Requirements

- REQ-NF-001: **Manifest output is byte-identical** after the constants refactor. The
  existing assertions in `manifest_test.go` (notably `name` = `"neo4j-cli"`,
  `display_name` = `"Neo4j CLI"`, and the env assertions at lines 91/97) pass **unchanged**
  and serve as the regression guard.
- REQ-NF-002: Writing gates explicitly as `"false"` rather than omitting them is
  load-bearing, not stylistic. The config `env` block overrides the ambient environment
  Desktop inherited, so an explicit `"false"` neutralises a stale
  `NEO4J_CLI_MCP_ALLOW_WRITES=true` in a login shell rc — exposure that setting
  `NEO4J_CLI_MCP_MANIFEST=1` would otherwise newly create. It also makes the toggles
  hand-editable and discoverable, which is config mode's substitute for the settings
  screen. Do not "tidy" this into `omitempty` behaviour.
- REQ-NF-003: `common/skill/mcpenv.go` must live under `common/` — the internal-package
  rule forbids `common/*` importing `neo4j-cli/internal/*`, and both `serve.go` and
  `manifest.go` import `common/skill` already.
- REQ-NF-004: Output field names stay snake_case; input identifiers stay kebab-case
  (CLI-127). Gates: `agentcontext/casing_input_gate_test.go`,
  `common/output/casing_gate_test.go`.
- REQ-NF-005: No new command is added or renamed, so
  `mcp/server/testdata/policy.golden` and the committed skill bundle must show **no**
  diff. Confirm, do not assume.
- REQ-NF-006: Tests must never write to the real filesystem or read real credentials —
  use `testfs.GetDefaultTestFs()` / MemMapFs. Config paths are derived via
  `skill.FindAgent("claude-desktop").MCPConfigPath()`, never hardcoded darwin paths
  (`$APP_SUPPORT` differs per GOOS).

## Technical Considerations

**Where the gates are actually consumed.** The chain is: `install` writes env →
Desktop spawns `neo4j-cli mcp serve` with that env → `serveRun` reads
`envBool(envManifestMarker)` (`serve.go:161`) → `resolveGates` consults the three gate
vars only if the marker is set → `server.Gates` reaches `allow.go`, which is what
`neo4j_cli_run_write` and the `gated:*` policies check. Breaking any link restores the
current dead-gate behaviour, so the end-to-end Desktop check in Acceptance Criteria is
the only proof that matters.

**Two independent meanings of `--rw`.** On `install`, `--rw` satisfies
`flags.EnforceWriteGate` for the write-annotated `install` command itself
(`install.go:63`). Overloading it to also imply the server's capability set is a
deliberate ergonomic choice (user-confirmed), and exactly why REQ-F-011 keys off the
**value**: an explicit `--rw=false` must not open gates. `RequireWriteAccess` never
mutates the flag, so in the interactive-waiver case the value is still `false` and gates
stay off — the safe direction.

**Existing test call sites.** Six functions in `common/skill/mcpconfig_test.go` call
`InstallMCPConfig` (lines 50, 101, 121, 215, 274, 407) and all need the new signature.
This is the main mechanical cost of REQ-F-007; consider a test helper for the default
all-off gates.

**Test package placement.** The `mcp` tests live in the **external** `mcp_test` package
so they can build the live tree via `app.NewCmd` — the only place the `flag.mcp-server`
gate is applied (`mcp_helpers_test.go:1-9`). `newMCPInstallFixture` (line 106) already
seeds `HOME` and the derived detect dir over a MemMapFs, so a read-back assertion on the
written config needs no new fixture machinery.

**Generation gate.** `gen/main.go` builds the tree over an empty MemMapFs with flags off,
so the flag-gated `mcp` subtree stays out of the committed bundle. Any
`NEO4J_CLI_FLAG_*` left set in the shell would drift the bundle — unset before running
`make generate-check`.

## Acceptance Criteria

- [ ] `common/skill/mcpenv.go` exists with the five constants, `MCPGates`, and
      `MCPServerEnv`; `mcpenv_test.go` covers every gate combination and asserts all five
      keys are always present.
- [ ] `serve.go` and `manifest.go` reference the shared constants; neither declares an
      MCP env var name literal of its own.
- [ ] `manifest_test.go` passes **unchanged** (byte-identical manifest).
- [ ] `mcp install --agent claude-desktop --rw` writes an entry whose `env` has all five
      keys, with the three gates `"true"`.
- [ ] `mcp install --agent claude-desktop --rw --allow-writes` writes writes=`"true"`,
      aura=`"false"`, cred=`"false"`.
- [ ] A table-driven test covers every row of the REQ-F-011 matrix, **including**
      `--rw=false` → all gates `"false"` and the interactive no-`--rw` row → all gates
      `"false"`. These two rows are what catch an implementation keying off `Changed`.
- [ ] Golden merge and cross-platform tests in `mcpconfig_test.go` assert the full env
      map and that the pre-existing `neo4j-data-modeling` sibling entry survives.
- [ ] `mcp check` reports `drift` for an installed entry with no `env` block, and `ok`
      for a freshly written one; exits non-zero on drift.
- [ ] On `openFile` failure, install falls back to the config write, reports
      `method: config`, and prints the `.mcpb` path.
- [ ] `mcp install --help` documents the implication rule, all three flags, and the
      config-vs-bundle difference including the no-settings-screen limitation.
- [ ] One new file under `.changes/unreleased/` with `kind: Patch`.
- [ ] `git diff` shows **no** change to `mcp/server/testdata/policy.golden`.
- [ ] `make generate-check` clean on a committed tree with `NEO4J_CLI_FLAG_*` unset.
- [ ] `make test`, `make fmt-check`, `make lint` all pass.
- [ ] **End-to-end against real Claude Desktop**: back up `claude_desktop_config.json`,
      run `mcp install --agent claude-desktop --rw`, confirm the env block is present and
      the real `neo4j-data-modeling` entry is intact, restart Desktop, and confirm a write
      tool call now succeeds — it cannot today. This is the only check that proves the
      whole chain.

## Out of Scope

- `mcp bundle`'s manifest contents, `user_config` fields, README or icon.
- The `neo4j_credential` / `allowed_databases` / `result_row_limit` user_config fields
  having no env counterpart — pre-existing asymmetry.
- `mcp serve`'s gate security model and the known-limitation comment at
  `serve.go:142-159`.
- Adding `MCPConfig` to any other agent in the catalog.
- `mcp list` output columns (`installed_command` already surfaces the command path).
- The `.mcpb` cache location (`$UserCacheDir/neo4j-cli-mcp/neo4j-cli.mcpb`).

## Open Questions

- **Should `mcp check` distinguish "path drift" from "env drift" in its `status`
  column?** This PRD folds both into `drift`, since the remediation is identical
  (`mcp install` refresh). A separate status value or a reason column would be more
  informative but adds an output field and a casing-gate surface. Defaulting to the simple
  form; revisit if the message proves confusing in practice.
- **Should `mcp install` warn when it detects an ambient `NEO4J_CLI_MCP_ALLOW_*` in the
  installing shell?** REQ-NF-002 neutralises it in the written config, so there is no
  security hole — but a user who set that var deliberately may be surprised the installed
  server ignores it. Cheap to add later; not required here.
