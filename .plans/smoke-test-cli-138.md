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

## Scenario 3 — Non-allowlisted author / bot (skip entirely)

Goal: PR opened by an author NOT in `MEMBER/OWNER/COLLABORATOR` and NOT
in the inline allowlist results in zero billable runs.

This is hardest to simulate cleanly without a second account. Three
options, easiest first:

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

**Option B — fork-based PR from a personal account not in the allowlist.**
Push a branch from a personal account that is not an org member and not
in the inline allowlist. Same verification as Option A.

**Option C — temporarily remove yourself from the allowlist** in a test
PR (do NOT merge). Open a draft, flip to ready, observe skip. Revert
the allowlist change before closing the PR. This is risky — prefer A
or B.

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

This runbook covers all eight task-005 acceptance criteria:

| AC | Scenario |
| -- | -------- |
| Clean PR shows both checks green | 1 |
| Draft skip + ready trigger | 2 |
| Bot / non-allowlisted skip | 3 |
| Convention-violation flips red | 4 |
| Security-smell flips red | 5 |
| Second push = second comment | 6 |
| `@claude` mention still works | 7 |
| `make test`/`fmt-check`/`lint` pass | (done locally, see progress log) |
