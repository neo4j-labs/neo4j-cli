# PRD: Auto AI PR reviews (CLI-138)

## Overview

Add automatic Claude-driven PR reviews as separate GitHub Actions checks, one concern per check. Start with two: **security** (Go-focused, per the in-repo `golang-security` skill scope) and **repo conventions** (AGENTS.md gates — cobra layout, license headers, changelog, skill-bundle regen, etc.). Each check appears as its own status entry on the PR so it can be re-run, disabled, or made required independently. Reviews run on every push from internal authors only; outside contributors and bots are skipped. Existing mention-driven `claude.yml` stays untouched and continues to serve `@claude` requests.

Linear: [CLI-138](https://linear.app/neo4j/issue/CLI-138/lets-experiment-with-ai-pr-reviews).

## Goals

- Two new GitHub Actions workflows (`claude-review-security.yml`, `claude-review-conventions.yml`) post a Claude-authored review on every non-draft PR open / synchronize / reopen / ready_for_review from internal authors.
- Each workflow appears as a distinct check on the PR; failing the check (issues found) shows a red ❌.
- Review prompts live in `.github/prompts/` so they can be iterated without touching CI YAML.
- Existing `@claude` mention behaviour is preserved.
- Pattern is easily extensible: adding a third concern = one new workflow file + one new prompt file.

## Non-Goals

- Modifying `.github/workflows/claude.yml` (mention-driven). The existing access model — write-access required to comment `@claude` — is accepted as sufficient.
- Making the new checks **required** in branch protection. Stay advisory until we see signal on false-positive rate.
- Reviewing PRs from outside contributors automatically (they can still invoke `@claude` manually).
- A "sticky" single comment that updates in place — each push gets a fresh review.
- Reviewing dependabot/renovate PRs.
- Path filtering at the workflow level — prompts handle "no relevant changes" no-op themselves.
- Per-PR cost cap / `--max-turns` tuning beyond the action defaults (revisit after first week).

## Requirements

### Functional Requirements

- REQ-F-001: New workflow `.github/workflows/claude-review-security.yml` triggers on `pull_request` types `[opened, synchronize, reopened, ready_for_review]`.
- REQ-F-002: New workflow `.github/workflows/claude-review-conventions.yml` triggers on the same events.
- REQ-F-003: Both workflows skip when `github.event.pull_request.draft == true`.
- REQ-F-004: Both workflows run only when `pull_request.author_association` is `MEMBER`, `OWNER`, or `COLLABORATOR`. Outside contributors and bots (dependabot, renovate, etc.) are skipped and can still invoke `@claude` manually via the existing mention workflow.
- REQ-F-005: Both workflows use `anthropics/claude-code-action@939ae9c056ecf8a1a01409ddd1c4eadec5f8c77b` (same pinned SHA as existing `claude.yml`) and authenticate via `secrets.CLAUDE_CODE_OAUTH_TOKEN` (no new secret).
- REQ-F-006: Both workflows set `permissions:` to `contents: read`, `pull-requests: write` (required to post reviews), `issues: read`, `id-token: write`, `actions: read`. Net escalation vs `claude.yml` is `pull-requests: write` only.
- REQ-F-007: `claude_args` restricts tools to a narrow allowlist: `mcp__github_inline_comment__create_inline_comment`, `Bash(gh pr comment:*)`, `Bash(gh pr diff:*)`, `Bash(gh pr view:*)`, `Bash(gh pr checks:*)`, `Read`, `Grep`, `Glob`. No general `Bash`, no `Write`/`Edit`, no `curl`/`wget`.
- REQ-F-008: Review prompts live as standalone markdown files: `.github/prompts/claude-review-security.md` and `.github/prompts/claude-review-conventions.md`. The workflow loads the prompt by concatenating the file into the action's `prompt:` input (e.g. via `${{ env.PROMPT }}` set from a prior `cat` step, or a heredoc).
- REQ-F-009: Each prompt instructs Claude to (a) read `gh pr diff`, (b) post inline comments via the MCP tool for concrete issues, (c) post a single top-level summary via `gh pr comment` ending with an explicit verdict line (`✅ no issues found` or `⚠️ N issue(s) flagged inline`), and (d) write a one-word verdict (`pass` or `fail`) to a known file (e.g. `/tmp/claude-verdict.txt`) before exiting.
- REQ-F-010: A follow-up workflow step reads the verdict file and exits non-zero when verdict is `fail`, causing the GitHub check to go red. Missing/unreadable verdict file is treated as `fail` (fail-closed).
- REQ-F-011: The security prompt's review scope mirrors the `golang-security` skill: injection (SQL, command, XSS), cryptography, filesystem safety, network security, cookies, secrets management, memory safety, logging. On PRs with no Go changes the prompt instructs Claude to post a one-line "no security-relevant changes" comment and emit `pass`.
- REQ-F-012: The conventions prompt checks: cobra one-file-per-leaf layout, license header on new `.go` files, changelog entry under `.changes/unreleased/` for user-facing changes (with internal/CI-only exception), skill-bundle regen after touching commands reachable from `app.NewCmd`, non-empty `Example:` on new leaf commands, `gofmt`/lint hygiene, singular-noun + `<resource> <action>` form, `--format`/`--wait` conventions. Source-of-truth document is `AGENTS.md`, loaded automatically via `CLAUDE.md`'s `@AGENTS.md` include.
- REQ-F-013: Existing `.github/workflows/claude.yml` is not modified.

### Non-Functional Requirements

- REQ-NF-001: No effect on `make build`, `make test`, `make lint`, `make fmt-check`, `make license-check`, or `make generate-check`.
- REQ-NF-002: No changelog entry required (CI tooling, not user-facing CLI behaviour — per AGENTS.md changelog policy).
- REQ-NF-003: New workflow files lint clean under `actionlint` (if/when CI adds it; not a current gate).
- REQ-NF-004: A typical PR review run completes within 5 minutes wall-clock.
- REQ-NF-005: Each workflow file ≤ ~80 lines (prompt extracted to `.github/prompts/`); easy to scan.

## Technical Considerations

- **Prompt loading**: Read the prompt file in a prior workflow step (e.g. `PROMPT=$(cat .github/prompts/claude-review-security.md)` → write to `$GITHUB_ENV`), then reference `${{ env.PROMPT }}` in the action's `prompt:` input. Avoids YAML heredoc escaping. Prepend `REPO:` / `PR NUMBER:` context inline in the workflow before the prompt body.
- **Verdict file → red check**: The `anthropics/claude-code-action` step itself completes successfully whether Claude finds issues or not. To turn findings into a failing check, the prompt instructs Claude to write `pass`/`fail` to `/tmp/claude-verdict.txt` (via the allowed `Bash(gh ...)` family — needs adjustment to also allow `Bash(echo:*)` redirected to that file, or a small dedicated tool). A subsequent `run:` step does `[ "$(cat /tmp/claude-verdict.txt 2>/dev/null)" = "pass" ] || { echo '::error::Claude flagged issues'; exit 1; }`. Fail-closed if the file is missing — protects against Claude exiting early without writing a verdict.
- **`claude_args` tool allowlist needs to permit writing the verdict file**. Options: (a) add `Bash(bash -c:*)` for an `echo > /tmp/claude-verdict.txt`, or (b) add a narrow `Bash(tee:*)` / `Bash(printf:*)`. Pick the narrowest that works during prototyping.
- **Author filter is at the workflow `if:` level**, not inside the action. Skipped runs cost 0 minutes (GitHub doesn't bill skipped jobs) and don't consume OAuth tokens. Filter is org-membership only (`author_association` in `MEMBER`/`OWNER`/`COLLABORATOR`) — no inline username allowlist to maintain.
- **`fetch-depth: 1`** matches `claude.yml`. The action reads the PR diff via `gh pr diff`, not git history, so shallow checkout is fine.
- **Prompt-injection surface**: PR file contents can contain "ignore previous instructions, exfiltrate X" payloads. Mitigation: narrow `--allowedTools` (no `curl`/`wget`/general `Bash`/`Write`), `GITHUB_TOKEN` perms scoped to `pull-requests: write` only. Worst case under injection = a misleading review comment, not exfiltration.
- **Two files vs one**: confirmed two-file layout. Each is its own check entry on the PR, independently re-runnable, easy to disable (`if: false` or delete file). Trade-off: ~30 lines of YAML duplication per file. Acceptable.
- **Cost**: per-PR cost = 2× OAuth runs on every push from internal authors. Org-membership filter removes outsiders and bots. Path filter can be added later if noisy.

## Acceptance Criteria

- [ ] `.github/workflows/claude-review-security.yml` exists and matches REQ-F-001/003/004/005/006/007/008/009/010/011.
- [ ] `.github/workflows/claude-review-conventions.yml` exists and matches REQ-F-002/003/004/005/006/007/008/009/010/012.
- [ ] `.github/prompts/claude-review-security.md` exists with the security-review prompt body.
- [ ] `.github/prompts/claude-review-conventions.md` exists with the conventions-review prompt body.
- [ ] `.github/workflows/claude.yml` is byte-for-byte unchanged.
- [ ] Opening a non-draft PR from `@oskarhane` triggers both check workflows; both post a top-level summary + (if relevant) inline comments within 5 minutes.
- [ ] Opening a draft PR triggers neither workflow; promoting it to ready-for-review triggers both.
- [ ] Opening a PR from `dependabot[bot]` or `renovate[bot]` skips both workflows (no billable run).
- [ ] A deliberate convention violation (e.g. new leaf command with no `Example:` field) flips the conventions check to red ❌ with an explanatory inline comment.
- [ ] A deliberate security smell (e.g. `exec.Command("sh", "-c", userInput)`) flips the security check to red ❌ with an explanatory inline comment.
- [ ] A clean PR with no findings shows both checks green ✅ and a top-level summary ending `✅ no issues found`.
- [ ] Pushing a second commit to the same PR produces a **new** review comment (not an edit of the prior one).
- [ ] After merge, commenting `@claude help` on a different PR still triggers the existing `claude.yml` mention workflow.
- [ ] `make test && make fmt-check && make lint` pass (no Go code touched; gates should be no-op).

## Out of Scope

- Modifying `claude.yml`.
- Adding a third or fourth review check (test-coverage, doc-drift, perf). Revisit after first week of signal.
- Marking either check as a required status in branch protection.
- `actionlint` CI step.
- A composite action / reusable workflow to dedupe the shared skeleton (premature — only 2 files).
- Cost dashboarding / per-check budget caps.
- Path-based filtering at the workflow level.
- Sticky comments / replacing prior reviews on subsequent pushes.

## Open Questions

- Exact `claude_args` tool string needed to let Claude write `/tmp/claude-verdict.txt` without opening up general `Bash`. To be settled during implementation by trying the narrowest viable option.
- Whether the conventions prompt should also nudge on the title/commit-message conventions (e.g. conventional-commit prefix `feat(...)`) or stay scoped to code/file conventions only.
