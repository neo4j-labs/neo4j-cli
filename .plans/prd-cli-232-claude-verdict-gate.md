# PRD: Make the Claude review verdict gate robust (CLI-232)

## Overview

The `Enforce verdict` step in `.github/workflows/claude-review-security.yml` failed
[PR #239](https://github.com/neo4j-labs/neo4j-cli/actions/runs/31708115923/job/94473741612)
on a review that found **nothing**: zero inline comments
(`pulls/239/comments` → `0`), summary ending `**Security review:** no issues found`.

Both verdict mechanisms are model-self-reported and neither is verifiable by the harness:

1. **`/tmp/claude-verdict.txt`** — `claude-review-security.md` Step 4 asks for it as the
   model's *final* action. The model posted its summary and stopped. File never written.
   The workflow's own comment at `:92-93` already concedes this happens.
2. **Emoji scrape fallback** (`:114-132`) — greps the claude[bot] comment for `⚠️` then
   `✅`. The mandated `✅` was absent from otherwise-correct prose. Both paths yielded
   nothing → **red required check on a passing review**.

Three further defects in that fallback, all confirmed by reading the code:

- **Whole-body grep.** `echo "${latest}" | grep -qF '⚠️'` searches the entire comment. A
  review whose analysis prose merely *mentions* `⚠️` is classified fail.
- **Fail-first ordering on the whole body.** `⚠️` is tested before `✅`, so the above
  misclassification silently wins over a correct trailing `✅` line.
- **No pagination.** `gh api ".../issues/${PR_NUMBER}/comments"` uses the default
  `per_page=30`. On a PR with >30 conversation comments the claude[bot] summary falls to
  page 2 and the fallback sees nothing → fail-closed on a clean review. Latent today.

`claude-review-conventions.yml:62-112` carries the identical copy-pasted block and has
every one of these bugs. It survived #239 only because its verdict file happened to be
written.

**Outcome:** a clean review with imperfect prose passes; a genuinely missing or
unparseable verdict still fails closed; the gate lives in one tested place instead of two
copies that can drift.

### Decision: summary-line parse, not inline-comment count

CLI-232's description proposes deriving the verdict from `pulls/N/comments` (pass iff
zero). Checked against the actual prompts — it does not work here:

- **Unattributable.** Both reviews post inline comments as the same `claude[bot]` user, on
  the same PR, on the same `pull_request` triggers, concurrently. The mandated body shapes
  — `**[Severity] Category:**` (security prompt `:26`) vs `**[Severity] Rule:**`
  (conventions prompt `:36`) — are prose conventions, not contractual discriminators. No
  per-comment attribution marker exists.
- **Stale.** Inline comments persist across pushes, so a count needs sha/time scoping —
  and both workflows share `head.sha` anyway, so `?commit_id=` does not disambiguate them.
- **Wrong failure direction.** A missed/mis-attributed marker *undercounts* → **false pass
  on a security check**. The summary-line parse fails **closed**. For a required check that
  asymmetry outweighs the elegance of "ground truth".

Preserved from the current design: fail-closed on a genuinely absent verdict. The problem
this PRD fixes is that "clean review, imperfect prose" is currently indistinguishable from
"review never ran".

### Decision: machine-readable marker, with phrase matching as tolerance

Rather than make the gate depend on either an emoji or a prose phrase, the prompts gain a
mandated HTML comment that renders invisibly on GitHub:

```
**Security review:** ✅ no issues found
<!-- claude-verdict: pass -->
```

The gate reads that exact token first and only falls back to phrase/emoji matching when
it is absent. Four tiers, most-exact to most-tolerant, fail-closed at the end.

## Goals

- A review that posts zero inline comments and states so in its summary **passes**,
  regardless of whether it emitted `✅` or wrote the verdict file.
- A missing, absent, or unparseable verdict still **fails** the check (fail-closed
  preserved).
- Analysis prose containing `⚠️` cannot flip a passing review to fail.
- A claude[bot] summary beyond the first API page is still found.
- One implementation, shared by both workflows, with behavioural tests that lock the
  #239 regression.

## Non-Goals

- Deriving the verdict from inline-comment count — rejected above.
- Changing what the reviews *analyse*, their severity rubrics, or the inline-comment body
  format.
- Touching `claude-review-oplane.yml` (advisory, `continue-on-error: true`, no verdict
  gate) or `claude.yml`.
- Making the verdict-file write mechanically enforceable — it remains an LLM instruction;
  the fallback is the safety net.
- Adding per-inline-comment attribution markers.
- Widening `shellcheck.yml`'s `scandir` — its header comment states the narrow scope is
  deliberate.

## Requirements

### Functional Requirements

**The shared script**

- REQ-F-001: Add `.github/scripts/enforce-claude-verdict.sh`, executable (`chmod +x`,
  committed as mode 100755), `#!/usr/bin/env bash`, `set -euo pipefail`, and
  `export LC_ALL=C.UTF-8` so multibyte `grep -F '⚠'` is locale-independent on the runner.

- REQ-F-002: Inputs are **env vars only**, no positional args: `REVIEW_KIND` (e.g.
  `Security review`), `PR_NUMBER`, `GITHUB_REPOSITORY`, `GH_TOKEN`, and optional
  `VERDICT_FILE` (default `/tmp/claude-verdict.txt`). Validate each required var is set
  and non-empty, and that `gh` and `jq` are on `PATH`; on any failure print a message
  naming the missing item and exit 1.

- REQ-F-003: **Tier 1 — verdict file.** If `VERDICT_FILE` exists and is non-empty, read it
  and strip all whitespace (`tr -d '[:space:]'`). Behaviour unchanged from today.

- REQ-F-004: **Comment fetch.** When tier 1 yields nothing, fetch the latest claude[bot]
  comment carrying the kind marker:

  ```bash
  body="$(gh api "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments?per_page=100" \
            --paginate --slurp \
          | jq -r --arg marker "**${REVIEW_KIND}:**" '
              [ .[][] | select(.user.login == "claude[bot]"
                               and (.body | contains($marker))) ]
              | sort_by(.created_at) | last | .body // ""')"
  ```

  Three points, each verified against the live API and `gh api --help`:
  - `gh api` has **no `--arg` flag** (only `-q/--jq`), so the marker must be passed to a
    real `jq` process via a pipe. Do **not** shell-interpolate `REVIEW_KIND` into a jq
    filter string — that is a quoting/injection hazard.
  - `--paginate --slurp` yields an array-of-pages (`[[...],[...]]`), hence `.[][]` to
    flatten. Without `--slurp`, `--jq` runs per page and a per-page `| last` silently
    drops earlier pages.
  - `per_page=100` plus `--paginate` covers arbitrarily long comment threads.

  Verified end-to-end against PR #239: the pipeline above returns
  `**Security review:** ✅ no issues found`.

- REQ-F-005: **Tier 2 — machine-readable marker.** Search the fetched body (CR-stripped)
  for a line matching `<!-- claude-verdict: pass -->` or `<!-- claude-verdict: fail -->`;
  take the **last** such line. Match case-insensitively and tolerate variable internal
  whitespace. This tier is exact — if it matches, classification stops here.

- REQ-F-006: **Tier 3 — marker-line phrase match.** Absent tier 2, isolate the **last line
  beginning with `**<REVIEW_KIND>:**`** and classify **only that line**:
  - fail if it contains `⚠` **or** `flagged inline`
  - else pass if it contains `no issues found`, `no security-relevant changes`,
    `no convention-relevant changes`, or `✅`
  - else leave unclassified

  Fail indicators are tested first so a line containing both is fail-closed. This must be
  a single-line classification — the current whole-body grep is the defect being fixed.

- REQ-F-007: **Tier 4 — fail-closed.** If no tier classified, print the offending line
  (or state that no marker line / no claude[bot] comment was found), emit a GitHub
  `::error::` annotation, and exit 1.

- REQ-F-008: Exit 0 iff the resolved verdict is exactly `pass`; otherwise `::error::` +
  exit 1. Log which tier produced the verdict on every path, so a future failure is
  diagnosable from the job log alone.

- REQ-F-009: Body handling must be safe against adversarial/odd content: `printf '%s\n'`
  never `echo` (leading `-`, backslash escapes); `tr -d '\r'` before any matching;
  `grep -F` with `REVIEW_KIND` as a **fixed string** then a separate prefix-position check
  — never build a regex out of `REVIEW_KIND`, whose literal `**` are regex metacharacters.
  A comment body is attacker-influenceable (anyone can comment on a PR), so the script
  must never `eval` or word-split it.

**Workflow wiring**

- REQ-F-010: Replace `claude-review-security.yml:88-138` with a step invoking
  `.github/scripts/enforce-claude-verdict.sh`, keeping its existing `env:` block
  (`GH_TOKEN`, `PR_NUMBER`, `REVIEW_KIND: "Security review"`). The long explanatory
  comments move into the script header — the workflow step keeps a one-line pointer.

- REQ-F-011: Same for `claude-review-conventions.yml:62-112` with
  `REVIEW_KIND: "Conventions review"`. Both workflows already allow `Bash(gh api:*)` in
  `claude_args`, so no allowlist change is needed.

**Prompt changes**

- REQ-F-012: In `.github/prompts/claude-review-security.md`, move the verdict write
  (Step 4) to **before** the summary post (Step 3), and make the same reordering in the
  Step 2 no-op path (currently: comment, then verdict) and Step 5 skill-unavailable path.
  The observed failure was exactly "posted summary, then stopped".

- REQ-F-013: Same reordering in `.github/prompts/claude-review-conventions.md` — step 7
  (verdict) moves before steps 5–6 (compose + post summary), and the step 2 no-op path is
  reordered likewise.

- REQ-F-014: Both prompts mandate the machine-readable marker as the final line of the
  summary body, immediately after the `**<Kind>:**` line:
  `<!-- claude-verdict: pass -->` or `<!-- claude-verdict: fail -->`. State that it renders
  invisibly on GitHub and that CI reads it. Update the summary-shape code fence in each
  prompt to show it.

- REQ-F-015: Both prompts state that the `**<Kind>:**` line's wording is what CI parses
  when the marker is absent, and that `no issues found` / `flagged inline` are the load-
  bearing phrases (the emoji is cosmetic). This removes the implicit "emoji is mandatory"
  reading that caused #239.

**Tests**

- REQ-F-016: Add `.github/scripts/tests/enforce-claude-verdict.bats` mirroring
  `distribution/installation-scripts/tests/install-neo4j-cli.bats`: Neo4j Apache-2.0
  header, `bats_require_minimum_version 1.5.0`, per-test `mktemp -d`, and a `gh` stub in a
  `STUBS_DIR` prepended to `PATH`. Stub `gh`, not an internal function — that keeps the
  real `jq` filter, the `--slurp` flattening, and pagination under test.

- REQ-F-017: Cases (each asserting exit status and, where noted, log content):

  | # | Scenario | Expect |
  |---|---|---|
  | 1 | verdict file `pass` | 0; log names tier 1 |
  | 2 | verdict file `fail` | 1 |
  | 3 | no file, marker line `**Security review:** no issues found` | **0** ← the #239 regression |
  | 4 | no file, `… ✅ no issues found` | 0 |
  | 5 | no file, `… ⚠️ 3 issue(s) flagged inline` | 1 |
  | 6 | no file, `… no security-relevant changes` (no-op path) | 0 |
  | 7 | analysis prose contains ⚠️, marker line clean | **0** ← whole-body-grep defect |
  | 8 | `<!-- claude-verdict: pass -->` present, marker line garbled | 0 ← tier 2 wins |
  | 9 | `<!-- claude-verdict: fail -->` present, marker line says "no issues found" | 1 ← tier 2 wins |
  | 10 | no file, marker line absent/garbled, no HTML marker | 1 (fail-closed) |
  | 11 | no claude[bot] comment at all | 1 (fail-closed) |
  | 12 | marker comment only on page 2 of a paginated response | 0 |
  | 13 | CRLF body | 0 |
  | 14 | two claude[bot] summaries; newest (by `created_at`) is fail | 1 |
  | 15 | body line `**Security review:** ✅ ⚠️ mixed` | 1 (fail-first) |
  | 16 | missing `GH_TOKEN` | 1; message names the var |
  | 17 | missing `PR_NUMBER` | 1; message names the var |
  | 18 | `REVIEW_KIND: "Conventions review"` end-to-end | 0 |

- REQ-F-018: One case replays the real #239 body verbatim with the `✅` stripped, asserting
  exit 0. Capture it as a fixture under `.github/scripts/tests/fixtures/` — source:
  `gh api repos/neo4j-labs/neo4j-cli/issues/comments/5281539476 --jq .body`. Do not have
  the test hit the network.

**CI + hygiene**

- REQ-F-019: Add `.github/workflows/ci-scripts-tests.yml` — one `ubuntu-latest` job,
  `push`/`pull_request` `paths`-scoped to `.github/scripts/**` and the workflow's own file,
  `permissions: contents: read`, running (a) `bats .github/scripts/tests/` and
  (b) `ludeeus/action-shellcheck` with `scandir: .github/scripts`, `severity: warning`.
  Reuse the exact pinned SHAs already in `installer-tests.yml` / `shellcheck.yml`
  (`actions/checkout@de0fac2e…`, `ludeeus/action-shellcheck@00cae500…`). Install bats via
  `sudo npm install -g bats`, matching `installer-tests.yml:25`.

  Ubuntu-only: the script only ever executes on `ubuntu-latest` in the review workflows.

- REQ-F-020: Add a `test-ci-scripts` Makefile target (`bats .github/scripts/tests/`) with
  a `##` doc comment and a `REQUIRES: bats-core >= 1.5.0` note, matching the
  `test-installer-sh` style, and add it to `.PHONY`.

- REQ-F-021: Add to `.gitattributes`, under a new comment block explaining why:
  ```
  .github/scripts/**/*.sh text eol=lf
  .github/scripts/**/*.bats text eol=lf
  ```
  Existing rules only cover `distribution/**`. A CRLF shebang makes the kernel look for an
  interpreter named `bash\r` and the script fails to execute.

- REQ-F-022: **No changelog entry.** CI-only, zero user-visible CLI change. The conventions
  prompt (`:13`) lists "CI/CD workflow fixes" as an explicit changelog exception.

### Non-Functional Requirements

- REQ-NF-001: Zero change to any Go source, the skill bundle, or `policy.golden`. `make
  test`, `make fmt-check`, `make lint`, and `make generate-check` must stay green and show
  no diff attributable to this work.

- REQ-NF-002: `shellcheck -S warning` clean on the new script and the bats file.

- REQ-NF-003: The script must remain portable enough to run under BSD userland — the
  primary developer machine is macOS and `make test-ci-scripts` runs there even though CI
  is ubuntu-only. Avoid GNU-only `grep`/`sed` flags (`-P`, `sed -i` without a suffix arg,
  `grep -o` with PCRE).

- REQ-NF-004: No secrets in the script's output. It handles `GH_TOKEN` but must never echo
  it; only pass it to `gh` via the inherited environment, never on a command line.

- REQ-NF-005: The gate must not become a new flake source: any `gh api` failure (network,
  rate limit, 404) must produce a clear message and exit 1 — fail-closed — rather than an
  unhandled `set -e` abort with no explanation. `--paginate` on a large thread must not
  hang indefinitely; rely on `gh`'s own timeouts.

## Technical Considerations

**Why four tiers rather than two.** Each tier trades exactness for tolerance. Tier 1 (file)
is what the model is *told* to do. Tier 2 (HTML comment) is exact but still model-emitted,
so it is not a strict improvement in reliability — only in parse determinism. Tier 3
(phrase) tolerates prose drift, which is the specific #239 failure. Tier 4 preserves the
fail-closed default. Collapsing tiers 2 and 3 would reintroduce coupling to prose wording;
dropping tier 3 would mean any prompt-following miss on the new marker reproduces #239.

**`gh api` flag correctness.** The natural-looking `gh api --arg marker …` does not exist —
`gh api` exposes only `-q/--jq`. The marker must reach a standalone `jq`. This was verified
against `gh api --help` and by running the full pipeline against PR #239.

**`--slurp` shape.** `gh api --paginate --slurp` returns `[[page1…],[page2…]]`, confirmed
by probing the live endpoint. The jq filter must flatten with `.[][]`. This is the reason
the naive `--jq 'map(...) | last'` in the current workflows cannot simply gain
`--paginate`: jq would run once per page and the final `last` would reflect only the last
page.

**Untrusted input.** PR comment bodies are attacker-influenceable — any GitHub user who can
comment on a PR contributes to the fetched corpus. The `.user.login == "claude[bot]"`
filter narrows it, but the script must still treat the body as data: no `eval`, no
unquoted expansion, no regex built from it. Note this is a *classification* input only —
the worst outcome of a crafted body is a wrong verdict, and the ordering (fail-first,
fail-closed) means crafted content can force a fail, not a pass. A third party cannot forge
`claude[bot]` authorship.

**Prompt reordering is probabilistic, not a fix.** Moving the verdict write earlier reduces
the odds of reaching the fallback but cannot eliminate them — the model can stop, hit a
tool error, or run out of context at any point. The fallback stays load-bearing; this is
why the tests weight it, not tier 1.

**Why a separate workflow rather than a step in `installer-tests.yml`.** That workflow's
identity is distribution-channel installers; its jobs are unpathed and run on every PR.
A `paths`-scoped workflow keeps CI-script tests off unrelated PRs and keeps the two
concerns independently readable. `.github/scripts/` currently has zero test or shellcheck
coverage (`version-to-pep440.sh`, `smoke-test-wheel.sh`) — this establishes it.

**Precedent for the bats stub pattern.** `install-neo4j-cli.bats` already stubs every
external command via `STUBS_DIR` on `PATH` and records invocations to a file. Reuse that
structure directly, including the license header and `mktemp -d` isolation.

## Acceptance Criteria

- [ ] `.github/scripts/enforce-claude-verdict.sh` exists, is committed executable
      (`git ls-files -s` shows mode `100755`), and passes
      `shellcheck -S warning`.
- [ ] `bats .github/scripts/tests/enforce-claude-verdict.bats` — all 18 cases pass, on both
      ubuntu CI and a local macOS run.
- [ ] Case 3 (marker line without `✅`) exits **0** — the direct CLI-232 regression lock.
- [ ] Case 7 (⚠️ in analysis prose, clean marker line) exits **0** — whole-body-grep defect.
- [ ] Case 12 (comment on page 2) exits **0** — pagination defect.
- [ ] Cases 10, 11, 16, 17 all exit 1 — fail-closed preserved.
- [ ] Replaying the real #239 body with `✅` stripped exits 0 (REQ-F-018 fixture).
- [ ] Neither `claude-review-security.yml` nor `claude-review-conventions.yml` contains an
      inline verdict shell block; both invoke the shared script.
      `grep -c "grep -qF" .github/workflows/claude-review-*.yml` → 0.
- [ ] Both prompts write the verdict file **before** posting the summary, on the main path
      and on every early-exit path.
- [ ] Both prompts mandate `<!-- claude-verdict: (pass|fail) -->` and show it in the
      summary-shape fence.
- [ ] `.github/workflows/ci-scripts-tests.yml` exists, ubuntu-only, `paths`-scoped, action
      SHAs identical to those already used elsewhere in `.github/workflows/`.
- [ ] `make test-ci-scripts` runs the suite; target is in `.PHONY`.
- [ ] `.gitattributes` pins `eol=lf` for `.github/scripts/**/*.{sh,bats}`.
- [ ] No new file under `.changes/unreleased/`.
- [ ] `git diff` shows **no** change to any `.go` file, any `bundle/**`, or
      `neo4j-cli/internal/subcommands/mcp/server/testdata/policy.golden`.
- [ ] `make test`, `make fmt-check`, `make lint`, `make generate-check` all pass.
- [ ] On the PR for this work, both `security-review` and `conventions-review` report
      green. (Note: a `.github/`-only diff takes the conventions no-op path and the
      security no-Go-changes path, both of which *do* write the verdict file — so this run
      exercises tier 1 only. Fallback confidence comes from the bats suite, not this run.)

## Out of Scope

- Inline-comment count as a verdict signal — rejected, see Overview.
- Per-inline-comment attribution markers (would be the prerequisite for the above).
- `claude-review-oplane.yml`, `claude.yml` — no verdict gate.
- Widening `shellcheck.yml`'s `scandir`, or adding coverage for the pre-existing
  `.github/scripts/version-to-pep440.sh` and `smoke-test-wheel.sh`. The new workflow's
  `scandir: .github/scripts` will incidentally lint them; if they emit warnings, fix or
  suppress narrowly rather than expanding this PRD's scope.
- Changing review scope, severity rubrics, or inline-comment body format.
- Making the verdict-file write mechanically enforceable.
- A Windows or macOS leg for the new CI job.

## Open Questions

- **Retiring tier 3 later.** Once the HTML marker (tier 2) has been observed emitting
  reliably over some weeks of PRs, the phrase-matching tier could be dropped for a simpler
  gate. Keep both until there is evidence; the phrase tier costs little and is exactly the
  tolerance #239 needed.
- **Pre-existing `.github/scripts/*.sh` shellcheck status is unknown.** Neither script has
  ever been linted. `scandir: .github/scripts` will cover them for the first time. If they
  produce `warning`-level findings the new job goes red on arrival; the implementer should
  run `shellcheck -S warning .github/scripts/*.sh` early and decide fix-vs-`# shellcheck
  disable` before wiring the workflow.
- **Should the gate cross-check tiers?** If both the verdict file and the HTML marker are
  present and *disagree*, the current design silently prefers the file. Surfacing that as a
  `::warning::` (without changing the verdict) would make prompt-following drift visible.
  Cheap to add; left out to keep tier logic simple. Worth revisiting if drift is observed.
