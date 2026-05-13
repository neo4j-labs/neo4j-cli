# PRD: Improved `skill --help` (CLI-74)

## Overview

`neo4j-cli skill install [agent]` and `neo4j-cli skill remove [agent]` accept a positional agent name (e.g. `claude-code`) but `--help` for these commands never tells the user which names are valid. The set of supported agents is only surfaced on failure — typing `claude` returns `unknown agent ... valid agents: claude-code, cursor, ...`. Steve hit this when wiring up Claude Code: first attempt `claude`, error, retry with `claude-code`.

Linear: https://linear.app/neo4j/issue/CLI-74/improved-skill-help

Fix: append a `Supported agents:` line to the `Long` field of both leaves (and add a small `Example` block), sourced from the existing `AGENTS` slice so the help text and the error text can't drift. No behaviour change; help text gets richer.

## Goals

- `neo4j-cli aura skill install --help` lists every supported agent name in its `Long` block, identical to the list shown in the `unknown agent` error.
- `neo4j-cli aura skill remove --help` does the same.
- Both leaves print a concrete `Example` block (no-arg form + single-agent form) so users see usable invocations alongside the synopsis.
- The help-text agent list comes from `AGENTS` (single source of truth) — adding a new agent to the catalog automatically extends help output, and an existing test forces both lists to stay in sync.

## Non-Goals

- Changing the wording of `formatAgentErr` output — the runtime `unknown agent ... valid agents: ...` string is kept verbatim.
- Adding shell tab-completion (`ValidArgs` / `ValidArgsFunction`) for the agent positional. (Explicitly declined to minimise diff.)
- Touching the `skill` parent's `Short`/`Long`, or the `list` / `check` / `print` leaves.
- Modifying `AGENTS`, `findAgent`, `Install`, `Remove`, or any installer logic.
- Reworking the skill bundle renderer or `references/` layout.

## Requirements

### Functional Requirements

- REQ-F-001: `neo4j-cli aura skill install --help` output contains the substring `Supported agents:` followed by a comma-separated list of every entry in `common/skill.AGENTS` (in catalog order — `claude-code, cursor, windsurf, copilot, gemini-cli, cline, codex, pi, opencode, junie` as of this PRD).
- REQ-F-002: `neo4j-cli aura skill remove --help` output satisfies REQ-F-001's contract.
- REQ-F-003: Both leaves render an `Examples:` block (cobra auto-prefixes `Example` content) containing two lines:
  - `neo4j-cli aura skill install` (no-arg) — install variant
  - `neo4j-cli aura skill install claude-code` (single-agent) — install variant
  - mirror pair for remove.
  Per the repo's render note (CLAUDE.md "Cobra Help / Skill Bundle Rendering Notes"), `Example` is written flush-left (no leading 2-space indent) so the regenerated bundle stays consistent.
- REQ-F-004: Running an unknown agent (e.g. `claude`) still exits non-zero with the existing `unknown agent\nvalid agents: <list>` message — no regression. `TestInstallCmd_UnknownAgent` (`install_test.go:59`) continues to pass unchanged.
- REQ-F-005: Help-text agent list and error-text agent list share a single source: `agentNames()` at `common/skill/helpers.go:29` (both render via `strings.Join(agentNames(), ", ")`).
- REQ-F-006: New tests pin the help-text contract:
  - `TestInstallCmd_HelpListsAgents` in `common/skill/install_test.go` — runs `--help` via the existing `newFixture` harness; asserts output contains `Supported agents:` AND every name from the `skill.AGENTS` slice (iterated, not literal).
  - `TestRemoveCmd_HelpListsAgents` in `common/skill/remove_test.go` — mirror.
- REQ-F-007: Skill bundles for both binaries are regenerated. `neo4j-cli/internal/skill/bundle/references/skill.md` and `neo4j-cli/aura/internal/skill/bundle/references/skill.md` reflect the new `Long` + `Example` content for the `install` and `remove` subsections.
- REQ-F-008: A `Minor`-kind changelog entry is added via `changie new --projects neo4j-cli --kind Minor --body "skill install/remove --help now lists every supported agent (CLI-74)"` (or an equivalent hand-authored YAML under `.changes/unreleased/`).

### Non-Functional Requirements

- REQ-NF-001: No public-API change. `agentNames()` stays package-private; `AGENTS` is already exported and is the catalog gate (`TestAGENTSCatalog` at `agents_test.go:16`).
- REQ-NF-002: No new dependencies.
- REQ-NF-003: Local gates green: `make test`, `make fmt-check`, `make lint`. CI gates green: `make generate-check`, license-check.
- REQ-NF-004: `TestGenerator_RoundTrip` (the bundle drift gate) passes after `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`.
- REQ-NF-005: Cross-OS: no new OS-specific code; existing `newFixture` harness already runs on linux/windows/macos in CI.
- REQ-NF-006: Changes are confined to `common/skill/install.go`, `common/skill/remove.go`, the two `_test.go` files for the new help-output tests, the two regenerated `bundle/references/skill.md` files, and one `.changes/unreleased/*.yaml` entry.

## Technical Considerations

### Files touched

- `common/skill/install.go` (currently `install.go:17-41`)
  - Extend `Long` from
    ```go
    Long: "Without an argument, installs into every detected agent. " +
        "With an [agent] argument (case-insensitive), installs into that " +
        "single agent. Unknown agent names exit non-zero with the list " +
        "of valid names.",
    ```
    to append `"\n\nSupported agents: " + strings.Join(agentNames(), ", ")`.
  - Add `Example` field flush-left:
    ```go
    Example: "neo4j-cli aura skill install              # install into every detected agent\n" +
        "neo4j-cli aura skill install claude-code  # install only into Claude Code",
    ```
  - Keep `Args: cobra.MaximumNArgs(1)` — no `ValidArgs`.

- `common/skill/remove.go` (currently `remove.go:12-36`)
  - Same `Long` extension (append `Supported agents: ...`).
  - `Example` mirror:
    ```go
    Example: "neo4j-cli aura skill remove              # remove from every detected agent\n" +
        "neo4j-cli aura skill remove claude-code  # remove only from Claude Code",
    ```

- `common/skill/install_test.go`
  - New `TestInstallCmd_HelpListsAgents`:
    ```go
    func TestInstallCmd_HelpListsAgents(t *testing.T) {
        f := newFixture(t, "/home/alice", "default")
        require.NoError(t, f.exec(t, "install", "--help"))
        out := f.stdout.String()
        assert.Contains(t, out, "Supported agents:")
        for _, a := range skill.AGENTS {
            assert.Contains(t, out, a.Name, "help must list agent %q", a.Name)
        }
    }
    ```
  - (Adapt `newFixture` arg list to match the existing signature in this file.)

- `common/skill/remove_test.go`
  - Mirror `TestRemoveCmd_HelpListsAgents` (the file already exists per `ls common/skill/`).

- `neo4j-cli/internal/skill/bundle/references/skill.md`
- `neo4j-cli/aura/internal/skill/bundle/references/skill.md`
  - Regenerated via `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`. Commit both.

- `.changes/unreleased/neo4j-cli-Minor-<ts>.yaml`
  - From `changie new --projects neo4j-cli --kind Minor --body "..."`, or hand-authored per CLAUDE.md "Changie Notes".

### Cobra help rendering contract

- Cobra prints `Long` (if set) instead of `Short` in the synopsis section of `--help`.
- Appending `\n\nSupported agents: <a, b, c, ...>` keeps the existing prose intact and adds the list as a clearly-separated trailing paragraph. Cobra wraps `Long` to terminal width, so a long single-line list is fine in production. Tests assert on raw substring content, not wrapped layout — unaffected by terminal width.
- `Example` is auto-rendered by cobra under an `Examples:` heading. Per the render note in CLAUDE.md ("Cobra Help / Skill Bundle Rendering Notes"), the skill-bundle renderer strips the FIRST line's leading indent via `strings.TrimSpace`, so multi-line `Example` strings MUST be written flush-left (no leading 2 spaces) to render consistently in the bundle.

### Source of truth & drift protection

- `common/skill/agents.go:31` defines `AGENTS` (10 entries, locked order).
- `common/skill/helpers.go:29` defines `agentNames()` which builds `[]string` from `AGENTS`.
- `common/skill/helpers.go:16` (`formatAgentErr`) already uses `strings.Join(agentNames(), ", ")` to render the `valid agents:` error suffix — the new help-text uses the identical call, guaranteeing the two lists match byte-for-byte.
- `TestAGENTSCatalog` at `agents_test.go:16` is the existing gate on catalog content/order. Adding an agent requires updating that test; the new help-output tests iterate `skill.AGENTS` directly so they need no additional update.

### Bundle regeneration impact

- The skill subtree is bundled per-binary under `bundle/references/skill.md` (single file containing the whole `skill` subtree, NOT separate `install.md`/`remove.md`). Both binaries (`neo4j-cli/internal/skill/` and `neo4j-cli/aura/internal/skill/`) carry this file.
- Per CLAUDE.md "Makefile Notes", `TestGenerator_RoundTrip` catches stale bundles in `make test`; `make generate-check` is the CI-equivalent gate.
- The renderer reads cobra `Long` and `Example` from the live command tree, so the regenerated `skill.md` will pick up both changes automatically.

## Acceptance Criteria

- [ ] `bin/neo4j-cli aura skill install --help` output contains `Supported agents: claude-code, cursor, windsurf, copilot, gemini-cli, cline, codex, pi, opencode, junie` (substring match, exact catalog order).
- [ ] `bin/neo4j-cli aura skill install --help` output contains an `Examples:` block with both the no-arg and `claude-code` lines.
- [ ] `bin/neo4j-cli aura skill remove --help` output contains the same `Supported agents:` line and an `Examples:` block (remove wording).
- [ ] `bin/neo4j-cli aura skill install claude` (wrong name) still exits non-zero with `unknown agent\nvalid agents: ...`.
- [ ] `bin/neo4j-cli aura skill install claude-code` and the no-arg form continue to behave identically to today.
- [ ] `TestInstallCmd_HelpListsAgents` and `TestRemoveCmd_HelpListsAgents` exist and pass.
- [ ] `TestInstallCmd_UnknownAgent` and `TestAGENTSCatalog` still pass unchanged.
- [ ] `TestGenerator_RoundTrip` passes after `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`.
- [ ] `make test`, `make fmt-check`, `make lint`, `make generate-check` all green.
- [ ] A `Minor` changelog entry exists under `.changes/unreleased/`.

## Out of Scope

- Shell tab-completion (`ValidArgs` / completion functions) — declined.
- Multi-line "tabular" agent listing with `DisplayName` column — single-line comma-join chosen for parity with the error message.
- Surfacing the list in the `skill` parent's `Long` — the leaves are where the positional is consumed; keeping it local avoids duplication.
- Any change to `install` / `remove` flow (idempotency, detection, write-gate semantics).
- Bundle renderer (`common/skill/render/`) changes.

## Open Questions

- Branch name: `oskar/cli-74-improved-skill-help` (default — matches Linear's `gitBranchName` plus the user's `oskar/` prefix convention).
