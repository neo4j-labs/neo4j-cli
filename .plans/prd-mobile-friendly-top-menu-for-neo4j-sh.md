# PRD: Mobile-friendly landing page for neo4j.sh

## Overview

The landing page at https://neo4j.sh (served from the `gh-pages` branch, file `index.html` at repo root of that branch) has no responsive rules for two surfaces that crowd or overflow at iPhone-class widths (~375px):

1. **Top header** — the inline row `brand + 4 nav links + theme toggle` is well wider than the viewport.
2. **Installer selector bar** — the six tabs (`macOS` / `Linux` / `Windows` / `Homebrew` / `pipx` / `npm`), each rendered as an icon + label inside a single `inline-flex` row, overflow horizontally.

This feature adds a single mobile breakpoint (`@media (max-width: 640px)`) that:

1. Collapses the four nav links (`Examples` / `Reference` / `Quickstart` / `GitHub`) behind a hamburger button. When opened, the nav becomes a full-width bar that drops down under the header. The hamburger sits at the far right edge (right of the existing theme toggle). The `labs` brand tag is hidden below the breakpoint to give the brand room to breathe.
2. Reflows the installer bar into a full-width, two-row grid: OSes (`macOS` / `Linux` / `Windows`) on row 1 and package managers (`Homebrew` / `pipx` / `npm`) on row 2, three tabs per row, each tab stretched to fill its third of the width.

Desktop layout (≥641px) is untouched for both surfaces.

## Goals

- Eliminate horizontal overflow of the top menu AND the installer bar on viewports as narrow as 320px.
- Keep all four nav destinations (Examples, Reference, Quickstart, GitHub) reachable on mobile.
- Keep all six installer options visible (no hidden labels, no horizontal-scroll hidden affordance) on mobile.
- Preserve the existing desktop header AND installer-bar layouts, fonts, and interactions exactly.
- Keep the change self-contained in `index.html` (this branch is rooted on `gh-pages`, where `index.html` lives at repo root). No new files, no build step.
- Keep the page accessible: hamburger is a `<button>` with `aria-controls`, `aria-expanded`, dynamic `aria-label`, Escape-to-close, and click-outside-to-close. The installer bar keeps its existing `role="tablist"` / `role="tab"` semantics.

## Non-Goals

- No redesign of the desktop header.
- No change to copy, copy style, or content beyond the markup needed for the hamburger button.
- No change to `install.sh`, `install.ps1`, `llms.txt`, or any non-header section of `index.html`.
- No new dependencies, build step, framework, or JS library — vanilla CSS + a small inline script appended to the existing `<script>` block.
- No backdrop overlay; closing relies on click-outside / Escape / link-click.
- No tampering with the `data-activate-tab="reference"` behavior on the Reference link — that link continues to work exactly as today (toggling the dropdown closed after the click is fine and expected).

## Requirements

### Functional Requirements

- **REQ-F-001:** Below 640px viewport width, `nav.top-nav` MUST be hidden from the inline header row by default. Above 640px (≥641px), it MUST render exactly as it does today — inline flex, four links visible, no hamburger.
- **REQ-F-002:** Below 640px, a hamburger button (`<button class="icon-btn nav-toggle">`) MUST be visible in the header, positioned to the right of the theme toggle (`[logo] neo4j-cli  …  [☼] [☰]`).
- **REQ-F-003:** Above 640px the hamburger button MUST be hidden (`display: none`).
- **REQ-F-004:** Clicking the hamburger MUST toggle the dropdown open/closed. When open, `nav.top-nav` MUST render as a full-width bar (left edge ~16px, right edge ~16px from viewport edges) directly under the 56px-tall header. Items stack vertically, left-aligned, with comfortable tap targets (≥10px vertical padding per link).
- **REQ-F-005:** When the dropdown is open, the hamburger icon MUST switch from the three-line glyph to an ✕ glyph, and the button's `aria-expanded` MUST be `"true"` and `aria-label` MUST be `"Close menu"`. When closed, the inverse (`aria-expanded="false"`, `aria-label="Open menu"`).
- **REQ-F-006:** The dropdown MUST close when (a) any link inside it is clicked, (b) the user clicks anywhere outside the nav or the hamburger, or (c) the user presses Escape.
- **REQ-F-007:** Below 640px, the `.brand-tag` ("labs") MUST be hidden to keep the brand from competing with the new icons for horizontal space.
- **REQ-F-008:** The theme toggle MUST continue to function identically at every viewport width, in both light and dark modes.
- **REQ-F-009:** The sticky-header-shadow behavior (`header.top.scrolled` class flip on `window.scrollY > 8`) MUST be unchanged.
- **REQ-F-010:** The Reference link's `data-activate-tab="reference"` behavior MUST be unaffected — clicking it from the dropdown still scrolls to `#examples` and activates the reference tab.
- **REQ-F-011:** At ≤640px viewport width, the installer bar (`.install-tabs`) MUST wrap to exactly two rows: `macOS` / `Linux` / `Windows` on row 1 and `Homebrew` / `pipx` / `npm` on row 2, in that order. Each row contains three tabs of equal width.
- **REQ-F-012:** At ≤640px the installer bar MUST stretch to the full content width of its parent (`.install` container, max 640px), so the two rows have generous horizontal space rather than the desktop `inline-flex` content-width.
- **REQ-F-013:** Tab labels (`macOS`, `Linux`, `Windows`, `Homebrew`, `pipx`, `npm`) and their leading icons MUST remain visible and readable at every supported mobile width. No icon-only mode, no horizontal scrolling, no hidden labels.
- **REQ-F-014:** At ≥641px the installer bar MUST render identically to today — single `inline-flex` row, content-width, centered.
- **REQ-F-015:** Each tab's tap target MUST be at least 36px tall on mobile. Existing `padding: 6px 12px` plus a 14px line-height yields ~32px; on mobile the padding MUST grow (e.g. to `8px 12px`) so the target meets the threshold.
- **REQ-F-016:** Active-tab visual treatment (`.install-tab.active` — raised background and box shadow) MUST be preserved at every viewport width. Click/tap behavior (`data-os`-driven content swap) MUST be unchanged.

### Non-Functional Requirements

- **REQ-NF-001:** Zero new files, zero new dependencies, zero new build steps. All changes are inside `index.html` (at the worktree root, which corresponds to the `gh-pages` branch layout).
- **REQ-NF-002:** No horizontal scrollbar at 320px viewport width on the header.
- **REQ-NF-003:** Vanilla CSS + ~20 lines of vanilla JS appended to the existing `<script>` block (no IIFE wrapper, matching the file's existing style).
- **REQ-NF-004:** Accessibility: hamburger is a real `<button>`; uses `aria-controls`, `aria-expanded`, `aria-label`; Escape closes; SVG glyphs are `aria-hidden`.
- **REQ-NF-005:** Cross-browser sanity: must render correctly in Safari iOS and Chrome DevTools' iPhone SE preset.
- **REQ-NF-006:** Installer-bar mobile reflow MUST live inside the SAME existing `@media (max-width: 640px)` block introduced for the header. No new breakpoint, no new top-level media query.

## Technical Considerations

### Files touched

- `index.html` (worktree root) — the only file modified.

### Insertion points (current line numbers in this branch)

1. **CSS** — appended after the existing header rules. Look for the line `[data-theme="dark"] .theme-toggle .moon { display: block; }` (currently near line 254) and insert the new CSS block immediately after the rules block it belongs to, before the comment `/* ---------- Hero ---------- */` (currently line 255).
2. **HTML** — modifications inside `<header class="top">` (currently lines 950–973):
   - Add `id="topNav"` to `<nav class="top-nav">` (currently line 957).
   - Insert a new `<button class="icon-btn nav-toggle" id="navToggle">` **after** the theme toggle button's closing `</button>` (currently line 971), so document order becomes brand → nav → theme-toggle → nav-toggle. This matches the "right of theme toggle" layout.
3. **JS** — appended to the existing `<script>` block. Look for the marker `// ---------- Sticky header shadow ----------` (currently line 1562). Insert the new JS block immediately after the `onScroll();` call (currently line 1566) and before the `// ---------- Toast ----------` comment (currently line 1568).

NOTE: Line numbers can shift if the file is edited concurrently. Implementers MUST locate insertion points by textual anchor (the comments and unique markup snippets named above), not by raw line number.

### Concrete CSS to add

```css
/* ---------- Mobile nav ---------- */
.nav-toggle { display: none; }
.nav-toggle .close { display: none; }
.nav-toggle[aria-expanded="true"] .open { display: none; }
.nav-toggle[aria-expanded="true"] .close { display: block; }

@media (max-width: 640px) {
  header.top .inner { gap: 8px; }
  .brand-tag { display: none; }
  .nav-toggle { display: inline-flex; }

  nav.top-nav {
    position: absolute;
    top: 56px;
    left: 16px;
    right: 16px;
    flex-direction: column;
    align-items: stretch;
    gap: 2px;
    padding: 8px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: var(--shadow-overlay);
    display: none;
  }
  nav.top-nav.open { display: flex; }
  nav.top-nav a { padding: 10px 12px; font-size: 14px; }
}
```

Notes:
- `nav.top-nav` keeps its desktop `margin-left: auto` — that rule is harmless once `position: absolute` takes over on mobile.
- `left: 16px; right: 16px` produces the full-width bar effect (16px gutter from each viewport edge).
- Reuses existing `.icon-btn` styling (32×32, border, hover) — no new button styling needed.

### Concrete CSS to add (installer bar)

Add the following rules INSIDE the same `@media (max-width: 640px)` block that the header rules use. Place them after the `nav.top-nav a { … }` rule so the breakpoint reads top-down: header rules first, installer rules second.

```css
  .install-tabs {
    display: flex;
    width: 100%;
    flex-wrap: wrap;
    margin: 0 0 14px;
  }
  .install-tab {
    flex: 1 1 calc(33.333% - 2px);
    justify-content: center;
    padding: 8px 12px;
  }
```

Notes:
- `flex: 1 1 calc(33.333% - 2px)` gives each tab a basis of one-third minus half the `gap: 2px` between siblings, which forces exactly 3 tabs per row. The 6 buttons therefore wrap into rows of `[macOS][Linux][Windows]` and `[Homebrew][pipx][npm]` in document order — no markup change needed.
- `display: flex` (not `inline-flex`) plus `width: 100%` replaces the desktop `inline-flex` + `margin: 0 auto` centering with a full-width stretch inside `.install` (which itself has `max-width: 640px; margin: 0 auto;` — see existing rule).
- `padding: 8px 12px` raises each tap target from ~32px to ~38px tall.
- `.install-tab.active` styling, `.install-tab svg` sizing, and the `data-os` JS handler all remain unchanged — they apply identically to the wrapped layout.

### Concrete HTML to add

Inside `<header class="top">`, after the existing `<button class="icon-btn theme-toggle" id="themeToggle">…</button>` closing tag, insert:

```html
<button class="icon-btn nav-toggle" id="navToggle" aria-label="Open menu" aria-controls="topNav" aria-expanded="false">
  <svg class="open" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    <path d="M3 6h18M3 12h18M3 18h18"></path>
  </svg>
  <svg class="close" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    <path d="M6 6l12 12M6 18L18 6"></path>
  </svg>
</button>
```

And add `id="topNav"` to the existing `<nav class="top-nav">` opening tag.

### Concrete JS to add

Appended to the existing `<script>` block, immediately after the Sticky-header-shadow section's `onScroll();` call:

```js
// ---------- Mobile nav toggle ----------
const navToggle = document.getElementById('navToggle');
const topNav = document.getElementById('topNav');
if (navToggle && topNav) {
  const closeNav = () => {
    topNav.classList.remove('open');
    navToggle.setAttribute('aria-expanded', 'false');
    navToggle.setAttribute('aria-label', 'Open menu');
  };
  navToggle.addEventListener('click', (e) => {
    e.stopPropagation();
    const open = topNav.classList.toggle('open');
    navToggle.setAttribute('aria-expanded', open ? 'true' : 'false');
    navToggle.setAttribute('aria-label', open ? 'Close menu' : 'Open menu');
  });
  topNav.addEventListener('click', (e) => {
    if (e.target.tagName === 'A') closeNav();
  });
  document.addEventListener('click', (e) => {
    if (!topNav.contains(e.target) && !navToggle.contains(e.target)) closeNav();
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') closeNav();
  });
}
```

Flat style, no IIFE — matches the existing theme-toggle and sticky-header-shadow blocks above it.

### Edge cases and risks

- **Sticky header z-index:** `header.top` has `z-index: 50`. The absolutely-positioned `nav.top-nav` is a child of the header, so it inherits the stacking context and will render above page content.
- **Reference tab click:** the close-on-A handler triggers BEFORE the existing tab-activation handler (or in tandem). Verify the Reference link still activates the reference tab AND closes the menu after click. Order should not matter because both handlers fire on the same event independently.
- **Tap target size:** each link gets `padding: 10px 12px` plus the existing 14px font; total tap target ≈ 38px tall.
- **Installer-bar wrap order:** the visual two-row split relies on document order — the six `<button class="install-tab">` elements are already in the desired order (`macos`, `linux`, `windows`, `homebrew`, `pipx`, `npm`). No reordering required; flexbox `flex-wrap` will break after the third tab on every supported viewport because each tab is exactly one-third wide.
- **Installer-bar overflow at very narrow widths:** at 320px each tab gets ~107px of width (`(320 - container padding - margins - gap) / 3`). The widest label "Homebrew" plus the 14px icon plus 6px gap is well under that. No label is at risk of clipping.

## Acceptance Criteria

- [ ] At viewport widths 320 / 375 / 414 / 540 / 640 px, the header has no horizontal overflow and renders: brand (logo + `neo4j-cli`, no `labs` tag) on the left, theme toggle + hamburger button on the right.
- [ ] At 641 / 720 / 1024 / 1440 px the header is pixel-identical to the current production page (four inline nav links visible, `labs` tag visible, no hamburger).
- [ ] Clicking the hamburger opens a full-width dropdown anchored 16px from each viewport edge, directly under the 56px header, containing the four links stacked vertically with ≥10px vertical padding each.
- [ ] The hamburger icon switches to ✕ while open; `aria-expanded` is `"true"` while open and `"false"` while closed; `aria-label` is `"Close menu"` while open and `"Open menu"` while closed.
- [ ] Clicking any link inside the dropdown closes it and navigates (in-page anchor for Examples/Quickstart; tab activation for Reference; new tab for GitHub via existing `target="_blank"`).
- [ ] Clicking anywhere outside the nav and the hamburger closes the dropdown.
- [ ] Pressing Escape closes the dropdown.
- [ ] The theme toggle continues to switch theme correctly at every viewport width, in both light and dark modes.
- [ ] Scrolling toggles the `header.top.scrolled` class as before (border-bottom appears on scroll > 8px).
- [ ] `git diff` on the feature branch shows changes confined to three regions of `index.html` (CSS near the header rules, HTML inside `<header class="top">`, and JS inside the existing `<script>` block).
- [ ] At 320 / 375 / 414 / 540 / 640 px the installer bar renders as two full-width rows of three tabs each in the order `macOS / Linux / Windows` (row 1) and `Homebrew / pipx / npm` (row 2). All six labels and icons are visible. No horizontal scroll.
- [ ] At 641 / 720 / 1024 / 1440 px the installer bar renders as today: a single centered `inline-flex` row, content-width, six tabs side-by-side.
- [ ] Each tab on mobile is ≥36px tall (verified with DevTools box inspector). The active-tab background and box-shadow are visually unchanged. Clicking a tab still swaps the install command shown in `#installBlock`.

## Out of Scope

- A redesign of the desktop header or desktop installer bar.
- A backdrop / scrim overlay behind the dropdown.
- Animation on dropdown open/close.
- Any changes to `install.sh`, `install.ps1`, `llms.txt`, `MAINTAINING.md`, or non-header / non-installer-bar sections of `index.html`.
- Changes to the `data-activate-tab` mechanism, the Examples toggle, or any other interactive surface.
- Hiding installer-tab labels or swapping to a `<select>` dropdown on mobile.
- A new responsive breakpoint other than 640px.
- Changes to the `data-os` selection JS or the `installCommands` content swap.

## Open Questions

None — the three open questions from the planning phase were resolved:

1. Hamburger placement → **right of theme toggle** (far-right edge).
2. Dropdown style → **full-width bar under header** (16px gutters left and right).
3. Backdrop → **none** (click-outside / Escape / link-click only).
