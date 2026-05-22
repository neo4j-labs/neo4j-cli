# PRD: Skill install support for Antigravity CLI (CLI-172)

## Overview

Add Google Antigravity to the `neo4j-cli` agent catalog so `neo4j-cli skill install [antigravity]` (and `list` / `check` / `remove`) treat it as a first-class supported agent. Antigravity is Google's successor to Gemini CLI (I/O 2026); per Google's skills tutorial, global custom skills live at `~/.gemini/antigravity/skills/`. Linear: CLI-172. Slack ask from michaelhunger.

The install pipeline is agent-agnostic — the agent catalog at `common/skill/agents.go` is the single source of truth, so adding one entry + updating its order-locking test is the entire change. No skill-bundle regeneration is required (the bundle is binary-scoped, not agent-scoped).

## Goals

- Detect Antigravity installations on a user's machine.
- Install / list / check / remove the `neo4j-cli` skill bundle into Antigravity's global skills directory.
- Surface Antigravity in user-facing supported-agents documentation.
- Ship the change via the standard `changie` patch flow.

## Non-Goals

- Workspace-scope Antigravity skills (`<workspace>/.agent/skills/`). Out of scope; catalog only supports global per-user paths today.
- Antigravity-specific skill metadata, layout, or templates. Bundle is identical across all agents.
- Removing or renaming the existing `gemini-cli` catalog entry. They coexist (different DetectDir).
- Regenerating the skill bundle (`go generate`). The bundle is binary-scoped — no command-tree change here.
- Auto-migrating any existing skill content from `~/.gemini/skills/` into `~/.gemini/antigravity/skills/`.

## Requirements

### Functional Requirements

- REQ-F-001: A new entry exists in `common/skill/agents.go` `AGENTS` slice with:
  - `Name: "antigravity"`
  - `DisplayName: "Antigravity"`
  - `DetectDir: "~/.gemini/antigravity"`
  - `SkillsDir: "~/.gemini/antigravity/skills"`
- REQ-F-002: The Antigravity entry is positioned **immediately before** the `gemini-cli` entry to group agents sharing the `~/.gemini` root.
- REQ-F-003: `TestAGENTSCatalog` in `common/skill/agents_test.go` is updated so the `expected` slice contains `"antigravity"` immediately before `"gemini-cli"`, matching catalog order. No length-constant bump needed (`require.Len(t, AGENTS, len(expected))`).
- REQ-F-004: `neo4j-cli skill install antigravity` lands the bundle (`SKILL.md` + per-subcommand references) at `~/.gemini/antigravity/skills/neo4j-cli/`.
- REQ-F-005: `neo4j-cli skill list` shows Antigravity as detected only when `~/.gemini/antigravity` exists — a bare `~/.gemini` left by Gemini CLI MUST NOT produce a false-positive detection.
- REQ-F-006: `neo4j-cli skill remove antigravity` is idempotent (same semantics as other agents).
- REQ-F-007: README `## Agent skills` "Supported agents:" sentence is updated to include `Antigravity` immediately before `Gemini CLI`.
- REQ-F-008: A changelog entry is added via `changie new --projects neo4j-cli --kind Patch --body "Add skill install support for Antigravity CLI."` (or an equivalent hand-authored YAML under `.changes/unreleased/`).

### Non-Functional Requirements

- REQ-NF-001: Cross-platform — the catalog entry uses `~` tokens, expanded at runtime by the existing `expandPath` helper. Must pass on linux/macOS/windows CI.
- REQ-NF-002: No skill bundle regeneration required; `make generate-check` must remain clean.
- REQ-NF-003: All three local gates pass on a clean tree: `make test`, `make fmt-check`, `make lint`.
- REQ-NF-004: Branch name: `oskar/cli-172-add-skill-install-for-antigravity-cli` (Linear branch name with user-required `oskar/` prefix).

## Technical Considerations

- **Catalog is the only seam.** Install/list/check/remove logic in `common/skill/install.go`, `installer.go`, etc. walk `AGENTS` generically. No new per-agent code paths are required.
- **DetectDir vs SkillsDir parent shared with `gemini-cli`.** Choosing `~/.gemini/antigravity` (not `~/.gemini`) for DetectDir avoids a false-positive when only Gemini CLI is installed. This mirrors the precedent set by other catalog entries that use a sub-path marker (e.g. `pi` uses `~/.pi/agent`).
- **Order matters.** `TestAGENTSCatalog` enforces catalog order to keep `skill list` output stable across releases. The Antigravity entry must be inserted at the same index in both `agents.go` and the test's `expected` slice.
- **Path expansion is already covered.** The existing `TestExpandPath*` cases and the `~/...` codepath in `expandPath` handle the new entry without test additions. `TestDetectAgents` walks the catalog and will exercise Antigravity automatically.
- **No skill bundle regeneration.** The bundle lives under `neo4j-cli/internal/skill/bundle/` and depends only on the cobra command tree, not on the agent catalog. `make generate-check` will stay green.
- **README touch is documentation-only.** README line 41 lists agents in catalog order today; inserting `Antigravity, ` before `Gemini CLI` keeps doc order in sync with the catalog.
- **Changelog kind = Patch.** User-facing surface change but no semver-significant API change.

## Acceptance Criteria

- [ ] `common/skill/agents.go` has the new entry immediately before `gemini-cli` with the four field values from REQ-F-001.
- [ ] `common/skill/agents_test.go` `TestAGENTSCatalog` `expected` slice contains `"antigravity"` immediately before `"gemini-cli"`.
- [ ] `README.md` "Supported agents:" sentence includes `Antigravity` immediately before `Gemini CLI`.
- [ ] A `changie` entry exists under `.changes/unreleased/` for `neo4j-cli` kind=`Patch` with the required body.
- [ ] `make test` passes (including `TestAGENTSCatalog`, `TestFindAgent`, `TestDetectAgents`, `TestAgentDetectAndSkillsPath`).
- [ ] `make fmt-check` passes (no gofmt drift).
- [ ] `make lint` passes (golangci-lint clean).
- [ ] `make generate-check` passes (bundle untouched).
- [ ] Manual smoke (local, optional but recommended):
  - `mkdir -p ~/.gemini/antigravity && go run ./neo4j-cli skill list` shows antigravity as detected.
  - `go run ./neo4j-cli skill install antigravity` writes `~/.gemini/antigravity/skills/neo4j-cli/SKILL.md`.
  - `go run ./neo4j-cli skill remove antigravity` cleans the install directory.
  - With `~/.gemini` present but no `~/.gemini/antigravity`, `skill list` shows antigravity NOT detected.

## Out of Scope

- Workspace-scope skills at `<workspace>/.agent/skills/`.
- Antigravity-specific skill content / additions / templates.
- Auto-migration of skill content from `~/.gemini/skills/` to `~/.gemini/antigravity/skills/`.
- Removing or restructuring the existing `gemini-cli` entry.
- Skill-bundle regeneration via `go generate`.
- Updates to website (`gh-pages` branch) — not part of this change set.

## Open Questions

None — all design questions resolved in the plan:
- DetectDir uses the `antigravity` subdir (avoids false-positive overlap with `gemini-cli`).
- Catalog position: immediately before `gemini-cli`.
- Changelog kind: `Patch`.
