# PRD: Fix Claude Review workflows (CLI-138 follow-up)

## Overview

PR #129 landed the auto AI PR review pair (`.github/workflows/claude-review-security.yml` + `.github/workflows/claude-review-conventions.yml`) on `main`. The first real run on a contributor PR (#120) surfaced two defects that make the system unusable in its current form:

1. The security workflow fails on every PR because the `claude-code-action` plugin installer shells out to `git clone` and defaults to SSH against `github.com`, but the runner has no SSH key.
2. Both workflow files declare `jobs.security-review:` — the resulting GitHub check name is identical for both, so the two checks are only distinguishable by their `workflowName` label.

This PRD covers a single small CI-only PR that unblocks the security check and disambiguates the conventions check.

## Goals

- Make `Claude Review — Security` reach a verdict (green or red) on every internal PR instead of failing during plugin install.
- Give each of the two Claude Review workflows a distinct GitHub check name so they can be referenced individually (logs, branch protection, status list).
- No behavioural change to the review logic, prompts, skill pin, or tool allowlists.

## Non-Goals

- Changing the `samber/cc-skills-golang` skill pin (`v1.4.0` / sha `e9761db859c6969b77a8fd0e8a243f4f28240211`).
- Editing the prompts under `.github/prompts/`.
- Vendoring the `cc-skills-golang` skill into the repo (the pinned external source remains canonical).
- Adding new gates that detect future drift between the two workflow files.
- Adding the renamed checks to branch protection rules.
- Touching PR #120 or any other in-flight PR.

## Requirements

### Functional Requirements

- **REQ-F-001**: `.github/workflows/claude-review-security.yml` adds a new step that rewrites `git@github.com:` URLs to `https://github.com/` on the runner before the `Run Claude Code (security review)` step. The rewrite uses `git config --global url."https://github.com/".insteadOf "git@github.com:"`. Step placement: after `Install govulncheck`, before `Load security prompt into env`. The step has a comment explaining why it exists (plugin installer shells out to `git clone`, runner has no SSH key, public-repo HTTPS needs no auth).
- **REQ-F-002**: `.github/workflows/claude-review-conventions.yml` renames the job key `jobs.security-review:` → `jobs.conventions-review:`. The `if:`, `runs-on:`, `permissions:`, and all step bodies stay byte-identical to the current main version.
- **REQ-F-003**: `.github/workflows/claude-review-conventions.yml` renames the step `Run Claude Code (security review)` → `Run Claude Code (conventions review)`. No other step name changes.
- **REQ-F-004**: Fallback path executed inside the same PR if REQ-F-001 does not unblock the plugin clone on the fix PR's own check run: edit `.claude-plugin/marketplace.json` to change the plugin source from `source: github` (with `repo: samber/cc-skills-golang`) to `source: git` with `url: https://github.com/samber/cc-skills-golang.git`. The `ref`/`sha` pin values stay the same. The `insteadOf` step from REQ-F-001 may stay in place as belt-and-braces or be removed if it becomes redundant — implementer chooses based on what verifies green.
- **REQ-F-005**: No other files are edited. Explicitly out: prompts under `.github/prompts/`, the marketplace pin values, the `enforce verdict` step error strings, the tool allowlists.

### Non-Functional Requirements

- **REQ-NF-001**: Change is CI-only. `make test`, `make fmt-check`, `make lint`, `make generate-check`, `make license-check` are not exercised by this PR. No changie entry required — per AGENTS.md, internal CI changes do not need changelog entries.
- **REQ-NF-002**: Workflow YAML must remain valid (parseable by GitHub Actions). Comment style and indentation in the new step matches the surrounding YAML.
- **REQ-NF-003**: The pinned action ref (`anthropics/claude-code-action@939ae9c…`) does not change.

## Technical Considerations

- **Why the SSH clone happens at all**: `claude-code-action`'s plugin installer treats a `source: github` entry as a git clone target and shells out to `git clone`. Git's default protocol for `git@github.com:` URLs is SSH. The runner inherits no SSH key from the workflow's secrets, so the clone fails immediately. The fix exploits git's URL-rewrite config so the cloner's SSH URL is silently transformed to HTTPS before the system git binary executes. Public-repo HTTPS needs no credentials.
- **Why the rename matters**: The GitHub check name surfaced on a PR is derived from `<job-id>` (rendered as the check's `name`), with the `workflowName` showing the workflow's `name:` value. Two workflows with the same job id produce two checks that share a name — branch protection rules that require "the security check" by name match both, which is confusing and brittle.
- **First-add limitation does not apply here**: PR #129 noted the action's own protection skipping execution on first add. That protection fires when the workflow file is new on the base branch. The workflow files are now on `main`, so a PR that modifies them runs the head-version workflow. The fix is exercisable on the fix PR's own check run.
- **Fallback design**: REQ-F-004 exists because there is residual uncertainty about whether `git config --global url.X.insteadOf Y` survives across all subprocesses the action spawns (it should, but the action's runtime is not fully transparent). Switching `marketplace.json` to `source: git` with an explicit https URL removes the SSH-default behaviour at the source, sidestepping the cloner's default protocol entirely.
- **Workflow files**: full pre-state visible at `git show origin/main:.github/workflows/claude-review-security.yml` and `git show origin/main:.github/workflows/claude-review-conventions.yml`. Marketplace at `git show origin/main:.claude-plugin/marketplace.json`.

## Acceptance Criteria

- [ ] Branch `oskar/cli-138-fix-claude-review-workflows` exists off `main` with the changes from REQ-F-001 through REQ-F-003.
- [ ] PR is opened with title roughly `ci: fix Claude Review SSH plugin clone + dedupe job names (CLI-138)`.
- [ ] On the fix PR's own check runs, `Claude Review — Security` reaches the `Enforce verdict` step. Logs show `Adding marketplace: ./` → `Installing plugin: cc-skills-golang@neo4j-cli-review` → a successful install line (no `Permission denied (publickey)`).
- [ ] On the fix PR's own check runs, the conventions check appears under the GitHub check name `conventions-review` (verifiable via `gh pr checks <num> --json name,workflow`), not `security-review`. Both check names are distinct.
- [ ] If REQ-F-001 alone does not satisfy the previous criterion, REQ-F-004 fallback is applied in the same PR and re-verified.
- [ ] Final state on the fix PR: both Claude Review checks reach a verdict (pass or fail) without infrastructure errors. A passing-verdict run is not required for merge — what matters is that the workflow no longer dies during plugin install or duplicates a check name.
- [ ] PR description spells out the two defects, the fix for each, and links CLI-138.
- [ ] PR is merged after the two acceptance check runs go green at the verdict level.

## Out of Scope

- Branch-protection rule updates to require the renamed `conventions-review` check.
- Any change to `.github/prompts/claude-review-*.md`.
- Vendoring `cc-skills-golang` into the repo as a `source: local` plugin.
- Re-tuning the `Bash(tee:*)` allowlist or other tool gates in either workflow.
- Adding a CI gate that fails if both Claude Review workflows ever share a job id again.
- Audit of why the conventions check produced `verdict: 'pass'` on PR #120 — separate question.

## Open Questions

None. Resolved during pre-PRD Q&A:

- Fallback (`source: git` + https url) is in-PRD as REQ-F-004, not a separate ticket.
- No changie entry — pure CI plumbing per AGENTS.md.
