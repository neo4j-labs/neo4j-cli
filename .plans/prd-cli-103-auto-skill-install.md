# PRD: Auto-Install Skill on CLI Install (CLI-103)

## Overview

When `NEO4J_CLI_AUTO_INSTALL_SKILL=1` is set, each supported distribution
channel (npm, Homebrew, curl|sh) automatically runs
`neo4j-cli skill install --rw` after placing the binary in PATH. The feature
is strictly opt-in: an unset env var behaves identically to `=0` and produces
no action.

| Value | Behaviour |
|-------|-----------|
| `"1"` | Installer calls `skill install --rw` (non-fatal) |
| `"0"` or unset | Skip entirely |

No new CLI commands are introduced. The existing `skill install` command is
invoked directly by each installer script.

## Goals

- Allow users who set `NEO4J_CLI_AUTO_INSTALL_SKILL=1` to get the skill bundle
  installed automatically on CLI install, with no additional manual step.
- Work across npm, Homebrew, and curl|sh distribution channels.
- Leverage the existing `skill install` command without introducing new CLI
  commands or Go code changes.

## Non-Goals

- Any automatic behaviour without explicit opt-in (`=1`).
- Interactive prompting during install.
- Per-agent selection (installs into all detected agents).
- Direct GitHub Release binary downloads.
- PyPI distribution — pip has no post-install hook mechanism for pure binary
  wheels; PyPI users run `neo4j-cli skill install --rw` manually.

## Requirements

### Functional Requirements

- REQ-F-001: The npm distribution must add a `scripts.postinstall` field to
  `distribution/npm/cli/package.json.tmpl`. The postinstall script must:
  1. If `NEO4J_CLI_AUTO_INSTALL_SKILL !== "1"`: skip.
  2. Call `neo4j-cli skill install --rw` via `bin/neo4j-cli.js`, wrapped in
     try/catch so any failure does not fail `npm install`.
- REQ-F-002: The curl|sh installer (`gh-pages/install.sh`) must, after placing
  the binary in PATH:
  1. If `NEO4J_CLI_AUTO_INSTALL_SKILL` is not `"1"`: skip.
  2. Call `neo4j-cli skill install --rw` (non-fatal; suppress non-zero exits).
- REQ-F-003: The Windows PowerShell installer (`gh-pages/install.ps1`) must,
  after placing the binary in PATH:
  1. If `$env:NEO4J_CLI_AUTO_INSTALL_SKILL -ne "1"`: skip.
  2. Call `neo4j-cli skill install --rw` (non-fatal; suppress non-zero exits).
- REQ-F-004: The Homebrew formula must include a `post_install` Ruby block
  added via the `post_install` field in GoReleaser's `brews:` configuration
  in `.goreleaser.yaml`:
  ```ruby
  def post_install
    return unless ENV["NEO4J_CLI_AUTO_INSTALL_SKILL"] == "1"
    system "#{bin}/neo4j-cli", "skill", "install", "--rw"
    true
  end
  ```
  This hook only fires for stable releases (`skip_upload: auto` gates
  prerelease formula pushes).

### Non-Functional Requirements

- REQ-NF-001: Installer scripts must be resilient to binary-not-found, "no
  agents detected" exits, and any other non-zero exit from `skill install --rw`.
  These failures must not abort the overall package install.
- REQ-NF-002: No new Go code or cobra commands are introduced. The existing
  `skill install` command is used as-is. `make test`, `make generate-check`,
  and all gate targets must pass without modification to the Go codebase.

## Technical Considerations

### No new Go code

This feature is entirely implemented in installer scripts. The existing
`skill install --rw` command handles the install logic. No cobra command
registration, no bundle regeneration, no new Go files.

### No-agents-detected exit code

`skill install --rw` exits non-zero with `ErrNoAgentsDetected` when no AI
agent harnesses are detected in the environment. Installer scripts must treat
all non-zero exits from `skill install --rw` as non-fatal (suppress with
`|| true`, try/catch, or PowerShell `-ErrorAction SilentlyContinue`). Users
who install with `=1` set but outside an agent harness will see a non-fatal
warning; no action is needed.

### Homebrew post_install

GoReleaser's `brews:` `post_install` string is embedded verbatim as a Ruby
method in the generated formula. Validate locally via `make snapshot` →
inspect `dist/homebrew/Formula/neo4j-cli.rb`.

### curl|sh and install.ps1

The canonical source for both scripts is `distribution/installation-scripts/`
on the `main` branch (`install-neo4j-cli.sh` and `install-neo4j-cli.ps1`).
Edit them there — changes go through normal PR review and are validated by the
`shellcheck.yml` CI workflow. The `gh-pages/install.sh` and
`gh-pages/install.ps1` served at neo4j.sh are updated separately by the
`update-website.yml` workflow on release; do not edit `gh-pages` directly.

## Acceptance Criteria

- [ ] `NEO4J_CLI_AUTO_INSTALL_SKILL=1 npm install -g @neo4j-labs/cli`: runs `skill install --rw`, non-fatal.
- [ ] `npm install -g @neo4j-labs/cli` (unset or `=0`): skips entirely.
- [ ] `NEO4J_CLI_AUTO_INSTALL_SKILL=1 npm install`, no agents in env: `skill install --rw` exits non-zero → suppressed, `npm install` succeeds.
- [ ] `NEO4J_CLI_AUTO_INSTALL_SKILL=1` + `install.sh`: runs `skill install --rw`; non-fatal.
- [ ] `install.sh` (unset or `=0`): skips entirely.
- [ ] `NEO4J_CLI_AUTO_INSTALL_SKILL=1` + `install.ps1`: runs `skill install --rw`; non-fatal.
- [ ] `install.ps1` (unset or `=0`): skips entirely.
- [ ] `NEO4J_CLI_AUTO_INSTALL_SKILL=1 brew install neo4j-labs/tap/neo4j-cli`: runs `skill install --rw` silently.
- [ ] `brew install/upgrade` (unset or `=0`): skips entirely.
- [ ] `make snapshot` renders formula with correct `post_install` block.
- [ ] `make test`, `make fmt-check`, `make lint` all pass with no Go changes.
- [ ] `go generate ./neo4j-cli/internal/skill/...` produces no diff in bundle.
- [ ] Changelog entry added (`make changelog`).

## Out of Scope

- Any automatic behaviour without `NEO4J_CLI_AUTO_INSTALL_SKILL=1`.
- Introducing any new `neo4j-cli` CLI commands or subcommands.
- Modifying the `skill install` command behaviour.
- Direct GitHub Release binary downloads.
- PyPI distribution.

## Open Questions

None.
