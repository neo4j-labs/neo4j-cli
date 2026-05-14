# PRD: Auto-Install Skill on CLI Install (CLI-103)

## Overview

When a user installs or upgrades `neo4j-cli` via any supported distribution
channel (npm, Homebrew, curl|sh), the CLI should automatically manage the
skill bundle for any detected AI agent harnesses with no prompting.

A single env var `NEO4J_CLI_AUTO_INSTALL_SKILL` controls the entire flow:

| Value | Command in tree? | Behaviour |
|-------|-----------------|-----------|
| `"1"` | Yes | Auto-install or upgrade silently |
| `"0"` | No | Skip entirely (explicit opt-out) |
| unset | No | Installer checks for existing skill installations; if found, sets to `"1"` and calls command; otherwise skips |

Installers set the var to `"1"` only when it is unset **and** `neo4j-cli skill list`
returns at least one installed agent. A fresh install with an unset var produces no
action — the user installs the skill manually via `neo4j-cli skill install --rw`.
A user who pre-exports `NEO4J_CLI_AUTO_INSTALL_SKILL=0` opts out of all channels.

## Goals

- Keep skill bundles in sync automatically when the CLI is upgraded via a
  distribution channel, with no user action required.
- Reduce friction of getting the skill bundle installed for users who explicitly
  opt in (`NEO4J_CLI_AUTO_INSTALL_SKILL=1`).
- Work across npm, Homebrew, and curl|sh distribution channels.
- Leverage the existing skill infrastructure without duplicating logic.
- Keep the command hidden except during installer invocation.

## Non-Goals

- Interactive prompting during install or upgrade.
- Per-agent selection (installs/refreshes all applicable agents).
- Auto-installing the skill on a fresh CLI install when `NEO4J_CLI_AUTO_INSTALL_SKILL`
  is unset — fresh-install users run `neo4j-cli skill install --rw` manually.
- Refreshing skill bundles for agents that don't yet have the skill installed
  during an upgrade (only already-installed agents are refreshed).
- Direct GitHub Release binary downloads — users who download manually are
  expected to run `neo4j-cli skill install --rw`.

## Requirements

### Functional Requirements

- REQ-F-001: The CLI binary must conditionally register a `neo4j-cli skill
  post-install` command **only when `NEO4J_CLI_AUTO_INSTALL_SKILL` equals
  `"1"`** (strict equality). When the var is absent, empty, or `"0"` the
  command must not exist in the cobra tree; invoking it must produce cobra's
  standard "unknown command" error.
- REQ-F-002: When `skill post-install` runs (`=1`), `RunE` must:
  1. Call `List()` to find agents with the skill already installed.
  2. If any are installed (**upgrade path**): call `Install()` for each
     silently, print which agents were refreshed, exit 0.
  3. If none are installed (**fresh-install path**): call `Install()` for all
     currently-detected agents silently. If no agent harnesses are detected,
     print a brief informational message and exit 0.
- REQ-F-003: `skill post-install` must bypass the `--rw` write gate entirely
  and must not register the `--rw` flag.
- REQ-F-004: Per-agent refresh or install failures must be non-fatal. The
  command must continue with remaining agents and print a warning per failure.
- REQ-F-005: `skill post-install` must trigger on every install and upgrade
  (not guarded by a "has run before" marker).
- REQ-F-006: The npm distribution must add a `scripts.postinstall` field to
  `distribution/npm/cli/package.json.tmpl`. The postinstall script must:
  1. If `NEO4J_CLI_AUTO_INSTALL_SKILL === "0"`: skip.
  2. If `NEO4J_CLI_AUTO_INSTALL_SKILL` is unset: invoke `neo4j-cli skill list
     --format json` via `bin/neo4j-cli.js`; if the result is a non-empty array,
     set `process.env.NEO4J_CLI_AUTO_INSTALL_SKILL = "1"` and proceed; otherwise
     skip.
  3. Call `neo4j-cli skill post-install` via `bin/neo4j-cli.js`, wrapped in
     try/catch so failure does not fail `npm install`.
- REQ-F-007: The curl|sh installer (`gh-pages/install.sh`) must, after placing
  the binary in PATH:
  1. If `NEO4J_CLI_AUTO_INSTALL_SKILL` equals `"0"`: skip.
  2. If `NEO4J_CLI_AUTO_INSTALL_SKILL` is unset: run `neo4j-cli skill list
     --format json`; if the output is non-empty (skills are installed), set
     `NEO4J_CLI_AUTO_INSTALL_SKILL=1` and proceed; otherwise skip.
  3. Call `neo4j-cli skill post-install` (non-fatal).
- REQ-F-008: The Windows PowerShell installer (`gh-pages/install.ps1`) must,
  after placing the binary in PATH:
  1. If `$env:NEO4J_CLI_AUTO_INSTALL_SKILL -eq "0"`: skip.
  2. If `$env:NEO4J_CLI_AUTO_INSTALL_SKILL` is not set: run `neo4j-cli skill
     list --format json`; if the output is non-empty (skills are installed), set
     `$env:NEO4J_CLI_AUTO_INSTALL_SKILL = "1"` and proceed; otherwise skip.
  3. Call `neo4j-cli skill post-install` (non-fatal).
- REQ-F-009: The Homebrew formula must include a `post_install` Ruby block
  added via the `post_install` field in GoReleaser's `brews:` configuration
  in `.goreleaser.yaml`:
  ```ruby
  def post_install
    return if ENV["NEO4J_CLI_AUTO_INSTALL_SKILL"] == "0"
    if ENV["NEO4J_CLI_AUTO_INSTALL_SKILL"].nil? || ENV["NEO4J_CLI_AUTO_INSTALL_SKILL"].empty?
      result = `#{bin}/neo4j-cli skill list --format json 2>/dev/null`.strip
      return if result.empty? || result == "[]"
      ENV["NEO4J_CLI_AUTO_INSTALL_SKILL"] = "1"
    end
    system "#{bin}/neo4j-cli", "skill", "post-install"
    true
  end
  ```
  This hook only fires for stable releases (`skip_upload: auto` gates
  prerelease formula pushes).
### Non-Functional Requirements

- REQ-NF-001: Post-install must not block or delay `npm install` on failure.
  Installer scripts must be resilient to binary-not-found and non-zero exits.
- REQ-NF-002: The `skill post-install` command must have colocated unit tests
  covering: upgrade path (agents installed) → all refreshed silently; upgrade
  partial failure → non-fatal, others refreshed; fresh-install path → all
  detected agents installed; no agents detected → informational message. A
  separate test must verify that with the env var unset **and** with `=0` the
  command is not present in the cobra tree.
- REQ-NF-003: Because `skill post-install` is absent from the cobra tree in
  normal runs, `TestAllLeafCommands_HaveExamples` requires no change. A
  dedicated test that builds the tree with `t.Setenv("NEO4J_CLI_AUTO_INSTALL_SKILL", "1")`
  must verify the command has a valid `Example:` field (≥3 `neo4j-cli`-prefixed
  invocations).
- REQ-NF-004: `skill post-install` must not appear in the generated skill bundle
  or any reference documentation. Because `go generate` runs without
  `NEO4J_CLI_AUTO_INSTALL_SKILL=1` set, the command is never added to the cobra
  tree during generation and therefore never emitted to
  `neo4j-cli/internal/skill/bundle/`. `TestGenerator_RoundTrip` must pass without
  any special handling for this command.

## Technical Considerations

### Env var and command registration

Command registration: `os.Getenv("NEO4J_CLI_AUTO_INSTALL_SKILL") == "1"` (strict)
in `skill.NewCmd()`. The command is absent from the tree in every other case —
no hiding, no filtering, it simply is not added.

`RunE` evaluation order:
1. `List()` → find agents with skill installed.
2. Agents found → upgrade path: `Install()` for each, print refreshed list, exit 0.
3. No agents found → fresh-install path: `Install()` for all detected agents.
4. No agents detected → print informational message, exit 0.

No TTY detection. No prompting. The same path runs regardless of interactive
context.

### Write gate bypass

`skill post-install`'s `RunE` must call `Install()` directly (bypassing
`EnforceWriteGate()`). It must not register the `--rw` flag.

### npm postinstall timing

npm runs `scripts.postinstall` after all packages are installed, so the
platform binary is guaranteed to be present when the hook fires.

### Homebrew post_install

GoReleaser's `brews:` `post_install` string is embedded verbatim as a Ruby
method in the generated formula. Validate locally via `make snapshot` →
inspect `dist/homebrew/Formula/neo4j-cli.rb`.

### Upgrade detection via skill list

Each installer detects whether to auto-run by calling `neo4j-cli skill list
--format json` (the binary is already in PATH at this point). Because skill files
live in agent config directories (not in the CLI install path), they survive a
binary upgrade intact. An empty or `[]` result means no skills were previously
installed (fresh install path) and the installer skips. A non-empty result means
the user has already opted in to the skill on at least one agent, and the
installer proceeds. This eliminates any need to inspect npm lifecycle vars,
binary mtimes, or previous PATH state — the skill state is the definitive signal.

### curl|sh and install.ps1

Scripts live on the `gh-pages` branch. Use `git worktree add gh-pages gh-pages`
to edit them. Wrap the skill invocation so its failure does not abort the
overall install.

### Skill bundle generation

`skill post-install` is only registered when `=1`, so `go generate` never sees
it. The skill bundle will not include a reference doc for `post-install`.
`TestGenerator_RoundTrip` requires no special handling. The REQ-NF-003
dedicated test must use `t.Setenv` before calling `app.NewCmd`.

## Acceptance Criteria

- [ ] Env var unset: `neo4j-cli skill post-install` → cobra "unknown command" error.
- [ ] `=0`: `neo4j-cli skill post-install` → cobra "unknown command" error.
- [ ] `=1`: command exists, has valid `Example:` field (dedicated test).
- [ ] `=1`, skill already installed on ≥1 agent: agents refreshed silently, no prompt.
- [ ] `=1`, skill not installed: all detected agents installed silently, no prompt.
- [ ] `=1`, no agent harnesses detected: informational message, exit 0.
- [ ] `=1`, partial failure: failed agents warned, others proceed, exit 0.
- [ ] `npm install -g @neo4j-labs/cli` (fresh, unset): skill list empty → no action.
- [ ] `npm upgrade -g @neo4j-labs/cli` (unset, skills installed): skill list non-empty → sets `=1`, refreshes, non-fatal.
- [ ] `npm install/upgrade -g @neo4j-labs/cli`: `=0` respected; skips entirely.
- [ ] `install.sh` fresh install (unset): skill list empty → no action; non-fatal.
- [ ] `install.sh` upgrade (unset, skills installed): skill list non-empty → refreshes; non-fatal.
- [ ] `install.sh`: `=0` respected; skips entirely.
- [ ] `install.ps1`: same guard logic as `install.sh`; non-fatal.
- [ ] `brew install neo4j-labs/tap/neo4j-cli` (fresh, unset): skill list empty → no action.
- [ ] `brew upgrade neo4j-labs/tap/neo4j-cli` (unset, skills installed): skill list non-empty → installed agents refreshed silently.
- [ ] `brew install/upgrade`: `=0` respected; skips entirely.
- [ ] `make snapshot` renders formula with correct `post_install` block.
- [ ] `make test`, `make fmt-check`, `make lint` all pass.
- [ ] `go generate ./neo4j-cli/internal/skill/...` produces no diff in bundle; `skill post-install` absent from all generated reference docs.
- [ ] Changelog entry added (`make changelog`).

## Out of Scope

- Interactive prompting during install or upgrade.
- Per-agent selection at post-install time.
- Auto-install for direct GitHub Release binary downloads.
- Modifying `skill install` to merge this behaviour.
- PyPI distribution — pip has no post-install hook mechanism for pure binary
  wheels; PyPI users run `neo4j-cli skill install --rw` manually.

## Open Questions

None.
