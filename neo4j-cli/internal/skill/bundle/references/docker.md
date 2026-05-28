# neo4j-cli docker

## Contents

- [neo4j-cli docker create](#neo4j-cli-docker-create)
- [neo4j-cli docker delete](#neo4j-cli-docker-delete)
- [neo4j-cli docker get](#neo4j-cli-docker-get)
- [neo4j-cli docker list](#neo4j-cli-docker-list)
- [neo4j-cli docker start](#neo4j-cli-docker-start)
- [neo4j-cli docker stop](#neo4j-cli-docker-stop)

Manage local Neo4j containers via Docker

Manage local Neo4j Docker containers (create, list, get, start, stop, delete). Shells out to the host `docker` CLI and discovers managed containers via the `org.neo4j.cli.managed=true` label — Docker itself is the source of truth, no separate state file is maintained. Use `--ephemeral` on `create` for a throwaway container plus an env-file consumable by `query --env <path>`.

Usage: `neo4j-cli docker`

## neo4j-cli docker create

Create a local Neo4j Docker container

Create a local Neo4j Docker container via `docker run -d` and (unless --no-store-credential) store a matching dbms credential so `neo4j-cli query --credential <name>` can connect immediately. The container carries `org.neo4j.cli.managed=true` plus a small set of metadata labels — Docker itself is the source of truth, no separate state file is maintained. When --password is omitted, a 16-byte base64 URL-safe password is generated and surfaced in the output. If --name collides with an existing container or stored dbms credential, the chosen name is auto-suffixed (`<name>-1`, `<name>-2`, …) and the chosen name is logged to stderr. Pass --wait to block until the container's Bolt endpoint accepts sessions (60s timeout); on timeout the container is left running so the operator can inspect it with `docker logs <name>`. Pass --ephemeral for a throwaway container (`docker run --rm`): no dbms credential is stored and an env-file blob (NEO4J_URI / NEO4J_USERNAME / NEO4J_PASSWORD / NEO4J_DATABASE) is emitted to stdout — or, with --env-out-file <path>, written to that path (mode 0600) while stdout stays silent so it can be piped into `neo4j-cli query --env <path>`. The env-file is written via a temp file in the same directory and atomically renamed; a pre-existing symlink at the target path is REPLACED by a regular file (the symlink is not followed). When the requested --bolt-port and --http-port pair is taken, both ports are auto-incremented by the same offset (up to 100 attempts) and the chosen pair is reported on stderr. Use --data-dir / --logs-dir / --import-dir to bind-mount host directories at /data, /logs, /import inside the container. Paths support `~` and environment-variable expansion and are resolved to absolute paths; missing directories are created at mode 0o755. All three volume flags are incompatible with --ephemeral. Pass --no-print-password to omit the generated password from stdout output; retrieve it later via `neo4j-cli credential dbms get <name>`.

Usage: `neo4j-cli docker create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--accept-license` | bool | false | Accept the Neo4j Commercial License (sets NEO4J_ACCEPT_LICENSE_AGREEMENT=yes; default is eval). Ignored for community edition. |
| `--bolt-port` | int | 7687 | Host port to publish for Bolt (container 7687). Auto-incremented along with --http-port if taken. |
| `--data-dir` | string | - | Host directory to bind-mount at /data inside the container. Empty = no mount (data lives in the container layer and is lost on delete). Path supports `~` and environment-variable expansion; resolved to an absolute path; created at mode 0o755 if missing. Incompatible with --ephemeral. |
| `--edition` | string | enterprise | Neo4j edition. Must be one of "community" or "enterprise". |
| `--env-out-file` | string | - | When --ephemeral, write the .env blob to this path (mode 0600) instead of stdout. Writes via a temp file in the same directory and atomically renames; a pre-existing symlink at the path is replaced by a regular file. |
| `--ephemeral` | bool | false | Run with `docker run --rm`; skip credential persistence and emit a .env blob consumable by `query --env`. |
| `--http-port` | int | 7474 | Host port to publish for the HTTP browser (container 7474). Auto-incremented along with --bolt-port if taken. |
| `--import-dir` | string | - | Host directory to bind-mount at /import inside the container (used by Neo4j's LOAD CSV). Empty = no mount. Same expansion + mkdir rules as --data-dir. Incompatible with --ephemeral. |
| `--logs-dir` | string | - | Host directory to bind-mount at /logs inside the container. Empty = no mount. Same expansion + mkdir rules as --data-dir. Incompatible with --ephemeral. |
| `--name` | string | - | (required) Container name. Also used as the dbms credential name. |
| `--no-print-password` | bool | false | Don't include the generated password in stdout output. Retrieve later via `neo4j-cli credential dbms get <name>`. |
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
neo4j-cli docker create --name tmp --ephemeral --env-out-file /tmp/n.env --rw

# Persist data on the host so it survives delete + recreate
neo4j-cli docker create --name dev --data-dir ~/n4j-data --rw

# Create an enterprise container with the commercial license accepted and a custom password (no credential stored)
neo4j-cli docker create --name licensed --edition enterprise --accept-license --password mysecret --no-store-credential --rw
```

## neo4j-cli docker delete

Remove a Neo4j container and its dbms credential

Remove a Neo4j Docker container by name and best-effort delete its stored dbms credential. Only containers carrying `org.neo4j.cli.managed=true` are eligible; unknown or unmanaged names return a usage error pointing at `neo4j-cli docker list`. Destructive: requires `--yes --force` (or a `y` answer at the TTY prompt) when invoked non-interactively. A missing dbms credential is NOT an error — the container is still removed. Daemon-side errors (Docker not running, socket permission denied, etc.) are surfaced verbatim and are distinct from the unknown-name error.

Usage: `neo4j-cli docker delete <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Delete a managed container; prompts on a TTY
neo4j-cli docker delete dev --rw

# Skip the prompt (required for scripts / non-TTY callers)
neo4j-cli docker delete dev --yes --force --rw

# Delete and confirm by listing remaining managed containers
neo4j-cli docker delete dev --yes --force --rw && neo4j-cli docker list --format json
```

## neo4j-cli docker get

Show details of a Neo4j container managed by neo4j-cli

Show details of a single Neo4j Docker container carrying the `org.neo4j.cli.managed=true` label. Renders name, status (Docker's human-readable state), edition, version, bolt-port, http-port, ephemeral, uri (neo4j://localhost:<bolt-port>), and image. Containers that exist in Docker but lack the managed label are treated as unknown; the error message points at `neo4j-cli docker list` so the operator can see the actual set of managed containers. Daemon-side errors (Docker not running, socket permission denied, etc.) are surfaced verbatim and are distinct from the unknown-name error so you can tell a missing container apart from a missing daemon.

Usage: `neo4j-cli docker get <name>`

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

Start a stopped Neo4j Docker container by name. Only containers carrying `org.neo4j.cli.managed=true` are eligible; unknown or unmanaged names return a usage error pointing at `neo4j-cli docker list`. Pass --wait to block until the container's Bolt endpoint is reachable (60s timeout). When a stored dbms credential exists for the container, --wait performs an authenticated Bolt handshake. When no credential is stored (e.g. created with --no-store-credential, or managed externally), --wait falls back to a TCP-only probe — weaker (Neo4j may bind the port briefly before Bolt is fully ready) but strictly better than no wait. Ephemeral containers (`--rm`) are removed by Docker when they stop, so attempting to start one after it has exited surfaces the same unknown-name error. Daemon-side errors (Docker not running, socket permission denied, etc.) are surfaced verbatim and are distinct from the unknown-name error.

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
```

## neo4j-cli docker stop

Stop a running Neo4j container managed by neo4j-cli

Stop a running Neo4j Docker container by name. Only containers carrying `org.neo4j.cli.managed=true` are eligible; unknown or unmanaged names return a usage error pointing at `neo4j-cli docker list`. Pass --wait to block until the container has actually exited (60s timeout). Ephemeral containers (`--rm`) are removed by Docker the moment they exit, so a subsequent `neo4j-cli docker get` will return the same unknown-name error. Daemon-side errors (Docker not running, socket permission denied, etc.) are surfaced verbatim and are distinct from the unknown-name error.

Usage: `neo4j-cli docker stop <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--wait` | bool | false | Wait until the container has exited before returning. |

Examples:

```
# Stop a managed container by name
neo4j-cli docker stop dev --rw

# Stop and block until the container has fully exited before returning
neo4j-cli docker stop dev --wait --rw
```

