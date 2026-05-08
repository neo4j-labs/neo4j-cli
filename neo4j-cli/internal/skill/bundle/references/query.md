# neo4j-cli query

Run Cypher against a Neo4j database via the Bolt protocol

Run a Cypher statement against a Neo4j database via the Bolt protocol. Cypher is taken from the positional argument, or from stdin when no argument is provided and stdin is piped. Use `--param NAME:embed=<text>` to inject an embedding vector inline (text is sent to the configured embedding provider, the resulting vector is bound to $NAME for both EXPLAIN preflight and the real run). The sibling `query :embed [text]` leaf computes a vector standalone without opening a Bolt connection.

Usage: `neo4j-cli query [cypher]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-c, --credential` | string | - | Name of a stored dbms credential to use for the connection (see 'neo4j-cli credential dbms list') |
| `-d, --database` | string | - | Target database name [env: NEO4J_DATABASE] (default "neo4j") |
| `--embed-base-url` | string | - | Embedding provider base URL [env: NEO4J_EMBED_BASE_URL] |
| `--embed-credential` | string | - | Name of a stored embed credential to seed embedding config (see 'neo4j-cli credential embed list') |
| `--embed-dimensions` | int | 0 | Embedding output dimensions (provider-dependent; ignored by Ollama) [env: NEO4J_EMBED_DIMENSIONS] |
| `--embed-model` | string | - | Embedding model name [env: NEO4J_EMBED_MODEL] |
| `--embed-provider` | string | - | Embedding provider: openai \| ollama \| huggingface [env: NEO4J_EMBED_PROVIDER] |
| `--env` | string | - | Path to a .env file (auto-discovered by walking up from cwd if unset) |
| `-f, --format` | string | - | Format to print console output in, from a choice of [default, json, table, toon]. (agents: prefer toon) |
| `--max-rows` | int | 100 | Maximum rows to print (0 = unlimited); when capped, prints a stderr warning and sets truncated=true in JSON |
| `--param` | stringArray | [] | Query parameter as key=value (repeatable); JSON-typed when value parses as JSON, otherwise treated as a string. Use `key:embed=<text>` to embed text via the configured provider and bind the resulting vector to $key (see `query :embed`). |
| `-p, --password` | string | - | Neo4j password [env: NEO4J_PASSWORD]; prompted on TTY if unset |
| `--truncate-arrays-over` | int | 100 | Recursively truncate any array longer than N inside row values (0 = off); rendered as ["<truncated: K items>"] |
| `--uri` | string | - | Neo4j Bolt URI [env: NEO4J_URI]. http://<host>[:p][/...] is auto-rewritten to neo4j://<host>:7687; https://<host>[:p][/...] is auto-rewritten to neo4j+s://<host>:7687. (default "neo4j://localhost:7687") |
| `-u, --username` | string | - | Neo4j username [env: NEO4J_USERNAME] (default "neo4j") |

## neo4j-cli query :embed

Compute an embedding vector for the given text

Compute an embedding vector for the supplied text using the configured embed provider. Text is taken from the positional argument, or from stdin when no argument is provided and stdin is piped. The embed provider configuration follows the same --embed-* flags as the parent `query` command. No Bolt connection is opened.

Usage: `neo4j-cli query :embed [text]`

Examples:

```
neo4j-cli query :embed "hello world" --format json
echo "hello world" | neo4j-cli query :embed --format toon
neo4j-cli query :embed "hello" --embed-provider openai --embed-model text-embedding-3-small
```

## neo4j-cli query :schema

Introspect the connected database (labels, rel types, indexes, constraints)

Introspect the connected database. Runs a sequence of read-only cypher calls and aggregates the result into one structured payload with database info, node/relationship properties, relationship paths, indexes, and constraints. --max-rows and --truncate-arrays-over do not apply.

Usage: `neo4j-cli query :schema`

