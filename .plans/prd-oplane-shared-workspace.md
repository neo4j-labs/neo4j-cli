# PRD: Route Oplane CI threat models to the shared org workspace

## Overview

`.github/workflows/claude-review-oplane.yml` runs an advisory Oplane threat-model
review on every non-draft member PR. It authenticates to the Gravity MCP server
with `secrets.OPLANE_PAT` — a *personal* token — and passes **no workspace ID
anywhere**. `new_threatmodel`'s `workspace_id` argument is optional and defaults
to the caller's personal workspace, so every model CI has ever produced landed in
`oskar.hane@neo4j.com` (personal) and is invisible to the rest of the team.

Verified against the live Oplane API during authoring: `list_threatmodels` for
workspace `neo4j-labs/neo4j-cli` returns `[]`, while the last five models for the
repo's PRs (#232, #238, #233, #237) all carry
`"workspace_type": "personal"`.

This PRD routes new models to the shared workspace
`neo4j-labs/neo4j-cli` (`6a246538-76d1-4857-830e-2aad4681d044`,
https://gravity.oplane.io/org/neo4j/workspaces/4) and makes that workspace a
single named constant at the top of the workflow, so another repo can copy the
file and change exactly one line.

A second, coupled defect is fixed at the same time. The PR-dedup lookup in
`.github/prompts/claude-review-oplane.md` Step 2 uses `my_recent_threatmodels`,
which is scoped to the **calling PAT**, not to the workspace. That was already
recorded as a known bus factor in the predecessor PRD
(`.plans/archive/prd-oplane-threat-model-pr-review.md`, "PAT ownership"):
swapping the token orphans every existing model and the next push duplicates.
Once models live in a shared workspace, an identity-scoped lookup is simply the
wrong query — `list_threatmodels` takes `workspace_id` and is identity-independent.

## Goals

- New CI threat models are created in workspace `6a246538-76d1-4857-830e-2aad4681d044`.
- Any team member with access to that workspace can see them in Gravity — the
  acceptance criterion is human-visible, not API-observable from the current PAT.
- The workspace is one clearly-marked constant; copying the workflow to another
  repo is a one-line edit.
- PR dedup survives a PAT rotation or a future service-account migration, because
  the lookup is scoped by workspace rather than by caller identity.
- The workflow's own comments stop asserting something that is no longer true
  (models owned by the PAT owner's account).

## Non-Goals

- Migrating the existing personal-workspace models. Settled: they age out.
- Introducing a service account. Still blocked on Oplane; the rotation warning
  in the workflow stays exactly as it is.
- Changing what is analysed, the four prompt steps' structure, the summary-comment
  discover-then-edit pattern, or the MCP-unavailable failure mode.
- Any change to `claude-review-security.yml` / `claude-review-conventions.yml`,
  the Go binary, the cobra tree, or the skill bundle.
- Vendoring or pinning the Oplane plugin.

## Requirements

### Functional Requirements

**Workflow — the single constant**

- REQ-F-001: Add a **top-level** `env:` block to
  `.github/workflows/claude-review-oplane.yml`, above `jobs:`, defining
  `OPLANE_WORKSPACE_ID: 6a246538-76d1-4857-830e-2aad4681d044`.

- REQ-F-002: That block carries a prominent comment marking it as the single
  copy-and-edit point. It must state: (a) a loud "copying this workflow? change
  this one value" line, (b) which workspace the UUID currently refers to —
  `neo4j-labs/neo4j-cli`, https://gravity.oplane.io/org/neo4j/workspaces/4 —
  and (c) how to obtain the UUID for a different workspace: the Oplane MCP tool
  `search_workspaces` with `name_pattern: "<org>/<repo>"`, or the workspace page
  in Gravity. Requirement (c) matters because the UUID is not derivable from the
  `/workspaces/4` URL a user would naturally start from.

- REQ-F-003: Thread the constant into the `prompt:` input as a third context
  line, alongside the existing two:

  ```yaml
  prompt: |
    REPO: ${{ github.repository }}
    PR NUMBER: ${{ github.event.pull_request.number }}
    OPLANE WORKSPACE ID: ${{ env.OPLANE_WORKSPACE_ID }}

    ${{ env.PROMPT }}
  ```

  `${{ env.X }}` already resolves in this `with:` block — `${{ env.PROMPT }}` on
  line 98 uses the same mechanism. The label is spaced (`OPLANE WORKSPACE ID:`),
  matching the existing `PR NUMBER:` style rather than the env var's underscore
  form, since these lines are read by the model, not by a shell.

- REQ-F-004: Do **not** add `workspace_id` to the `--mcp-config` JSON or to
  `OPLANE_BASE_URL`. Workspace selection is a per-call argument on
  `new_threatmodel`, not a connection property. The MCP config and the
  `env: OPLANE_BASE_URL` step-level entry are unchanged.

**Workflow — stale comment**

- REQ-F-005: Correct the `OPLANE_PAT` comment block (currently lines 111-116).
  The sentence "All threat models are therefore created under his Gravity
  account." is now false and must be replaced with the accurate statement: models
  are created in the shared workspace named by `OPLANE_WORKSPACE_ID`; the PAT
  supplies identity and write access only. **Retain unchanged** the surrounding
  warnings — no service account exists, CI reviews stop working if the token is
  rotated or the account is deprovisioned, the failure is visible via the
  MCP-unavailable path, migrate to a service account when Oplane offers one.
  This comment is load-bearing documentation (it satisfies REQ-F-027 of the
  predecessor PRD); trim the false clause, not the warning.

**Prompt — Step 2 dedup lookup**

- REQ-F-006: In `.github/prompts/claude-review-oplane.md` Step 2 item 1, replace
  `my_recent_threatmodels` with `list_threatmodels`, passing `workspace_id` set
  to the `OPLANE WORKSPACE ID` value from the context line. Rationale to record
  in the prompt text: `my_recent_threatmodels` is scoped to the calling PAT, so
  it cannot see a teammate's or a future service account's models in the shared
  workspace.

- REQ-F-007: Because `list_threatmodels` does **not** return a
  `pull_request_url` field, the match key is the **title** only. Verified against
  the live API: the response objects expose `threatmodel_id`, `title`,
  `description`, `user_id`, `workspaces`, `workspace_id`, `workspace_name`,
  `workspace_type`, `created_at`, `updated_at` — and nothing else. Step 2 item 1
  must therefore drop the `pull_request_url` match arm and match on the title
  marker alone.

- REQ-F-008: Pin the title marker to the exact literal `[<owner/repo>#<PR_NUMBER>]`
  — square brackets, no space, e.g. `[neo4j-labs/neo4j-cli#232]`. This is the
  format real runs already produce (confirmed on the four most recent models), so
  models created before this change remain findable by the new lookup if they are
  ever moved. The current prompt's looser `<${GITHUB_REPOSITORY}>#${PR_NUMBER}`
  wording — which renders angle brackets literally — must be corrected to this
  form in **both** places it appears: the lookup (item 1) and the create
  instruction (item 3).

**Prompt — Step 2 create path**

- REQ-F-009: In Step 2 item 3 (the new-model path), add `workspace_id` to the
  enumerated `new_threatmodel` arguments, sourced from the `OPLANE WORKSPACE ID`
  context line, alongside the existing `pull_request_url` and title-shape
  requirements. (`new_threatmodel` still accepts `pull_request_url` on write even
  though `list_threatmodels` does not return it on read — keep passing it.)

- REQ-F-010: State explicitly that `workspace_id` must be supplied **on every**
  `new_threatmodel` call, including the run where the upstream `oplane:analyze-pr`
  skill would otherwise create the model itself. This is the load-bearing
  instruction: the skill's own step 3 calls `new_threatmodel` with no workspace,
  and the skill is upstream (`oplane@oplane-plugins`) and not vendored, so it
  cannot be edited. Our prompt already overrides the skill's create path — Step 2
  opens by noting the skill "calls `new_threatmodel` unconditionally and has no
  dedup step of its own" — so the override point exists and is being extended.
  Word it so it cannot be read as applying only to the dedup-miss branch.

- REQ-F-011: Step 2 item 4's existing fallback is unchanged in intent and must be
  preserved: if the existing model cannot be updated for any reason, create a new
  one rather than failing the run. A duplicate beats no model.

**Prompt — Step 1 wording**

- REQ-F-012: Fix the final sentence of Step 1 (line 13), which currently reads
  "The skill's own step 1 verifies the MCP connection via `my_recent_threatmodels`
  and hard-stops on failure, which feeds into Step 4 below." That statement about
  the *skill* stays true — the skill is upstream and unchanged — but after
  REQ-F-006 it must not read as describing our dedup. Reword so the two calls are
  plainly distinct: the skill's connectivity probe, versus our workspace-scoped
  dedup lookup.

**Tool allowlist**

- REQ-F-013: The `--allowed-tools` wildcard `mcp__oplane__*` already covers
  `list_threatmodels`; no change to `claude_args` is required. However, the
  predecessor PRD's REQ-F-016 fallback — "if the wildcard does not match at
  runtime, list all six tools explicitly" — enumerates six names that do **not**
  include `list_threatmodels`. Record in this PRD that the fallback list must
  gain `mcp__oplane__list_threatmodels` if it is ever adopted. Without that, a
  wildcard failure would silently disable dedup and every push would duplicate.

### Non-Functional Requirements

- REQ-NF-001: `.github/`-only change. No Go source, no `go generate`, no skill
  bundle diff. `make test` / `make lint` / `make fmt-check` are unaffected and are
  not meaningful gates for this work.

- REQ-NF-002: The workflow passes `actionlint` cleanly (confirmed present at
  `/opt/homebrew/bin/actionlint`).

- REQ-NF-003: No changelog entry. CI-only, zero user-facing CLI impact, per the
  AGENTS.md changie rule.

- REQ-NF-004: `OPLANE_WORKSPACE_ID` is **not** a secret. It is a workspace
  identifier, not a credential — access is enforced by the PAT. It belongs in
  plaintext `env:` precisely so it is visible and editable.

- REQ-NF-005: The check stays advisory. `continue-on-error: true` is untouched;
  nothing here can turn the job red or block a merge.

## Technical Considerations

**A top-level `env:` is a new convention in this repo.** Verified: all 16
workflows under `.github/workflows/` have **zero** top-level `env:` blocks, zero
job-level `env:` blocks, and zero `vars.X` usage. Every existing constant is
either a step-level `env:` entry or an inline literal. Choosing top-level `env:`
here is deliberate — the entire requirement is a visible single edit point at the
top of the file, which a step-level entry buried at line 72 does not provide.
Because it deviates, the PR description must call it out so it does not read as
an accident and get "normalised" away later. This mirrors how the predecessor PRD
handled the `concurrency` deviation (REQ-F-004 there), which is likewise the only
one of its kind in the repo and is defended by an in-file comment.

**Why not `vars.OPLANE_WORKSPACE_ID`.** A repo variable would move the value out
of the file entirely — the opposite of the stated goal. A copying repo would then
have to discover and set an invisible repo-level variable rather than edit a line
they can see in the file they just copied. It also has no precedent here.

**Dedup is title-only, and that is a real narrowing.** The current prompt offers
two match keys (`pull_request_url` or title); after REQ-F-007 only the title
remains, because `list_threatmodels` does not project `pull_request_url`. This is
why REQ-F-008 pins the exact bracket literal rather than leaving the format
descriptive. A third option was considered and rejected: list, then call
`get_threatmodel` on each candidate to read `pull_request_url`. It is more robust
but costs N extra MCP round-trips per run inside an already 20-minute-bounded job,
for a marker format that has been stable across every model the workflow has
produced.

**The upstream skill is the actual leak point.** `oplane:analyze-pr` step 3 calls
`new_threatmodel` with no `workspace_id`. It is installed from
`https://github.com/oplane/oplane-plugin.git` at run time, unpinned and
uneditable by this repo. Everything routing models to the shared workspace
therefore depends on our prompt's override winning over the skill's default
behaviour. If a future upstream change makes the skill's create path harder to
override, models will silently revert to personal — which is why the acceptance
criteria include a positive check that the model actually landed in workspace 4,
not merely that the run was green.

**Write access is confirmed, so the first run should succeed.** Settled during
planning: the `OPLANE_PAT` owner can write to workspace 4. If that ever ceases to
hold, `new_threatmodel` fails and the existing Step 4 path posts the
"⚠️ Oplane MCP unavailable" comment — visible, non-blocking, no silent
degradation. No new failure mode is introduced by this change.

**Known wrinkle when smoke-testing from the PR itself.** Recorded in commit
e241c447: Anthropic's token exchange refuses to mint a GitHub App token when the
workflow file on the PR branch differs from the copy on `main`
(`workflow_not_found_on_default_branch`). A temporary
`github_token: ${{ secrets.GITHUB_TOKEN }}` override was used previously to work
around it. If used again it **must be reverted before merge** — with it set, the
action posts as `github-actions[bot]` rather than `claude[bot]`, which breaks the
Step 3 discover-then-edit lookup (it filters on `.user.login == "claude[bot]"`)
and produces a new comment per push instead of an edit.

**Old models are not migrated.** Settled. Personal-workspace models for open PRs
will not be found by the new workspace-scoped lookup, so the next push to any
currently-open PR creates one fresh model in workspace 4. That is the intended
one-time cost of the cutover, not a bug to work around.

## Acceptance Criteria

- [ ] `.github/workflows/claude-review-oplane.yml` has a top-level `env:` block
      defining `OPLANE_WORKSPACE_ID: 6a246538-76d1-4857-830e-2aad4681d044`.
- [ ] That block's comment names the target workspace, its Gravity URL, and how
      to find another workspace's UUID via `search_workspaces`.
- [ ] The `prompt:` input carries an `OPLANE WORKSPACE ID:` line next to the
      existing `REPO:` and `PR NUMBER:` lines.
- [ ] `actionlint .github/workflows/claude-review-oplane.yml` reports no findings.
- [ ] The `OPLANE_PAT` comment no longer claims models are created under the PAT
      owner's account, and still carries the rotation / service-account warning.
- [ ] `.github/prompts/claude-review-oplane.md` Step 2 uses `list_threatmodels`
      with `workspace_id`; no `my_recent_threatmodels` call remains in Step 2.
- [ ] Step 2 matches on the title literal `[<owner/repo>#<N>]` and no longer
      references `pull_request_url` as a *lookup* key.
- [ ] Step 2's create path passes `workspace_id` to `new_threatmodel`, and says
      so in terms that cover the skill-initiated create.
- [ ] Step 1's closing sentence distinguishes the skill's connectivity probe from
      our dedup lookup.
- [ ] Step 3 (summary comment) and Step 4 (MCP-unavailable) are unmodified.
- [ ] `claude_args` is unmodified; `--mcp-config` and `OPLANE_BASE_URL` unchanged.
- [ ] No file added under `.changes/unreleased/`; no diff outside `.github/` and
      `.plans/`.
- [ ] **First live run:** `list_threatmodels` with
      `workspace_id=6a246538-76d1-4857-830e-2aad4681d044` returns the PR's model.
      Baseline is `[]` today, so any result proves the routing works.
- [ ] **First live run:** the returned model's `workspace_type` is not `personal`.
- [ ] **Second push to the same PR:** the workspace still holds exactly one model
      for that PR — updated, not duplicated — and the summary comment is edited
      rather than re-posted.
- [ ] **Visibility (the actual goal):** a person who is not the `OPLANE_PAT` owner
      opens https://gravity.oplane.io/org/neo4j/workspaces/4 and sees the model.
      Cannot be verified through the API with the current PAT; requires a human.
- [ ] **Advisory preserved:** the job exits 0 regardless of findings.

## Out of Scope

- Migrating existing personal-workspace threat models.
- Introducing an Oplane service account, or any change to how `OPLANE_PAT` is
  minted or stored.
- Vendoring or SHA-pinning `oplane@oplane-plugins`.
- Changing the analysis itself, the summary-comment format, or the failure mode.
- Adopting the REQ-F-016 explicit-tool fallback list (only annotated here).
- `.plans/archive/**` — historical record, left as written.
- Making the check a required status in branch protection.

## Open Questions

1. **Does the upstream skill's own `new_threatmodel` call reliably yield to our
   prompt's `workspace_id` instruction?** Cannot be verified before a live run —
   the skill is fetched at run time. The first-run acceptance criteria are written
   to catch a silent revert to personal rather than assume success.
2. **Is `[<owner/repo>#<N>]` guaranteed stable?** It is what the last four models
   used, but it is produced by the model following prose, not by a schema. If
   dedup ever misfires, promoting the marker to something machine-generated (or
   adopting the `get_threatmodel` lookup rejected above) is the next lever.
