# PRD: Website agentic-mode rendering invariants

## Overview

The neo4j.sh landing page (served from the `gh-pages` branch of this repo) has two `agentic | cli` toggles — one above the Quickstart cards (`#quickstartSteps`) and one above the Examples grid (`#examplesSection`). In `agentic` mode today, the agent prompt renders as `# <text>` in terminal-green, and directly below it sits the same fully syntax-highlighted shell command that `cli` mode shows (green `$`, pink flags, green strings, lavender keywords, etc.). The bright, colored command outshouts the agent prompt — but the agent prompt is the actual differentiator we want readers to register.

This PRD captures the durable change: update `.github/prompts/website-update.md` so every future website-update run enforces a new set of agentic-mode rendering invariants, and add a short pointer in `AGENTS.md` so future contributors / agents discover the workflow. The Go CLI source is not touched; the `gh-pages` branch is not touched in this PR.

## Goals

- Make the agent prompt the visual hero in agentic mode by demoting the underlying shell command to a single readable low-contrast grey, with no syntax colors.
- Add a one-line `→ loading skill neo4j-cli` cue under each agent prompt to make the rendering feel like a real agent session.
- Switch the agent-prompt prefix from `# ` to `> ` so it visually mirrors a chat-style invocation, not a shell comment.
- Codify all three rules as **invariants in the website-update prompt** so they survive every future sync run and cannot be silently regressed by drift.
- Document the prompt-driven website workflow in `AGENTS.md` so future agents and contributors find it on first read.

## Non-Goals

- Editing `gh-pages/index.html` itself in this PR. The user will run the website-update action manually after this lands; the prompt change is what makes the rules durable.
- Touching the `cli` mode rendering. CLI mode keeps full syntax highlighting and continues to hide `.agentic-line` as today.
- Changing colored table-art (`.out` blocks) in Examples panes. They keep their existing colors.
- Re-skinning the broader site (layout, typography, palette, copy style).
- Generating the website from code in `main` (Go templates, static-site generator, etc.). The site stays prompt-driven.
- Changelog entry. Per AGENTS.md, internal/doc-only changes that produce no end-user-visible behavior in the CLI binary do not require a changelog entry.

## Requirements

### Functional Requirements

- REQ-F-001: `.github/prompts/website-update.md` MUST contain a new step labelled `5b. Enforce agentic-mode rendering invariants`, placed between the existing `5. Make surgical updates` and `6. Validate the examples` steps.

- REQ-F-002: The new step MUST state, as an invariant the runner verifies and fixes on every run, that inside every `.agentic-comment` span in `gh-pages/index.html` the prompt text starts with `> ` (greater-than + space), NOT `# ` (hash + space). The `.agentic-comment` color stays bound to `var(--terminal-prompt)`.

- REQ-F-003: The new step MUST state that exactly ONE `loading skill` line appears per agent prompt — even when the prompt generates multiple `$` sub-commands, only one loading line sits between the prompt and the first command. It MUST be rendered as `<span class="agentic-loading">→ loading skill neo4j-cli</span>` placed inside the same `.agentic-line` wrapper, immediately after the `.agentic-comment`. Literal text: Unicode arrow `→` (U+2192), single space, `loading skill neo4j-cli`. No ellipsis, no trailing dots, no `->` ASCII fallback.

- REQ-F-004: The new step MUST include a CSS block (to be present in `gh-pages/index.html`'s `<style>`) that:
  - Styles `.agentic-loading` as `display: block; color: var(--fg-faint); font-style: italic; opacity: 0.85;`.
  - When `#quickstartSteps` does NOT have the `.cli-mode` class, paints `.step-cmd` and all its direct-child non-`.agentic-line` non-`.copy-icon` content (and descendants) in `var(--fg-faint)` with `!important`.
  - When `#examplesSection .examples` does NOT have the `.cli-mode` class, paints `.example-pane pre` and all its direct-child non-`.agentic-line` non-`.out` content (and descendants) in `var(--fg-faint)` with `!important`.
  - Specifically: the `$` prompt token is INCLUDED in the dim sweep (merges with the rest of the block — no brighter accent).
  - Specifically: `.out` blocks (rendered table-art / result output) are EXCLUDED from the dim sweep and keep their existing token colors.

- REQ-F-005: The new step MUST state that the existing `.cli-mode` behaviour is unchanged: adding `.cli-mode` to either toggle container restores full-color syntax highlighting and hides `.agentic-line`. The runner must not weaken existing `.cli-mode` rules.

- REQ-F-006: The new step MUST state that both toggles (Quickstart and Examples) follow the same three rules — prompt prefix, loading line, dim sweep — and changes to one MUST be mirrored to the other in the same run.

- REQ-F-007: The new step MUST instruct the runner to verify, after editing, that in default agentic mode the only colored content above the dim block is the `> ` prompt and the `→ loading skill neo4j-cli` line; toggling to `cli` mode restores syntax highlighting.

- REQ-F-008: `AGENTS.md` MUST gain a new section `## Website (neo4j.sh)`, placed after `## Repo Doc Notes` and before `## Repo Layout Notes`. The section MUST state:
  - The landing page is served from the `gh-pages` branch (`gh-pages/index.html`, `gh-pages/llms.txt`, `gh-pages/install.sh`, `gh-pages/install.ps1`).
  - The site is **prompt-driven** by `.github/prompts/website-update.md`, not generated from Go code in `main`.
  - The 4-step update workflow: `git worktree add gh-pages gh-pages` → hand the prompt to an agent → review the gh-pages diff → commit & push on `gh-pages`.
  - The prompt encodes rendering invariants that are enforced on every run; future changes to those invariants MUST be made by editing the prompt, not by hand-editing `gh-pages/index.html`.

- REQ-F-009: `CLAUDE.md` is a symlink to `AGENTS.md` and MUST NOT be edited directly; both surfaces update via the single edit to `AGENTS.md`.

### Non-Functional Requirements

- REQ-NF-001: The edits MUST NOT introduce Go source changes; `make fmt-check`, `make lint`, `make test`, and `make generate-check` continue to pass with their existing outputs.

- REQ-NF-002: The PR diff MUST be limited to exactly two tracked files in `main`: `.github/prompts/website-update.md` and `AGENTS.md`. No skill-bundle regeneration, no `gh-pages/` edits, no generated-content drift.

- REQ-NF-003: The new prompt section MUST be self-contained — a fresh agent reading only `.github/prompts/website-update.md` (no other context) MUST have enough information to enforce all invariants in a single pass.

## Technical Considerations

- **Prompt is the source of truth.** Because `gh-pages/index.html` is hand-edited by an agent on every sync, the invariants survive only if they are written into the prompt. Trying to enforce them via a CI check on the `gh-pages` branch would also work but is out of scope for this PRD — the prompt-only approach matches the existing maintenance model.
- **CSS selector strategy.** The dim sweep uses `:not(.cli-mode)` on the parent toggle container, so the existing `.cli-mode` toggle (set by the existing JS) flips between dimmed and fully-colored without any new JS. The `:not(.agentic-line)` and `:not(.out)` exclusions are scoped to direct children to keep the rule narrow and predictable; the `.out` exclusion is the key carve-out that lets table-art outputs keep their existing colors while the `$` command lines above them flatten to grey.
- **`$` prompt merges with the block.** The user explicitly chose to drop the `$`'s brighter accent and let it fully merge into `var(--fg-faint)`. This is a deliberate departure from the "keep `$` slightly brighter" pattern many terminal renderings use; the rationale is that in agentic mode the user is reading prompt + skill cue, not scanning for shell-command boundaries.
- **`.agentic-comment` keeps `var(--terminal-prompt)`.** The agent prompt is the visual hero, so it stays in terminal-green (the same green the `$` token used to be). This implicit contrast — green agent prompt + grey command block — is what makes the agentic story legible.
- **`AGENTS.md` placement.** Slotting the new section between "Repo Doc Notes" and "Repo Layout Notes" keeps doc-related sections grouped. Placing it elsewhere (e.g. near "Deployment") would also be defensible; the choice here keeps maintainability docs together.
- **End-to-end verification is deferred.** Because gh-pages is out of scope, the visual outcome cannot be verified on this branch. The user will run the website-update action manually after the prompt change lands; if the runner's interpretation of the invariants needs adjustment, the prompt will be tightened in a follow-up.

## Acceptance Criteria

- [ ] `.github/prompts/website-update.md` contains a new step `5b. Enforce agentic-mode rendering invariants` between steps 5 and 6.
- [ ] The new step specifies the `> ` prompt prefix invariant (REQ-F-002).
- [ ] The new step specifies the `→ loading skill neo4j-cli` invariant, exact literal text, one per agent prompt, inside `.agentic-line`, in a `.agentic-loading` span (REQ-F-003).
- [ ] The new step includes the CSS block covering `.agentic-loading` styling and the `:not(.cli-mode)` dim sweep for both `#quickstartSteps .step-cmd` and `#examplesSection .examples .example-pane pre`, with `.out` excluded (REQ-F-004).
- [ ] The new step explicitly preserves existing `.cli-mode` behaviour (REQ-F-005).
- [ ] The new step requires symmetric treatment across both toggles (REQ-F-006).
- [ ] The new step includes a post-edit verification instruction (REQ-F-007).
- [ ] `AGENTS.md` gains a `## Website (neo4j.sh)` section between `## Repo Doc Notes` and `## Repo Layout Notes`, covering the 4 bullet points in REQ-F-008.
- [ ] `CLAUDE.md` is not edited directly; it stays a symlink to the updated `AGENTS.md`.
- [ ] `git diff` shows changes only in `.github/prompts/website-update.md` and `AGENTS.md`.
- [ ] `make fmt-check`, `make lint`, `make test`, and `make generate-check` all pass on the same outputs as before this change.
- [ ] No changelog entry added (matches AGENTS.md guidance for internal/doc-only changes).

## Out of Scope

- Editing `gh-pages/index.html`, `gh-pages/llms.txt`, `gh-pages/install.sh`, or `gh-pages/install.ps1`. The user will run the website-update action manually after this PR merges.
- Adding a CI gate on the `gh-pages` branch that verifies the invariants on every commit.
- Generating the website from `main` (Go templates, static-site generator, build-time HTML render). The site stays prompt-driven.
- Changing CLI-mode rendering, table-art colors, layout, fonts, palette, copy, or any other site design surface beyond the three invariants above.
- Adding a changelog entry. Internal docs/prompt changes do not require one.
- Touching the Go CLI binary, skill bundle, distribution scripts, or any other code path.

## Open Questions

None. All design choices (prompt prefix `>`, literal loading line text, single loading line per prompt, fully-merged `$` accent, `.out` exclusion, AGENTS.md placement, out-of-scope gh-pages run, no changelog) have been confirmed by the user.
