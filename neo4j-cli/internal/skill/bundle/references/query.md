# neo4j-cli query

Run Cypher against a Neo4j database via the Bolt protocol

Run a Cypher statement against a Neo4j database via the Bolt protocol. Cypher is taken from the positional argument, or from stdin when no argument is provided and stdin is piped.

Usage: `neo4j-cli query [cypher]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--credential` | string | - | Name of a stored dbms credential to use for the connection (see 'credential dbms list') |
| `-d, --database` | string | - | Target database name [env: NEO4J_DATABASE] (default "neo4j") |
| `--env` | string | - | Path to a .env file (auto-discovered by walking up from cwd if unset) |
| `-f, --format` | string | - | Format to print console output in, from a choice of [default, json, table, toon]. (agents: prefer toon) |
| `--max-rows` | int | 100 | Maximum rows to print (0 = unlimited); when capped, prints a stderr warning and sets truncated=true in JSON |
| `--param` | stringArray | [] | Query parameter as key=value (repeatable); JSON-typed when value parses as JSON, otherwise treated as a string |
| `-p, --password` | string | - | Neo4j password [env: NEO4J_PASSWORD]; prompted on TTY if unset |
| `--truncate-arrays-over` | int | 100 | Recursively truncate any array longer than N inside row values (0 = off); rendered as ["<truncated: K items>"] |
| `--uri` | string | - | Neo4j Bolt URI [env: NEO4J_URI]. http://<host>[:p][/...] is auto-rewritten to neo4j://<host>:7687; https://<host>[:p][/...] is auto-rewritten to neo4j+s://<host>:7687. (default "neo4j://localhost:7687") |
| `-u, --username` | string | - | Neo4j username [env: NEO4J_USERNAME] (default "neo4j") |

## neo4j-cli query :schema

Introspect the connected database (labels, rel types, indexes, constraints)

Introspect the connected database. Runs a sequence of read-only cypher calls and aggregates the result into one structured payload with database info, node/relationship properties, relationship paths, indexes, and constraints. --max-rows and --truncate-arrays-over do not apply.

Usage: `neo4j-cli query :schema`

