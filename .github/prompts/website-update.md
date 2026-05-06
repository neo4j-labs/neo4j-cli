You are updating the neo4j.sh landing page to keep it accurate with the latest neo4j-cli source.

## Context

The repo has two relevant branches checked out:
- `.` (current directory) — the `main` branch with the CLI source code
- `gh-pages/` — the website files served at https://neo4j.sh/

The website is a minimal developer landing page. Read `gh-pages/MAINTAINING.md` before making any changes — it describes the design intent and what must never be added.

## Your task

1. **Find commits to review**

   Read `gh-pages/last-sync-sha.txt` to get the last-processed commit SHA.
   Run:
   ```
   git log <last-sha>..HEAD --oneline
   ```
   If the file is missing or the SHA is invalid, use the last 20 commits:
   ```
   git log --oneline -20
   ```

2. **Understand what changed**

   For each commit that looks user-visible (new commands, removed commands, changed flags, changed defaults, new agent support, version bumps), look at the diff:
   ```
   git show <sha> -- README.md AGENTS.md neo4j-cli/app/app.go 'neo4j-cli/aura/internal/subcommands/**' 'common/skill/**'
   ```
   Focus on changes that affect what a developer reading the page would need to know.

3. **Read the current website**

   Read `gh-pages/index.html` and `gh-pages/llms.txt`.

4. **Make surgical updates**

   Edit only what is factually wrong or out of date. Specifically:
   - Installer version numbers (pipx and npm)
   - Command names or flags shown in examples
   - Agent names supported by `skill install`
   - Default values or connection precedence rules
   - The `--rw` requirement for write operations

   Do **not**:
   - Add new sections or features not yet shipped
   - Change design, layout, or copy style
   - Add marketing language or speculation
   - Remove working examples unless the command was deleted

5. **Update the sync marker**

   Write the current HEAD SHA to `gh-pages/last-sync-sha.txt`:
   ```
   git rev-parse HEAD > gh-pages/last-sync-sha.txt
   ```

6. **Print a summary**

   End your response with a brief summary of what you changed (or "No changes needed" if nothing required updating), preceded by the marker line:
   ```
   <!-- SUMMARY_START -->
   ```
