# neo4j-cli credential

## Contents

- [neo4j-cli credential aura-client](#neo4j-cli-credential-aura-client)
- [neo4j-cli credential aura-client add](#neo4j-cli-credential-aura-client-add)
- [neo4j-cli credential aura-client list](#neo4j-cli-credential-aura-client-list)
- [neo4j-cli credential aura-client remove](#neo4j-cli-credential-aura-client-remove)
- [neo4j-cli credential aura-client use](#neo4j-cli-credential-aura-client-use)
- [neo4j-cli credential dbms](#neo4j-cli-credential-dbms)
- [neo4j-cli credential dbms add](#neo4j-cli-credential-dbms-add)
- [neo4j-cli credential dbms list](#neo4j-cli-credential-dbms-list)
- [neo4j-cli credential dbms remove](#neo4j-cli-credential-dbms-remove)
- [neo4j-cli credential dbms set-embed](#neo4j-cli-credential-dbms-set-embed)
- [neo4j-cli credential dbms use](#neo4j-cli-credential-dbms-use)
- [neo4j-cli credential embed](#neo4j-cli-credential-embed)
- [neo4j-cli credential embed add](#neo4j-cli-credential-embed-add)
- [neo4j-cli credential embed list](#neo4j-cli-credential-embed-list)
- [neo4j-cli credential embed remove](#neo4j-cli-credential-embed-remove)
- [neo4j-cli credential embed use](#neo4j-cli-credential-embed-use)

Manage and view credential values

Manage stored credentials. Three subtrees are available: `aura-client` for Aura Console API client credentials, `dbms` for Neo4j Bolt connection profiles consumed by `query`, and `embed` for embedding-provider credentials consumed by `query --param NAME:embed=...` and `query :embed`. Note: `query --credential desktop` and `query --credential desktop-connection:<uuid>` are runtime-resolved against the running Neo4j Desktop 2 instance and are NOT stored here — Desktop owns those credential lifecycles. See `neo4j-cli desktop list` to discover saved Desktop connections.

Usage: `neo4j-cli credential`

## neo4j-cli credential aura-client

Manage and view aura-client credential values

Manage Aura Console API client credentials (client ID + client secret). These credentials are required by every `aura ...` subcommand that calls the Aura Console API. The first credential added is set as default.

Usage: `neo4j-cli credential aura-client`

### neo4j-cli credential aura-client add

Adds an aura-client credential

Add an Aura Console API client credential (client ID + secret). The first credential added becomes the default; switch later with `credential aura-client use <name>`. Pass `--env <path>` to import an Aura console–exported aura-client credentials file (recognised keys: CLIENT_ID, CLIENT_SECRET, CLIENT_NAME); explicit flags override file values.

Usage: `neo4j-cli credential aura-client add [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--client-id` | string | - | (required) Client ID |
| `--client-secret` | string | - | (required) Client secret |
| `--env` | string | - | Path to an Aura console–exported aura-client credentials file. Recognised keys: CLIENT_ID, CLIENT_SECRET, CLIENT_NAME. Explicit flags override file values. |
| `--name` | string | - | (required) Name |

Examples:

```
# Add the first aura-client credential (becomes the default)
neo4j-cli credential aura-client add --name work --client-id <id> --client-secret <secret> --rw

# Import an Aura console–exported aura-client credentials file
neo4j-cli credential aura-client add --name work --env ~/Downloads/aura-client-creds.txt --rw

# Switch the default after adding a second credential
neo4j-cli credential aura-client use personal --rw
```

### neo4j-cli credential aura-client list

List aura-client credentials

List stored Aura Console API client credentials. The `default` column flags the credential used by `aura ...` commands when no other selector is set.

Usage: `neo4j-cli credential aura-client list`

Examples:

```
# List all aura-client credentials as a table
neo4j-cli credential aura-client list

# List as JSON for scripting / agent consumption
neo4j-cli credential aura-client list --format json

# List as toon (compact, agent-friendly)
neo4j-cli credential aura-client list --format toon
```

### neo4j-cli credential aura-client remove

Removes an aura-client credential

Remove a stored Aura Console API client credential by name.

Usage: `neo4j-cli credential aura-client remove`

Examples:

```
# Remove an aura-client credential by name
neo4j-cli credential aura-client remove work --rw

# Remove the personal credential
neo4j-cli credential aura-client remove personal --rw

# Remove a stale credential that no longer authenticates
neo4j-cli credential aura-client remove old-tenant --rw
```

### neo4j-cli credential aura-client use

Sets the default aura-client credential to be used

Set the named aura-client credential as the default consumed by `aura ...` commands.

Usage: `neo4j-cli credential aura-client use`

Examples:

```
# Switch the default to the personal credential
neo4j-cli credential aura-client use personal --rw

# Switch the default to the work credential
neo4j-cli credential aura-client use work --rw

# Switch the default after adding a new credential
neo4j-cli credential aura-client use new-tenant --rw
```

## neo4j-cli credential dbms

Manage and view dbms credential values

Manage stored Neo4j Bolt connection profiles (URI, username, password, database, optional embed-credential link). `query` consumes the default profile (or one selected by `--credential <name>`) when no `--uri`/`NEO4J_URI`/.env value is set.

Usage: `neo4j-cli credential dbms`

### neo4j-cli credential dbms add

Adds a dbms credential

Add a Neo4j Bolt connection profile. The first credential added becomes the default. Pass `--embed-credential <name>` to link this profile to an existing embed credential — `query --credential <name>` will then pick up the embed config automatically. The link can be added later with `credential dbms set-embed`. Pass `--env <path>` to import a Neo4j Aura–exported credentials file (recognised keys: NEO4J_URI, NEO4J_USERNAME, NEO4J_PASSWORD, NEO4J_DATABASE, AURA_INSTANCENAME); explicit flags override file values. The name `desktop` and any name starting with `desktop-connection:` are reserved by `query --credential` runtime dispatch and cannot be used.

Usage: `neo4j-cli credential dbms add [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--database-name` | string | neo4j | Database name |
| `--embed-credential` | string | - | Name of an embed credential to link (must already exist; see `credential embed list`) |
| `--env` | string | - | Path to a Neo4j Aura–exported credentials file. Recognised keys: NEO4J_URI, NEO4J_USERNAME, NEO4J_PASSWORD, NEO4J_DATABASE, AURA_INSTANCENAME. Explicit flags override file values. |
| `--name` | string | - | (required) Name |
| `--password` | string | - | (required) Password |
| `--uri` | string | - | (required) URI |
| `--username` | string | - | (required) Username |

Examples:

```
# Add a dbms credential from an Aura-exported credentials file
neo4j-cli credential dbms add --env ./Neo4j-12345-Created-2025-01-01.txt --rw

# Add a dbms credential with explicit flags (becomes the default if it is the first one)
neo4j-cli credential dbms add --name local --uri neo4j://localhost:7687 --username neo4j --password secret --rw

# Add a dbms credential and link it to an existing embed credential
neo4j-cli credential dbms add --name local --uri neo4j://localhost:7687 --username neo4j --password secret --embed-credential openai-small --rw
```

### neo4j-cli credential dbms list

Lists dbms credentials

List stored Bolt connection profiles. Columns include any linked embed credential (empty when unset). Passwords are never printed.

Usage: `neo4j-cli credential dbms list`

Examples:

```
# List dbms credentials as a table
neo4j-cli credential dbms list

# List dbms credentials as JSON (machine-readable)
neo4j-cli credential dbms list --format json

# List dbms credentials in toon format
neo4j-cli credential dbms list --format toon
```

### neo4j-cli credential dbms remove

Removes a dbms credential

Remove a stored Bolt connection profile by name. Linked embed-credential references on other profiles are not modified.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli credential dbms remove <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Remove a dbms credential by name
neo4j-cli credential dbms remove local --rw --yes --force

# Remove a staging dbms credential
neo4j-cli credential dbms remove staging --rw --yes --force

# Remove a prod dbms credential
neo4j-cli credential dbms remove prod --rw --yes --force
```

### neo4j-cli credential dbms set-embed

Links (or clears) an embed credential on a dbms credential

Link a stored dbms credential to an existing embed credential by name. Pass only the dbms name to clear the link. No embed-credential is required for `query` to run plain Cypher; this only links one for downstream embedding via `--param NAME:embed=...` and `query :embed`. With a link in place, `query --credential <dbms-name>` picks up both the connection and the embed config in a single selector.

Usage: `neo4j-cli credential dbms set-embed <dbms-name> [embed-name]`

Examples:

```
# Link a dbms credential to an embed credential
neo4j-cli credential dbms set-embed local openai-small --rw

# Replace the linked embed credential
neo4j-cli credential dbms set-embed local ollama-nomic --rw

# Clear the embed-credential link on a dbms credential
neo4j-cli credential dbms set-embed local --rw
```

### neo4j-cli credential dbms use

Sets the default dbms credential to be used

Set the named dbms credential as the default consumed by `query` when no `--credential <name>` flag and no connection flags / env vars / .env values are present.

Usage: `neo4j-cli credential dbms use <name>`

Examples:

```
# Make 'local' the default dbms credential
neo4j-cli credential dbms use local --rw

# Switch the default to 'staging'
neo4j-cli credential dbms use staging --rw

# Switch the default to 'prod'
neo4j-cli credential dbms use prod --rw
```

## neo4j-cli credential embed

Manage and view embed credential values

Manage stored embedding-provider credentials (provider, model, base URL, dimensions, optional API key). `query --param NAME:embed=<text>` and `query :embed [text]` consume the resolved embed credential when no `--embed-*` flag or `NEO4J_EMBED_*` env var overrides it. Supported providers: openai, ollama, huggingface.

Usage: `neo4j-cli credential embed`

### neo4j-cli credential embed add

Adds an embed credential

Add an embedding-provider credential. Provider must be one of openai, ollama, huggingface. `--api-key` is optional for ollama (no auth required) and may be omitted for openai/huggingface if you intend to provide it via env var (`OPENAI_API_KEY` / `HF_TOKEN` / `NEO4J_EMBED_API_KEY`). The first credential added becomes the default; switch later with `credential embed use <name>`.

Usage: `neo4j-cli credential embed add [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--api-key` | string | - | API key for the provider |
| `--base-url` | string | - | Base URL for the provider (overrides provider default) |
| `--dimensions` | int | 0 | Embedding dimensions (provider-specific; 0 means provider default) |
| `--model` | string | - | (required) Model |
| `--name` | string | - | (required) Name |
| `--provider` | string | - | (required) Provider (one of: openai, ollama, huggingface) |

Examples:

```
# Add an OpenAI embed credential (becomes the default if it is the first one)
neo4j-cli credential embed add --name openai-small --provider openai --model text-embedding-3-small --api-key sk-... --rw

# Add a local Ollama embed credential (no api-key required)
neo4j-cli credential embed add --name ollama-nomic --provider ollama --model nomic-embed-text --base-url http://localhost:11434 --rw

# Add a HuggingFace embed credential with explicit dimensions
neo4j-cli credential embed add --name hf-bge --provider huggingface --model BAAI/bge-small-en-v1.5 --api-key hf_... --dimensions 384 --rw
```

### neo4j-cli credential embed list

Lists embed credentials

List stored embedding-provider credentials. The `api-key` column is never shown — keys are persisted on disk but redacted in every printable form.

Usage: `neo4j-cli credential embed list`

Examples:

```
# List embed credentials as a table
neo4j-cli credential embed list

# List embed credentials as JSON (machine-readable)
neo4j-cli credential embed list --format json

# List embed credentials in toon format
neo4j-cli credential embed list --format toon
```

### neo4j-cli credential embed remove

Removes an embed credential

Remove a stored embedding-provider credential by name. Removal is non-cascading: dbms credentials linked to the removed embed credential keep their `embed-credential` field; the stale link is reported lazily at query time. Run `credential dbms list` to find linked profiles or `credential dbms set-embed <dbms-name>` to clear them.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli credential embed remove <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Remove an embed credential by name
neo4j-cli credential embed remove openai-small --rw --yes --force

# Remove the local Ollama embed credential
neo4j-cli credential embed remove ollama-nomic --rw --yes --force

# Remove a HuggingFace embed credential
neo4j-cli credential embed remove hf-bge --rw --yes --force
```

### neo4j-cli credential embed use

Sets the default embed credential to be used

Set the named embed credential as the default consumed by `query --param NAME:embed=...` and `query :embed` when no `--embed-credential` flag, no `NEO4J_EMBED_*` env, no `.env` value, and no dbms→embed link resolves first.

Usage: `neo4j-cli credential embed use <name>`

Examples:

```
# Make 'openai-small' the default embed credential
neo4j-cli credential embed use openai-small --rw

# Switch the default to the local Ollama embedder
neo4j-cli credential embed use ollama-nomic --rw

# Switch the default to a HuggingFace embedder
neo4j-cli credential embed use hf-bge --rw
```

