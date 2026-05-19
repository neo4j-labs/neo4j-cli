# PRD: CLI-148 — let @claude tagging push commits back to PRs

## Overview

When a repo member tags `@claude` on a PR or PR review comment, the
existing `.github/workflows/claude.yml` workflow boots the
`anthropics/claude-code-action@v1` runner and lets Claude edit files,
but the workflow's `GITHUB_TOKEN` is read-only, so Claude commits
locally and then bails out with a "push requires user approval"
message (see PR #133 thread, comment `3259511332`). This PRD scopes a
small CI-only change to give that workflow the permissions and
configuration it needs to finish the loop — push a verified
`claude[bot]` commit straight to the PR branch — while preserving the
trust posture of the surrounding Claude review workflows.

## Goals

- Tagging `@claude` on a PR results in Claude's requested change being
  committed AND pushed to the PR branch automatically, without the user
  having to run `git push` manually.
- Pushed commits land as `claude[bot]` with a verified-commit badge.
- Write-capable `@claude` invocations are restricted to repo members
  (MEMBER / OWNER / COLLABORATOR) — same gating model already used by
  `claude-review-conventions.yml` and `claude-review-security.yml`.
- Bot comments that contain `@claude` cannot trigger the workflow
  (avoids loops where another bot echoes the marker).

## Non-Goals

- Pushing back to PR branches in **forks**. GitHub's default
  `GITHUB_TOKEN` cannot push to a fork's head ref; supporting that
  would require provisioning a separate GitHub App and a token-swap
  step. Out of scope here; for fork PRs `@claude` will continue to
  comment and the human can pull the diff manually.
- Changes to any other workflow (`claude-review-conventions.yml`,
  `claude-review-security.yml`, etc.). Those stay read-only and
  PR-event-triggered.
- Changes to Go code under `neo4j-cli/`. No source, tests, or skill
  bundle regen required.
- Changelog entry. Per AGENTS.md, CI/CD-only changes do not require a
  `changie` entry.
- Branch creation, PR creation, or auto-merge by `@claude`. The
  workflow only pushes commits to the existing PR branch the user
  invoked Claude on.

## Requirements

### Functional Requirements

- **REQ-F-001**: `.github/workflows/claude.yml`'s `permissions:` block
  MUST grant `contents: write` and `pull-requests: write`. All other
  permissions (`issues: read`, `id-token: write`, `actions: read`)
  stay unchanged.
- **REQ-F-002**: The `anthropics/claude-code-action@v1` step in
  `claude.yml` MUST be configured with `use_commit_signing: true` so
  Claude's commits are created via the GitHub API and carry the
  verified badge as `claude[bot]`.
- **REQ-F-003**: The workflow's `if:` MUST gate execution to invocations
  whose `author_association` is one of `MEMBER`, `OWNER`, or
  `COLLABORATOR`. The check dispatches per event type (because the
  field path differs):
    - `issue_comment` → `github.event.comment.author_association`
    - `pull_request_review_comment` → `github.event.comment.author_association`
    - `pull_request_review` → `github.event.review.author_association`
    - `issues` → `github.event.issue.author_association`
- **REQ-F-004**: The workflow's `if:` MUST additionally require
  `github.event.sender.type != 'Bot'`, so a comment from another bot
  containing `@claude` does not trigger a write-capable run.
- **REQ-F-005**: Existing `@claude` body/title trigger checks (one per
  event type) MUST be preserved — the new gates are added, not
  replacements.
- **REQ-F-006**: No `claude_args` tool allowlist is added. `@claude`
  remains the trusted-member, free-form surface (matching today's
  behaviour); the locked-down allowlists stay scoped to the review
  workflows.

### Non-Functional Requirements

- **REQ-NF-001**: The change MUST be limited to a single file:
  `.github/workflows/claude.yml`. No edits to other workflows, scripts,
  Go sources, or generated bundles.
- **REQ-NF-002**: When a non-MEMBER/OWNER/COLLABORATOR or a Bot author
  posts `@claude`, the workflow MUST be skipped without consuming
  OAuth tokens or Actions minutes (i.e. the `if:` fails before any
  step runs).
- **REQ-NF-003**: Pinned action SHAs in `claude.yml` (`actions/checkout`,
  `anthropics/claude-code-action`) MUST NOT be downgraded as part of
  this change.

## Technical Considerations

- **Single source of truth for gating**: the existing review workflows
  already use `contains(fromJSON('["MEMBER","OWNER","COLLABORATOR"]'),
  github.event.pull_request.author_association)` — but they run on
  `pull_request` events, where the author is on
  `pull_request.author_association`. `claude.yml` listens to four
  different event types whose author field lives in a different
  object each time, so the check has to be dispatched per
  `github.event_name`. The cleanest form folds the existing
  per-event-name body check together with the new
  `author_association` check inside the same conditional branch.
- **Verified commits**: `use_commit_signing: true` makes
  `anthropics/claude-code-action` create commits via the GitHub
  contents API (not `git commit` on the runner). Commits are signed
  by GitHub on Claude's behalf, surface as `claude[bot]`, and carry
  the green Verified badge. Trade-off acknowledged in the action's
  docs: signed-API commits cannot rebase. For our use case (apply
  changes, push), that's fine.
- **Why not a GitHub App**: a custom App would unlock fork pushes and
  finer-grained scopes, but it adds rotation, secret management, and
  installation review burden. The default GITHUB_TOKEN with
  `contents: write` is sufficient for the same-repo branch case that
  CLI-148 reports.
- **Concurrency / collisions**: Out of scope — no concurrency group
  changes. If two `@claude` invocations race on the same branch, the
  second push will fail and Claude will report the error in the
  comment, same as a human would see.

## Acceptance Criteria

- [ ] `.github/workflows/claude.yml` has `contents: write` and
      `pull-requests: write` under `permissions:`.
- [ ] `.github/workflows/claude.yml` passes `use_commit_signing: true`
      to `anthropics/claude-code-action@v1`.
- [ ] `.github/workflows/claude.yml`'s `if:` rejects invocations where
      `github.event.sender.type == 'Bot'`.
- [ ] `.github/workflows/claude.yml`'s `if:` rejects invocations whose
      author_association is not one of `MEMBER`, `OWNER`,
      `COLLABORATOR` (dispatched per event type).
- [ ] No other workflow files are modified.
- [ ] No source files under `neo4j-cli/`, `common/`, or `distribution/`
      are modified.
- [ ] No `.changes/unreleased/*.yaml` entry is added.
- [ ] **End-to-end verification**: open a throwaway PR, comment
      `@claude please add a trailing newline to <some file>`, observe
      the workflow run, and confirm a verified `claude[bot]` commit
      appears on the PR branch without any "approve push" message
      from Claude.
- [ ] **Negative verification**: have a non-MEMBER account (or a bot
      account) comment `@claude` on a PR and confirm the workflow run
      is skipped (no Claude steps execute).

## Out of Scope

- Fork PR support (requires GitHub App + token swap).
- Branch creation / PR creation / auto-merge by `@claude`.
- Tightening or changing the tool allowlist for the `@claude`
  workflow.
- Any change to `claude-review-conventions.yml` or
  `claude-review-security.yml`.
- Go source or test changes.
- Skill bundle regeneration.
- Changelog (`changie`) entry.

## Open Questions

None. Earlier ambiguity around `author_association` field paths and
the bot-echo gate has been resolved (confirmed against the GitHub REST
docs; bot gating included per user direction).
