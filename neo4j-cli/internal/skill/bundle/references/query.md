# neo4j-cli query

## Contents

- [neo4j-cli query :embed](#neo4j-cli-query-embed)
- [neo4j-cli query :lint](#neo4j-cli-query-lint)
- [neo4j-cli query :schema](#neo4j-cli-query-schema)

Run Cypher, inspect the database schema (:schema), lint Cypher offline (:lint), and embed text against a Neo4j database via the Bolt protocol

Use the :schema subcommand to introspect labels, relationship types, and properties before writing Cypher — never guess the schema. Run a Cypher statement against a Neo4j database via the Bolt protocol. Cypher is taken from the positional argument, or from stdin when no argument is provided and stdin is piped. Use `--param NAME:embed=<text>` to inject an embedding vector inline (text is sent to the configured embedding provider, the resulting vector is bound to $NAME for both EXPLAIN preflight and the real run). The sibling `query :embed [text]` leaf computes a vector standalone without opening a Bolt connection. The sibling `query :lint [cypher]` leaf checks Cypher for syntax and semantic errors offline, also without opening a Bolt connection. Multiple statements may be passed in a single string: they are split on a `;` at the end of a line (a mid-line `;` is kept verbatim; the terminating `;` is stripped). By default each statement runs in its own transaction, in order, failing fast on the first error; pass `--atomic` to run them all in one transaction that rolls back if any statement fails, or `--continue-on-error` (non-atomic only) to report each failure and keep going, exiting non-zero at the end. Multiple result sets render as a JSON array with `--format json` or as stacked blocks with `--format table`/`toon`. Write operations require `--rw`; without `--rw`, an EXPLAIN preflight runs first and statements classified as writes are blocked.

Usage: `neo4j-cli query [cypher]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--atomic` | bool | false | Run all statements in a single transaction; roll back on any failure (default: each statement in its own transaction, fail-fast) |
| `--continue-on-error` | bool | false | Keep running after a statement fails: report each failure and execute the rest, then exit non-zero (non-atomic only; mutually exclusive with --atomic) |
| `-c, --credential` | string | - | Credential to use for the connection. Forms: 'desktop' (the single running Neo4j Desktop 2 DBMS), 'desktop-connection:<uuid>' (a saved Neo4j Desktop 2 connection; see 'neo4j-cli desktop list'), or '<name>' (a persisted dbms credential; see 'neo4j-cli credential dbms list'). Combine with --database/NEO4J_DATABASE to target a specific database |
| `-d, --database` | string | - | Target database name; defaults to the connecting user's home database when unset - typically "neo4j", but can vary by deployment (e.g. the instance DBID on Aura Free). Also applies with --credential, overriding the credential-supplied database [env: NEO4J_DATABASE] |
| `--debug` | bool | false | Route Neo4j driver activity (connection, auth, routing, retries) to stderr at DEBUG level; stdout is unaffected [env: NEO4J_DEBUG (set to 1 to enable)] |
| `--embed-base-url` | string | - | Embedding provider base URL [env: NEO4J_EMBED_BASE_URL] |
| `--embed-credential` | string | - | Name of a stored embed credential to seed embedding config (see 'neo4j-cli credential embed list') |
| `--embed-dimensions` | int | 0 | Embedding output dimensions (provider-dependent; ignored by Ollama) [env: NEO4J_EMBED_DIMENSIONS] |
| `--embed-model` | string | - | Embedding model name [env: NEO4J_EMBED_MODEL] |
| `--embed-provider` | string | - | Embedding provider: openai \| ollama \| huggingface \| gemini \| vertex [env: NEO4J_EMBED_PROVIDER] |
| `--env` | string | - | Path to a .env file (auto-discovered by walking up from cwd if unset) |
| `--format` | string | - | Format to print console output in, from a choice of [default, json, table, toon]. (agents: prefer toon) |
| `--max-rows` | int | 100 | Maximum rows to print (0 = unlimited); when capped, prints a stderr warning and sets truncated=true in JSON |
| `--param` | stringArray | [] | Query parameter as key=value (repeatable); JSON-typed when value parses as JSON, otherwise treated as a string. Use `key:embed=<text>` to embed text via the configured provider and bind the resulting vector to $key (see `query :embed`). |
| `-p, --password` | string | - | Neo4j password [env: NEO4J_PASSWORD]; prompted on TTY if unset |
| `--truncate-arrays-over` | int | 100 | Recursively truncate any array longer than N inside row values (0 = off); rendered as ["<truncated: K items>"] |
| `--uri` | string | - | Neo4j Bolt URI [env: NEO4J_URI]. http://<host>[:p][/...] is auto-rewritten to neo4j://<host>:7687; https://<host>[:p][/...] is auto-rewritten to neo4j+s://<host>:7687. (default "neo4j://localhost:7687") |
| `-u, --username` | string | - | Neo4j username [env: NEO4J_USERNAME] (default "neo4j") |

Examples:

```
# Introspect the schema before writing Cypher (always do this first)
neo4j-cli query :schema --format toon

# Run inline Cypher (read-only — no --rw needed)
neo4j-cli query "MATCH (n) RETURN count(n) AS n" --format json

# Pipe Cypher from stdin
echo "MATCH (n) RETURN n LIMIT 5" | neo4j-cli query --format json

# Pass typed parameters with --param (repeatable; JSON values are auto-typed)
neo4j-cli query "MATCH (p:Person {name: $name}) RETURN p" --param name=Alice --format json

# Embed text inline as a vector parameter via the :embed modifier
neo4j-cli query "CALL db.index.vector.queryNodes('idx', 5, $v) YIELD node RETURN node" --param v:embed="hello world" --format json

# Route to the single running Neo4j Desktop 2 DBMS at runtime (no persisted credential)
neo4j-cli query "MATCH (n) RETURN count(n)" --credential desktop --format json

# Route to a saved Neo4j Desktop 2 remote connection by uuid (see 'neo4j-cli desktop list')
neo4j-cli query "MATCH (n) RETURN count(n)" --credential desktop-connection:f4e2f3c0-1111-2222-3333-444455556666 --format json

# Target a specific database on the running Neo4j Desktop 2 DBMS
neo4j-cli query "MATCH (n) RETURN count(n)" --credential desktop --database movies --format json

# Use a persisted dbms credential by name (see 'neo4j-cli credential dbms list')
neo4j-cli query "MATCH (n) RETURN count(n)" --credential local --format json

# Write Cypher requires --rw (opt-in)
neo4j-cli query "CREATE (n:Person {name: \"Alice\"}) RETURN n" --rw --format json

# Run multiple read statements in one call (split on ; at end of line); results render as a JSON array
neo4j-cli query "MATCH (n:Person) RETURN count(n) AS people; MATCH (m:Movie) RETURN count(m) AS movies" --format json

# Run multiple write statements atomically — all in one transaction, rolled back if any fails
neo4j-cli query "CREATE (:Person {name: \"Alice\"}); CREATE (:Person {name: \"Bob\"})" --rw --atomic --format json

# Import many write statements, skipping over any that fail (reports each failure, exits non-zero)
neo4j-cli query "CREATE (:Person {name: \"Alice\"}); CREATE (:Person {name: \"Bob\"})" --rw --continue-on-error --format json
```

## neo4j-cli query :embed

Compute an embedding vector for the given text

Compute an embedding vector for the supplied text using the configured embed provider. Text is taken from the positional argument, or from stdin when no argument is provided and stdin is piped. The embed provider configuration follows the same --embed-* flags as the parent `query` command. No Bolt connection is opened.

Usage: `neo4j-cli query :embed [text]`

Examples:

```
# Compute an embedding vector for inline text (JSON output)
neo4j-cli query :embed "hello world" --format json

# Pipe text from stdin and render the vector as toon
echo "hello world" | neo4j-cli query :embed --format toon

# Override the embed provider and model inline (no stored credential needed)
neo4j-cli query :embed "hello" --embed-provider openai --embed-model text-embedding-3-small --format json
```

## neo4j-cli query :lint

Lint Cypher: report syntax and semantic errors, offline by default

Check Cypher for syntax and semantic problems using the same analysis that powers Neo4j's language tooling. Offline by default: no Bolt connection is opened and no credentials are needed. With `--fetch-schema` the database schema (labels, relationship types, property keys, graph shape, default Cypher version) is fetched first — connection resolved like the other query commands — enabling additional schema-aware warnings: unknown labels or relationship types, and relationship patterns that contradict the graph's actual direction. Schema warnings never affect the exit code. Declaring parameters with `--param` switches parameter checking on: any $parameter not declared is an error; without `--param` parameter checks are skipped. Cypher is taken from the positional argument, or from stdin when no argument is provided and stdin is piped. `--cypher-version` selects the language dialect (5 or 25; default 5); a `CYPHER 5`/`CYPHER 25` prologue in the query always wins, and with `--fetch-schema` the database's default language applies unless `--cypher-version` is set explicitly. Each diagnostic renders as one row with a severity of `error` or `warning`, a message, and 1-indexed line/column plus 0-indexed character offsets. Exit code is 6 when any error-severity diagnostic is found; a clean or warnings-only result exits 0. The first call in a process takes a few seconds to initialize the analysis engine.

Usage: `neo4j-cli query :lint [cypher] [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--cypher-version` | string | 5 | Cypher language version to lint against: 5 or 25 |
| `--fetch-schema` | bool | false | Fetch the schema from the database before linting, enabling schema-aware warnings (unknown labels/relationship types, path directionality) |

Examples:

```
# Lint a Cypher statement offline; diagnostics as JSON, exit code 6 on errors
neo4j-cli query :lint "MATCH (n) RETURN m" --format json

# Lint against the connected database's schema (catches unknown labels/rel-types)
neo4j-cli query :lint "MATCH (n:Persn)-[:ACTED_IN]->(m) RETURN m" --fetch-schema --format json

# Declare parameters to catch misspelled ones (undeclared $unknown errors)
neo4j-cli query :lint "RETURN $known + $unknown" --param known=1 --format json

# Pipe Cypher from stdin and lint against Cypher 25 semantics
cat query.cypher | neo4j-cli query :lint --cypher-version 25 --format json

# Human-readable diagnostics table
neo4j-cli query :lint "MATCH (n) RETURN n" --format table
```

## neo4j-cli query :schema

Introspect the connected database (labels, rel types, indexes, constraints)

Run this BEFORE generating Cypher to discover the database's actual labels, relationship types, and properties — never guess. Introspect the connected database. Runs a sequence of read-only cypher calls and aggregates the result into one structured payload with database info, node/relationship properties, relationship paths, indexes, and constraints. --max-rows and --truncate-arrays-over do not apply.

Usage: `neo4j-cli query :schema`

Examples:

```
# Discover labels, rel types, and properties (run this before writing Cypher)
neo4j-cli query :schema --format toon

# Same introspection rendered as a single JSON payload (machine-readable)
neo4j-cli query :schema --format json

# Pipe the JSON into jq to inspect just the index names
neo4j-cli query :schema --format json | jq '.indexes[].name'
```

