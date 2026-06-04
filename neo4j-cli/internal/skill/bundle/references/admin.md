# neo4j-cli admin

Manage Neo4j databases, users, and roles

Manage Neo4j databases, users, and roles via the system database. All subcommands connect over Bolt using the dbms credential named by --credential (see `neo4j-cli credential dbms list`). Subcommands: `database` (list, get, create, drop, start, stop), `user` (list, get, create, drop, rename, set-password, suspend, activate), `role` (list, get, create, drop, grant, revoke — Enterprise only).

Usage: `neo4j-cli admin`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--credential` | string | - | Name of the stored dbms credential to use (see `neo4j-cli credential dbms list`) |

## neo4j-cli admin database

Manage Neo4j databases via the system database

Manage Neo4j databases. Read commands (list, get) do not require --rw. Write commands (create, drop, start, stop) require --rw and use the dbms credential named by --credential on the parent `admin` command.

Usage: `neo4j-cli admin database`

Examples:

```
# Show help for the database subcommands
neo4j-cli admin database --help

# List all databases (read-only)
neo4j-cli admin database list --credential local --format json
```

## neo4j-cli admin role

Manage Neo4j roles via the system database

Manage Neo4j roles (Enterprise edition only). Read commands (list, get) do not require --rw. Write commands (create, drop, grant, revoke) require --rw and use the dbms credential named by --credential on the parent `admin` command.

Usage: `neo4j-cli admin role`

Examples:

```
# Show help for the role subcommands
neo4j-cli admin role --help

# List all roles (read-only)
neo4j-cli admin role list --credential local --format json
```

## neo4j-cli admin user

Manage Neo4j users via the system database

Manage Neo4j users. Read commands (list, get) do not require --rw. Write commands (create, drop, rename, set-password, suspend, activate) require --rw and use the dbms credential named by --credential on the parent `admin` command.

Usage: `neo4j-cli admin user`

Examples:

```
# Show help for the user subcommands
neo4j-cli admin user --help

# List all users (read-only)
neo4j-cli admin user list --credential local --format json
```

