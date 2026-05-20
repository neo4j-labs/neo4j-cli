<!-- Hand-written tips & gotchas inlined into the generated SKILL.md "Tips & Gotchas" section. Edit this file (not bundle/SKILL.md) and re-run `go generate ./...`. -->

- **Before generating ANY Cypher: run `neo4j-cli query :schema --format toon` first to discover the real labels, relationship types, and properties. Do not guess the schema.** Read [query-additions.md](query-additions.md) for the full schema-first workflow, parameters, embeddings, and Cypher 25 vs 5.
- The `aura` subcommand under neo4j-cli does NOT carry a nested `skill` group — install agent skills via `neo4j-cli skill install` at the top level.
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

- Aura Agents: `neo4j-cli aura agent` (`list` / `get` / `create` / `update` / `replace` / `delete` / `invoke`) manages Aura Agents — LLM-backed assistants bound to an Aura database. `--organization-id` / `--project-id` honour the default workspace, identical to every other aura command. `invoke` is dual-mode: `--format json` returns the full server response verbatim (content blocks, usage, end_reason, errors), while the default table output joins the text content blocks and prints a single stats line `Status: <S> | End reason: <ER> | Tool calls: <N> | Tokens: <req> req / <res> res / <total> total`. HTTP 403 on `invoke` surfaces as `agent invocation forbidden: agent may be disabled or private`; an HTTP 200 body with `type: "error"` surfaces as `agent invocation failed: <message>`.

```sh
# List, create, and invoke an agent (workspace default already set)
neo4j-cli aura agent list --format toon
neo4j-cli aura agent create --name docs-bot --description "Docs assistant" --dbid <dbid> --tools '[{"type":"text2cypher","name":"ask","description":"Answer questions about the graph"}]' --rw
neo4j-cli aura agent invoke <agent-id> --input "hello" --rw
```

- `--tools` is a JSON array of tool objects. Every tool shares the envelope `{type, name, description, config}` (plus optional `enabled`) — `name` ≤64 chars, `description` ≤2000 chars. The `type` discriminator is **camelCase** (NOT snake_case): `text2cypher`, `cypherTemplate`, `similaritySearch`. The `config` object is type-specific; the four canonical shapes are:

```json
[{"type":"text2cypher","name":"ask-graph","description":"Convert natural-language questions into Cypher and run them against the database."}]
```

```json
[{"type":"cypherTemplate","name":"top-customers","description":"Return the top N customers by total order value.","config":{"template":"MATCH (c:Customer)-[:PLACED]->(o:Order) RETURN c.name AS name, sum(o.total) AS spent ORDER BY spent DESC LIMIT $limit","parameters":[{"name":"limit","data_type":"integer","description":"Maximum number of customers to return."}]}}]
```

```json
[{"type":"similaritySearch","name":"doc-search","description":"Find docs most similar to the user question.","config":{"provider":"openai","model":"text-embedding-3-small","index":"docs_embedding_index","top_k":5}}]
```

```json
[{"type":"similaritySearch","name":"doc-search-enriched","description":"Vector-search docs, then enrich each hit with its parent page.","config":{"provider":"openai","model":"text-embedding-3-small","index":"docs_embedding_index","top_k":5,"post_processing_cypher":"MATCH (node)<-[:HAS_CHUNK]-(page:Page) RETURN page.title AS title, page.url AS url, node.text AS chunk, score ORDER BY score DESC"}}]
```

Tool-config field reference (v2beta1):
  - `text2cypher` — no `config` fields required.
  - `cypherTemplate.config` — required `template` (Cypher string); optional `parameters[]` of `{name, data_type, description}` with `data_type ∈ {string, number, boolean, integer}`.
  - `similaritySearch.config` — required `provider` (`openai` | `vertexai`), `model` (provider-compatible embedding model), `index` (vector-index name, ≤100 chars), `top_k` (1–100); optional `dimensions` (integer, when the provider/index needs an explicit size); optional `post_processing_cypher` — read-only Cypher appended to the vector search and run as a single query, with `node` and `score` exposed to the post-processing block (write queries are rejected server-side).
