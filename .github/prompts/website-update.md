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
git show <sha> -- README.md AGENTS.md neo4j-cli/app/app.go neo4j-cli/internal/subcommands/ 'neo4j-cli/aura/internal/subcommands/**' 'common/skill/**'
```
Focus on changes that affect what a developer reading the page would need to know.

### 2b. Check for coverage gaps

List every top-level command tree that ships in the binary. The authoritative source is `neo4j-cli/app/app.go` (`cmd.AddCommand(…)` calls) and README.md's command surface. Then compare against:
- the `data-tab` values in `gh-pages/index.html` (example tabs)
- the `<!-- BEGIN:docs:<name> -->` markers in the reference accordion

For each command tree that has no example tab **and** no reference section, decide whether it warrants coverage (see step 5c threshold below). Record the gap so you act on it in step 5c.

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
- Add new sections or features **not yet shipped** (unreleased, on open PRs, or behind a flag)
- Change design, layout, or copy style
- Add marketing language or speculation
- Remove working examples unless the command was deleted

New tabs and reference sections for **shipped** features belong in step 5c, not here.

### 5c. Add coverage for newly shipped command trees

For each gap recorded in step 2b, apply the following threshold and templates.

**Threshold for a new example tab** — add a tab when the command tree:
- Has 4 or more distinct leaf commands, **or** represents a self-contained workflow (create → use → delete), **and**
- The workflow is meaningfully different from existing tabs (not just an alias or thin wrapper)

If only the reference section is warranted (fewer commands, or the workflow is too credential-heavy to demo), skip the tab and add the reference section only.

**Naming convention**
- Cloud-hosted services: lowercase product name (`aura`)
- Local runtimes: `<Name> (local)` in the reference section title; tab label-tag `local` and tab `data-tab` matches the command name (e.g. `data-tab="docker"` for `neo4j-cli docker …`)
- Follow the same pattern for any future local runtime: Desktop → `data-tab="desktop"`, reference title `Desktop (local)`

**Insertion points**
- New tab `<button>` elements: insert before the `reference` tab button (the last tab in the row)
- New `<div class="example-pane">` elements: insert before the `reference` pane
- New `paneCommands` key: add alongside the existing keys in the JS object
- New `panePrompts` key: add alongside the existing keys in the JS object
- New reference accordion section: insert before the `<!-- BEGIN:docs:write-gate -->` section

**Tab button template**
```html
<button class="example-tab" data-tab="TABNAME" role="tab">
  <span>TABNAME</span>
  <span class="label-tag">LABEL</span>  <!-- e.g. "local" or "cloud" -->
</button>
```

**Example pane template**
```html
<div class="example-pane" data-tab="TABNAME">
  <div class="pane-header">
    <span class="pane-title">TITLE</span>
    <button class="copy-mini" data-tab="TABNAME" title="Copy">⎘</button>
  </div>
  <pre><span class="agentic-line"><span class="agentic-comment">&gt; AGENT_PROMPT_TEXT</span>
<span class="agentic-loading">→ loading skill neo4j-cli</span></span>
<span class="ps">$</span> <span class="cmd">COMMAND_1</span> <span class="flag">--flag</span> <span class="str">"value"</span>
<span class="out">EXAMPLE_OUTPUT</span>
<span class="ps">$</span> <span class="cmd">COMMAND_2</span>
<span class="out">EXAMPLE_OUTPUT_2</span>
<span class="agentic-line"><span class="agentic-result">One-sentence summary of what the agent accomplished.</span></span></pre>
</div>
```

Replace `AGENT_PROMPT_TEXT` with a natural-language task description. Follow invariant 1 (no `> ` in the HTML — it is rendered as the literal `>` character via `&gt;` only in the attribute/text; the CSS adds the visual `> ` via the span). Use the real CLI commands with `--rw` on write operations. Wrap multi-line result output in `<span class="out">…</span>`.

**JS entries** — add inside `paneCommands` and `panePrompts`:
```js
TABNAME: 'neo4j-cli COMMAND --flag …',          // cli copy payload
TABNAME: 'AGENT_PROMPT_TEXT',                   // agentic copy payload
```

**Reference section template**
```html
<!-- BEGIN:docs:TABNAME -->
<div class="accordion-item">
  <button class="accordion-header" onclick="toggleAccordion(this)">
    SECTION_TITLE <span class="accordion-arrow">▶</span>
  </button>
  <div class="accordion-body">
    <pre>REFERENCE_CONTENT</pre>
  </div>
</div>
<!-- END:docs:TABNAME -->
```

`REFERENCE_CONTENT` should mirror the style of existing reference sections: a `# comment` line introducing the command group, then a block of `$ neo4j-cli …` invocations with brief inline comments. Show the most important flags; do not exhaustively list every option.

**After adding** any new tab or reference section, verify step 5b invariants still hold for the new pane.

### 5b. Enforce agentic-mode rendering invariants

The `gh-pages/index.html` page has two `agentic | cli` toggles — one above the Quickstart cards (`#quickstartSteps`) and one above the Examples grid (`#examplesSection`, the same `<div>` that also has `class="examples"`). Toggling adds/removes the `.cli-mode` class on the respective container. The id and class live on a single element, so write the selector as `#examplesSection:not(.cli-mode)` — `#examplesSection .examples` is a descendant selector that never matches anything. In agentic mode (the default — `.cli-mode` absent) the rendering MUST follow the four invariants below. These rules apply to BOTH toggles symmetrically; any change to one MUST be mirrored on the other in the same run. Audit the file, fix any drift, and verify visually before continuing.

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

**Invariant 4 — Copy actions are mode-aware.**
Any clickable copy target whose visible content differs between cli and agentic modes (`.step-cmd` in Quickstart and `.copy-mini` in Examples) MUST copy the cli payload (`data-copy` / `paneCommands`) in cli mode and the agent prompt in agentic mode. For `.step-cmd` the agent prompt is sourced from the descendant `.agentic-comment` text content with the leading `> ` (and any following whitespace) stripped before the clipboard write — do NOT duplicate the agent prompt as a separate `data-` attribute, read it from the DOM. `.step-cmd` elements that have no `.agentic-line` (e.g. the install commands in Quickstart steps 1–2) MUST fall back to `data-copy` in BOTH modes so install behaviour stays unchanged. Mirror the existing `.copy-mini` handler pattern: read `viewMode` at click time, branch on it, and emit a toast reflecting the actually-copied string. Symmetric treatment between Quickstart and Examples applies here too — any change to one handler MUST be mirrored on the other.

Ensure the following CSS block is present inside `gh-pages/index.html`'s `<style>` (add it if missing; reconcile if a weaker variant exists). Keep selectors exact:

```css
.agentic-loading {
    display: block;
    color: var(--fg-faint);
    font-style: italic;
    opacity: 0.85;
}

.agentic-result {
    display: block;
    color: var(--terminal-prompt);
}

#quickstartSteps:not(.cli-mode) .step-cmd:has(.agentic-line),
#quickstartSteps:not(.cli-mode) .step-cmd:has(.agentic-line) *:not(.agentic-line):not(.agentic-line *):not(.copy-icon) {
    color: var(--fg-faint) !important;
}

#examplesSection:not(.cli-mode) .example-pane pre:has(.agentic-line),
#examplesSection:not(.cli-mode) .example-pane pre:has(.agentic-line) *:not(.agentic-line):not(.agentic-line *):not(.out):not(.out *) {
    color: var(--fg-faint) !important;
}
```

**Optional pattern — agent result summary.**
An example pane MAY end with a one-sentence agent-side wrap-up describing what the run accomplished. Use this markup so it inherits the dim exemption and cli-mode hiding:
```html
<span class="agentic-line"><span class="agentic-result">One-sentence summary of the agent's outcome.</span></span>
```
Keep it under ~75 characters so it doesn't horizontally overflow the `<pre>`. No `> ` prefix — that's the prompt convention; `.agentic-result` is the answer.

**Optional pattern — high-contrast multi-line CLI result.**
When a `$` command's result is multi-line ASCII art / structured output (e.g. `:schema`'s `▸ section` blocks), wrap the whole result in `<span class="out">…</span>` so it stays sharp in agentic mode like the table-art results in other panes. Without the `.out` wrapper the dim sweep folds the result into the grey command block, which is misleading because real CLI output is not dimmed in the terminal.

**Preserve existing `.cli-mode` behaviour.**
Do NOT weaken or remove any existing `.cli-mode` rules. Adding `.cli-mode` to a toggle container MUST continue to restore full-color syntax highlighting and hide `.agentic-line` exactly as today. The `:not(.cli-mode)` dim sweep above is purely additive — it kicks in only when the toggle is in agentic mode, and the existing `.cli-mode` rules override it when the toggle flips.

**Symmetric treatment.**
Quickstart (`#quickstartSteps`) and Examples (`#examplesSection`) MUST follow all four invariants. If you adjust the prompt prefix, the loading line, the CSS, or the copy-action handlers for one section, mirror the same change to the other in the same edit pass.

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
