# Neo4j CLI

## Installation

```bash
curl -sSfL https://neo4j.sh/install.sh | bash
```

Verify with `neo4j-cli --help`.

#### Alternatives

- **Homebrew**: `brew install neo4j-labs/tap/neo4j-cli` (stable releases only; prereleases ship via npm/PyPI).
- **npm**: `npm i -g @neo4j-labs/cli` (also works with `pnpm add -g` / `yarn global add`). Prereleases: `@alpha`, `@beta`, `@rc`, `@next`. Platform matrix: [`distribution/npm/cli/README.md`](./distribution/npm/cli/README.md).
- **PyPI**: `pip install neo4j-cli`, `pipx install neo4j-cli`, or `uv tool install neo4j-cli`. One-shot: `uvx -i neo4j-cli <commands>`. Pin a prerelease with `==`, e.g. `pipx install neo4j-cli==0.1.0a6`.
- **Prebuilt archive**: grab your OS/arch from [releases](https://github.com/neo4j-labs/neo4j-cli/releases/latest).

### Self-update

`neo4j-cli update` swaps the running binary with the latest GitHub release. By default only stable semver tags are considered.

```bash
neo4j-cli update                         # update to latest stable
neo4j-cli update check                   # report availability, exit 1 if newer; never downloads
neo4j-cli update --pre-releases          # opt into alpha/beta/rc tags
neo4j-cli update --version v0.1.0        # pin to a named tag (also the only way to downgrade)
```

If the binary was installed via Homebrew / npm / pipx / uv, `update` prints the channel-correct upgrade command instead of overwriting; pass `--force` to swap in place anyway. See `neo4j-cli update --help`.

Installed agent skill bundles are refreshed automatically after a successful swap; if none are installed, `update` suggests running `neo4j-cli skill install`.

## Agent skills

`neo4j-cli` ships an embedded skill bundle (`SKILL.md` + per-subcommand references) that teaches AI coding agents how to drive the CLI. `skill install` drops it into each detected agent's skill directory; pass an agent name to target one.

Supported agents: Claude Code, Cursor, Windsurf, Copilot, Gemini CLI, Cline, Codex, Pi, OpenCode, Junie.

```bash
neo4j-cli skill install                  # all detected agents
neo4j-cli skill install claude-code      # one agent
neo4j-cli skill list                     # per-agent install state
neo4j-cli skill check                    # version drift vs running binary
neo4j-cli skill remove [agent]           # idempotent
```

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

# Professional-db on AWS, waiting for readiness
neo4j-cli aura instance create --name my-pro-db --type professional-db --cloud-provider aws \
  --region us-east-1 --memory 4GB --tenant-id <tenant-id> --wait --rw
```

`aura tenant list` shows tenant IDs. Initial DB credentials returned by `instance create` are auto-stored as a `dbms` credential (named `<instance-id>-default`), so `neo4j-cli query` can connect immediately. Use `--no-credential-storage` to skip that.

## Local Neo4j (Docker)

`neo4j-cli docker` runs Neo4j locally by shelling out to the host `docker` CLI. Managed containers carry the `org.neo4j.cli.managed=true` label — Docker is the source of truth, no separate state file is maintained. Requires Docker Desktop (or the `docker` CLI) on `PATH`.

Defaults: enterprise edition with the evaluation license (`NEO4J_ACCEPT_LICENSE_AGREEMENT=eval`); pass `--accept-license` to upgrade to the commercial license (`=yes`). Use `--edition community` for the community image. Host ports default to 7474 (HTTP) and 7687 (Bolt); override with `--http-port` / `--bolt-port`. If `--name` collides with an existing container or stored `dbms` credential, an auto-suffix (`<name>-1`, `<name>-2`, …) is chosen and logged to stderr.

### Persistent flow (stored credential)

```bash
# Create a managed container; a 16-byte password is generated and stored as a dbms credential
neo4j-cli docker create --name dev --wait --rw

# Run Cypher via the stored credential
neo4j-cli query --credential dev 'RETURN 1 AS n'

# Inspect / list managed containers
neo4j-cli docker list --format toon
neo4j-cli docker get dev --format json

# Stop / start
neo4j-cli docker stop dev --rw
neo4j-cli docker start dev --wait --rw

# Remove both container and stored credential (TTY prompts; non-TTY requires --force)
neo4j-cli docker delete dev --force --rw
```

Heads up: the generated password is part of the standard `create` output. Redirects (`> file`) and pipes (`| tee`, `| jq`) will capture it. Pass `--password <s>` to choose the password yourself, or `--no-store-credential` if you want neither a stored credential nor the rendered password.

### Persisting data across container deletes

By default the Neo4j data, logs, and import dirs live in the container layer and are lost on `docker delete`. Bind-mount a host directory with `--data-dir`, `--logs-dir`, or `--import-dir` to keep them:

```bash
# Persist data on the host so it survives delete + recreate
neo4j-cli docker create --name dev --data-dir ~/neo4j-dev/data --rw
neo4j-cli docker delete dev --force --rw
neo4j-cli docker create --name dev --data-dir ~/neo4j-dev/data --rw  # reuses the same data
```

`--logs-dir` and `--import-dir` mount `/logs` and `/import` similarly. Each flag is optional and independent; combine any subset. Paths support `~` (HOME) and environment-variable expansion, and missing directories are created at mode 0o755. All three flags are incompatible with `--ephemeral` (which is, by definition, disposable). Note: the Neo4j container's entrypoint adjusts ownership of the mounted directories at startup; expect them to show up under the container's neo4j UID on the host after first start.

### Ephemeral flow (env-file into `query --env`)

```bash
# Throwaway container (`docker run --rm`); no credential persisted; env-file written for query --env
neo4j-cli docker create --name tmp --ephemeral --env-out-file /tmp/n.env --wait --rw

# Connect using the emitted env-file
neo4j-cli query --env /tmp/n.env 'RETURN 1 AS n'

# Container is auto-removed by Docker when stopped — nothing to delete
neo4j-cli docker stop tmp --rw
```

Without `--env-out-file`, the env-file blob (with `NEO4J_URI` / `NEO4J_USERNAME` / `NEO4J_PASSWORD` / `NEO4J_DATABASE`) is emitted to stdout for piping. `--wait` blocks until Bolt is reachable (60s timeout); on timeout the container is left running so you can inspect `docker logs <name>`.

`--env-out-file` writes via a temp file in the target's directory and atomically renames it into place; a pre-existing symlink at the path is replaced by a regular file (the symlink is not followed). Use a non-symlink target if you rely on the path being a symlink.

## Querying Neo4j

`neo4j-cli query` runs Cypher against any Neo4j database via the Bolt protocol. Cypher comes from the positional argument or piped stdin.

```bash
neo4j-cli query 'RETURN 1 AS n'
echo 'MATCH (n) RETURN count(n)' | neo4j-cli query
```

**Preferred:** add a `dbms` credential (see [Credentials](#credentials)) and `query` connects with no further config. `aura instance create` auto-stores one for new instances.

Flags, env vars, and `.env` files are optional overrides — useful for one-offs or CI without persisting a credential. When a stored credential exists, an override must supply **all four** of URI/username/password/database (any partial set is rejected). Without a stored credential, resolution is flag → env var → `.env` file (auto-discovered walking up from cwd) → built-in default.

`.env` discovery walks up from cwd and stops at the first `.git` ancestor or your `$HOME` boundary (whichever comes first), so a `.env` outside your repo or above your home directory is never loaded. When the loaded `.env` lives strictly above cwd, an `info: loading .env from <path>` line is printed to stderr so the overlay is never silent.

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

Write commands are gated by `--rw`. `neo4j-cli query` runs `EXPLAIN` first when `--rw` is absent and blocks mutating Cypher before execution.

```bash
neo4j-cli aura instance delete <id> --rw
neo4j-cli config set telemetry false --rw
neo4j-cli query 'CREATE (:Person {name:"Alice"})' --rw
```

### Auto-detect: when `--rw` is required

When `neo4j-cli` runs in an interactive terminal, `--rw` is auto-applied for write commands — humans typing at a prompt don't need the flag. Two cases still require `--rw` explicitly:

- **Agent harnesses** (Claude Code, Codex, Cursor, Gemini CLI, Replit, OpenCode, Auggie, Goose, Devin, Kiro, pi, …). Detection is by env var; the authoritative list lives at [`unjs/std-env`](https://github.com/unjs/std-env/blob/main/src/agents.ts).
- **Non-interactive scripts** (CI, piped invocations, `nohup`, redirected stdout).

Precedence: `--rw` set → allow; agent detected → require `--rw`; stdout is a TTY → allow; otherwise → require `--rw`.

Set `DO_NOT_TRACK=1` to disable telemetry without writing config.

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
