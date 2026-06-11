# neo4j-cli admin

## Contents

- [neo4j-cli admin database](#neo4j-cli-admin-database)
- [neo4j-cli admin database create](#neo4j-cli-admin-database-create)
- [neo4j-cli admin database drop](#neo4j-cli-admin-database-drop)
- [neo4j-cli admin database get](#neo4j-cli-admin-database-get)
- [neo4j-cli admin database list](#neo4j-cli-admin-database-list)
- [neo4j-cli admin database start](#neo4j-cli-admin-database-start)
- [neo4j-cli admin database stop](#neo4j-cli-admin-database-stop)

Manage Neo4j databases, users, and roles

Manage Neo4j databases via the system database. Connects over Bolt using the supplied connection flags or a stored dbms credential (use '--credential <name>' for a named credential, '--credential desktop' for a running Neo4j Desktop 2 DBMS, or '--credential desktop-connection:<uuid>' for a saved Desktop connection). Subcommands: `database` (list, get, create, drop, start, stop).

Usage: `neo4j-cli admin`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-c, --credential` | string | - | Name of a stored dbms credential, 'desktop' for a running Neo4j Desktop 2 DBMS, or 'desktop-connection:<uuid>' for a saved connection |
| `--debug` | bool | false | Enable Bolt driver debug logging to stderr (env: NEO4J_DEBUG=1) |
| `--env` | string | - | Path to a .env file with NEO4J_URI / NEO4J_USERNAME / NEO4J_PASSWORD (walks up from cwd when unset) |
| `-p, --password` | string | - | Neo4j password (env: NEO4J_PASSWORD) |
| `--uri` | string | - | Neo4j server URI (env: NEO4J_URI) |
| `-u, --username` | string | - | Neo4j username (env: NEO4J_USERNAME) |

## neo4j-cli admin database

Manage Neo4j databases via the system database

Manage Neo4j databases. Read commands (list, get) do not require --rw. Write commands (create, drop, start, stop) require --rw.

Usage: `neo4j-cli admin database`

Examples:

```
# Show help for the database subcommands
neo4j-cli admin database --help

# List all databases (read-only)
neo4j-cli admin database list --credential local --format json
```

### neo4j-cli admin database create

Create a database

Create a database via CREATE DATABASE <name> IF NOT EXISTS against the system database. Pass --wait to block until the database status is online (polls every 1 second, 60-second timeout).

Usage: `neo4j-cli admin database create <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--wait` | bool | false | Wait until the database is online before returning (polls every 1 second, 60-second timeout). |

Examples:

```
# Create a database
neo4j-cli admin database create mydb --credential local --rw

# Create a database and wait until it is online before returning
neo4j-cli admin database create mydb --credential local --wait --rw
```

### neo4j-cli admin database drop

Drop a database

Drop a database via DROP DATABASE <name> against the system database. Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli admin database drop <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Drop a database, prompting for confirmation on a TTY
neo4j-cli admin database drop mydb --credential local --rw

# Drop a database without prompting (required for scripts and non-TTY callers)
neo4j-cli admin database drop mydb --credential local --rw --yes --force
```

### neo4j-cli admin database get

Get details of a database

Get the full record for a single database by name. Executes SHOW DATABASE $name against the system database.

Usage: `neo4j-cli admin database get <name>`

Examples:

```
# Get a database record as a table
neo4j-cli admin database get neo4j --credential local

# Get a database record as JSON for scripting
neo4j-cli admin database get neo4j --credential local --format json
```

### neo4j-cli admin database list

List all databases

List all databases visible from the system database. Renders name, type, currentStatus, access, and default columns.

Usage: `neo4j-cli admin database list`

Examples:

```
# List all databases as a table
neo4j-cli admin database list --credential local

# List all databases as JSON for scripting
neo4j-cli admin database list --credential local --format json
```

### neo4j-cli admin database start

Start a database

Start a database via START DATABASE <name> against the system database. Pass --wait to block until the database status is online (polls every 1 second, 60-second timeout).

Usage: `neo4j-cli admin database start <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--wait` | bool | false | Wait until the database is online before returning (polls every 1 second, 60-second timeout). |

Examples:

```
# Start a database
neo4j-cli admin database start mydb --credential local --rw

# Start a database and wait until it is online before returning
neo4j-cli admin database start mydb --credential local --wait --rw
```

### neo4j-cli admin database stop

Stop a database

Stop a database via STOP DATABASE <name> against the system database. Pass --wait to block until the database status is offline (polls every 1 second, 60-second timeout).

Usage: `neo4j-cli admin database stop <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--wait` | bool | false | Wait until the database is offline before returning (polls every 1 second, 60-second timeout). |

Examples:

```
# Stop a database
neo4j-cli admin database stop mydb --credential local --rw

# Stop a database and wait until it is offline before returning
neo4j-cli admin database stop mydb --credential local --wait --rw
```

