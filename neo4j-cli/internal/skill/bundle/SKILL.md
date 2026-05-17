---
name: neo4j-cli
description: Runs Cypher, inspects the database schema / data model via `neo4j-cli query :schema`, manages Neo4j Aura, Neo4j connection (dbms) credentials, and embedding-provider credentials, and manages local Neo4j Docker containers (create / list / get / start / stop / delete, including ephemeral runs that emit a .env file consumable by `query --env`) from the terminal via the neo4j-cli CLI. Use when the user wants to execute or pipe Cypher against Neo4j, inspect/view/introspect the schema or generate a data model from the schema (labels, relationship types, properties) via `:schema`, embed text inline as a Cypher parameter, list/create/get/delete/provision/resize Aura instances or tenants, manage Aura/dbms/embed credentials, run / start / stop a local Neo4j Docker container, install/remove the neo4j-cli skill in an agent, or self-update the neo4j-cli binary via `neo4j-cli update`. Skip for Cypher syntax questions, Neo4j drivers, Kubernetes, Neo4j Browser, or other databases.
version: {{VERSION}}
---

# neo4j-cli

Allows you to manage Neo4j resources

Allows you to manage Neo4j resources. Write operations require --rw.

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | - | Format to print console output in, from a choice of [default, json, table, toon]. (agents: prefer toon) |
| `--rw` | bool | false | Allow write operations. Auto-applied in interactive terminals; required when running under an agent harness or non-interactive script. |

## Subcommands

| Command | Description |
|---------|-------------|
| [`agent-context`](references/agent-context.md) | Emit the full CLI shape as JSON for AI-agent discovery |
| [`aura`](references/aura.md) | Allows you to programmatically provision and manage your Aura resources |
| [`config`](references/config.md) | Manage and view global configuration values |
| [`credential`](references/credential.md) | Manage and view credential values |
| [`docker`](references/docker.md) | Manage local Neo4j containers via Docker |
| [`query`](references/query.md) | Run Cypher, inspect the database schema (:schema), and embed text against a Neo4j database via the Bolt protocol |
| [`skill`](references/skill.md) | Install agent skills for this CLI into supported AI agents |
| [`update`](references/update.md) | Self-update the neo4j-cli binary |

## Tips & Gotchas

<!-- Hand-written tips & gotchas inlined into the generated SKILL.md "Tips & Gotchas" section. Edit this file (not bundle/SKILL.md) and re-run `go generate ./...`. -->

- **Before generating ANY Cypher: run `neo4j-cli query :schema --format toon` first to discover the real labels, relationship types, and properties. Do not guess the schema.** Read [query-additions.md](query-additions.md) for the full schema-first workflow, parameters, embeddings, and Cypher 25 vs 5.
- The `aura` subcommand under neo4j-cli mirrors the standalone aura-cli surface but does NOT carry a duplicate `skill` group — install agent skills via `neo4j-cli skill install` at the top level.
- `credential` lives at the top level of neo4j-cli (not nested under `aura`) so credentials apply across every subcommand that talks to Aura.
- All read commands accept `--format json|table|toon`. Write commands print confirmation text only; pipe-friendly output requires explicit `--format json` where supported.
- **Always pass `--format toon` on read commands** — toon uses ~40% fewer tokens than JSON while encoding the same data, so default to it for every list/get/show command. Only use `--format json` when piping into a JSON-aware tool that requires it; only use `--format table` when the user explicitly asks for a human-readable table.
- Async resource operations (instance create/resize/destroy) accept `--wait` to block until the resource reaches a terminal state.
- The `version:` line in an installed SKILL.md reflects the binary that wrote it. Run `neo4j-cli skill check` after upgrading to detect drift; v1 reports drift only — re-run `skill install` to refresh.
- If you pass an HTTP-style URI (e.g. `http://host:7474`) to `query` it is auto-rewritten to `neo4j://host:7687` (and `https://` to `neo4j+s://host:7687`); this command speaks the Bolt protocol. Use `neo4j+ssc://` for self-signed certs.
- Write operations require `--rw` when running under an agent harness. The CLI auto-detects known agents (Claude Code, Codex, Cursor, Gemini CLI, Replit, …) via environment variables, so agents always need `--rw` for writes — interactive humans in a terminal do not. `neo4j-cli query run` additionally runs `EXPLAIN` over Bolt to detect write cypher when `--rw` is not set and blocks statements classified as writes before execution.
- Do NOT preemptively add `--rw`. Run write commands without it by default. If a command fails with `this command writes; pass --rw to allow it`, surface the error and ask the user once to confirm the write, then re-run with `--rw` — do not add it on your own.
- Use `--param NAME:embed=<text>` on `neo4j-cli query` to inject an embedding vector inline; the text is sent to the configured embedding provider and the resulting `[]float32` is bound to `$NAME` for both the EXPLAIN preflight and the real run. The sibling `neo4j-cli query :embed [text]` leaf computes a vector standalone (no Bolt connection opened).
- Embedding config (`--embed-provider`, `--embed-model`, `--embed-base-url`, `--embed-dimensions`) resolves with the same precedence as connection config: flag > OS env (`NEO4J_EMBED_*`) > `.env` walk-up > stored embed credential. API keys layer per provider: `OPENAI_API_KEY` / `HF_TOKEN` (per-provider) beats `NEO4J_EMBED_API_KEY` (generic) beats the stored credential. Ollama needs no API key.
- Linking dbms→embed: `credential dbms add --embed-credential <name>` or `credential dbms set-embed <dbms-name> [embed-name]` attaches an embed cred to a dbms cred so `query --credential <dbms-name> --param NAME:embed=...` picks up both connection and embed config in one selector. Removing an embed cred is non-cascading; stale links surface lazily at query time.
- Updating the CLI: run `neo4j-cli update` to self-update the binary in place. Use `neo4j-cli update check` to report whether a newer version is available without downloading (exits 1 when newer), and `--pre-releases` to opt into alpha/beta/rc tags (default is stable-only). When the binary lives under a known package-manager prefix (Homebrew, npm-global, pipx, uv tool) the command refuses to overwrite and prints the channel-correct upgrade command; pass `--force` to override. After a successful swap, any installed agent skill bundles are refreshed automatically — no manual `skill install` follow-up needed.
- Local Neo4j via Docker: `neo4j-cli docker` (`create` / `list` / `get` / `start` / `stop` / `delete`) shells out to the host `docker` CLI. Docker itself is the source of truth — managed containers carry the `org.neo4j.cli.managed=true` label plus a small set of metadata labels (edition, version, bolt-port, http-port, ephemeral); no separate state file is maintained. `docker create` defaults to the enterprise image with the evaluation license (`NEO4J_ACCEPT_LICENSE_AGREEMENT=eval`); pass `--accept-license` for the commercial license, or `--edition community` for the community image. Host ports default to 7474 (HTTP) and 7687 (Bolt); override with `--http-port` / `--bolt-port`. When the requested pair is taken, both ports are auto-incremented by the same offset until a free pair is found. If `--name` collides with an existing container or stored dbms credential, the chosen name is auto-suffixed (`<name>-1`, `<name>-2`, …) and the chosen name is logged to stderr. All `--rw`, `--format json|table|toon`, and `--wait` conventions from the aura tree apply equally here.

```sh
# Persistent flow: create stores a dbms credential, then query --credential connects with no further config
neo4j-cli docker create --name dev --wait --rw
neo4j-cli query --credential dev 'RETURN 1 AS n'
neo4j-cli docker delete dev --force --rw
```

- The generated password appears in `docker create`'s rendered output; redirects and pipes capture it — pass `--password <s>` to pick your own, or `--no-store-credential` to suppress storage and rendering.

- Persistent volume mounts: `--data-dir <host>` bind-mounts at `/data` (persist DB across `docker delete`), `--logs-dir <host>` at `/logs`, `--import-dir <host>` at `/import` (for `LOAD CSV`). Paths support `~` and `$VAR` expansion; missing dirs are created at mode 0o755. All three are incompatible with `--ephemeral`. Neo4j adjusts ownership of the mounted dirs at container startup, so they appear under the container's neo4j UID on the host after first start.

```sh
# Persist data on the host so it survives delete + recreate
neo4j-cli docker create --name dev --data-dir ~/neo4j-dev/data --rw
neo4j-cli docker delete dev --force --rw
neo4j-cli docker create --name dev --data-dir ~/neo4j-dev/data --rw  # reuses the same data
```

- Ephemeral runs: `neo4j-cli docker create --ephemeral` shells `docker run --rm`, skips credential persistence, and emits a `.env` blob (`NEO4J_URI` / `NEO4J_USERNAME` / `NEO4J_PASSWORD` / `NEO4J_DATABASE`) to stdout. With `--env-out-file <path>` the blob is written to that path (mode 0600) and stdout stays silent so it can be piped into `neo4j-cli query --env <path>`. The env-file write goes through a temp file in the same directory + atomic rename; a pre-existing symlink at the target path is replaced by a regular file (the symlink is not followed). Docker auto-removes the container when it stops — nothing to delete.

```sh
# Ephemeral flow: throwaway container + env-file consumed by query --env
neo4j-cli docker create --name tmp --ephemeral --env-out-file /tmp/n.env --wait --rw
neo4j-cli query --env /tmp/n.env 'RETURN 1 AS n'
neo4j-cli docker stop tmp --rw
```
