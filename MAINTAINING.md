# Maintaining neo4j.sh

Guidance for future Claude Code sessions (and humans) editing the
`neo4j.sh` landing page. Read this before changing anything visible.

The page is **a single static HTML file**, `index.html`, plus a `fonts/`
folder. There is no build step, no framework, no bundler in the source —
the `neo4j-sh.bundled.html` artifact is generated, never edited by hand.
Everything you need to change lives in `index.html`.

---

## Design intent

This page is a **developer landing page for a CLI**. It is not a
marketing site. The reader is a developer who already knows what a graph
database is and wants to know:

1. What is the install command for my OS?
2. What does the tool actually do — show me real commands and output.
3. How do I get from zero to a working query?

The page answers those three questions in that order. Everything else is
secondary. **Resist adding sections that don't serve those questions.**

### Aesthetic principles

- **Terminal-first.** Monospace is the dominant typeface. The hero is a
  copyable command. Code blocks are first-class content, not decoration.
- **Calm, not loud.** Neo4j blue (`--accent: #018bff`) is used sparingly
  — primary action, single accent word in the hero, focus rings, link
  underlines. Most of the page is `--fg`/`--fg-muted`/`--bg-alt` neutrals.
  Resist adding gradients, glows, or secondary accent colors.
- **No filler.** Every section earns its place. If a section feels empty,
  fix it with composition — don't pad with placeholder copy, generic
  stats, or "trusted by" logos that aren't real.
- **Prefer real over decorative.** The terminal blocks show *real* CLI
  output formats (table format, JSON envelope, schema introspection).
  When the CLI's actual output changes, update these — don't invent
  output that looks plausible but isn't accurate.
- **No emoji.** This is a developer tool. Use Unicode glyphs sparingly
  (`✓ ▸ ●`) for status; never emoji.
- **No icons-for-icons-sake.** Icons appear only on the install OS tabs
  and the copy button. Don't sprinkle generic "feature icons" through
  the page.

### What the page must keep

These are load-bearing — don't remove without a real reason:

- **Hero install block** with OS auto-detect and tabs (macOS / Linux /
  Windows / npm). Default tab is set by `detectOS()` from the user agent.
- **Examples tabs** showing `aura`, `query`, `:schema`, `pipe`, `skill`
  — these mirror the actual command surface from the
  [neo4j-cli README](https://github.com/neo4j-labs/neo4j-cli#readme).
  When commands or flags change upstream, update these examples.
- **Quickstart steps** (1-2-3-4) walking from install → credential → list
  → query. The narrative must end at "you ran a Cypher query."
- **Copy buttons everywhere** a command appears. Every code block the
  user might want to run gets a copy affordance. The global ⌘C/Ctrl+C
  hint must keep working.

---

## Source of truth: the CLI itself

When updating example commands, the canonical reference is the upstream
[neo4j-cli README](https://github.com/neo4j-labs/neo4j-cli#readme).
Specifically:

- The binary is `neo4j-cli` (with the dash). Never `neo4j` alone, never
  `neo` — these are common AI hallucinations to watch for.
- Command surface today: `aura credential add`, `aura instance list`,
  `aura instance create`, `query`, `query :schema`, `skill install`,
  `skill list`, `skill check`, `skill remove`.
- Connection precedence: **flag → env var → `.env` → default**. Don't
  reorder.
- Default URI is `http://localhost:7474`, default username `neo4j`,
  default database `neo4j`. Password is prompted on TTY.
- Bolt-family URIs are auto-rewritten to HTTP; Aura hosts are forced to
  HTTPS:443. Mention this nuance only if a section is specifically about
  connection handling.
- JSON envelope shape: `{columns, rows, truncated, arrays_truncated}`.
  Don't invent extra fields.

If the upstream README adds new top-level commands (e.g. `import`,
`dataapi`, `deployment` — currently beta-gated), consider whether they
deserve a tab. Don't add tabs for commands that aren't shipped.

---

## How to change things safely

### Editing copy or commands

Open `index.html`, find the relevant `<div class="example-pane">` or
`<div class="step">`, edit the `<pre>` block. Keep the existing token
classes (`prompt`, `flag`, `string`, `comment`, `keyword`, `num`, `ok`,
`info`, `out`) — they're styled in the `<style>` block and removing them
breaks the syntax look.

When you change a command in a `<pre>`, **also update**:

1. The matching entry in the `paneCommands` JS object (used by the "copy"
   button on each example).
2. Any `data-copy="..."` attribute on quickstart `step-cmd` blocks.

If these get out of sync, the visible command and the copied command
diverge — that's the most embarrassing bug this page can ship.

### Adding a new example tab

1. Add a `<button class="example-tab">` to `.example-tabs` with a unique
   `data-tab` value.
2. Add a matching `<div class="example-pane" data-pane="...">` with the
   same value.
3. Add an entry to `paneCommands` in the script for the copy button.
4. Keep the total to 5 tabs or fewer. More than that, the tab strip
   wraps and the design breaks down — pick what to drop.

### Changing colors

CSS variables live at the top of the `<style>` block (`:root { ... }`).

- `--accent: #018bff` is Neo4j brand blue. Don't change it.
- `--fg`, `--fg-muted`, `--fg-faint`, `--bg`, `--bg-alt`, `--border`,
  `--border-strong` are the neutral ramp. If you adjust one, check the
  whole page — they're used everywhere.
- The code-token colors (`--code-string`, `--code-comment`, etc.) are
  tuned for the terminal blocks. Adjust together, not individually.

### Changing the install commands

The install commands in the hero come from the `installers` object in
the inline script. Each OS has a `prompt`, `html` (with token spans),
`raw` (plain text for the clipboard), and a `label`.

When the real install URL exists (`neo4j.sh/install`, `install.ps1`,
etc.), update both `html` and `raw`. They must stay in sync — `raw` is
what gets copied; `html` is what's shown.

### Adding sections

**Default answer: don't.** The page is intentionally short. If you think
a section is missing, ask:

- Does it answer one of the three reader questions above?
- Is the content *real* (a real feature, real command, real output)?
- Will it still be true in six months?

If yes to all three, add it after the existing sections, matching the
section header pattern (`section-eyebrow` + `section-title` +
`section-sub`). Otherwise leave it out.

---

## What to never add

- **Testimonials, "trusted by" logos, customer quotes.** This is a CLI;
  no one is on record yet, and fake logos are a trust-killer.
- **Animated hero illustrations.** The terminal *is* the illustration.
- **A pricing section.** The CLI is free; Aura's pricing lives on
  neo4j.com. Link out, don't reproduce.
- **Newsletter signup, cookie banners, analytics scripts.** Keep the
  page a single static file with zero third-party requests at runtime.
  (Fonts are self-hosted in `fonts/` for the same reason.)
- **Generic feature icons** (rocket, lightning, gear, brain). If a
  concept needs an icon, draw a specific one or skip it.
- **Marketing-speak.** "Empower," "unleash," "supercharge," "unlock,"
  "next-generation," "AI-powered." If you find these in copy, cut them.

---

## Verifying changes

After editing `index.html`:

1. Open it in a browser (no build step). Check that:
   - The install tabs all show the right command.
   - Each example tab's copy button copies the same command shown.
   - The ⌘C / Ctrl+C global shortcut still copies the install command.
   - The quickstart steps' copy buttons copy the right text.
   - Nothing wraps awkwardly at narrow widths (try 375px, 768px, 1280px).
2. Re-bundle for distribution if needed:
   - The bundled file `neo4j-sh.bundled.html` is generated; don't edit
     it. Regenerate it from `index.html` whenever you ship a change.

---

## File map

```
neo4j-sh/
├── index.html                  ← the page. Edit this.
├── neo4j-sh.bundled.html       ← generated single-file build. Don't edit.
├── fonts/                      ← self-hosted Public Sans + Fira Code.
└── MAINTAINING.md              ← this file.
```

---

## When in doubt

The page exists to get a developer from "I heard about neo4j-cli" to
"I just ran my first query" in under two minutes. If a change makes
that path longer or noisier, it's the wrong change.
