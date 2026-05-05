# PRD: Skill install command (CLI-14)

## Overview

Add a top-level `skill` subcommand (`install` / `remove` / `list` / `check`) to every standalone CLI binary in this repo (`neo4j-cli`, `aura-cli`, future `cypher-cli`, …). Each binary embeds a generated `SKILL.md` bundle that documents its own cobra tree; `skill install` drops that bundle into supported AI-agent skill dirs (`~/.claude/skills/<bin>/`, `~/.cursor/skills/<bin>/`, etc.) so agents driving the CLI get accurate, per-build instructions.

Reference impls: Rust `oskarhane/homebrew-neo4j-query/src/skill.rs`; Go port `LackOfMorals/go-cli-tool#9` (merged — mirror its architecture). Linear: CLI-14.

## Goals

-   Ship a one-command install path so users can hand a CLI's documentation to any supported AI agent.
-   Keep each binary's skill content scoped to that binary's actual cobra tree (subset binaries advertise only their own subset).
-   Auto-generate the body from the cobra tree; preserve a per-binary hand-written gotchas section.
-   Detect skill drift between installed `SKILL.md` and the binary that wrote it.
-   Make adding a new standalone CLI a copy-the-template operation, no shared-code changes.

## Non-Goals

-   Auto-update of installed skills on version mismatch (v1 reports drift only).
-   Discovery of skills not authored in this repo.
-   Custom install destinations beyond the supported agent catalog.
-   Multi-level reference nesting (`references/<dir>/<file>.md`) — Anthropic best-practices say one level only.
-   Configuration UI for picking which subcommands a generated skill includes (always full tree of the binary).

## Requirements

### Functional Requirements

-   **REQ-F-001** — Each binary exposes `<bin> skill install [agent]`. Without arg: install to every detected agent (DetectDir exists). With arg: install to that single agent (case-insensitive lookup; unknown agent → non-zero exit + listed valid names).
-   **REQ-F-002** — `<bin> skill remove [agent]` removes the installed bundle. Idempotent — second run on an already-clean target succeeds with a no-op message.
-   **REQ-F-003** — `<bin> skill list` prints a table with columns `name / display / detected / installed / installed-version`. Supports `--output json` per existing CLI conventions.
-   **REQ-F-004** — `<bin> skill check` parses each installed `SKILL.md`'s frontmatter `version:` and compares to the running binary's version. Exits non-zero on any drift; prints a table of `agent / installed-version / current-version / status`.
-   **REQ-F-005** — Install writes the full bundle: `SKILL.md` plus `references/*.md`. Substitutes `{{VERSION}}` in `SKILL.md` with the runtime binary version (`cfg.Version` for aura-cli, exported `Version` for neo4j-cli) before writing — installed file always matches the binary that wrote it.
-   **REQ-F-006** — Generated `SKILL.md` body stays ≤500 lines. Always splits per top-level subcommand into `references/<sub>.md`. SKILL.md acts as an index (overview, global flags, subcommand list with reference links, gotchas).
-   **REQ-F-007** — Frontmatter contains `name` (= binary name, lowercase letters/digits/hyphens, ≤64 chars), `description` (third-person what + when, ≤1024 chars, sourced from per-binary `description.txt`), `version`.
-   **REQ-F-008** — Reference files >100 lines include a TOC at the top (per Anthropic best-practices).
-   **REQ-F-009** — Agent catalog matches the Rust reference (10 agents: claude-code, cursor, windsurf, copilot, gemini-cli, cline, codex, pi, opencode, junie). Path expansion supports `~`, bare `~`, and `$XDG_CONFIG_HOME` (fallback `$HOME/.config`).
-   **REQ-F-010** — `go generate ./...` invokes each per-binary generator (`<bin>/internal/skill/gen/main.go`) and rewrites that binary's `bundle/`. CI runs `make generate-check` and fails on diff.
-   **REQ-F-011** — `skill` is mounted at the top level of `neo4j-cli` and inside `aura.NewStandaloneCmd`. The `aura` subtree nested under `neo4j-cli` must NOT carry a duplicate `skill` cmd.
-   **REQ-F-012** — Adding a new standalone CLI requires only: (a) `<newcli>/app/app.go` exposing `NewCmd(cfg)`, (b) copy of `<bin>/internal/skill/` template with `description.txt` + `additions.md` + import update in `gen/main.go`, (c) `cmd.AddCommand(skill.NewCmd(cfg, binskill.Bundle, "<newcli>"))` in main.go. No edits to `common/skill/`.

### Non-Functional Requirements

-   **REQ-NF-001** — All new `.go` files carry the Neo4j copyright header (enforced by `make license-check`).
-   **REQ-NF-002** — Cross-platform paths: forward slashes inside generated markdown; `filepath.Join` for filesystem ops. CI runs on ubuntu, windows, macos — all must pass.
-   **REQ-NF-003** — Changelog: dual-project entry via `changie new --projects aura-cli --projects neo4j-cli --kind Minor`. Future binaries add their own project.
-   **REQ-NF-004** — Lint clean under `golangci-lint v2` config in `.golangci.yml`.

### Testing Requirements

-   **REQ-T-001** — All tests hermetic: `afero.NewMemMapFs` only. `Agent.DetectPath` / `DetectAgents` accept `afero.Fs` so tests don't touch real disk. Use `t.Setenv("HOME", ...)` for path expansion.
-   **REQ-T-002** — `common/skill/agents_test.go` (table-driven): `expandPath` for `~`, `~/foo`, bare `~`, `$XDG_CONFIG_HOME` set, `$XDG_CONFIG_HOME` unset (fallback `$HOME/.config`), missing HOME; `FindAgent` for exact, mixed-case, unknown; `DetectAgents` against an in-memory FS that fakes selected agent dirs.
-   **REQ-T-003** — `common/skill/installer_test.go` (table-driven): Install with no agents detected (error path), Install with one agent, Install with `--agent` selecting an unknown agent (error), Install overwriting an existing skill, Install creates references/ subdir with all files; Remove on uninstalled (idempotent), Remove cleans empty parent dir; List reflects detected/installed/version state; Check on no-drift, Check on stale-version (non-zero exit), Check on missing skill. Verifies `{{VERSION}}` substitution writes runtime version.
-   **REQ-T-004** — `common/skill/command_test.go`: exec each cobra subcommand with captured stdout/stderr; assert table layout columns and `--output json` envelope.
-   **REQ-T-005** — `common/skill/render/render_test.go` (golden-file): render a fixture cobra tree (root + 2 nested subcommands + flags + examples), assert byte-equal SKILL.md and references; cover the >100-line TOC trigger; cover sub-with-no-flags; cover hidden subcommand exclusion.
-   **REQ-T-006** — Per-binary generator: each `<bin>/internal/skill/gen/main.go` has a `main_test.go` that runs the generator into a `t.TempDir()` and asserts the bundle is byte-equal to the committed `bundle/` (fast local guard, complementary to CI's `make generate-check`).
-   **REQ-T-007** — Frontmatter validation test: `name` ≤64 chars, allowed charset; `description` non-empty, ≤1024 chars; `version` line present after substitution. Run against the actual committed bundles for every binary.
-   **REQ-T-008** — Line-count test: every committed `<bin>/internal/skill/bundle/SKILL.md` ≤500 lines (test fails if breached).
-   **REQ-T-009** — Coverage gate: shared `common/skill/` package ≥80% line coverage. Wire into `make test` only as informational, not blocking, to avoid CI noise on minor refactors.
-   **REQ-T-010** — CI runs the full skill suite on ubuntu, windows, macos. Path-related tests must explicitly cover Windows separator handling.

### Documentation Requirements

-   **REQ-D-001** — `README.md` gets a new "Agent skills" section (after "Usage") explaining what `skill install` does, the supported agent list, and the four subcommands with one example each.
-   **REQ-D-002** — `docs/usageGuide/Neo4j CLI.md` and `docs/usageGuide/A Guide To The New Aura CLI.md` each get a "Skill" section documenting the four subcommands. Match the doc's existing style (short prose + code blocks).
-   **REQ-D-003** — `CONTRIBUTING.md` gains a "Generated content" subsection under Development explaining: `go generate ./...` regenerates skill bundles; `make generate-check` is the CI gate; bundles are committed; how to add gotchas via `additions.md` per binary.
-   **REQ-D-004** — `CLAUDE.md` (project AGENTS.md) gets a short note in the Architecture section pointing at `common/skill/` and the per-binary `internal/skill/` template, plus the rule that adding a new standalone CLI follows the documented copy-template procedure.
-   **REQ-D-005** — Each generator (`<bin>/internal/skill/gen/main.go`) carries a top-of-file doc comment describing what it generates, where it writes, and how it's invoked (mirrors LackOfMorals#9 style).
-   **REQ-D-006** — Each `additions.md` opens with a one-line comment explaining its purpose so contributors editing it know it lands inline in the generated SKILL.md "Gotchas" section.

## Technical Considerations

**Layout** — split into shared logic and per-binary content:

```
common/skill/                              # shared, binary-agnostic
  agents.go        agents_test.go
  filesystem.go    installer.go            installer_test.go
  command.go       command_test.go         # NewCmd(cfg, bundle, skillName)
  render/render.go render/render_test.go   # cobra tree → SKILL.md + references/

<bin>/internal/skill/                      # per binary (neo4j-cli, aura, cypher-cli, …)
  embed.go                                 # //go:generate go run ./gen ; //go:embed bundle ; var Bundle embed.FS
  description.txt                          # frontmatter `description`
  additions.md                             # gotchas, inlined into SKILL.md
  bundle/                                  # generated, committed
    SKILL.md
    references/<top-level-cmd>.md
  gen/main.go                              # imports the binary's app pkg, calls common/skill/render
```

**Required refactor (one-time)** — `neo4j-cli/main.go` currently builds the cobra tree in `package main`, which the per-binary generator can't import. Move to `neo4j-cli/app/app.go` exposing `NewCmd(cfg) *cobra.Command` and `Version`. `main.go` becomes a thin entrypoint. `neo4j-cli/aura/aura.go` already exposes `NewCmd`/`NewStandaloneCmd` — no aura refactor.

**Versioning** — `{{VERSION}}` placeholder in the embedded `SKILL.md` is substituted at install/check time from the runtime binary version. Same template ships across releases without per-tag regeneration. `skill check` reads the installed file's frontmatter line and compares; no YAML dep needed.

**Reused** —

-   `common/clicfg/fileutils/fileutils.go` (`WriteFile`, `FileExists`) for atomic writes via `cfg.Aura.Fs()`.
-   `clicfg.Config.Version` for runtime version.
-   `neo4j-cli/aura/internal/subcommands/credential/` as the cobra subcommand layout template (one file per leaf, colocated tests).

**Cobra mounting** — in `neo4j-cli/main.go`: `cmd.AddCommand(skill.NewCmd(cfg, binskill.Bundle, "neo4j-cli"))`. In `neo4j-cli/aura/aura.go::NewStandaloneCmd` (NOT in `NewCmd`): `cmd.AddCommand(skill.NewCmd(cfg, aurabinskill.Bundle, "aura-cli"))` so the super-CLI's nested `aura` subtree doesn't pick up a duplicate.

**CI** — new `make generate-check` target runs `go generate ./...` then fails if `git diff --exit-code` is non-zero. Wire into existing PR workflow.

**Out of scope risks** — symlink fallback to copy on Windows is non-trivial; v1 always copies (no symlink). Matches LackOfMorals#9's posture for a Go binary running on three OSes.

## Acceptance Criteria

-   [ ] `make build` produces both binaries with bundles embedded.
-   [ ] `make test` passes on ubuntu, windows, macos (final gate).
-   [ ] `make lint` and `make license-check` pass.
-   [ ] `make generate-check` is green; intentionally skipping `go generate` on a branch makes CI fail.
-   [ ] `bin/neo4j-cli skill list` shows 10 agents; on a Mac with Claude Code installed, `claude-code` is `detected=yes`, `installed=no`.
-   [ ] `bin/neo4j-cli skill install` writes `~/.claude/skills/neo4j-cli/SKILL.md` + `references/*.md`. Frontmatter `version:` line equals output of `bin/neo4j-cli --version`.
-   [ ] `wc -l ~/.claude/skills/neo4j-cli/SKILL.md` returns ≤500.
-   [ ] References under `~/.claude/skills/neo4j-cli/references/` exist for every top-level subcommand of `neo4j-cli` (aura, credential, skill).
-   [ ] `bin/aura-cli skill install claude-code` writes `~/.claude/skills/aura-cli/...`; references include only aura-scoped subcommands and `credential` + `skill` (per `NewStandaloneCmd`), no nested `aura` group.
-   [ ] Bundles for `neo4j-cli` vs `aura-cli` differ — confirmed by `diff` between the two installed `SKILL.md` files.
-   [ ] `bin/neo4j-cli skill check` exits 0 immediately after install; after manually editing the installed `version:` to a stale value, exits 1 with a drift row.
-   [ ] `bin/neo4j-cli skill remove` removes the install dir; second run is idempotent.
-   [ ] Editing any subcommand's flag help and running `go generate ./...` updates both binaries' bundles.
-   [ ] Changelog entries land under `.changes/unreleased/` for both `aura-cli` and `neo4j-cli` projects, kind `Minor`.
-   [ ] Tests cover all REQ-T-* — hermetic FS, table-driven flag/agent/path cases, golden-file render, generator round-trip, frontmatter + line-count gates. ≥80% line coverage on `common/skill/`.
-   [ ] CI matrix (ubuntu/windows/macos) green for every new test.
-   [ ] `README.md` "Agent skills" section present with examples for install/list/check/remove.
-   [ ] `docs/usageGuide/Neo4j CLI.md` and `docs/usageGuide/A Guide To The New Aura CLI.md` updated with skill-subcommand sections.
-   [ ] `CONTRIBUTING.md` documents `go generate ./...` + `make generate-check` + how to edit `additions.md`.
-   [ ] `CLAUDE.md` Architecture section references `common/skill/` and the per-binary template.

## Out of Scope

-   Auto-update on version drift (only reporting via `skill check`).
-   Skills for non-bundled tools, MCP servers, or external integrations.
-   YAML parsing of frontmatter (single-line regex is sufficient).
-   Symlink-based installs (always copy).
-   Per-user customization of which subcommands a binary's skill advertises (binary-scope only).

## Open Questions

None — all design decisions resolved with the user prior to PRD generation.
