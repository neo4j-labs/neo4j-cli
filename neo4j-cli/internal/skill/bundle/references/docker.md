# neo4j-cli docker

Manage local Neo4j containers via Docker

Manage local Neo4j Docker containers (create, list, get, start, stop, delete). Shells out to the host `docker` CLI and discovers managed containers via the `org.neo4j.cli.managed=true` label — Docker itself is the source of truth, no separate state file is maintained. Use `--ephemeral` on `create` for a throwaway container plus an env-file consumable by `query --env <path>`.

Usage: `neo4j-cli docker`

