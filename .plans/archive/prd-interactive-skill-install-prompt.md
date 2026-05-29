# PRD: Interactive skill-install prompt in curl installer (CLI-181)

## Overview

When `neo4j-cli` is installed via the curl install script (`curl … | bash` from `neo4j.sh/install.sh`), prompt the user — only in an interactive terminal — whether to also install the agent-skill bundle (`neo4j-cli skill install --rw`). The existing `NEO4J_CLI_AUTO_INSTALL_SKILL` env var becomes the non-interactive override (`=1` force install, `=0` force skip). Mirror the same flow in the PowerShell installer. Update README so the curl one-liner no longer leads with the env-var prefix.

Linear: CLI-181.

## Goals

- Lower the friction for new users to get the agent-skill bundle installed alongside the binary.
- Keep CI / unattended installs deterministic and untouched (env var still works exactly as today).
- Drop the awkward `NEO4J_CLI_AUTO_INSTALL_SKILL=1 curl … | bash` from README's headline install command.

## Non-Goals

- No change to the npm postinstall (`distribution/npm/cli/bin/postinstall.js`) — still env-var driven.
- No change to the Homebrew formula `post_install` block in `.goreleaser.yaml` — still env-var driven.
- No change to `neo4j-cli skill install` itself (no new flags, no behaviour changes).
- No new agent-selection prompt — the installer calls `skill install --rw` which installs into all detected agents (existing behaviour).
- No PTY-based automated test harness for the interactive prompt.

## Requirements

### Functional Requirements

- REQ-F-001: `distribution/installation-scripts/install-neo4j-cli.sh` — when `NEO4J_CLI_AUTO_INSTALL_SKILL=1`, install the skill bundle without prompting (current behaviour preserved).
- REQ-F-002: Same script — when `NEO4J_CLI_AUTO_INSTALL_SKILL=0`, skip the skill install without prompting (new explicit opt-out).
- REQ-F-003: Same script — when the env var is unset/empty AND the installer is running in an interactive terminal (`[ -t 1 ]` true AND `/dev/tty` readable), prompt: `Install neo4j-cli skill bundle for detected AI agents? [Y/n]`. Read the response from `/dev/tty` (not stdin, which is the curl pipe).
- REQ-F-004: Prompt default is **yes**: empty input or `y`/`Y`/`yes`/`YES` → run `neo4j-cli skill install --rw`. `n`/`N`/`no`/`NO` → skip.
- REQ-F-005: Same script — when the env var is unset AND the installer is non-interactive (no readable `/dev/tty` or stdout not a TTY), skip the skill install with no prompt. This matches today's behaviour.
- REQ-F-006: A failed `skill install --rw` invocation must NOT cause the installer to exit non-zero (preserve `|| true` behaviour).
- REQ-F-007: `distribution/installation-scripts/install-neo4j-cli.ps1` — mirror REQ-F-001..F-006 with PowerShell idioms: gate the prompt on `[Environment]::UserInteractive -and -not [Console]::IsInputRedirected`; use `Read-Host`; wrap the invocation in `try { … } catch { }`.
- REQ-F-008: `README.md` — drop the `NEO4J_CLI_AUTO_INSTALL_SKILL=1 ` prefix from the curl one-liner only. The brew and npm example lines keep their `NEO4J_CLI_AUTO_INSTALL_SKILL=1 ` prefix because those channels do not prompt.
- REQ-F-009: `README.md` — replace the explanatory paragraph below the curl block with a single short sentence: in TTY mode the curl installer asks; set `NEO4J_CLI_AUTO_INSTALL_SKILL=1` (or `=0`) for unattended installs.
- REQ-F-010: Add a changelog entry via `changie new --projects neo4j-cli --kind Minor --body "…"` describing the new interactive prompt.

### Non-Functional Requirements

- REQ-NF-001: No change to existing bats test outcomes — all four tests in `distribution/installation-scripts/tests/install-neo4j-cli.bats` must keep passing because bats runs the script non-interactively (`[ -t 1 ]` false), which now routes to the "skip" branch identical to today.
- REQ-NF-002: Same for Pester tests in `distribution/installation-scripts/tests/install-neo4j-cli.Tests.ps1` — they run non-interactive so `[Environment]::UserInteractive` is false.
- REQ-NF-003: Updated `install-neo4j-cli.ps1` keeps CRLF line endings (per AGENTS.md PowerShell installer notes).
- REQ-NF-004: `make test && make fmt-check && make lint` must all pass before merge.
- REQ-NF-005: Apache copyright header on any new/modified `.sh`/`.ps1` files unchanged (no new files; existing headers preserved).

## Technical Considerations

**TTY detection under `curl | bash`** — stdin (fd 0) is the curl pipe, so `[ -t 0 ]` is always false in the headline install flow. The only reliable signal is `/dev/tty` (the bash process's controlling terminal), which stays available across the pipe. `[ -r /dev/tty ]` is true on real terminals and false in CI/Docker/sandboxes. Pair with `[ -t 1 ]` (stdout TTY) for belt-and-braces. The prompt then reads input via `read -r answer < /dev/tty`.

**Precedence** — env var beats prompt; explicit `=0` is honoured (new). Pattern:

```bash
case "${NEO4J_CLI_AUTO_INSTALL_SKILL:-}" in
  1) install ;;
  0) skip ;;
  *) if interactive: prompt; else skip ;;
esac
```

**PowerShell parity** — `[Environment]::UserInteractive` is false under most automation hosts (Pester default, CI). For the headline `iex (irm 'https://neo4j.sh/install.ps1')` flow from an interactive PowerShell, it's true.

**gh-pages sync** — `gh-pages/install.sh` and `gh-pages/install.ps1` are byte-identical copies of the canonical files (`.github/prompts/website-update.md` lines 45–46 do `cp`). Post-merge, refresh by running the website-update prompt in a `gh-pages` worktree — existing workflow, NOT part of this PR.

**Files to touch:**

- `distribution/installation-scripts/install-neo4j-cli.sh` — replace lines 170–173 (`# ── Auto-install skill ──` block) with a case-dispatch.
- `distribution/installation-scripts/install-neo4j-cli.ps1` — replace lines 189–192 (`# -- Auto-install skill (opt-in) --` block) with a switch.
- `README.md` — edit lines 6 and 9 only.
- `.changes/unreleased/neo4j-cli-Minor-…yaml` — new changelog entry.

**Reuses existing patterns:**

- TTY detection via `[ -t 1 ]` mirrors the existing colour-output gate at `install-neo4j-cli.sh:15`.
- `|| true` suppression mirrors the existing `skill install` invocation it replaces.

**Risks:**

- Users in a TTY who don't read the prompt and habit-hit Enter will silently install the skill. Mitigation: the prompt text says exactly what will happen; default-Y matches ticket intent and the new README messaging.
- Some terminal multiplexers / Docker exec sessions can fail `[ -r /dev/tty ]` despite being interactive-feeling. In those cases the installer silently skips and the user can re-run with the env var — acceptable fallback.

## Acceptance Criteria

- [ ] `bash install-neo4j-cli.sh` from a normal terminal prompts `Install neo4j-cli skill bundle for detected AI agents? [Y/n]`, default-installs on Enter, skips on `n`.
- [ ] `NEO4J_CLI_AUTO_INSTALL_SKILL=1 bash install-neo4j-cli.sh < /dev/null` installs the skill without prompting (today's behaviour).
- [ ] `NEO4J_CLI_AUTO_INSTALL_SKILL=0 bash install-neo4j-cli.sh` skips the skill install without prompting (new).
- [ ] `bash install-neo4j-cli.sh < /dev/null` (non-interactive, no env var) skips the skill install without prompting.
- [ ] `iex (irm 'install-neo4j-cli.ps1')` from an interactive PowerShell session prompts and respects the response.
- [ ] All four existing bats tests in `distribution/installation-scripts/tests/install-neo4j-cli.bats` still pass.
- [ ] All existing Pester tests in `distribution/installation-scripts/tests/install-neo4j-cli.Tests.ps1` still pass.
- [ ] `README.md` curl one-liner is `curl -sSfL https://neo4j.sh/install.sh | bash` (no env-var prefix). Brew + npm lines still carry their `NEO4J_CLI_AUTO_INSTALL_SKILL=1 ` prefix.
- [ ] `README.md` has a one-sentence note below the curl block explaining the TTY prompt and the env-var override.
- [ ] `make test && make fmt-check && make lint` all clean.
- [ ] Changelog entry committed under `.changes/unreleased/`.

## Out of Scope

- gh-pages refresh (`gh-pages/install.sh` and `.ps1`) — separate post-merge action via the website-update prompt.
- Agent picker / per-agent prompts. The installer calls the same `skill install --rw` (all detected agents) as today.
- Bats/Pester tests that simulate the interactive prompt via a PTY. Manual verification covers the new branch.
- Renaming or repurposing the `NEO4J_CLI_AUTO_INSTALL_SKILL` env var.
- Changes to the npm postinstall or Homebrew `post_install` (both remain env-var driven).

## Open Questions

(none)
