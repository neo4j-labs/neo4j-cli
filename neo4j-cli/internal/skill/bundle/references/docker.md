# neo4j-cli docker

Manage local Neo4j containers via Docker

Manage local Neo4j Docker containers (create, list, get, start, stop, delete). Shells out to the host `docker` CLI and discovers managed containers via the `org.neo4j.cli.managed=true` label — Docker itself is the source of truth, no separate state file is maintained. Use `--ephemeral` on `create` for a throwaway container plus an env-file consumable by `query --env <path>`.

Usage: `neo4j-cli docker`

## neo4j-cli docker create

Create a local Neo4j Docker container

Create a local Neo4j Docker container via `docker run -d` and (unless --no-store-credential) store a matching dbms credential so `neo4j-cli query --credential <name>` can connect immediately. The container carries `org.neo4j.cli.managed=true` plus a small set of metadata labels — Docker itself is the source of truth, no separate state file is maintained. When --password is omitted, a 16-byte base64 URL-safe password is generated and surfaced in the output. If --name collides with an existing container or stored dbms credential, the chosen name is auto-suffixed (`<name>-1`, `<name>-2`, …) and the chosen name is logged to stderr.

Usage: `neo4j-cli docker create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--accept-license` | bool | false | Accept the Neo4j Commercial License (sets NEO4J_ACCEPT_LICENSE_AGREEMENT=yes; default is eval). Ignored for community edition. |
| `--bolt-port` | int | 7687 | Host port to publish for Bolt (container 7687). |
| `--edition` | string | enterprise | Neo4j edition. Must be one of "community" or "enterprise". |
| `--http-port` | int | 7474 | Host port to publish for the HTTP browser (container 7474). |
| `--name` | string | - | (required) Container name. Also used as the dbms credential name. |
| `--no-store-credential` | bool | false | Skip persisting a dbms credential for this container. |
| `--password` | string | - | Neo4j password. When empty, a 16-byte base64 URL-safe password is generated. |
| `--version` | string | latest | Neo4j version tag (e.g. 5.20, latest). |

Examples:

```
# Create an enterprise container with auto-generated password and store a dbms credential
neo4j-cli docker create --name dev --rw

# Create a community container on a non-default bolt port; emit JSON for scripting
neo4j-cli docker create --name local --edition community --bolt-port 7688 --http-port 7475 --rw --format json

# Create an enterprise container with the commercial license accepted and a custom password (no credential stored)
neo4j-cli docker create --name licensed --edition enterprise --accept-license --password mysecret --no-store-credential --rw
```

