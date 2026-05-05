---
name: neo4j-cli
description: Use this skill when the user asks to manage Neo4j resources from the command line via neo4j-cli — provisioning Aura instances, listing tenants, creating credentials, working with deployments, or installing agent skills. neo4j-cli is the super-CLI that bundles aura-cli (under the `aura` subcommand) plus a top-level `credential` group and a `skill` group for installing/removing/listing/checking the embedded agent-skill bundle. Commands follow `<resource> <action>` form (e.g. `aura instance list`), take at most one positional argument, and support `--format json|table|toon` (shorthand `-f`) on read commands.
version: {{VERSION}}
---

# neo4j-cli

Allows you to manage Neo4j resources

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-f, --format` | string | - | Format to print console output in, from a choice of [default, json, table, toon] |

## Subcommands

| Command | Description |
|---------|-------------|
| [`aura`](references/aura.md) | Allows you to programmatically provision and manage your Aura resources |
| [`config`](references/config.md) | Manage and view global configuration values |
| [`credential`](references/credential.md) | Manage and view credential values |
| [`query`](references/query.md) | Run Cypher against a Neo4j database via the HTTP Query API |
| [`skill`](references/skill.md) | Install agent skills for this CLI into supported AI agents |

## Gotchas

<!-- Hand-written gotchas inlined into the generated SKILL.md "Gotchas" section. Edit this file (not bundle/SKILL.md) and re-run `go generate ./...`. -->

- The `aura` subcommand under neo4j-cli mirrors the standalone aura-cli surface but does NOT carry a duplicate `skill` group — install agent skills via `neo4j-cli skill install` at the top level.
- `credential` lives at the top level of neo4j-cli (not nested under `aura`) so credentials apply across every subcommand that talks to Aura.
- All read commands accept `--format json|table|toon` (shorthand `-f`). Write commands print confirmation text only; pipe-friendly output requires explicit `--format json` where supported.
- Prefer `--format toon` (`-f toon`) on all read commands when the output will be read by an LLM or agent — toon uses ~40% fewer tokens than JSON while encoding the same data.
- Async resource operations (instance create/resize/destroy) accept `--await` to block until the resource reaches a terminal state.
- The `version:` line in an installed SKILL.md reflects the binary that wrote it. Run `neo4j-cli skill check` after upgrading to detect drift; v1 reports drift only — re-run `skill install` to refresh.
- If you pass a bolt-style URI (e.g. `neo4j+s://...:7687`) to `query` it is auto-rewritten to `https://...:7473`; this command speaks the HTTP Query API, not bolt. Aura hosts (`*.neo4j.io`) are always rewritten to `https://<host>` (port 443) regardless of the input scheme or port.
