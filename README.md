# Neo4j CLI

## Installation

### Prebuilt archives (recommended)

Download the archive for your OS/arch from the [releases](https://github.com/neo4j-labs/neo4j-cli/releases/latest) page and extract the `neo4j-cli` binary.

### npm

```bash
npm i -g @neo4j-labs/cli
```

Installs a small Node wrapper plus a prebuilt `neo4j-cli` binary matching your OS and CPU. `pnpm add -g @neo4j-labs/cli` and `yarn global add @neo4j-labs/cli` work the same way. Prereleases via `npm i -g @neo4j-labs/cli@alpha` (also `@beta`, `@rc`, `@next`). See [`distribution/npm/cli/README.md`](./distribution/npm/cli/README.md) for the supported platform matrix.

> Note: To install a pre-release, the version must be specified for pip, pipx etc..  This is achieved by adding == immediately after neo4j-cli with the version you want. For example, pipx install neo4j-cli==0.1.0a6  will install 0.1.0a6 of Neo4j CLI
>

### pip

```bash
pip install neo4j-cli
```

### pipx

```bash
pipx install neo4j-cli
```

### uv

```bash
uv tool install neo4j-cli
```

### uvx

uvx will install and execute neo4j-cli. This means that you must include the commands you want to use with uvx

```bash
uvx -i neo4j-cli YOUR_COMMANDS
```

Check the installation by running neo4j-cli --help


## Credentials

`neo4j-cli` stores three kinds of credentials in `credentials.json` under your OS config directory. All three trees share the same `add / list / use / remove` shape; `use` sets the default consumed by downstream commands.

`credential aura-client` — Aura Console API credentials (client ID + secret). Required for any `aura ...` subcommand that calls the Console API.

```bash
neo4j-cli credential aura-client add --name "my-org" --client-id <id> --client-secret <secret>
neo4j-cli credential aura-client list
neo4j-cli credential aura-client use my-org
neo4j-cli credential aura-client remove my-org
```

`credential dbms` — Neo4j Bolt connection profiles (URI, username, password, database, optional embed-credential link). When a default profile exists, `neo4j-cli query` connects without any connection flags or env vars.

```bash
neo4j-cli credential dbms add --name prod --uri neo4j+s://example.databases.neo4j.io --username neo4j --password '<pw>'
neo4j-cli credential dbms list
neo4j-cli credential dbms use prod
neo4j-cli credential dbms set-embed prod openai-shared    # link an embed credential
neo4j-cli credential dbms set-embed prod                  # clear the link
neo4j-cli credential dbms remove prod
```

`credential embed` — Embedding-provider credentials (provider, model, base URL, dimensions, optional API key). Consumed by `query --param NAME:embed=...` and `query :embed`. Supported providers: `openai`, `ollama`, `huggingface`.

```bash
neo4j-cli credential embed add --name openai-shared --provider openai --model text-embedding-3-small --api-key '<key>'
neo4j-cli credential embed list                           # api-key is never printed
neo4j-cli credential embed use openai-shared
neo4j-cli credential embed remove openai-shared
```

## Aura

Manage Neo4j Aura instances from the terminal. Requires an `aura-client` credential — create one in your Aura [Account Settings](https://console.neo4j.io/#account) and add it via [Credentials](#credentials) above.

### List your instances

```bash
neo4j-cli aura instance list --format table
```

### Create an instance

```bash
# Free-db — no cloud provider, region, or memory required
neo4j-cli aura instance create --name my-free-db --type free-db --tenant-id <tenant-id> --rw

# Professional-db on AWS, awaiting readiness
neo4j-cli aura instance create --name my-pro-db --type professional-db --cloud-provider aws \
  --region us-east-1 --memory 4GB --tenant-id <tenant-id> --await --rw
```

`aura tenant list` shows tenant IDs. Initial DB credentials returned by `instance create` are auto-stored as a `dbms` credential (named `<instance-id>-default`), so `neo4j-cli query` can connect immediately. Use `--no-credential-storage` to skip that.

## Querying Neo4j

`neo4j-cli query` runs Cypher against any Neo4j database via the Bolt protocol. Cypher comes from the positional argument or piped stdin.

```bash
neo4j-cli query 'RETURN 1 AS n'
echo 'MATCH (n) RETURN count(n)' | neo4j-cli query
```

Connection settings are resolved with this precedence (highest first): flag → env var → `.env` file (auto-discovered by walking up from cwd) → built-in default.

| Setting  | Flag         | Env var          | Default                   |
| -------- | ------------ | ---------------- | ------------------------- |
| URI      | `--uri`      | `NEO4J_URI`      | `neo4j://localhost:7687`  |
| Username | `--username` | `NEO4J_USERNAME` | `neo4j`                   |
| Password | `--password` | `NEO4J_PASSWORD` | prompted on TTY           |
| Database | `--database` | `NEO4J_DATABASE` | `neo4j`                   |

`http://` and `https://` URIs are auto-rewritten to `neo4j://<host>:7687` and `neo4j+s://<host>:7687` respectively (path/query stripped). For self-signed certs use `neo4j+ssc://`.

Pass parameters with `--param key=value` (repeatable). Values that parse as JSON are typed; everything else is a string:

```bash
neo4j-cli query 'MATCH (p:Person {name:$name}) RETURN p' --param name=Alice
neo4j-cli query 'RETURN $ids' --param 'ids=[1,2,3]'
echo 'MATCH (p:Person {name:$name}) RETURN p' | neo4j-cli query --param name=Alice
```

Output is a table by default; pass `--format json` for a stable envelope (`columns`, `rows`, `truncated`, `arrays_truncated`). When stdout is not a terminal (piped or redirected), `--format` defaults to `json`. Applies to both `query` and `:schema`. Large results are capped at 100 rows and arrays inside cells at 100 items — tune with `--max-rows` / `--truncate-arrays-over` (0 = unlimited).

Schema introspection:

```bash
neo4j-cli query :schema
```

### Embedding parameters

Bind a vector parameter inline by passing `--param NAME:embed=<text>` — the text is sent to the configured embedding provider and the resulting `[]float32` is bound to `$NAME` for both the EXPLAIN preflight and the real run. The sibling `query :embed [text]` leaf computes a vector standalone without opening a Bolt connection.

```bash
neo4j-cli query --param q:embed='sci-fi movies' --param k=5 \
  "CALL db.index.vector.queryNodes('idx', \$k, \$q) YIELD node, score RETURN node, score"

neo4j-cli query :embed "hello world" --format json
echo "hello world" | neo4j-cli query :embed --format toon
```

Embedding settings resolve with this precedence (highest first): flag → env var → `.env` file → stored embed credential → provider built-in default.

| Setting    | Flag                  | Env var                  |
| ---------- | --------------------- | ------------------------ |
| Credential | `--embed-credential`  | —                        |
| Provider   | `--embed-provider`    | `NEO4J_EMBED_PROVIDER`   |
| Model      | `--embed-model`       | `NEO4J_EMBED_MODEL`      |
| Base URL   | `--embed-base-url`    | `NEO4J_EMBED_BASE_URL`   |
| Dimensions | `--embed-dimensions`  | `NEO4J_EMBED_DIMENSIONS` |
| API key    | (none — see below)    | `NEO4J_EMBED_API_KEY`, `OPENAI_API_KEY`, `HF_TOKEN` |

API-key precedence (highest first): per-provider OS env (`OPENAI_API_KEY` / `HF_TOKEN`) → generic OS env (`NEO4J_EMBED_API_KEY`) → per-provider `.env` value → generic `.env` value → stored credential `api-key`. Ollama needs no API key.

`--embed-credential <name>` selects a stored embed credential explicitly; without it the resolver falls back to the embed credential linked from the resolved dbms credential (via `credential dbms add --embed-credential` or `credential dbms set-embed`), then to `credential embed`'s default. So one `--credential <name>` can drive both DB connection and embedding when the dbms credential carries an embed link.

Provider defaults: OpenAI base URL `https://api.openai.com/v1`, Ollama `http://localhost:11434`, HuggingFace `https://router.huggingface.co/hf-inference/models` (serverless mode). Setting `--embed-base-url` switches HuggingFace to dedicated-endpoint mode.

## Write operations

Write commands require `--rw`. `neo4j-cli query` runs `EXPLAIN` first when `--rw` is absent and blocks mutating Cypher before execution.

```bash
neo4j-cli aura instance delete <id> --rw
neo4j-cli config set telemetry false --rw
neo4j-cli query 'CREATE (:Person {name:"Alice"})' --rw
```

## Agent skills

`neo4j-cli` ships an embedded skill bundle (`SKILL.md` + per-subcommand references) that teaches AI coding agents how to drive the CLI. `skill install` drops that bundle into the supported agents' skill directories so the agent picks it up on next run.

Supported agents:

-   Claude Code
-   Cursor
-   Windsurf
-   Copilot
-   Gemini CLI
-   Cline
-   Codex
-   Pi
-   OpenCode
-   Junie

Install into every detected agent (or pass an agent name to target one):

```bash
neo4j-cli skill install
neo4j-cli skill install claude-code
```

List supported agents and per-agent install state:

```bash
neo4j-cli skill list
```

Check installed bundles for version drift against the running binary:

```bash
neo4j-cli skill check
```

Remove the installed bundle (idempotent):

```bash
neo4j-cli skill remove
neo4j-cli skill remove claude-code
```

Beta features (commands gated by `AuraBetaEnabled`, e.g. `dataapi`, `import`, `deployment`) are not included in the generated bundle — the bundle reflects the default-config command surface.

## Feedback / Issues

Please use [GitHub issues](https://github.com/neo4j-labs/neo4j-cli/issues) to provide feedback and report any issues that you have encountered.

## Building locally

Clone the repository and run:

```bash
make build
```

This produces `bin/neo4j-cli`. To run without building:

```bash
make run-neo4j
```

## Developing and contributing

Read [CONTRIBUTING.md](./CONTRIBUTING.md)
