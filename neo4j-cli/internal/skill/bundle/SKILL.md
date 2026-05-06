---
name: neo4j-cli
description: Use this skill when the user wants to interact with Neo4j via the neo4j-cli command-line tool — the canonical CLI for running Cypher and managing Aura from a terminal.

TRIGGER when: run, execute, or pipe Cypher against any Neo4j database from a terminal/shell/bash; run Cypher given a bolt/neo4j+s:// URI with user/password from a shell; introspect schema, labels, indexes, or constraints via CLI; provision, list, get, create, delete, or resize Aura instances or tenants; set up, add, remove, list, or get Aura API credentials or keys from CLI; install, remove, uninstall, list, or check the neo4j-cli skill in Claude Code, Cursor, or another agent (e.g. "remove the neo4j-cli skill from my agent" → `neo4j-cli skill remove`); user mentions neo4j-cli, aura-cli, `neo4j query`, `aura instance`, or `neo4j credential`.

SKIP: Cypher syntax/semantics questions; graph data modeling; Neo4j driver code (Python/Java/JS/Go); Docker/Kubernetes/kubectl; other databases (Postgres, Redis); generic shell tasks; Neo4j Browser UI.

CLI: `query` (run Cypher via HTTP API, `--schema` flag, pipe stdin, env file with connection details), `aura` (instance/tenant/deployment), `credential` (Aura API client-id+secret), `skill` (install/remove/list/check in any AI agent).
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
