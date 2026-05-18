# CLI-138 Smoke-Test Runbook (Live PR)

Manual smoke test for the two new review workflows
(`claude-review-security.yml`, `claude-review-conventions.yml`). Run this
**after the PR introducing the workflows is merged to `main`** — the
workflows trigger from the version on `main`, not from the PR branch.

Branch under test: `oskar/cli-138-auto-ai-pr-reviews`.

All commands use `gh`. Replace `<PR>` with the actual PR number.

---

## Pre-flight

```
# Confirm the workflows are present on main
gh workflow list | grep -i "Claude Review"

# Expect two entries:
#   Claude Review — Security        active   <id>
#   Claude Review — Conventions     active   <id>
```

If either is missing, the merge didn't land — stop and investigate.

---

## Scenario 1 — Clean PR (green path)

Goal: both checks finish green within 5 min, each posts a top-level
summary ending with `✅ no issues found`.

Setup:

```
git checkout -b oskar/smoke-138-clean main
# Make a trivial doc-only change, e.g. fix a typo in README.md
git commit -am "docs: smoke test clean PR"
git push -u origin oskar/smoke-138-clean
gh pr create --fill --base main
PR=$(gh pr view --json number -q .number)
```

Verify:

```
# Wait up to 5 min, then list checks
gh pr checks $PR

# Expect both rows present and PASS:
#   Claude Review — Security      pass  ...
#   Claude Review — Conventions   pass  ...

# Confirm one summary comment per workflow
gh pr view $PR --comments | grep -E "Security review:|Conventions review:"
# Expect two lines, each ending with "✅ no issues found"
```

Pass criteria:
- Both checks green within 5 min wall-clock.
- One top-level comment per workflow, each ending with the green
  verdict line.
- Zero inline review comments.

Fail criteria:
- Either check red.
- Workflow didn't run at all (check `gh run list --workflow=claude-review-security.yml`).
- Run took > 5 min (note REQ-NF-004 but don't fail the gate; capture
  for follow-up).

---

## Scenario 2 — Draft PR (skip until ready)

Goal: neither workflow runs while PR is draft; both run on
`ready_for_review`.

Setup:

```
git checkout -b oskar/smoke-138-draft main
git commit --allow-empty -m "docs: smoke test draft skip"
git push -u origin oskar/smoke-138-draft
gh pr create --fill --draft --base main
PR=$(gh pr view --json number -q .number)
```

Verify (draft state):

```
sleep 60
gh pr checks $PR
# Expect neither "Claude Review — Security" nor "— Conventions" present.

gh run list --workflow=claude-review-security.yml --branch oskar/smoke-138-draft
# Expect "no runs found" or empty.
```

Now flip to ready:

```
gh pr ready $PR
sleep 120
gh pr checks $PR
# Expect both rows present and PASS.
```

Pass criteria:
- During draft state: zero rows for either review workflow.
- After `gh pr ready`: both workflows trigger and complete.

Fail criteria:
- Any review workflow run shows up while PR is still in draft state.

---

## Scenario 3 — Non-member author / bot (skip entirely)

Goal: PR opened by an author NOT in `MEMBER/OWNER/COLLABORATOR` results
in zero billable runs.

This is hardest to simulate cleanly without a second account. Two
options:

**Option A (recommended) — wait for organic bot PR.** Watch the Actions
tab for the next Renovate or Dependabot PR. Verify:

```
gh pr list --author "app/renovate" --limit 1
PR=<that number>
gh pr checks $PR
# Expect NEITHER review workflow row.

gh run list --workflow=claude-review-security.yml --branch <bot branch>
# Expect empty.
```

**Option B — fork-based PR from a personal account that is not a
neo4j-labs org member.** Same verification as Option A.

Pass criteria:
- `gh pr checks` shows no rows for either review workflow.
- Actions tab shows zero workflow runs against that PR's head SHA for
  either workflow ID.

Fail criteria:
- Either workflow triggered (would burn OAuth tokens on outside PRs).

---

## Scenario 4 — Convention violation (conventions check red)

Goal: introduce a deliberate AGENTS.md violation, observe conventions
check red with an explanatory inline comment.

Setup — choose ONE of the cheap violations below (do not commit any of
these to `main`; smoke branch only, close PR without merge):

- Add a new leaf cobra command file under
  `neo4j-cli/internal/subcommands/<resource>/foo.go` with NO `Example:`
  field on the `cobra.Command`. (Will fail
  `TestAllLeafCommands_HaveExamples` locally too, but workflow should
  flag it inline before tests run.)
- Add a new `.go` file with NO Neo4j copyright header.
- Change a `Short:` string on an existing command without running
  `go generate ./neo4j-cli/internal/skill/...` (stale bundle).

```
git checkout -b oskar/smoke-138-conventions-violation main
# Make ONE of the violations above
git commit -am "feat: smoke test convention violation"
git push -u origin oskar/smoke-138-conventions-violation
gh pr create --fill --base main
PR=$(gh pr view --json number -q .number)
```

Verify:

```
sleep 180
gh pr checks $PR
# Expect:
#   Claude Review — Conventions   fail  ...
#   Claude Review — Security      pass  ...  (no Go security issue introduced)

gh api repos/:owner/:repo/pulls/$PR/comments --jq '.[] | {path, line, body}'
# Expect at least one inline comment naming the violated rule
# (e.g. "Example: field required" or "license header missing").
```

Pass criteria:
- Conventions check red.
- At least one inline comment on the offending line naming the AGENTS.md
  rule.
- Security check stays green (the violation is convention-only).

Fail criteria:
- Conventions check green (false negative).
- No inline comment posted (only top-level summary).
- Security check also red (cross-contamination).

Cleanup: `gh pr close $PR --delete-branch` (do NOT merge).

---

## Scenario 5 — Security smell (security check red)

Goal: introduce a deliberate security smell, observe security check
red with an explanatory inline comment.

Setup — pick ONE cheap smell (do not commit to `main`):

- In any test file, add `exec.Command("sh", "-c", os.Getenv("FOO"))`.
- In any test file, add `_ = math.Rand()` used as a "token" (with a
  comment claiming it's a token).
- Add an HTTP client with `Transport: &http.Transport{TLSClientConfig:
  &tls.Config{InsecureSkipVerify: true}}`.

```
git checkout -b oskar/smoke-138-security-smell main
# Add one of the smells above to a test file (not production code)
git commit -am "test: smoke test security smell"
git push -u origin oskar/smoke-138-security-smell
gh pr create --fill --base main
PR=$(gh pr view --json number -q .number)
```

Verify:

```
sleep 180
gh pr checks $PR
# Expect:
#   Claude Review — Security      fail  ...
#   Claude Review — Conventions   pass  ...  (file is well-formed)

gh api repos/:owner/:repo/pulls/$PR/comments --jq '.[] | {path, line, body}'
# Expect at least one inline comment naming the security category
# (e.g. "Command injection" or "Cryptography").
```

Pass criteria:
- Security check red.
- Inline comment on the smell line, severity-prefixed.
- Conventions check green.

Fail criteria:
- Security check green (false negative on an obvious smell).
- Conventions check red (cross-contamination).

Cleanup: `gh pr close $PR --delete-branch`.

---

## Scenario 6 — Second push produces a NEW review comment

Goal: each push to a PR produces a fresh top-level summary (not an
edit of the prior one — per PRD "Non-Goals").

Reuse the Scenario 1 PR (or any clean PR):

```
PR=<from scenario 1>
git checkout oskar/smoke-138-clean
git commit --allow-empty -m "docs: trigger second review"
git push
sleep 180

# Count summary comments from each workflow
gh pr view $PR --json comments --jq \
  '.comments[] | select(.body | startswith("**Security review")) | .body' | wc -l
# Expect: 2

gh pr view $PR --json comments --jq \
  '.comments[] | select(.body | startswith("**Conventions review")) | .body' | wc -l
# Expect: 2
```

Pass criteria:
- After second push, two distinct summary comments per workflow on the
  PR timeline.

Fail criteria:
- Only one summary comment per workflow (would mean the action is
  editing rather than appending).

---

## Scenario 7 — `@claude` mention still works

Goal: existing `claude.yml` mention behaviour is preserved.

```
PR=<from scenario 1 or a new PR>
gh pr comment $PR --body "@claude please summarise this PR"
sleep 60
gh run list --workflow=claude.yml --branch oskar/smoke-138-clean --limit 1
# Expect one run, status completed, conclusion success.
```

Pass criteria:
- `claude.yml` triggers on the `@claude` comment.
- The two review workflows do NOT also re-trigger from the comment
  (they only fire on PR events, not comments).

Fail criteria:
- `claude.yml` didn't run (mention coexistence regression).
- Review workflows re-ran from the comment event (over-triggering).

---

## Scenario 8 — Plugin install succeeds + skill is invoked

Goal: on a clean Go PR, the security workflow's action step installs the
vendored `cc-skills-golang` marketplace plugin, and the prompt invokes
the `golang-security` skill (rather than falling back to a hand-rolled
review). Confirms REQ-F-015 (install lines visible in logs) and REQ-F-018
(skill invoked, review mode).

Setup:

```
git checkout -b oskar/smoke-138-skill-success main
# Touch a trivial Go file so the prompt's non-Go no-op path does NOT
# fire. A whitespace-only edit to a comment is enough; reverting it in
# a follow-up commit keeps the smoke branch clean.
$EDITOR neo4j-cli/aura/aura.go  # add then remove a trailing space, or
                                # tweak a doc comment
git commit -am "chore: smoke test skill invocation (no-op Go edit)"
git push -u origin oskar/smoke-138-skill-success
gh pr create --fill --base main
PR=$(gh pr view --json number -q .number)
```

Verify:

```
# Wait for the security workflow to finish, grab the run id
sleep 240
RUN=$(gh run list --workflow=claude-review-security.yml \
        --branch oskar/smoke-138-skill-success \
        --limit 1 --json databaseId -q '.[0].databaseId')

# (a) Action installer logs include the marketplace + plugin lines
gh run view "$RUN" --log | grep -E "Adding marketplace: \./|Installing plugin: cc-skills-golang@neo4j-cli-review"
# Expect BOTH lines to appear (one each). If either is missing the
# vendored install is broken — treat as fail even if the check is green.

# (b) Prompt actually invoked the Skill tool. Two signals; either is enough.
#     Signal 1: top-level summary comment mentions the skill by name.
gh pr view $PR --json comments \
  --jq '.comments[] | select(.body | startswith("**Security review")) | .body' \
  | grep -iE "golang-security|skill"
# Expect a non-empty match.

#     Signal 2: the run transcript / step output references a Skill tool call.
gh run view "$RUN" --log | grep -iE "Skill\(|tool: Skill|invoking skill: golang-security"
# Expect at least one hit.

# (c) Verdict file written and check is green (clean Go change has no findings)
gh pr checks $PR | grep "Claude Review — Security"
# Expect "pass".
```

Pass criteria:
- Both installer log lines present (`Adding marketplace: ./` AND
  `Installing plugin: cc-skills-golang@neo4j-cli-review`).
- Summary comment mentions `golang-security` (or `skill`) OR the run
  log shows a Skill tool invocation.
- Security check green.

Fail criteria:
- Either installer log line missing — vendored marketplace not picked
  up; the workflow ran with no plugin (silent regression).
- Summary mentions findings the hand-rolled prompt would have produced
  but no Skill invocation in the transcript — falling back to hand-rolled
  checks (violates REQ-F-019).
- Security check red on a clean PR with no smells.

Cleanup: `gh pr close $PR --delete-branch` (do NOT merge).

---

## Scenario 9 — Pin is broken (skill unavailable turns check red)

Goal: a broken plugin pin must produce a loud red check, NOT a silent
green. Confirms REQ-F-019 (no fallback to hand-rolled checks when the
skill is unavailable).

**Do NOT merge this scenario's branch.** It deliberately edits
`.claude-plugin/marketplace.json` to point at a non-existent commit.
Revert the edit before opening any other PR off the same branch.

Setup:

```
git checkout -b oskar/smoke-138-pin-broken main

# Edit .claude-plugin/marketplace.json — change the `sha` field to a
# value that does not exist on samber/cc-skills-golang. All-zeros is
# the canonical "obviously fake" SHA.
#
#   "sha": "e9761db859c6969b77a8fd0e8a243f4f28240211"
# becomes
#   "sha": "0000000000000000000000000000000000000000"
$EDITOR .claude-plugin/marketplace.json

git commit -am "test: break plugin pin SHA (smoke scenario 9, do NOT merge)"
git push -u origin oskar/smoke-138-pin-broken
gh pr create --fill --base main \
  --title "DO NOT MERGE — smoke test pin breakage" \
  --body "Smoke scenario 9 from .plans/smoke-test-cli-138.md. Close without merging."
PR=$(gh pr view --json number -q .number)
```

Verify:

```
sleep 240
gh pr checks $PR | grep "Claude Review — Security"
# Expect "fail".

# Top-level comment names the failure mode
gh pr view $PR --json comments \
  --jq '.comments[] | select(.body | startswith("**Security review") or
                                  contains("skill unavailable") or
                                  contains("pin may be broken")) | .body'
# Expect at least one comment whose body contains language like
# "golang-security skill unavailable" or "pin may be broken" or the
# explicit ⚠️ failure-mode verdict line.

# Run log should show the marketplace install step failing
RUN=$(gh run list --workflow=claude-review-security.yml \
        --branch oskar/smoke-138-pin-broken --limit 1 \
        --json databaseId -q '.[0].databaseId')
gh run view "$RUN" --log | grep -iE "failed to install|sha .* not found|0000000000000000000000000000000000000000"
# Expect a non-empty match — the action either refused the install
# or fetched and failed to resolve the SHA.
```

Pass criteria:
- Security check **red** (verdict file = `fail` OR missing → enforce
  step exits non-zero).
- Top-level comment explicitly names the failure mode (skill
  unavailable / pin broken). The check title alone is not enough.
- No silent fallback: the comment must NOT read like a normal "no
  issues found" green review.

Fail criteria:
- Security check green — the workflow fell back to hand-rolled checks
  or the verdict file was written `pass` despite the install failure.
  This is the regression REQ-F-019 exists to prevent.
- Comment posted but generic (no mention of skill / pin / unavailability)
  — operator cannot diagnose without digging into run logs.

Cleanup (**important — revert before any further work**):

```
gh pr close $PR --delete-branch
# Locally, the broken-SHA edit only lives on the throwaway branch; the
# delete-branch flag above also removes it from origin. Verify
# .claude-plugin/marketplace.json on main still has the real SHA:
git show main:.claude-plugin/marketplace.json | grep sha
# Expect: "sha": "e9761db859c6969b77a8fd0e8a243f4f28240211"
```

---

## Cleanup

```
# Close all smoke PRs without merging (or close + delete branches)
for PR in <list of smoke PR numbers>; do
  gh pr close $PR --delete-branch
done
```

---

## Rollback (if either workflow misbehaves on real PRs)

Two options, in order of reversibility:

**1. Disable via `if: false` (preserves file, fastest)**

```
# On main, edit the offending workflow
gh workflow disable "Claude Review — Security"
# OR
gh workflow disable "Claude Review — Conventions"
```

`gh workflow disable` flips the workflow to disabled state in the
Actions tab without modifying source. Re-enable with `gh workflow
enable`.

**2. Delete the workflow file (hard rollback)**

```
git checkout main
git pull
git checkout -b oskar/cli-138-rollback
git rm .github/workflows/claude-review-security.yml
# or .github/workflows/claude-review-conventions.yml
git commit -m "revert(ci): disable Claude review workflow (CLI-138)"
gh pr create --fill --base main
```

Merge to main to fully remove. Both workflows are independent — you
can drop just one and keep the other.

**3. Force-skip via `if: false`**

If you want to keep the file in tree but disabled:

```yaml
jobs:
  security-review:
    if: false
    # ... rest unchanged
```

This is the most discoverable rollback (visible in the file diff) but
requires a PR round-trip.

---

## Acceptance Criteria Mapping

This runbook covers task-005 + the post-extension PRD acceptance criteria
introduced by the vendored-skill work (task-007 through task-010). Not
every PRD AC has a runbook row — only those observable at runtime via
`gh` / Actions logs are listed. Static AC (file exists, YAML parses,
SHA matches) are checked at task-completion time by reading the diff,
not in this runbook.

| AC | Scenario |
| -- | -------- |
| Clean PR shows both checks green | 1 |
| Draft skip + ready trigger | 2 |
| Bot / non-member skip | 3 |
| Convention-violation flips red | 4 |
| Security-smell flips red | 5 |
| Second push = second comment | 6 |
| `@claude` mention still works | 7 |
| Action logs show `Adding marketplace: ./` + `Installing plugin: cc-skills-golang@neo4j-cli-review` (REQ-F-015) | 8 |
| Prompt invokes `golang-security` skill in review mode (REQ-F-018) | 8 |
| `setup-go` + `govulncheck install` steps run before the action (REQ-F-016) | 8 (visible in `gh run view` step list) |
| `--allowed-tools` includes `Skill` and `Bash(govulncheck:*)` (REQ-F-017) | 8 (visible in the action step's expanded `claude_args`) |
| Broken pin turns security check red with named failure (REQ-F-019) | 9 |
| `marketplace.json` correctly pinned (ref + sha) | static — checked in task-007 diff, exercised end-to-end in 8 + 9 |
| `make test`/`fmt-check`/`lint` pass | (done locally, see progress log) |
