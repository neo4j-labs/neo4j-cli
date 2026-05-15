# PRD: Mobile-friendly top menu for neo4j.sh

## Overview

The landing page at https://neo4j.sh (served from the `gh-pages` branch, file `index.html` at repo root of that branch) has no responsive rules for its header. At iPhone-class widths (~375px) the inline row `brand + 4 nav links + theme toggle` is well wider than the viewport — content overflows horizontally or links get crushed.

This feature adds a single mobile breakpoint (`@media (max-width: 640px)`) that collapses the four nav links (`Examples` / `Reference` / `Quickstart` / `GitHub`) behind a hamburger button. When opened, the nav becomes a full-width bar that drops down under the header. The hamburger sits at the far right edge (right of the existing theme toggle). The `labs` brand tag is hidden below the breakpoint to give the brand room to breathe. Desktop layout (≥641px) is untouched.

## Goals

- Eliminate horizontal overflow of the top menu on viewports as narrow as 320px.
- Keep all four nav destinations (Examples, Reference, Quickstart, GitHub) reachable on mobile.
- Preserve the existing desktop header layout, fonts, and interactions exactly.
- Keep the change self-contained in `index.html` (this branch is rooted on `gh-pages`, where `index.html` lives at repo root). No new files, no build step.
- Keep the page accessible: hamburger is a `<button>` with `aria-controls`, `aria-expanded`, dynamic `aria-label`, Escape-to-close, and click-outside-to-close.

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

### Non-Functional Requirements

- **REQ-NF-001:** Zero new files, zero new dependencies, zero new build steps. All changes are inside `index.html` (at the worktree root, which corresponds to the `gh-pages` branch layout).
- **REQ-NF-002:** No horizontal scrollbar at 320px viewport width on the header.
- **REQ-NF-003:** Vanilla CSS + ~20 lines of vanilla JS appended to the existing `<script>` block (no IIFE wrapper, matching the file's existing style).
- **REQ-NF-004:** Accessibility: hamburger is a real `<button>`; uses `aria-controls`, `aria-expanded`, `aria-label`; Escape closes; SVG glyphs are `aria-hidden`.
- **REQ-NF-005:** Cross-browser sanity: must render correctly in Safari iOS and Chrome DevTools' iPhone SE preset.

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

## Out of Scope

- A redesign of the desktop header.
- A backdrop / scrim overlay behind the dropdown.
- Animation on dropdown open/close.
- Any changes to `install.sh`, `install.ps1`, `llms.txt`, `MAINTAINING.md`, or non-header sections of `index.html`.
- Changes to the `data-activate-tab` mechanism, the Examples toggle, or any other interactive surface.
- A new responsive breakpoint other than 640px.

## Open Questions

None — the three open questions from the planning phase were resolved:

1. Hamburger placement → **right of theme toggle** (far-right edge).
2. Dropdown style → **full-width bar under header** (16px gutters left and right).
3. Backdrop → **none** (click-outside / Escape / link-click only).
