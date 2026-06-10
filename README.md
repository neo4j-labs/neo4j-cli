# Neo4j CLI

## Installation

```bash
curl -sSfL https://neo4j.sh/install.sh | bash
```

In an interactive terminal the installer asks whether to install the agent-skill bundle. For unattended installs prepend `NEO4J_CLI_AUTO_INSTALL_SKILL=1` to opt in or `=0` to opt out — the same env var is honoured by the npm postinstall and Homebrew formula.

Verify with `neo4j-cli --help`.

#### Alternatives

- **Homebrew**: `NEO4J_CLI_AUTO_INSTALL_SKILL=1 brew install neo4j-labs/tap/neo4j-cli` (stable releases only; prereleases ship via npm/PyPI).
- **npm**: `NEO4J_CLI_AUTO_INSTALL_SKILL=1 npm i -g @neo4j-labs/cli` (also works with `pnpm add -g` / `yarn global add`). Prereleases: `@alpha`, `@beta`, `@rc`, `@next`. Platform matrix: [`distribution/npm/cli/README.md`](./distribution/npm/cli/README.md).
- **PyPI**: `pip install neo4j-cli`, `pipx install neo4j-cli`, or `uv tool install neo4j-cli`. One-shot: `uvx -i neo4j-cli <commands>`. Pin a prerelease with `==`, e.g. `pipx install neo4j-cli==0.1.0a6`.
- **Prebuilt archive**: grab your OS/arch from [releases](https://github.com/neo4j-labs/neo4j-cli/releases/latest), place the binary on your `PATH`, then run `neo4j-cli skill install --rw`.

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

**Security:** how `neo4j-cli update` validates downloads and our vulnerability reporting channel are documented in [SECURITY.md](./SECURITY.md).

## Agent skills

`neo4j-cli` installs either the embedded self-skill (`SKILL.md` + per-subcommand references that teach AI coding agents how to drive the CLI) or any skill from the curated catalog at [`github.com/neo4j-contrib/neo4j-skills`](https://github.com/neo4j-contrib/neo4j-skills) (Cypher, modeling, drivers, GraphRAG, GDS, Aura, …). The self-skill is addressable as `self` (canonical) or by the binary name `neo4j-cli` (alias). Passing an agent name as the positional is a hard error — use `--agent <name>` instead.

Supported agents: Claude Code, Cursor, Windsurf, Copilot, Antigravity, Gemini CLI, Cline, Codex, Pi, OpenCode, Junie.

```bash
neo4j-cli skill install                              # self-skill into every detected agent
neo4j-cli skill install neo4j-cypher-skill           # one catalog skill into every detected agent
neo4j-cli skill install --all                        # self-skill + every catalog skill
neo4j-cli skill install --agent claude-code          # scope any install to one agent
neo4j-cli skill install neo4j-cypher-skill --refresh # force catalog fetch first
neo4j-cli skill list                                 # per-(skill × agent) state
neo4j-cli skill check                                # drift vs source (self + catalog)
neo4j-cli skill remove neo4j-cypher-skill            # remove one skill
neo4j-cli skill remove --all                         # remove every catalog skill (self preserved)
neo4j-cli skill refresh                              # force a catalog fetch
```

The catalog is cached under `os.UserCacheDir()/neo4j-cli/skill-catalog/` with a 24h TTL; `install` / `list` / `check` auto-refresh when stale, and fall back to the cached copy on network failure with a stderr warning.

Using a multi-agent orchestrator like [Conductor](https://www.conductor.build)? It reuses the underlying agents' skill directories (e.g. Claude Code, Codex), so `skill install` with no argument covers every detected agent in one go, and `skill check` surfaces any version drift between them.

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

`credential embed` — Embedding-provider credentials (provider, model, base URL, dimensions, optional API key). Consumed by `query --param NAME:embed=...` and `query :embed`. Supported providers: `openai`, `ollama`, `huggingface`, `gemini`, `vertex`.

```bash
neo4j-cli credential embed add --name openai-shared --provider openai --model text-embedding-3-small --api-key '<key>'
neo4j-cli credential embed list                           # api-key is never printed
neo4j-cli credential embed use openai-shared
neo4j-cli credential embed remove openai-shared
```

## Aura

Manage Neo4j Aura instances from the terminal. Requires an `aura-client` credential — create one in your Aura [Account Settings](https://console.neo4j.io/#account) and add it via [Credentials](#credentials) above.

### Organizations and projects

Aura resources are organized in a hierarchy: organization → project → instance. Use `organization` and `project` commands to discover the IDs you need.

```bash
# List all organizations accessible to the current credential
neo4j-cli aura organization list
neo4j-cli aura organization get <org-id>

# List projects within an organization
neo4j-cli aura project list --organization-id <org-id>
neo4j-cli aura project get <project-id>
```

### Setting a default workspace

All Aura resource commands (`instance`, `instance snapshot`, `customer-managed-key`, `graph-analytics session`) require `--organization-id` and `--project-id` to scope the request to a project. `workspace use` sets a default `{organizationId}/{projectId}` pair so these flags can be omitted. Explicit `--organization-id`/`--project-id` flags always override the workspace default when present.

```bash
# Discover available workspaces (all org/project pairs)
neo4j-cli aura workspace list
neo4j-cli aura workspace list --format json | jq -r '.data[] | select(.default == true) | .workspace'

# Set the active workspace (validates the pair against the API before persisting)
neo4j-cli aura workspace use <org-id>/<project-id> --rw

# Equivalently, using flags
neo4j-cli aura workspace use --organization-id <org-id> --project-id <project-id> --rw

# Read or update the workspace directly via config
neo4j-cli aura config get default-workspace
neo4j-cli config set aura.default-workspace <org-id>/<project-id> --rw
```

### List your instances

```bash
# With a default workspace configured
neo4j-cli aura instance list --format table

# With explicit flags
neo4j-cli aura instance list --organization-id <org-id> --project-id <project-id> --format table
```

### Create an instance

```bash
# Free-db — no cloud provider, region, or memory required (workspace already set)
neo4j-cli aura instance create --name my-free-db --type free-db --rw

# Professional-db on AWS, waiting for readiness (with explicit flags)
neo4j-cli aura instance create --name my-pro-db --type professional-db --cloud-provider aws \
  --region us-east-1 --memory 4GB --organization-id <org-id> --project-id <project-id> --wait --rw
```

Initial DB credentials returned by `instance create` are auto-stored as a `dbms` credential (named `<instance-id>-default`), so `neo4j-cli query` can connect immediately. Use `--no-credential-storage` to skip that.

### Aura Agent

Aura Agent — LLM-backed assistants bound to a database. Manage them from the terminal.

Seven leaves: `list`, `get`, `create`, `update`, `replace`, `delete`, `invoke`. `--organization-id`/`--project-id` honour the default workspace from [`workspace use`](#setting-a-default-workspace).

`--tools` accepts a JSON array of tool objects shaped `{type, name, description, config}` — the `type` discriminator is **camelCase**: `text2cypher`, `cypherTemplate`, `similaritySearch`. Full per-type JSON shapes (including `post_processing_cypher` for `similaritySearch`) live in [`neo4j-cli/internal/skill/additions.md`](neo4j-cli/internal/skill/additions.md) and the [Aura v2beta1 spec](https://neo4j.com/docs/aura/platform/api/specification/?urls.primaryName=Aura%20v2beta1#/agents).

```bash
# List agents in the current workspace
neo4j-cli aura agent list --format json

# Create an agent bound to a database, with one tool
neo4j-cli aura agent create --name docs-bot --description "Docs assistant" --dbid <dbid> \
  --tools '[{"type":"text2cypher","name":"ask","description":"Answer questions about the graph"}]' --rw

# Invoke an agent — JSON output returns the full server response; default (table) prints
# the joined text blocks followed by a stats line (Status / End reason / Tool calls / Tokens)
neo4j-cli aura agent invoke <agent-id> --input "hello" --rw
neo4j-cli aura agent invoke <agent-id> --input "hello" --format json --rw
```

## Local Neo4j

Two ways to run Neo4j locally from the CLI: drive a [Neo4j Desktop 2](https://neo4j.com/download/) install via its local relate API, or shell out to `docker`.

### Neo4j Desktop

`neo4j-cli desktop` manages a local Neo4j Desktop 2 install via its relate API on `http://localhost:<port>/fastify/api` (Desktop must be running). Manage DBMSes, plugins, and saved remote connections from the terminal. Desktop owns the credential lifecycle — DBMS passwords live in Desktop's safeStorage, not in `credentials.json`.

```bash
# Install Neo4j Desktop 2 itself (already-installed detection runs first)
neo4j-cli desktop install --rw

# Diagnose a local Desktop install end-to-end
neo4j-cli desktop doctor

# Composed view: local DBMSes + saved remote connections
neo4j-cli desktop list --format table
```

Local DBMSes — create, lifecycle, and plugins (Desktop 2 runs one DBMS at a time on port 7687; `create` / `start` refuse when another is running, pass `--force` to stop the conflicting one first):

```bash
# Create a DBMS at Desktop's highest-known stable enterprise version
neo4j-cli desktop dbms create --name dev --password '<pw>' --wait --rw

# Lifecycle
neo4j-cli desktop dbms start <id> --wait --rw
neo4j-cli desktop dbms stop <id> --wait --rw
neo4j-cli desktop dbms delete <id> --force --rw

# Plugins
neo4j-cli desktop dbms plugin available <id>
neo4j-cli desktop dbms plugin install <id> --plugin apoc --rw
```

Saved remote connections (Aura, self-hosted, …) — `query` selects them via `--credential desktop-connection:<uuid>`, or `--credential desktop` for the single running local DBMS:

```bash
neo4j-cli desktop connection create --name aura-prod --uri neo4j+s://abc.databases.neo4j.io --username neo4j --password '<pw>' --rw
neo4j-cli desktop connection list
neo4j-cli query --credential desktop-connection:<uuid> 'RETURN 1'
```

### Docker

`neo4j-cli docker` runs Neo4j locally by shelling out to the host `docker` CLI. Managed containers carry the `org.neo4j.cli.managed=true` label — Docker is the source of truth, no separate state file is maintained. Requires Docker Desktop (or the `docker` CLI) on `PATH`.

If you use podman instead of docker, you can alias it (`alias docker=podman` in your shell rc, or `Set-Alias docker podman` in Windows PowerShell). `neo4j-cli docker` shells out to whatever resolves as `docker` on your `PATH` — it doesn't care which runtime backs it.

Defaults: enterprise edition with the evaluation license (`NEO4J_ACCEPT_LICENSE_AGREEMENT=eval`); pass `--accept-license` to upgrade to the commercial license (`=yes`). Use `--edition community` for the community image. Host ports default to 7474 (HTTP) and 7687 (Bolt); override with `--http-port` / `--bolt-port`. When the requested pair is taken, both ports are auto-incremented by the same offset until a free pair is found. If `--name` collides with an existing container or stored `dbms` credential, an auto-suffix (`<name>-1`, `<name>-2`, …) is chosen and logged to stderr.

#### Persistent flow (stored credential)

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

Heads up: the generated password is part of the standard `create` output. Redirects (`> file`) and pipes (`| tee`, `| jq`) will capture it. Pass `--password <s>` to choose the password yourself, `--no-print-password` to keep the stored credential but suppress the password from stdout (recover later with `neo4j-cli credential dbms get <name>`), or `--no-store-credential` if you want neither a stored credential nor the rendered password.

#### Persisting data across container deletes

By default the Neo4j data, logs, and import dirs live in the container layer and are lost on `docker delete`. Bind-mount a host directory with `--data-dir`, `--logs-dir`, or `--import-dir` to keep them:

```bash
# Persist data on the host so it survives delete + recreate
neo4j-cli docker create --name dev --data-dir ~/neo4j-dev/data --rw
neo4j-cli docker delete dev --force --rw
neo4j-cli docker create --name dev --data-dir ~/neo4j-dev/data --rw  # reuses the same data
```

`--logs-dir` and `--import-dir` mount `/logs` and `/import` similarly. Each flag is optional and independent; combine any subset. Paths support `~` (HOME) and environment-variable expansion, and missing directories are created at mode 0o755. All three flags are incompatible with `--ephemeral` (which is, by definition, disposable). Note: the Neo4j container's entrypoint adjusts ownership of the mounted directories at startup; expect them to show up under the container's neo4j UID on the host after first start.

#### Ephemeral flow (env-file into `query --env`)

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

## Example datasets

`neo4j-cli` can load published example Neo4j datasets into a local Docker container, a Neo4j Desktop 2 DBMS, or a new Aura instance. A dataset is any GitHub `<owner>/<repo>` carrying a `relate.project-install.json` manifest — the CLI resolves the manifest, downloads the matching `.dump`, and loads it.

`dataset list` prints a curated set of suggestions, but it's only a suggestion set: any repo with a manifest works. `dataset --help` signposts the three per-target load commands.

```bash
# Browse the curated suggestions (slug / title / description / repo)
neo4j-cli dataset list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli dataset list --format json
```

Datasets are addressed by their `<owner>/<repo>`, e.g. `neo4j-graph-examples/movies`. Loading defaults to the **latest** Neo4j; `--version` accepts `5`, `5.26`, a calver like `2026.04.0`, or `latest`. `--database` (default `neo4j`) selects the target database — on an **existing** target it must already exist, since the load overwrites that database (it does not create it).

### Into a local Docker container

A new container is created automatically when `--name` doesn't match an existing one. Loading into an **existing** managed container overwrites that database and requires `--force`.

```bash
# Load into a NEW container (created automatically), waiting for Bolt
neo4j-cli docker load neo4j-graph-examples/movies --name movies --wait --rw

# Overwrite an EXISTING container's database (requires --force)
neo4j-cli docker load neo4j-graph-examples/movies --name movies --force --rw
```

### Into a Neo4j Desktop 2 DBMS

Exactly one of `--name` (create a new DBMS) or `--dbms-id` (target an existing one) is required; they are mutually exclusive. Loading into an existing DBMS overwrites its database and requires `--force`. Prefer the interactive TTY password prompt — omit `--password` so the value never lands in argv.

```bash
# Create a NEW Desktop DBMS and load the dataset into it
neo4j-cli desktop dbms load neo4j-graph-examples/movies --name movies --rw

# Overwrite an EXISTING DBMS's database (requires --force)
neo4j-cli desktop dbms load neo4j-graph-examples/movies --dbms-id <uuid> --force --rw
```

### Into a new Aura instance

`aura instance load` always creates a **new** instance (Aura has no dump-upload API, so the dump is staged through an ephemeral local Docker container and pushed over Bolt — a local Docker daemon is required). It reuses the `instance create` flag set (`--type`, plus `--cloud-provider`/`--region`/`--memory` for paid types). `free-db` is the simplest. Datasets requiring the `graph-data-science` plugin can't be loaded into Aura and are rejected up front.

```bash
# Load into a new free-db instance (simplest)
neo4j-cli aura instance load neo4j-graph-examples/movies --name movies-demo --type free-db --rw

# Load into a new professional-db instance on AWS
neo4j-cli aura instance load neo4j-graph-examples/recommendations --name recs --type professional-db \
  --cloud-provider aws --region us-east-1 --memory 2GB --rw
```

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

### Multiple statements

A single Cypher string may contain multiple statements. They are split on a `;` at the **end of a line** (followed only by trailing whitespace, or at the end of input) — a `;` in the middle of a line is kept verbatim, and the terminating `;` is stripped. A trailing `;` on a single statement is therefore harmless.

```bash
neo4j-cli query 'MATCH (n:Person) RETURN count(n) AS people;
MATCH (m:Movie) RETURN count(m) AS movies'
```

By default each statement runs in its own transaction, in order, **failing fast** on the first error (statements that already committed are not rolled back). Pass `--atomic` to run every statement inside a single transaction that **rolls back** if any statement fails:

```bash
neo4j-cli query 'CREATE (:Person {name:"Alice"});
CREATE (:Person {name:"Bob"})' --rw --atomic
```

When more than one statement runs, results are combined: `--format json` emits a JSON **array** of result envelopes (one per statement, in order); `--format table` and `--format toon` print **stacked blocks** separated by a blank line. A single statement renders exactly as before. Truncation warnings are prefixed `statement N:` only when more than one statement ran.

Schema introspection:

```bash
neo4j-cli query :schema
```

### Offline linting

`query :lint` checks Cypher for syntax and semantic errors using the same semantic analysis that powers Neo4j's language tooling — fully offline, no database connection or credentials needed. `--cypher-version` selects the dialect (`5`, the default, or `25`). Each diagnostic renders as one row (`severity`, `message`, 1-indexed `line`/`column`, 0-indexed offsets). The command exits with code 6 when any error-severity diagnostic is found; warnings alone exit 0.

```bash
neo4j-cli query :lint 'MATCH (n) RETURN m' --format json
cat query.cypher | neo4j-cli query :lint --cypher-version 25
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
| API key    | (none — see below)    | `NEO4J_EMBED_API_KEY`, `OPENAI_API_KEY`, `HF_TOKEN`, `GEMINI_API_KEY`, `GOOGLE_API_KEY` |

API-key precedence (highest first): per-provider OS env (`OPENAI_API_KEY` for openai; `HF_TOKEN` for huggingface; `GEMINI_API_KEY` then `GOOGLE_API_KEY` for gemini) → generic OS env (`NEO4J_EMBED_API_KEY`) → per-provider `.env` value → generic `.env` value → stored credential `api-key`. Ollama needs no API key. Vertex AI does not use an API key at all — it authenticates via Application Default Credentials (`gcloud auth application-default login` or `GOOGLE_APPLICATION_CREDENTIALS` pointing at a service-account JSON), so the API-key precedence rules above do not apply when `--embed-provider vertex`.

`--embed-credential <name>` selects a stored embed credential explicitly; without it the resolver falls back to the embed credential linked from the resolved dbms credential (via `credential dbms add --embed-credential` or `credential dbms set-embed`), then to `credential embed`'s default. So one `--credential <name>` can drive both DB connection and embedding when the dbms credential carries an embed link.

Provider defaults: OpenAI base URL `https://api.openai.com/v1`, Ollama `http://localhost:11434`, HuggingFace `https://router.huggingface.co/hf-inference/models` (serverless mode). Setting `--embed-base-url` switches HuggingFace to dedicated-endpoint mode. Gemini base URL `https://generativelanguage.googleapis.com/v1beta` (called as `{base}/models/{model}:embedContent`). Vertex AI builds the URL from `--vertex-project` and `--vertex-location` (no `--embed-base-url` knob) and authenticates via Application Default Credentials (`gcloud auth application-default login` or `GOOGLE_APPLICATION_CREDENTIALS`).

## Command history

`neo4j-cli history` shows a local log of recently run commands. Every invocation is appended to a `history.jsonl` file in the config dir (alongside `config.json`, mode `0600`).

```bash
# Show the last 20 commands, newest first
neo4j-cli history list

# Show the last 5, or emit the full structured log
neo4j-cli history list --limit 5
neo4j-cli history list --format json

# Empty the log (requires --force)
neo4j-cli history clear --force
```

`history list` prints the human form `[time] <command> {invoker:...}` by default; `--format json|table|toon` emits the structured entries and `--limit N` caps how many are shown (0 = all).

### Privacy

History recording is **on by default**. Known secret flags are redacted to `***` — `--password`/`-p`, `--client-secret`, `--api-key`, `--instance-password`, the `user:pass@` userinfo in `--uri`, and `--param` values under a secret-looking key (e.g. `token`, `secret`). Disable recording with `neo4j-cli config set history-enabled false --rw`, and cap retained entries with `neo4j-cli config set history-limit <n> --rw` (default `1000`; `0` disables recording).

**However, the `neo4j-cli query` positional Cypher body and `--param` values are stored largely UNREDACTED** (a deliberate decision to keep history useful). They can contain inline secrets — e.g. `CREATE USER ... SET PASSWORD '...'`, connection strings, or PII — and a secret under an innocuous param key (or embedded directly in Cypher text) is **not** redacted.

Because the history is designed to be read by agents and may be sent to an LLM for distillation or reporting, that flow transmits the recorded commands **off-machine**. Avoid embedding secrets in Cypher, `--param`, or `--uri`; prefer the `--password` TTY prompt or the `NEO4J_PASSWORD` environment variable.

## Tee on failure

When a command fails, `neo4j-cli` saves the command's full emitted output (stdout + stderr, combined and redacted of secrets) to a `<timestamp>_<command>.log` file in the `tee/` subdirectory of the config dir (mode `0600`). The path is surfaced as `tee_path` in the JSON/toon error envelope and as a `Full output saved: <path>` line on stderr, so the captured output can be read after the fact without re-running the command. Output is captured into a bounded ~5 MiB buffer (head kept, footer appended on overflow) and is only persisted on failure — never on success or user cancellation.

Tee is **on by default**. Disable it with `neo4j-cli config set tee-enabled false --rw`, and cap retained files per command type with `neo4j-cli config set tee-limit <n> --rw` (default `20`; `0` disables tee). Writing is best-effort: a tee error never changes the command's exit code and simply omits `tee_path`.

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
