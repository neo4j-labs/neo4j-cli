# PRD: Post release notes to feature-labs Slack (CLI-147)

Linear: [CLI-147](https://linear.app/neo4j/issue/CLI-147/post-releases-and-changelog-to-feature-labs-neo4j-cli).

## Overview

Today, after each `neo4j-cli` release, `.github/workflows/release.yml` runs GoReleaser and the GitHub release body is just the raw `.changes/neo4j-cli/v{VERSION}.md` changelog wrapped in `## Versions` / `## Changes`. Nothing is posted to `#feature-labs-neo4j-cli` and the release body offers no narrative or examples.

This feature adds a new GitHub Actions workflow that, on every release (and on demand), uses the Claude Code CLI to generate richer release notes (highlights + runnable usage examples + dry humour) from the changelog and git history, and prepares them for both Slack and the GitHub release body.

**First PR scope:** generate + print to the job log only. The Slack POST and GitHub-release-edit steps are written, syntactically correct, and committed *commented out* so a follow-up PR (or simply uncommenting once the `SLACK_WEBHOOK_URL` secret is set) can ship them without rework.

**Extension (2026-05-19):** activation PR. The two commented blocks are now turned on, plus a fail-fast check for the webhook secret, plus a human-readable Slack print step to work around GitHub's log UI truncating one giant single-line JSON. See REQ-F-017 onwards.

## Goals

- A new workflow `.github/workflows/post-release-notes.yml` triggers on `release: published` (auto) and `workflow_dispatch` with a `version` input (manual recovery).
- Both triggers run identical generation logic — there is no auto-vs-manual behaviour split.
- A new prompt `.github/prompts/release-notes.md` produces highlights with runnable `neo4j-cli` examples plus a brief "Other changes" tail, in a witty + wholesome tone.
- The full Slack payload (preamble + Claude body + release URL) is built and printed.
- The GitHub-release-body append payload (`## Release notes` from Claude, `---`, existing GoReleaser body) is computed in a commented step so the future state is reviewable in the diff.
- No edits to `release.yml`, `publish-npm.yml`, or `update-website.yml`.

## Non-Goals

- Live Slack posting in this PR. The `curl` step is committed commented out; the `SLACK_WEBHOOK_URL` secret is added later, and the comment is removed in a follow-up.
- Live overwrite of the GitHub release body in this PR. The `gh release edit` step is committed commented out.
- Replacing the existing GoReleaser-generated changelog body. The eventual end-state appends Claude notes *above* it; the GoReleaser body is preserved verbatim.
- Generating release notes from inside `release.yml` (we keep release and post-release concerns in separate workflows).
- Changing how `.changes/neo4j-cli/v{VERSION}.md` is produced (changie unchanged).
- A `release: edited` re-run; first PR only fires on `release: published`.
- Backfilling notes for past releases.

## Requirements

### Functional Requirements

- **REQ-F-001:** Create `.github/workflows/post-release-notes.yml` with `on: { release: { types: [published] }, workflow_dispatch: { inputs: { version: { description: "neo4j-cli version (no leading v), e.g. 1.4.0", required: true, type: string } } } }` (matches `publish-npm.yml`'s `workflow_dispatch.version` shape and input description style).
- **REQ-F-002:** The workflow MUST resolve a tag and a bare version:
  - On `release`: `TAG = github.event.release.tag_name` (already `vX.Y.Z`); `VERSION = ${TAG#v}`.
  - On `workflow_dispatch`: `TAG = v${{ inputs.version }}`; `VERSION = ${{ inputs.version }}`.
  - Both are exported to `$GITHUB_ENV` as `RELEASE_TAG` and `RELEASE_VERSION`.
- **REQ-F-003:** Validate `RELEASE_TAG` against `^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$` (the same regex as `publish-npm.yml:72`); abort with non-zero exit on mismatch.
- **REQ-F-004:** Checkout `main` with `fetch-depth: 0` so the prompt can run `git log <prev-tag>..<TAG>` and `git show <sha>` across the full release range.
- **REQ-F-005:** Resolve the previous release tag via `git describe --tags --abbrev=0 "${RELEASE_TAG}^" 2>/dev/null || true`; export as `PREV_TAG` (empty string if not found). The prompt MUST handle both cases.
- **REQ-F-006:** Install Claude Code via `npm install -g @anthropic-ai/claude-code` (verbatim copy from `update-website.yml:54-55`).
- **REQ-F-007:** Guard `ANTHROPIC_API_KEY` presence with the same check pattern as `update-website.yml:57-62`.
- **REQ-F-008:** Invoke Claude with `cat .github/prompts/release-notes.md | claude --bare -p - --output-format stream-json --verbose --include-partial-messages --no-session-persistence --exclude-dynamic-system-prompt-sections --allowedTools "Bash,Read,Write" --max-turns 30 --max-budget-usd 1.50`, env: `ANTHROPIC_API_KEY`, `ENABLE_PROMPT_CACHING_1H=1`, `RELEASE_TAG`, `RELEASE_VERSION`, `PREV_TAG`. Stream through the same `jq` tee as `update-website.yml:84-100`. The prompt writes `/tmp/release-notes-generated.md`.
- **REQ-F-009:** Add a step that fails the job if `/tmp/release-notes-generated.md` is missing or empty after the Claude step.
- **REQ-F-010:** Add a print step `cat /tmp/release-notes-generated.md`, always-on, to make the generation visible in the job log.
- **REQ-F-011:** Build a Slack payload at `/tmp/slack-payload.json` from `/tmp/release-notes-generated.md` plus a fixed preamble plus the release URL `https://github.com/${GITHUB_REPOSITORY}/releases/tag/${RELEASE_TAG}`. Use `jq -nR --rawfile body /tmp/release-notes-generated.md ...` (or equivalent) so newlines and quotes are JSON-safe. The text body has the shape:
  ```
  neo4j-labs/neo4j-cli ${RELEASE_TAG} is now out. Update with `neo4j-cli update` or use your favourite package manager.

  Release notes
  <body>

  https://github.com/neo4j-labs/neo4j-cli/releases/tag/${RELEASE_TAG}
  ```
  This step runs always-on; the JSON is printed to the log.
- **REQ-F-012:** Add a commented-out step that POSTs the Slack payload: `curl -X POST -H 'Content-Type: application/json' --data @/tmp/slack-payload.json "${SLACK_WEBHOOK_URL}"`, with env `SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK_URL }}`. The commented body MUST include a hard-fail guard: if `SLACK_WEBHOOK_URL` is empty, `echo` an error and `exit 1` BEFORE the `curl`. The comments MUST keep YAML syntactically valid (i.e. comment the whole step block; do not leave a half-step). Mark with a `# CLI-147 follow-up:` lead comment.
- **REQ-F-013:** Add a commented-out step that appends Claude notes to the GitHub release body:
  ```bash
  CURRENT=$(gh release view "$RELEASE_TAG" --json body -q '.body')
  {
    printf '## Release notes\n\n'
    cat /tmp/release-notes-generated.md
    printf '\n\n---\n\n'
    printf '%s\n' "$CURRENT"
  } > /tmp/release-body.md
  gh release edit "$RELEASE_TAG" --notes-file /tmp/release-body.md
  ```
  with env `GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}`. Wrap the whole step in comments with a `# CLI-147 follow-up:` lead comment.
- **REQ-F-014:** Set workflow `permissions: { contents: write }` (needed by the commented `gh release edit`; harmless while commented out).
- **REQ-F-015:** Create `.github/prompts/release-notes.md`. The prompt MUST:
  1. Read `.changes/neo4j-cli/v${RELEASE_VERSION}.md` for the canonical changelog body.
  2. Run `git log "${PREV_TAG}..${RELEASE_TAG}" --oneline` when `PREV_TAG` is non-empty; otherwise `git log --oneline -30`.
  3. For the biggest user-facing changes (new command trees, new flags, breaking changes, deprecations), `git show <sha> -- <paths>` and write a 1–2 line highlight with a runnable `neo4j-cli <example>` block. Pull command syntax from `neo4j-cli/aura/internal/subcommands/**`, `neo4j-cli/app/app.go`, and `README.md` — never invent flags.
  4. Group remaining entries under a brief "Other changes" bullet list.
  5. Tone: a little witty and wholesome — make releases feel enjoyable. Dry humour over emoji. No corporate fluff or marketing language.
  6. Constraints: no top-level `# h1` (the workflow concatenates a `## Release notes` header); ≤300 lines; no emoji unless the source changelog entry has one; only mention commands/flags present in the live cobra tree at `${RELEASE_TAG}`.
  7. Output structure:
     ```
     ### Highlights

     - **<headline>** — <why it matters in one sentence>. Example:
       ```
       neo4j-cli <invocation>
       ```

     ### Other changes

     - <one-line summary>
     ```
  8. Write the final markdown to `/tmp/release-notes-generated.md`. Do not print to stdout — the workflow logs the file.
- **REQ-F-016:** No code is added to the Go module; no Go file changes; no Makefile, no skill bundle regeneration.

#### Activation extension (added 2026-05-19)

- **REQ-F-017:** Add an early-fail guard step for `SLACK_WEBHOOK_URL`, placed AFTER the tag-validation step and BEFORE checkout / Claude install. The step `exit 1`s with a clear error if the secret is empty. Goal: avoid spending ~$1.50 of Claude generation when posting will fail anyway. Env: `SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK_URL }}`.
- **REQ-F-018:** Insert a human-readable Slack print step using `jq -r '.text' /tmp/slack-payload.json`, with a `----- Slack message (rendered) -----` header line and a matching `----- end -----` footer. Placed BETWEEN the existing "Build Slack payload" step (which still prints the JSON) and the new "Post to Slack" step. Motivation: GitHub's log UI clips the single-line JSON visually; the rendered text shows actual newlines.
- **REQ-F-019:** Activate the previously commented Slack POST step (originally specified by REQ-F-012). Uncomment the block; keep the hard-fail-on-empty `SLACK_WEBHOOK_URL` guard inside the step body (defense in depth on top of REQ-F-017). The step runs on BOTH `release: published` AND `workflow_dispatch` triggers — no `if:` gating.
- **REQ-F-020:** Activate the previously commented GH release append step (originally specified by REQ-F-013). Uncomment the block; runs on BOTH triggers.
- **REQ-F-021:** The GH release append step (REQ-F-020) MUST be idempotent on re-runs. Detection rule: if the current release body starts with `## Release notes\n` AND contains a `\n---\n` separator, the leading `## Release notes ... ---\n` block is removed BEFORE prepending the new Claude block. The split point is the FIRST `\n---\n` occurrence. If the marker is absent, the new block is prepended as-is. Concretely:
  ```bash
  CURRENT=$(gh release view "${RELEASE_TAG}" --json body -q '.body')
  case "${CURRENT}" in
    "## Release notes"*)
      # strip the leading Claude block up to and including the first `---` separator
      REMAINING="${CURRENT#*$'\n---\n'}"
      # If no separator was present, ${CURRENT#*...} returns the original string unchanged
      [ "${REMAINING}" = "${CURRENT}" ] && REMAINING="${CURRENT}"
      ;;
    *)
      REMAINING="${CURRENT}"
      ;;
  esac
  {
    printf '## Release notes\n\n'
    cat /tmp/release-notes-generated.md
    printf '\n\n---\n\n'
    printf '%s\n' "${REMAINING}"
  } > /tmp/release-body.md
  gh release edit "${RELEASE_TAG}" --notes-file /tmp/release-body.md
  ```
  This means re-running `workflow_dispatch` for v1.4.0 cleanly replaces the prior Claude block rather than stacking another one on top.
- **REQ-F-022:** Step ordering for the activation MUST be (top-to-bottom in the YAML): tag-validation → REQ-F-017 webhook fail-fast → checkout → prev-tag resolve → Claude install → API-key guard → Claude generate → guard generated file → print generated → build Slack payload → REQ-F-018 rendered print → Slack POST (REQ-F-019) → GH release append (REQ-F-020). The Slack POST runs BEFORE the GH release edit so a transient GH API failure doesn't prevent the Slack message from going out; the idempotent replace from REQ-F-021 means a `workflow_dispatch` re-run cleanly recovers the GH side.

### Non-Functional Requirements

- **REQ-NF-001:** The workflow YAML must parse on GitHub Actions (no syntax errors, all commented blocks valid as comments). Verify by pushing a draft and checking the Actions tab.
- **REQ-NF-002:** Generation cost per run capped at USD 1.50 via `--max-budget-usd 1.50`.
- **REQ-NF-003:** Total wall time for the workflow under 5 minutes for typical releases (≤30 commits since previous tag).
- **REQ-NF-004:** The workflow MUST be idempotent against re-runs — running `workflow_dispatch` twice for the same `version` MUST not produce different *committed* state (since the live mutating steps stay commented, this trivially holds for this PR; the requirement is documented to constrain future uncommenting).
- **REQ-NF-005:** Secrets used: `ANTHROPIC_API_KEY` (existing). `SLACK_WEBHOOK_URL` will be referenced only inside the commented step; the workflow MUST still pass YAML validation when that secret is absent. When uncommented in the follow-up, a missing `SLACK_WEBHOOK_URL` MUST hard-fail the job (no graceful skip).
- **REQ-NF-006:** No changelog entry under `.changes/unreleased/` — this is CI infrastructure with no user-visible CLI behaviour change (per AGENTS.md: "Internal changes (CI/CD workflow fixes, build scripts, code refactors with no visible effect) do not need changelog entries").
- **REQ-NF-007:** No license header is required for `.yml` or `.md` files under `.github/` (consistent with existing `update-website.yml` and `claude.yml`).

#### Activation extension (added 2026-05-19)

- **REQ-NF-008:** Pre-merge prerequisite — the `SLACK_WEBHOOK_URL` repo secret MUST be set BEFORE the activation PR merges. The fail-fast guard from REQ-F-017 makes a missing secret immediately visible; merging without it would break every subsequent release. The webhook points at `#feature-labs-neo4j-cli` per the original ticket.
- **REQ-NF-009:** Re-running `workflow_dispatch` against the same version is supported and SHOULD leave the GH release body in a clean state (Claude block replaced, not stacked). Slack is fundamentally non-idempotent — re-running posts the message again. Maintainers know this.
- **REQ-NF-010:** The activation must not regress actionlint cleanliness — the modified workflow file must still pass `actionlint .github/workflows/post-release-notes.yml` with zero errors and zero warnings.

## Technical Considerations

### Architecture / integration points

- **`.github/workflows/post-release-notes.yml`** (new) — single job `generate`, ubuntu-latest, timeout-minutes ~20. Mirrors `update-website.yml` step-for-step for Claude invocation; mirrors `publish-npm.yml`'s dual-trigger version resolution.
- **`.github/prompts/release-notes.md`** (new) — sibling to `.github/prompts/website-update.md` and `.github/prompts/claude-review-*.md`.
- **No changes** to `release.yml`, `publish-npm.yml`, or `update-website.yml`. Keeps blast radius to two new files.
- **Version source of truth on auto path** = `github.event.release.tag_name`, which GoReleaser sets when it publishes the GH release. This is more direct than the `workflow_run` + artefact dance that `publish-npm.yml` uses, and is appropriate because we don't need any binary artefacts — only the tag and the in-repo changelog.

### Trigger semantics

- ONLY two triggers are wired: `release: { types: [published] }` and `workflow_dispatch`. No `push`, no `release: { types: [created, edited, ...] }`, no `workflow_run`. Both triggers are anchored on an actual published release tag.
- `release: { types: [published] }` fires only once per release publish event (no double-fire on draft → publish transitions in this repo's flow, since GoReleaser publishes directly).
- `workflow_dispatch` lets a maintainer regenerate notes for any past tag. The `version` input MUST refer to an actual released tag; the workflow's tag-validation regex (REQ-F-003) is the only gate, but a follow-up step MAY add `gh release view "$RELEASE_TAG"` to fail fast on bogus inputs.
- On `workflow_dispatch`, the workflow runs against `main` HEAD (default behaviour). That's intentional — release notes don't need historical-checkout fidelity, only the tag string, the changelog file (committed before the tag), and the git log range.

### Constraints

- The Claude generation step runs untrusted prompt-driven Bash, so `--allowedTools` is restricted to `Bash,Read,Write` (read repo + write `/tmp`) — no Edit, no network beyond `git`, no `Agent` recursion. The prompt itself further constrains paths.
- `ANTHROPIC_API_KEY` is reused; no new Anthropic secret.
- The repo's existing `claude.yml` uses `CLAUDE_CODE_OAUTH_TOKEN` for issue-comment-driven Claude. The CLI flow (`update-website.yml`) uses `ANTHROPIC_API_KEY`. We follow the CLI flow — consistent with `update-website.yml`.
- Windows / macOS CI: not applicable (release-notes workflow is ubuntu-only).

### Risk register

| Risk | Mitigation |
|------|------------|
| Claude hallucinates a flag that does not exist. | Prompt instructs grounding in `neo4j-cli/...` + `README.md`; future PR can add a `neo4j-cli --help` validation pass. Not in v1 scope. |
| Generation exceeds `--max-budget-usd 1.50`. | Job fails loudly; manual re-run with raised budget or trimmed range. |
| `release: published` fires before GoReleaser finishes writing the body. | Not relevant in v1 — the body update is commented out. When uncommented, `gh release view` will read whatever GoReleaser wrote (atomic single-call). |
| Prompt drifts from CLI surface. | Out-of-scope for v1; a follow-up can add a doctor/`agent-context` cross-check step. |
| YAML "commented step" pattern silently invalid. | Verify with `actionlint` locally (`brew install actionlint && actionlint .github/workflows/post-release-notes.yml`) and by pushing to a feature branch and inspecting the Actions tab. |

## Acceptance Criteria

- [ ] `.github/workflows/post-release-notes.yml` exists and is parsed without errors by GitHub Actions (visible in the Actions tab after push).
- [ ] `.github/prompts/release-notes.md` exists and is a complete, self-contained prompt for the generator.
- [ ] `on:` block contains both `release: { types: [published] }` and `workflow_dispatch` with a `version` input matching `publish-npm.yml`'s description style.
- [ ] `permissions.contents: write` is set.
- [ ] Tag/version resolution and the `^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$` validation both run before any other logic.
- [ ] The Claude invocation uses the same flag set as `update-website.yml` except `--allowedTools`, `--max-turns`, `--max-budget-usd`, and the prompt path/env.
- [ ] A post-generation guard fails the job when `/tmp/release-notes-generated.md` is empty or missing.
- [ ] `cat /tmp/release-notes-generated.md` runs always-on as a dedicated step (not folded into the Claude step).
- [ ] The Slack payload step (build + print JSON) runs always-on; it is *not* commented out.
- [ ] The `curl` POST to Slack and the `gh release edit` append are present as commented step blocks, each preceded by a `# CLI-147 follow-up:` lead comment.
- [ ] `actionlint` (run locally) reports no errors.
- [ ] Manual `workflow_dispatch` run against `version: 1.4.0` produces a sensible markdown body in the job log, a printed Slack JSON payload, and zero mutations to the GitHub release or Slack.
- [ ] After the next real release, the auto path fires on `release: published` and produces the same shape of output.
- [ ] No edits to `release.yml`, `publish-npm.yml`, `update-website.yml`, any Go file, the Makefile, or the skill bundle.

### Activation extension (added 2026-05-19)

- [ ] Early-fail guard (REQ-F-017) exits non-zero before checkout when `SLACK_WEBHOOK_URL` is empty.
- [ ] A `jq -r '.text'` rendered-print step (REQ-F-018) sits between the JSON-payload print and the Slack POST, with `----- Slack message (rendered) -----` header.
- [ ] Slack POST step (REQ-F-019) runs unconditionally on both triggers and the `# CLI-147 follow-up:` comments are gone.
- [ ] GH release append step (REQ-F-020) runs unconditionally on both triggers and the `# CLI-147 follow-up:` comments are gone.
- [ ] Idempotent replace (REQ-F-021): a `workflow_dispatch` re-run for `version: 1.4.0` REPLACES the existing `## Release notes` block on the v1.4.0 release rather than stacking a new one.
- [ ] Step ordering (REQ-F-022) is exactly as specified — Slack POST precedes GH release edit.
- [ ] `actionlint .github/workflows/post-release-notes.yml` is clean.
- [ ] `SLACK_WEBHOOK_URL` is provisioned in the repo BEFORE this PR merges (REQ-NF-008).
- [ ] End-to-end verification: trigger `workflow_dispatch` for `version: 1.4.0`. A single message appears in `#feature-labs-neo4j-cli`; the v1.4.0 GH release body now leads with `## Release notes`; re-triggering does not stack the section.

## Out of Scope

- Slack live posting and `SLACK_WEBHOOK_URL` secret creation (follow-up).
- GitHub release body append going live (follow-up).
- Generating notes from a different source (commit messages alone, PR titles, etc.) — we stay anchored on `.changes/neo4j-cli/v*.md`.
- A re-run trigger on `release: edited` (could cause duplicate Slack posts; revisit when going live).
- A doctor that cross-checks Claude's output against the live cobra tree to catch hallucinated flags.
- Backfilling notes for releases prior to CLI-147 merge.
- Localised / multi-language output.

## Resolved decisions

- **Workflow filename**: `.github/workflows/post-release-notes.yml`.
- **Changelog source**: `.changes/neo4j-cli/v${RELEASE_VERSION}.md` (the per-version file already used by `release.yml`). The prompt MUST NOT parse the aggregate `CHANGELOG.md`.
- **Triggers**: ONLY `release: { types: [published] }` and `workflow_dispatch`. No other trigger types.
- **Missing `SLACK_WEBHOOK_URL`**: hard-fail the job (both at the early-fail guard and inside the Slack POST step).
- **Re-run idempotency (GH release body)**: detect-and-replace the existing `## Release notes ... ---` block before prepending. Stacking is prevented.
- **Re-run idempotency (Slack)**: not provided — Slack posts again. Maintainers accept this.
- **Step ordering**: Slack POST runs BEFORE GH release edit. A transient GH API failure does not block the Slack message.
- **Webhook check timing**: fail fast at the top of the job (before Claude runs).

## Open Questions

(None — all design questions resolved.)
