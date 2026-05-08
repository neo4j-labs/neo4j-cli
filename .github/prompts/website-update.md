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
