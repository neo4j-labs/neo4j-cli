# aura-cli skill

Install agent skills for this CLI into supported AI agents

Install, remove, list, and check the per-binary agent-skill bundle. The bundle teaches AI agents (Claude Code, Cursor, Windsurf, etc.) how to drive this CLI.

Usage: `aura-cli skill`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-f, --format` | string | - | Format to print console output in, from a choice of [default, json, table, toon] |

## aura-cli skill check

Check installed skills for version drift against this binary

Reads each installed SKILL.md frontmatter `version:` and compares to the running binary version. Exits non-zero on any drift; prints a per-agent table.

Usage: `aura-cli skill check`

## aura-cli skill install

Install the skill bundle into supported AI agents

Without an argument, installs into every detected agent. With an [agent] argument (case-insensitive), installs into that single agent. Unknown agent names exit non-zero with the list of valid names.

Usage: `aura-cli skill install [agent]`

## aura-cli skill list

List supported agents and per-agent install state

Usage: `aura-cli skill list`

## aura-cli skill print

Print the embedded SKILL.md to stdout

Writes the bundled SKILL.md verbatim to stdout so you can preview the skill markdown before running `skill install`. The {{VERSION}} placeholder is left literal; substitution happens at install time.

Usage: `aura-cli skill print`

## aura-cli skill remove

Remove the installed skill bundle

Without an argument, removes from every detected agent. With an [agent] argument (case-insensitive), removes from that single agent. Idempotent: a second run on a clean target is a no-op.

Usage: `aura-cli skill remove [agent]`

