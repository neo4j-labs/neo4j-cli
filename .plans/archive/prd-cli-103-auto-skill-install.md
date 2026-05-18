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

- REQ-F-005: Behavioral tests for `install-neo4j-cli.sh` using bats-core must
  be added under `distribution/installation-scripts/tests/`. Tests must mock
  the `neo4j-cli` binary with a stub script (records invocations to a temp
  file) and verify:
  1. With `NEO4J_CLI_AUTO_INSTALL_SKILL=1`: the stub is called with
     `skill install --rw`.
  2. With env var unset or `=0`: the stub is never called.
  3. With `=1` but the stub exits non-zero: the install script still exits 0.
- REQ-F-006: Behavioral tests for `install-neo4j-cli.ps1` using Pester must
  be added under `distribution/installation-scripts/tests/`. Tests must mock
  the `neo4j-cli` invocation (via a stub `.ps1` or function override) and
  verify the same three cases as REQ-F-005.
- REQ-F-007: Behavioral tests for the npm postinstall hook must be added under
  `distribution/npm/cli/`. Tests must mock the invocation of
  `bin/neo4j-cli.js` (e.g. via Jest module mocking or a child_process stub)
  and verify:
  1. With `NEO4J_CLI_AUTO_INSTALL_SKILL=1`: the mock receives `skill install
     --rw`.
  2. With env var unset or `=0`: the mock is never invoked.
  3. With `=1` but the mock throws: the postinstall script exits 0 and does
     not propagate the error.
- REQ-F-008: Behavioral tests for the Homebrew `post_install` Ruby block must
  be added under `distribution/homebrew/tests/`. Tests must source or `eval`
  the Ruby snippet extracted from `.goreleaser.yaml` with a stubbed `system`
  method (records calls) and verify the same three cases as REQ-F-005.
- REQ-F-009: A new GitHub Actions workflow
  (`.github/workflows/installer-tests.yml`) must run all installer tests on
  pull requests. Shell (bats-core) tests run on `ubuntu-latest` and
  `macos-latest`; Pester tests run on `windows-latest`; npm/Jest tests run on
  all three platforms; Ruby tests run on `ubuntu-latest`.
- REQ-F-010: The `Makefile` must add a `test-installer-sh` target that runs
  the bats-core suite: `bats distribution/installation-scripts/tests/install-neo4j-cli.bats`.
  Fails with a clear error if `bats` is not found on PATH.
- REQ-F-011: The `Makefile` must add a `test-installer-ps1` target that runs
  the Pester suite via `pwsh -NoProfile -NonInteractive -Command "Invoke-Pester -Path distribution/installation-scripts/tests/install-neo4j-cli.Tests.ps1 -Output Detailed"`.
  If `pwsh` is not found on PATH, the target must print a skip message and
  exit 0 (graceful skip on non-Windows machines).
- REQ-F-012: The `Makefile` must add a `test-installer-npm` target that runs
  `npm ci && npm test` inside `distribution/npm/cli/`.
  Fails with a clear error if `npm` is not found on PATH.
- REQ-F-013: The `Makefile` must add a `test-installer-rb` target that runs
  `ruby distribution/homebrew/tests/post_install_test.rb`.
  Fails with a clear error if `ruby` is not found on PATH.
- REQ-F-014: The `Makefile` must add a `test-installer` aggregate target that
  calls `test-installer-sh`, `test-installer-ps1`, `test-installer-npm`, and
  `test-installer-rb` in order. All targets that are not skipped must pass for
  `test-installer` to succeed.

### Non-Functional Requirements

- REQ-NF-001: Installer scripts must be resilient to binary-not-found, "no
  agents detected" exits, and any other non-zero exit from `skill install --rw`.
  These failures must not abort the overall package install.
- REQ-NF-002: No new Go code or cobra commands are introduced. The existing
  `skill install` command is used as-is. `make test`, `make generate-check`,
  and all gate targets must pass without modification to the Go codebase.
- REQ-NF-003: All installer tests must run without a real `neo4j-cli` binary
  installed. All binary invocations must be intercepted by stubs/mocks.
- REQ-NF-004: Installer tests must be runnable locally with a single command
  per channel (e.g. `bats distribution/installation-scripts/tests/`,
  `Invoke-Pester`, `npm test`, `ruby -Ilib:test`) in addition to running in
  CI.
- REQ-NF-005: The `test-installer-ps1` Makefile target must degrade gracefully
  on platforms where `pwsh` is unavailable (macOS without PowerShell installed,
  Linux) by printing a skip notice and exiting 0, so `make test-installer` is
  usable on all developer machines without requiring PowerShell.

## Testing Strategy

### bats-core (shell)

`bats-core` provides a `@test` function DSL for POSIX shell scripts. Each test
can override `PATH` to inject a stub `neo4j-cli` script that writes its
arguments to a temp file so assertions can confirm whether and how the binary
was called. Tests live in
`distribution/installation-scripts/tests/install-neo4j-cli.bats`. Install
bats-core via Homebrew (`brew install bats-core`) or npm
(`npm install --save-dev bats`); the CI workflow installs it via `apt-get` /
Homebrew action.

### Pester (PowerShell)

Pester v5 is the standard PowerShell test framework. Tests live in
`distribution/installation-scripts/tests/install-neo4j-cli.Tests.ps1`. The
mock strategy replaces `neo4j-cli` with a PowerShell function that captures
its arguments before the script under test runs. Pester is pre-installed on
GitHub-hosted `windows-latest` runners.

### Jest (npm)

The postinstall script entry point (`distribution/npm/cli/postinstall.js`) is
extracted from the inline `package.json.tmpl` script into a separate file so
Jest can `require` and test it in isolation. Jest is added as a dev dependency.
`NEO4J_CLI_AUTO_INSTALL_SKILL` is set/unset via `process.env` in each test
case; the `neo4j-cli.js` invocation is mocked via `jest.mock`.

### Ruby minitest (Homebrew)

The `post_install` Ruby snippet is extracted verbatim into
`distribution/homebrew/tests/post_install_test.rb`. The test uses Ruby's
built-in `minitest` and overrides the `system` kernel method with a lambda that
records calls, allowing assertions that `system` is called with the expected
arguments when the env var is `"1"` and not called otherwise. No Homebrew
installation is required.

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
- [ ] `bats distribution/installation-scripts/tests/` passes on macOS and Linux.
- [ ] Shell tests confirm stub called with `skill install --rw` when `=1`, not called otherwise, and failure suppressed.
- [ ] `Invoke-Pester distribution/installation-scripts/tests/` passes on Windows.
- [ ] PowerShell tests confirm same three cases as shell tests.
- [ ] `npm test` in `distribution/npm/cli/` passes on all platforms.
- [ ] npm tests confirm mock receives `skill install --rw` when `=1`, not called otherwise, and error caught.
- [ ] `ruby distribution/homebrew/tests/post_install_test.rb` passes.
- [ ] Ruby tests confirm `system` called with correct args when `=1`, not called otherwise.
- [ ] `installer-tests.yml` workflow runs and passes on a sample PR.
- [ ] `make test-installer-sh` passes on macOS and Linux.
- [ ] `make test-installer-ps1` skips gracefully (exit 0 + message) when `pwsh` is not installed.
- [ ] `make test-installer-npm` passes on macOS, Linux, and Windows.
- [ ] `make test-installer-rb` passes on macOS and Linux.
- [ ] `make test-installer` runs all four sub-targets and succeeds when non-ps1 targets pass.

## Out of Scope

- Any automatic behaviour without `NEO4J_CLI_AUTO_INSTALL_SKILL=1`.
- Introducing any new `neo4j-cli` CLI commands or subcommands.
- Modifying the `skill install` command behaviour.
- Direct GitHub Release binary downloads.
- PyPI distribution.

## Open Questions

None.
