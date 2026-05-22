# PRD: Agentic-tab copy copies the agent prompt, not the CLI command

Linear: [CLI-166](https://linear.app/neo4j/issue/CLI-166/agentic-tab-copy-action-uses-cli-instructions)
Plan: `/Users/oskarhane/.claude/plans/enchanted-strolling-dewdrop.md`

## Overview

The marketing site at https://neo4j.sh (served from the `gh-pages` branch) has a Quickstart with a `cli / agentic` mode toggle. In agentic mode the visible command is replaced by a green natural-language prompt (e.g. `> Add my Aura credentials — grab client ID and secret from console.neo4j.io`). Clicking the step still copies the raw CLI command from `data-copy`, regardless of mode. This change makes the Quickstart copy action mode-aware so agentic-tab users get the agent prompt on their clipboard instead.

The fix is a small JS change in `gh-pages/index.html` plus a copy-UX invariant added to `.github/prompts/website-update.md` (on `main`) so the regen prompt encodes it permanently.

## Goals

- Clicking any Quickstart step (`.step-cmd`) in agentic mode copies the agent prompt, not the CLI command.
- CLI-mode copy behaviour is preserved bit-for-bit.
- The fix mirrors the existing mode-aware pattern already in the Examples section (`.copy-mini` handler at `gh-pages/index.html:1856-1869`) so the page has one consistent approach.
- The copy-UX invariant is documented in `.github/prompts/website-update.md` so the next site-regen run does not silently drop it.

## Non-Goals

- No changes to which steps exist, their order, or their visible HTML rendering. The `> ` prefix stays in the DOM (Invariant 1 of `website-update.md`); only the *copied* text is stripped.
- No changes to the Examples section copy behaviour (already correct).
- No new CLI features, Go code changes, tests, `go generate`, or changelog entry. This is a website-only fix; per `AGENTS.md`, internal/website changes do not get a `changie` entry.
- No restructuring of `gh-pages/index.html` content via the `website-update.md` prompt — the invariant is added, but a full prompt-driven regen is out of scope for this ticket.
- No changes to `gh-pages/llms.txt`, `install.sh`, or `install.ps1`.

## Requirements

### Functional Requirements

- **REQ-F-001**: When `viewMode === 'agentic'` and the clicked `.step-cmd` contains an `.agentic-comment` descendant, the clipboard receives the `.agentic-comment` text content with the leading `> ` prefix (and any whitespace after it) stripped.
- **REQ-F-002**: When `viewMode === 'cli'`, the clipboard receives the `data-copy` attribute value unchanged (current behaviour).
- **REQ-F-003**: When `viewMode === 'agentic'` and the clicked `.step-cmd` has no `.agentic-comment` (Quickstart steps 1 and 2, the install commands), the clipboard receives `data-copy` — the fallback matches CLI mode because there is no agentic alternative.
- **REQ-F-004**: The post-copy toast message (`Copied: <first 40 chars>…`) reflects whichever string was actually copied.
- **REQ-F-005**: Switching modes via the `cli` / `agentic` mode buttons (`[data-qs-mode]`, `[data-ex-mode]`) continues to update `viewMode` globally; the new `.step-cmd` handler reads `viewMode` at click-time, not at page-load-time, so a mid-session toggle takes effect on the next click.
- **REQ-F-006**: The Examples-section `.copy-mini` handler (`gh-pages/index.html:1856-1869`) is unchanged.
- **REQ-F-007**: `.github/prompts/website-update.md` gains a copy-UX invariant block describing REQ-F-001…REQ-F-003 in prose, so any future prompt-driven regen of `index.html` re-emits the same behaviour.

### Non-Functional Requirements

- **REQ-NF-001**: JS diff is minimal — replace only the existing `.step-cmd` handler block. No new top-level constants, no refactor of unrelated code.
- **REQ-NF-002**: No new browser-API dependencies. The handler stays on `addEventListener('click', …)` + `el.querySelector('.agentic-comment')?.textContent` + existing `copyText` helper. ES2020 optional-chaining is already used elsewhere on the page.
- **REQ-NF-003**: No regressions for users on browsers that block the async Clipboard API — the existing `copyText` fallback path is reused as-is.
- **REQ-NF-004**: Two commits, two branches: the JS fix lands on `gh-pages` (auto-served by GitHub Pages); the invariant doc lands on `main` via PR. No cross-branch coupling, no required ordering between the two.

## Technical Considerations

**Root cause**. The Quickstart `.step-cmd` click handler at `gh-pages/index.html:1872-1879` unconditionally reads `el.dataset.copy`, which always holds the CLI command. The Examples section handler at lines 1856-1869 already does the right thing via `viewMode === 'agentic' ? panePrompts[id] : paneCommands[id]` — we just need to mirror that pattern for Quickstart, sourcing the agent prompt from the `.agentic-comment` DOM node already present in the markup.

**Data source for agent prompt**. The agent prompt text lives in the DOM as `<span class="agentic-comment">&gt; …</span>` inside each `.step-cmd`. Reading it via `el.querySelector('.agentic-comment')?.textContent.trim().replace(/^>\s*/, '')` keeps the prompt text colocated with its visible rendering — no parallel JS lookup table to drift from the HTML.

**`> ` prefix handling**. Invariant 1 of `website-update.md` requires the `> ` prefix to remain in the rendered HTML. The Linear ticket's expected copy text is *without* the prefix. The fix renders `> ` on screen and strips it from the clipboard payload — one regex, no HTML change.

**Steps without an agentic alternative**. Steps 1 (install via curl) and 2 (`neo4j-cli skill install --rw`) have no `.agentic-comment` because they apply identically in both modes. The handler falls back to `data-copy` when `.agentic-comment` is absent, preserving current behaviour.

**Sibling concerns left intact**. The `.alt-install` handler (lines 1882-1888) is mode-agnostic by design (it copies alternate install commands like brew / scoop / npm). No change.

**Why edit `website-update.md` on `main`**. `.github/prompts/website-update.md` is the source of truth for content updates per `AGENTS.md` "Website (neo4j.sh)". A future prompt-driven regen of `gh-pages/index.html` would otherwise regenerate the broken pre-fix handler. Adding the invariant block (slotted under the existing block ~line 75) encodes the mode-aware copy contract permanently.

**Verification approach**. No automated test suite exists for `gh-pages/index.html` (no Cypress, no Playwright). Verification is manual: open the file locally, exercise each step in each mode, paste and compare. Cross-browser spot-check on Chrome and Safari (clipboard API behaviour differs slightly).

## Acceptance Criteria

- [ ] In agentic mode (default), clicking Quickstart step 3 copies exactly `Add my Aura credentials — grab client ID and secret from console.neo4j.io` (no `> ` prefix, no CLI fragment).
- [ ] Same applies to steps 4, 5, 6 — each pastes its respective `.agentic-comment` text minus the `> `.
- [ ] In agentic mode, clicking steps 1 and 2 still copies the install commands from `data-copy` (no agentic alternative exists).
- [ ] In CLI mode, clicking each of steps 1–6 copies the existing CLI command from `data-copy` — byte-identical to pre-fix behaviour.
- [ ] Toast message reflects the actually-copied string (truncated to 40 chars + `…` as today).
- [ ] Examples-section `.copy-mini` buttons still work in both modes (regression check on at least one pane per mode).
- [ ] Tested on Chrome and Safari; clipboard contents verified via paste, not console.
- [ ] `.github/prompts/website-update.md` gains a copy-UX invariant block describing the mode-aware copy contract for `.step-cmd` and `.copy-mini`, including the `> ` strip rule.
- [ ] `gh-pages` branch gets one commit; `main` branch gets one PR; neither requires a `changie` entry.

## Out of Scope

- Full prompt-driven regen of `gh-pages/index.html`.
- Adding automated browser/clipboard tests for the site.
- Changes to install scripts, `llms.txt`, or any other `gh-pages` file.
- CLI binary changes.
- Localisation of the agent prompts.

## Open Questions

None. Confirmed by user during planning: strip `> ` on copy; encode the invariant in `website-update.md` in the same change set.
