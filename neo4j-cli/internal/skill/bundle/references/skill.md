# neo4j-cli skill

## Contents

- [neo4j-cli skill check](#neo4j-cli-skill-check)
- [neo4j-cli skill install](#neo4j-cli-skill-install)
- [neo4j-cli skill list](#neo4j-cli-skill-list)
- [neo4j-cli skill print](#neo4j-cli-skill-print)
- [neo4j-cli skill remove](#neo4j-cli-skill-remove)

Install agent skills for this CLI into supported AI agents

Install, remove, list, and check the per-binary agent-skill bundle. The bundle teaches AI agents (Claude Code, Cursor, Windsurf, etc.) how to drive this CLI.

Usage: `neo4j-cli skill`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | - | Format to print console output in, from a choice of [default, json, table, toon]. (agents: prefer toon) |

## neo4j-cli skill check

Check installed skills for version drift against this binary

Reads each installed SKILL.md frontmatter `version:` and compares to the running binary version. Exits non-zero on any drift; prints a per-agent table.

Usage: `neo4j-cli skill check`

Examples:

```
# Check installed skills for version drift (table output)
neo4j-cli skill check

# Check installed skills as JSON (machine-readable)
neo4j-cli skill check --format json

# Check installed skills in toon format
neo4j-cli skill check --format toon
```

## neo4j-cli skill install

Install the skill bundle into supported AI agents

Without an argument, installs into every detected agent. With an [agent] argument (case-insensitive), installs into that single agent. Unknown agent names exit non-zero with the list of valid names.

Supported agents: claude-code, cursor, windsurf, copilot, antigravity, gemini-cli, cline, codex, conductor, pi, opencode, junie

Usage: `neo4j-cli skill install [agent]`

Examples:

```
# Install the skill into every detected agent
neo4j-cli skill install --rw

# Install the skill into a single agent (case-insensitive name)
neo4j-cli skill install claude-code --rw

# Install and emit the result as JSON (machine-readable)
neo4j-cli skill install --format json --rw
```

## neo4j-cli skill list

List supported agents and per-agent install state

Usage: `neo4j-cli skill list`

Examples:

```
# List supported agents as a table
neo4j-cli skill list

# List supported agents as JSON (machine-readable)
neo4j-cli skill list --format json

# List supported agents in toon format
neo4j-cli skill list --format toon
```

## neo4j-cli skill print

Print the embedded SKILL.md to stdout

Writes the bundled SKILL.md verbatim to stdout so you can preview the skill markdown before running `skill install`. The {{VERSION}} placeholder is left literal; substitution happens at install time.

Usage: `neo4j-cli skill print`

Examples:

```
# Print the embedded SKILL.md to stdout
neo4j-cli skill print

# Save the embedded SKILL.md to a file for review
neo4j-cli skill print > skill-preview.md

# Print the embedded SKILL.md (--format is accepted for parity but ignored — output is always raw markdown)
neo4j-cli skill print --format json
```

## neo4j-cli skill remove

Remove the installed skill bundle

Without an argument, removes from every detected agent. With an [agent] argument (case-insensitive), removes from that single agent. Idempotent: a second run on a clean target is a no-op.

Supported agents: claude-code, cursor, windsurf, copilot, antigravity, gemini-cli, cline, codex, conductor, pi, opencode, junie

Usage: `neo4j-cli skill remove [agent]`

Examples:

```
# Remove the skill from every detected agent
neo4j-cli skill remove --rw

# Remove the skill from a single agent (case-insensitive name)
neo4j-cli skill remove claude-code --rw

# Remove and emit the result as JSON (machine-readable)
neo4j-cli skill remove --format json --rw
```

