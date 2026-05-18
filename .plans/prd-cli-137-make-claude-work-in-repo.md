# PRD: Make @claude work in the repo (CLI-137)

## Overview

The `@claude` GitHub Action (`.github/workflows/claude.yml`, `anthropics/claude-code-action@v1`) does not pick up this repo's contributor instructions because `CLAUDE.md` is a symlink to `AGENTS.md` rather than a real file. Replace the symlink with a regular file whose only contents are `@AGENTS.md` — Claude Code's file-include directive — so the action loads `AGENTS.md` while we keep a single source of truth.

Linear: [CLI-137](https://linear.app/neo4j/issue/CLI-137/make-claude-work-in-the-repo). Failing example: [neo4j-labs/neo4j-cli#118](https://github.com/neo4j-labs/neo4j-cli/pull/118).

## Goals

- `@claude` mentions on PRs/issues/comments in this repo load the project's AGENTS.md instructions.
- Continue to maintain instructions in exactly one place (`AGENTS.md`); no duplication.

## Non-Goals

- Changing AGENTS.md contents.
- Restructuring agent docs across `AGENTS.md` / `CONTRIBUTING.md` / `.agents/*.md`.
- Adding a regression test/gate against future re-symlinking.
- Changelog entry (internal repo tooling, not user-facing CLI behaviour — per AGENTS.md changelog policy).

## Requirements

### Functional Requirements

- REQ-F-001: `CLAUDE.md` at repo root must be a regular file, not a symlink (`ls -la CLAUDE.md` shows `-rw-r--r--`, not `lrwxr-xr-x ... -> AGENTS.md`).
- REQ-F-002: `CLAUDE.md` contents must be exactly `@AGENTS.md` followed by a trailing newline.
- REQ-F-003: `AGENTS.md` content is unchanged.
- REQ-F-004: The "Repo Doc Notes" bullet in `AGENTS.md` stating `CLAUDE.md is a symlink to AGENTS.md` is removed (now stale / wrong).

### Non-Functional Requirements

- REQ-NF-001: No effect on `make build`, `make test`, `make lint`, `make fmt-check`, `make license-check`, or `make generate-check`.
- REQ-NF-002: No CI workflow file changes required (`.github/workflows/claude.yml` already exists and is correctly wired; the fix is filesystem-only).

## Technical Considerations

- The symlink swap is a single git operation: `git rm CLAUDE.md` then `git add CLAUDE.md` for the new file (or one `git mv`-equivalent commit). Git stores the mode change (`120000` → `100644`) plus the new blob.
- `@AGENTS.md` is the Claude Code file-include syntax — Claude Code resolves it and loads `AGENTS.md` into context. No path-resolution gotcha because both files sit at repo root.
- No Go source touched. `.go` license-header check is unaffected.
- Verification = read the file back after the swap; `ls -la` to confirm it's no longer a symlink; `git diff --stat HEAD~1` after commit shows the mode change.

## Acceptance Criteria

- [ ] `ls -la CLAUDE.md` shows a regular file (`-rw-r--r--`), not a symlink.
- [ ] `cat CLAUDE.md` outputs exactly `@AGENTS.md\n`.
- [ ] `AGENTS.md` content unchanged except for removal of the "CLAUDE.md is a symlink to AGENTS.md" bullet under "Repo Doc Notes".
- [ ] `make test && make fmt-check && make lint` pass.
- [ ] `git diff HEAD~1 -- CLAUDE.md` shows mode change `120000 → 100644` plus the new blob contents.
- [ ] After merge, an `@claude` mention on a PR or issue in this repo triggers a response that reflects AGENTS.md guidance (smoke-tested post-merge).

## Out of Scope

- Regression test asserting `CLAUDE.md` is not a symlink.
- Any change to `AGENTS.md` other than dropping the now-stale bullet.
- Touching `.github/workflows/claude.yml`.
- Documentation updates beyond the one stale bullet.

## Open Questions

None.
