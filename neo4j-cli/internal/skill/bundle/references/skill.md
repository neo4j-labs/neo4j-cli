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

Install a skill bundle into supported AI agents

Without a positional, installs the embedded self-skill into every detected agent. With a [skill-name] positional, installs that named skill (self-skill or a curated catalog skill from github.com/neo4j-contrib/neo4j-skills). Use --all to install the self-skill plus every catalog entry, --agent <name> (case-insensitive) to scope to one agent, and --refresh to force a network fetch of the catalog before installing. Passing an agent name as the positional is a hard error — use --agent <name> instead.

Supported agents: claude-code, cursor, windsurf, copilot, antigravity, gemini-cli, cline, codex, pi, opencode, junie

Usage: `neo4j-cli skill install [skill-name] [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--agent` | string | - | Restrict install to a single agent (case-insensitive). See --help for supported agents. |
| `--all` | bool | false | Install the self-skill plus every curated catalog skill. |
| `--refresh` | bool | false | Force a network refresh of the catalog before installing. |

Examples:

```
# Install the embedded self-skill into every detected agent
neo4j-cli skill install --rw

# Install a curated catalog skill into every detected agent
neo4j-cli skill install neo4j-cypher-skill --rw

# Install the self-skill plus every catalog skill into every detected agent
neo4j-cli skill install --all --rw

# Force a catalog refresh before installing
neo4j-cli skill install neo4j-cypher-skill --refresh --rw

# Install the self-skill into a single agent
neo4j-cli skill install --agent claude-code --rw

# Install and emit the result as JSON (machine-readable)
neo4j-cli skill install --format json --rw
```

## neo4j-cli skill list

List skills × agents and per-row install state

Lists one row per (skill × agent) combining the embedded self-skill (always first) with every curated catalog skill from the cached plugin.json. Columns: skill, source, agent, detected, installed, installed_version, available_version, status. Auto-refreshes the catalog cache on 24h staleness when network is available; otherwise shows cached content. On a cold cache only self-skill rows are listed and a hint is printed to stderr pointing at `skill refresh`. Use --refresh to force a network fetch.

Usage: `neo4j-cli skill list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--refresh` | bool | false | Force a network refresh of the catalog before listing. |

Examples:

```
# List skills × agents (table)
neo4j-cli skill list

# List as JSON (machine-readable)
neo4j-cli skill list --format json

# List in toon format
neo4j-cli skill list --format toon

# Force a catalog refresh before listing
neo4j-cli skill list --refresh
```

## neo4j-cli skill print

Print a skill's SKILL.md to stdout

Writes the SKILL.md for the named skill verbatim to stdout. Defaults to the embedded self-skill when no positional is supplied. The {{VERSION}} placeholder in the self-skill bundle is left literal; substitution happens at install time. Passing an agent name as the positional is a hard error — use the --agent flag on install/remove instead.

Usage: `neo4j-cli skill print [skill-name]`

Examples:

```
# Print the embedded self-skill SKILL.md to stdout
neo4j-cli skill print

# Print the self-skill explicitly by canonical name
neo4j-cli skill print self

# Save the embedded SKILL.md to a file for review
neo4j-cli skill print > skill-preview.md
```

## neo4j-cli skill remove

Remove an installed skill bundle

Removes the named skill (self-skill or catalog skill) from every detected agent. Use --agent <name> (case-insensitive) to scope the removal to one agent. Use --all to remove every curated catalog skill from every detected agent — the embedded self-skill is preserved. Passing 'self' (or the binary-name alias) removes the self-skill and prints a reinstall hint. Idempotent: a name with no installation present exits zero. Passing an agent name as the positional is a hard error — use --agent <name> instead. --all reads only the cached catalog; with no cache it is a no-op.

Supported agents: claude-code, cursor, windsurf, copilot, antigravity, gemini-cli, cline, codex, pi, opencode, junie

Usage: `neo4j-cli skill remove [skill-name] [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--agent` | string | - | Restrict remove to a single agent (case-insensitive). See --help for supported agents. |
| `--all` | bool | false | Remove every curated catalog skill (self-skill preserved). |

Examples:

```
# Remove the self-skill from every detected agent
neo4j-cli skill remove self --rw

# Remove a curated catalog skill from every detected agent
neo4j-cli skill remove neo4j-cypher-skill --rw

# Remove every catalog skill (self-skill is preserved)
neo4j-cli skill remove --all --rw

# Remove the self-skill from a single agent
neo4j-cli skill remove self --agent claude-code --rw

# Remove and emit the result as JSON (machine-readable)
neo4j-cli skill remove self --format json --rw
```

