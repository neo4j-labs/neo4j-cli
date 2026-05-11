You are updating the neo4j.sh landing page to keep it accurate with the latest neo4j-cli source.

## Context

The repo has two relevant branches checked out:
- `.` (current directory) — the `main` branch with the CLI source code
- `gh-pages/` — the website files served at https://neo4j.sh/

The website is a minimal developer landing page. Read `gh-pages/MAINTAINING.md` before making any changes — it describes the design intent and what must never be added.

## Your task

### 1. Find commits to review

Read `gh-pages/last-sync-sha.txt` to get the last-processed commit SHA.
Run:
```
git log <last-sha>..HEAD --oneline
```
If the file is missing or the SHA is invalid, use the last 20 commits:
```
git log --oneline -20
```

### 2. Understand what changed

For each commit that looks user-visible (new commands, removed commands, changed flags, changed defaults, new agent support, version bumps), look at the diff:
```
git show <sha> -- README.md AGENTS.md neo4j-cli/app/app.go 'neo4j-cli/aura/internal/subcommands/**' 'common/skill/**'
```
Focus on changes that affect what a developer reading the page would need to know.

### 3. Copy the install scripts

Copy the latest install scripts from main into gh-pages so the website always serves them:
```
cp distribution/installation-scripts/install-neo4j-cli.sh gh-pages/install.sh
cp distribution/installation-scripts/install-neo4j-cli.ps1 gh-pages/install.ps1
```

### 4. Read the current website

Read `gh-pages/index.html` and `gh-pages/llms.txt`.

### 5. Make surgical updates

Edit only what is factually wrong or out of date. Specifically:
- Installer version numbers (pipx and npm install commands)
- Command names or flags shown in examples
- Agent names supported by `skill install`
- Default values or connection precedence rules
- The `--rw` requirement for write operations

Do **not**:
- Add new sections or features not yet shipped
- Change design, layout, or copy style
- Add marketing language or speculation
- Remove working examples unless the command was deleted

### 5b. Enforce agentic-mode rendering invariants

The `gh-pages/index.html` page has two `agentic | cli` toggles — one above the Quickstart cards (`#quickstartSteps`) and one above the Examples grid (`#examplesSection .examples`). Toggling adds/removes the `.cli-mode` class on the respective container. In agentic mode (the default — `.cli-mode` absent) the rendering MUST follow the three invariants below. These rules apply to BOTH toggles symmetrically; any change to one MUST be mirrored on the other in the same run. Audit the file, fix any drift, and verify visually before continuing.

**Invariant 1 — Agent prompt prefix is `> `, not `# `.**
Every `.agentic-comment` span MUST start with `> ` (greater-than U+003E + ASCII space). Replace any `# ` prefix you find. The `.agentic-comment` color stays bound to `var(--terminal-prompt)` (terminal-green) — do not change that binding. The agent prompt is the visual hero in agentic mode.

**Invariant 2 — One `→ loading skill neo4j-cli` line per agent prompt.**
Immediately after every `.agentic-comment` span, inside the SAME `.agentic-line` wrapper, there MUST be exactly one:
```html
<span class="agentic-loading">→ loading skill neo4j-cli</span>
```
The literal text uses the Unicode rightwards arrow `→` (U+2192), a single ASCII space, then `loading skill neo4j-cli`. No ellipsis (`…` or `...`), no trailing dots, no `->` ASCII fallback. Exactly ONE such line per agent prompt, even when the prompt's block expands into multiple `$` sub-commands below — only the first command line gets a loading cue above it, sharing the same `.agentic-line`.

**Invariant 3 — Dim sweep below the prompt.**
The shell command block under each agent prompt MUST render in a single low-contrast grey (`var(--fg-faint)`) when the parent toggle container lacks `.cli-mode`. The `$` prompt token is INCLUDED in the dim (no brighter accent — it merges with the rest of the block). The `.out` blocks (rendered table-art / result output) are EXCLUDED and keep their existing token colors. The `.agentic-line` content is also EXCLUDED so the green agent prompt and the loading cue stay visible. The quickstart copy icon (`.copy-icon`) is also EXCLUDED.

The dim sweep applies ONLY to blocks that contain an `.agentic-line`. A `.step-cmd` or example `<pre>` with no agent prompt (e.g. the install commands in Quickstart steps 1–2) MUST keep its full color — the `:has(.agentic-line)` qualifier on each selector enforces this. Do not drop the `:has(.agentic-line)` clause.

Ensure the following CSS block is present inside `gh-pages/index.html`'s `<style>` (add it if missing; reconcile if a weaker variant exists). Keep selectors exact:

```css
.agentic-loading {
    display: block;
    color: var(--fg-faint);
    font-style: italic;
    opacity: 0.85;
}

#quickstartSteps:not(.cli-mode) .step-cmd:has(.agentic-line),
#quickstartSteps:not(.cli-mode) .step-cmd:has(.agentic-line) *:not(.agentic-line):not(.agentic-line *):not(.copy-icon) {
    color: var(--fg-faint) !important;
}

#examplesSection .examples:not(.cli-mode) .example-pane pre:has(.agentic-line),
#examplesSection .examples:not(.cli-mode) .example-pane pre:has(.agentic-line) *:not(.agentic-line):not(.agentic-line *):not(.out):not(.out *) {
    color: var(--fg-faint) !important;
}
```

**Preserve existing `.cli-mode` behaviour.**
Do NOT weaken or remove any existing `.cli-mode` rules. Adding `.cli-mode` to a toggle container MUST continue to restore full-color syntax highlighting and hide `.agentic-line` exactly as today. The `:not(.cli-mode)` dim sweep above is purely additive — it kicks in only when the toggle is in agentic mode, and the existing `.cli-mode` rules override it when the toggle flips.

**Symmetric treatment.**
Quickstart (`#quickstartSteps`) and Examples (`#examplesSection .examples`) MUST follow all three invariants. If you adjust the prompt prefix, the loading line, or the CSS for one section, mirror the same change to the other in the same edit pass.

**Post-edit visual verification.**
After editing, open `gh-pages/index.html` in a browser (or render it mentally from the markup) and confirm that in default agentic mode the ONLY colored content above each `.out` block is (a) the `> ` agent prompt in terminal-green and (b) the italic faint `→ loading skill neo4j-cli` line; the `$` prompt and every command, flag, string, keyword below them render in the same grey (`var(--fg-faint)`). Also confirm that any code block WITHOUT an `.agentic-line` (Quickstart steps 1–2: `$ curl … | bash` and `$ neo4j-cli skill install --rw`) renders in full color — the `:has(.agentic-line)` scope must keep the dim off these. Toggle to `cli` mode and confirm full syntax highlighting returns everywhere and `.agentic-line` is hidden. Do not finish this run until all three states render correctly in both toggles.

### 6. Validate the examples

The `neo4j-cli` binary is already installed in this environment. Extract every shell command shown in `gh-pages/index.html` (inside `<pre>` blocks and `data-copy` attributes) and validate them.

For commands that don't need credentials (e.g. `--help`, `skill list`, `skill check`), run them directly.

For query commands, use the demo database available via environment variables that are already set:
```
NEO4J_URI=neo4j+s://demo.neo4jlabs.com
NEO4J_USERNAME=movies
NEO4J_PASSWORD=movies
NEO4J_DATABASE=movies
```

Run each query command and check the exit code. If a command fails because of a changed API (wrong subcommand name, removed flag, etc.), fix the example in `gh-pages/index.html` and `gh-pages/llms.txt` accordingly.

Skip commands that require real Aura credentials (e.g. `credential aura-client add`, `aura instance list`) — just note them as untested.

### 7. Update the sync marker

Write the current HEAD SHA to `gh-pages/last-sync-sha.txt`:
```
git rev-parse HEAD > gh-pages/last-sync-sha.txt
```

### 8. Print a summary

End your response with a brief summary of what you changed (or "No changes needed" if nothing required updating), preceded by the marker line:
```
<!-- SUMMARY_START -->
```
Include which commands passed validation, which failed and were fixed, and which were skipped.
