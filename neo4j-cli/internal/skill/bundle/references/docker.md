# neo4j-cli docker

## Contents

- [neo4j-cli docker create](#neo4j-cli-docker-create)
- [neo4j-cli docker get](#neo4j-cli-docker-get)
- [neo4j-cli docker list](#neo4j-cli-docker-list)
- [neo4j-cli docker start](#neo4j-cli-docker-start)

Manage local Neo4j containers via Docker

Manage local Neo4j Docker containers (create, list, get, start, stop, delete). Shells out to the host `docker` CLI and discovers managed containers via the `org.neo4j.cli.managed=true` label — Docker itself is the source of truth, no separate state file is maintained. Use `--ephemeral` on `create` for a throwaway container plus an env-file consumable by `query --env <path>`.

Usage: `neo4j-cli docker`

## neo4j-cli docker create

Create a local Neo4j Docker container

Create a local Neo4j Docker container via `docker run -d` and (unless --no-store-credential) store a matching dbms credential so `neo4j-cli query --credential <name>` can connect immediately. The container carries `org.neo4j.cli.managed=true` plus a small set of metadata labels — Docker itself is the source of truth, no separate state file is maintained. When --password is omitted, a 16-byte base64 URL-safe password is generated and surfaced in the output. If --name collides with an existing container or stored dbms credential, the chosen name is auto-suffixed (`<name>-1`, `<name>-2`, …) and the chosen name is logged to stderr. Pass --wait to block until the container's Bolt endpoint accepts sessions (60s timeout); on timeout the container is left running so the operator can inspect it with `docker logs <name>`. Pass --ephemeral for a throwaway container (`docker run --rm`): no dbms credential is stored and an env-file blob (NEO4J_URI / NEO4J_USERNAME / NEO4J_PASSWORD / NEO4J_DATABASE) is emitted to stdout — or, with --env-file <path>, written to that path (mode 0600) while stdout stays silent so it can be piped into `neo4j-cli query --env <path>`.

Usage: `neo4j-cli docker create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--accept-license` | bool | false | Accept the Neo4j Commercial License (sets NEO4J_ACCEPT_LICENSE_AGREEMENT=yes; default is eval). Ignored for community edition. |
| `--bolt-port` | int | 7687 | Host port to publish for Bolt (container 7687). |
| `--edition` | string | enterprise | Neo4j edition. Must be one of "community" or "enterprise". |
| `--env-file` | string | - | When --ephemeral, write the .env blob to this path (mode 0600) instead of stdout. |
| `--ephemeral` | bool | false | Run with `docker run --rm`; skip credential persistence and emit a .env blob consumable by `query --env`. |
| `--http-port` | int | 7474 | Host port to publish for the HTTP browser (container 7474). |
| `--name` | string | - | (required) Container name. Also used as the dbms credential name. |
| `--no-store-credential` | bool | false | Skip persisting a dbms credential for this container. |
| `--password` | string | - | Neo4j password. When empty, a 16-byte base64 URL-safe password is generated. |
| `--version` | string | latest | Neo4j version tag (e.g. 5.20, latest). |
| `--wait` | bool | false | Wait until Bolt is reachable before returning. |

Examples:

```
# Create an enterprise container with auto-generated password and store a dbms credential
neo4j-cli docker create --name dev --rw

# Create a community container on a non-default bolt port; emit JSON for scripting
neo4j-cli docker create --name local --edition community --bolt-port 7688 --http-port 7475 --rw --format json

# Create an enterprise container and block until Bolt is reachable before returning
neo4j-cli docker create --name dev --wait --rw

# Create an ephemeral container and emit an env-file blob to stdout for piping into another tool
neo4j-cli docker create --name tmp --ephemeral --rw

# Create an ephemeral container and write the env-file to a path that 'query --env' can consume
neo4j-cli docker create --name tmp --ephemeral --env-file /tmp/n.env --rw

# Create an enterprise container with the commercial license accepted and a custom password (no credential stored)
neo4j-cli docker create --name licensed --edition enterprise --accept-license --password mysecret --no-store-credential --rw
```

## neo4j-cli docker get

Show details of a Neo4j container managed by neo4j-cli

Show details of a single Neo4j Docker container carrying the `org.neo4j.cli.managed=true` label. Renders name, status (Docker's human-readable state), edition, version, bolt-port, http-port, ephemeral, uri (neo4j://localhost:<bolt-port>), and image. Containers that exist in Docker but lack the managed label are treated as unknown; the error message points at `neo4j-cli docker list` so the operator can see the actual set of managed containers.

Usage: `neo4j-cli docker get <name>`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | - | Format to print console output in, from a choice of [default, json, table, toon]. (agents: prefer toon) |

Examples:

```
# Show details of a managed container by name
neo4j-cli docker get dev

# Emit JSON for scripting (e.g. piping into jq to extract the URI)
neo4j-cli docker get dev --format json

# Emit TOON for token-efficient ingestion by agents
neo4j-cli docker get dev --format toon
```

## neo4j-cli docker list

List Neo4j containers managed by neo4j-cli

List all Neo4j Docker containers carrying the `org.neo4j.cli.managed=true` label. Renders one row per container with name, status (Docker's human-readable state), edition, version, bolt-port, http-port, and ephemeral. Unmanaged containers (no label) are excluded. An empty result renders as an empty table or empty JSON array (exit 0).

Usage: `neo4j-cli docker list`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | - | Format to print console output in, from a choice of [default, json, table, toon]. (agents: prefer toon) |

Examples:

```
# List managed Neo4j containers as a table
neo4j-cli docker list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli docker list --format json

# Emit TOON for token-efficient ingestion by agents
neo4j-cli docker list --format toon
```

## neo4j-cli docker start

Start a stopped Neo4j container managed by neo4j-cli

Start a stopped Neo4j Docker container by name. Only containers carrying `org.neo4j.cli.managed=true` are eligible; unknown or unmanaged names return a usage error pointing at `neo4j-cli docker list`. Pass --wait to block until the container's Bolt endpoint accepts sessions (60s timeout); --wait requires a stored dbms credential for the container (the credential supplies the password used to authenticate the readiness probe). Ephemeral containers (`--rm`) are removed by Docker when they stop, so attempting to start one after it has exited surfaces the same unknown-name error.

Usage: `neo4j-cli docker start <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--wait` | bool | false | Wait until Bolt is reachable before returning. |

Examples:

```
# Start a managed container by name
neo4j-cli docker start dev --rw

# Start and block until Bolt accepts sessions before returning
neo4j-cli docker start dev --wait --rw

# Same as above using the deprecated --await alias
neo4j-cli docker start dev --await --rw
```

