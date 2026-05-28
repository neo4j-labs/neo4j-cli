# neo4j-cli skill

## Contents

- [neo4j-cli skill check](#neo4j-cli-skill-check)
- [neo4j-cli skill install](#neo4j-cli-skill-install)
- [neo4j-cli skill list](#neo4j-cli-skill-list)
- [neo4j-cli skill print](#neo4j-cli-skill-print)
- [neo4j-cli skill refresh](#neo4j-cli-skill-refresh)
- [neo4j-cli skill remove](#neo4j-cli-skill-remove)

Install agent skills for this CLI into supported AI agents

Install, remove, list, and check the per-binary agent-skill bundle. The bundle teaches AI agents (Claude Code, Cursor, Windsurf, etc.) how to drive this CLI.

Usage: `neo4j-cli skill`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | - | Format to print console output in, from a choice of [default, json, table, toon]. (agents: prefer toon) |

## neo4j-cli skill check

Check installed skills for version drift

Inspects every installed skill across detected agents and compares its frontmatter `version:` against the source version (binary version for the self-skill, plugin.json version for catalog skills). Columns: skill, agent, installed_version, current_version, status where status ∈ ok | drift | unknown-version. Exits non-zero when any row is drift or unknown-version. Auto-refreshes the catalog cache on 24h staleness when network is available; --refresh forces a fetch.

Usage: `neo4j-cli skill check [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--refresh` | bool | false | Force a network refresh of the catalog before checking. |

Examples:

```
# Check installed skills for version drift (table)
neo4j-cli skill check

# Check installed skills as JSON (machine-readable)
neo4j-cli skill check --format json

# Check installed skills in toon format
neo4j-cli skill check --format toon

# Force a catalog refresh before checking
neo4j-cli skill check --refresh
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

Lists the embedded self-skill and curated catalog skills from the cached plugin.json. Default table/toon output is a compact two-section view: an 11-row self-skill matrix (columns: agent, detected, installed, installed_version, available_version, status) followed by an aggregated catalog section (columns: skill, available_version, status, installed_in). --format json keeps the flat per-(skill × agent) array shape for back-compat with script consumers. Auto-refreshes the catalog cache on 24h staleness when network is available; otherwise shows cached content. On a cold cache only the self-skill section renders and a hint is printed to stderr pointing at `skill refresh`. Use --refresh to force a network fetch.

Usage: `neo4j-cli skill list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--refresh` | bool | false | Force a network refresh of the catalog before listing. |

Examples:

```
# List skills × agents (table)
neo4j-cli skill list

# List as JSON (machine-readable, flat per-(skill × agent) array)
neo4j-cli skill list --format json

# List in toon format
neo4j-cli skill list --format toon

# Force a catalog refresh before listing
neo4j-cli skill list --refresh
```

## neo4j-cli skill print

Print a skill's SKILL.md to stdout

Writes the SKILL.md for the named skill verbatim to stdout. Defaults to the embedded self-skill when no positional is supplied; pass 'self' (or the binary-name alias) for the same effect. Pass a curated catalog skill name to print its cached SKILL.md — print is offline-only and will not fetch the catalog. Run 'neo4j-cli skill refresh' first if the catalog cache is missing. The {{VERSION}} placeholder in the self-skill bundle is left literal; substitution happens at install time. Passing an agent name as the positional is a hard error — use the --agent flag on install/remove instead.

Usage: `neo4j-cli skill print [skill-name]`

Examples:

```
# Print the embedded self-skill SKILL.md to stdout
neo4j-cli skill print

# Print the self-skill explicitly by canonical name
neo4j-cli skill print self

# Print a curated catalog skill's cached SKILL.md
neo4j-cli skill print neo4j-cypher-skill

# Emit the SKILL.md (raw markdown — --format json is accepted for parity with other read cmds)
neo4j-cli skill print --format json

# Save the embedded SKILL.md to a file for review
neo4j-cli skill print > skill-preview.md
```

## neo4j-cli skill refresh

Force a fresh download of the curated skill catalog

Forces a network fetch of the curated catalog `plugin.json` from github.com/neo4j-contrib/neo4j-skills. When the upstream version differs from the cached one, the repo tarball is re-downloaded and extracted into the local cache. On network failure with a usable cache the previous content is preserved and a warning is emitted to stderr; on network failure with no cache the command exits non-zero with a connectivity hint.

Usage: `neo4j-cli skill refresh`

Examples:

```
# Force a catalog refresh
neo4j-cli skill refresh --rw

# Emit the result as JSON (machine-readable)
neo4j-cli skill refresh --format json --rw

# Refresh and view result in toon format
neo4j-cli skill refresh --format toon --rw
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

