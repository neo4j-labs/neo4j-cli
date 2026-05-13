---
name: neo4j-cli
description: Runs Cypher and manages Neo4j Aura, Neo4j connection (dbms) credentials, and embedding-provider credentials from the terminal via the neo4j-cli CLI. Use when the user wants to execute or pipe Cypher against Neo4j, embed text inline as a Cypher parameter, introspect a schema, list/create/get/delete/provision/resize Aura instances or tenants, manage Aura/dbms/embed credentials, install/remove the neo4j-cli skill in an agent, or self-update the neo4j-cli binary via `neo4j-cli update`. Skip for Cypher syntax questions, graph data modeling, Neo4j drivers, Docker/Kubernetes, Neo4j Browser, or other databases.
version: {{VERSION}}
---

# neo4j-cli

Allows you to manage Neo4j resources

Allows you to manage Neo4j resources. Write operations require --rw. `neo4j-cli query` runs EXPLAIN first when --rw is not set and blocks statements classified as writes.

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-f, --format` | string | - | Format to print console output in, from a choice of [default, json, table, toon]. (agents: prefer toon) |
| `--rw` | bool | false | Allow write operations. Auto-applied in interactive terminals; required when running under an agent harness or non-interactive script. |

## Subcommands

| Command | Description |
|---------|-------------|
| [`agent-context`](references/agent-context.md) | Emit the full CLI shape as JSON for AI-agent discovery |
| [`aura`](references/aura.md) | Allows you to programmatically provision and manage your Aura resources |
| [`config`](references/config.md) | Manage and view global configuration values |
| [`credential`](references/credential.md) | Manage and view credential values |
| [`query`](references/query.md) | Run Cypher against a Neo4j database via the Bolt protocol |
| [`skill`](references/skill.md) | Install agent skills for this CLI into supported AI agents |
| [`update`](references/update.md) | Self-update the neo4j-cli binary |

## Tips & Gotchas

<!-- Hand-written tips & gotchas inlined into the generated SKILL.md "Tips & Gotchas" section. Edit this file (not bundle/SKILL.md) and re-run `go generate ./...`. -->

- **Before using `neo4j-cli query`, read [query-additions.md](query-additions.md) — required pre-reading covering schema-first workflow, parameters, embeddings, Cypher 25 vs 5, and tips.**
- The `aura` subcommand under neo4j-cli mirrors the standalone aura-cli surface but does NOT carry a duplicate `skill` group — install agent skills via `neo4j-cli skill install` at the top level.
- `credential` lives at the top level of neo4j-cli (not nested under `aura`) so credentials apply across every subcommand that talks to Aura.
- All read commands accept `--format json|table|toon` (shorthand `-f`). Write commands print confirmation text only; pipe-friendly output requires explicit `--format json` where supported.
- **Always pass `--format toon` (`-f toon`) on read commands** — toon uses ~40% fewer tokens than JSON while encoding the same data, so default to it for every list/get/show command. Only use `--format json` when piping into a JSON-aware tool that requires it; only use `--format table` when the user explicitly asks for a human-readable table.
- Async resource operations (instance create/resize/destroy) accept `--await` to block until the resource reaches a terminal state.
- The `version:` line in an installed SKILL.md reflects the binary that wrote it. Run `neo4j-cli skill check` after upgrading to detect drift; v1 reports drift only — re-run `skill install` to refresh.
- If you pass an HTTP-style URI (e.g. `http://host:7474`) to `query` it is auto-rewritten to `neo4j://host:7687` (and `https://` to `neo4j+s://host:7687`); this command speaks the Bolt protocol. Use `neo4j+ssc://` for self-signed certs.
- Write operations require `--rw` when running under an agent harness. The CLI auto-detects known agents (Claude Code, Codex, Cursor, Gemini CLI, Replit, …) via environment variables, so agents always need `--rw` for writes — interactive humans in a terminal do not. `neo4j-cli query run` additionally runs `EXPLAIN` over Bolt to detect write cypher when `--rw` is not set and blocks statements classified as writes before execution.
- Do NOT preemptively add `--rw`. Run write commands without it by default. If a command fails with `this command writes; pass --rw to allow it`, surface the error and ask the user once to confirm the write, then re-run with `--rw` — do not add it on your own.
- Use `--param NAME:embed=<text>` on `neo4j-cli query` to inject an embedding vector inline; the text is sent to the configured embedding provider and the resulting `[]float32` is bound to `$NAME` for both the EXPLAIN preflight and the real run. The sibling `neo4j-cli query :embed [text]` leaf computes a vector standalone (no Bolt connection opened).
- Embedding config (`--embed-provider`, `--embed-model`, `--embed-base-url`, `--embed-dimensions`) resolves with the same precedence as connection config: flag > OS env (`NEO4J_EMBED_*`) > `.env` walk-up > stored embed credential. API keys layer per provider: `OPENAI_API_KEY` / `HF_TOKEN` (per-provider) beats `NEO4J_EMBED_API_KEY` (generic) beats the stored credential. Ollama needs no API key.
- Linking dbms→embed: `credential dbms add --embed-credential <name>` or `credential dbms set-embed <dbms-name> [embed-name]` attaches an embed cred to a dbms cred so `query --credential <dbms-name> --param NAME:embed=...` picks up both connection and embed config in one selector. Removing an embed cred is non-cascading; stale links surface lazily at query time.
- Updating the CLI: run `neo4j-cli update` to self-update the binary in place. Use `neo4j-cli update check` to report whether a newer version is available without downloading (exits 1 when newer), and `--pre-releases` to opt into alpha/beta/rc tags (default is stable-only). When the binary lives under a known package-manager prefix (Homebrew, npm-global, pipx, uv tool) the command refuses to overwrite and prints the channel-correct upgrade command; pass `--force` to override. After a successful swap, any installed agent skill bundles are refreshed automatically — no manual `skill install` follow-up needed.
