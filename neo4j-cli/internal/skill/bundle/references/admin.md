# neo4j-cli admin

## Contents

- [neo4j-cli admin database](#neo4j-cli-admin-database)
- [neo4j-cli admin database create](#neo4j-cli-admin-database-create)
- [neo4j-cli admin database drop](#neo4j-cli-admin-database-drop)
- [neo4j-cli admin database get](#neo4j-cli-admin-database-get)
- [neo4j-cli admin database list](#neo4j-cli-admin-database-list)
- [neo4j-cli admin database start](#neo4j-cli-admin-database-start)
- [neo4j-cli admin database stop](#neo4j-cli-admin-database-stop)
- [neo4j-cli admin privilege](#neo4j-cli-admin-privilege)
- [neo4j-cli admin privilege deny](#neo4j-cli-admin-privilege-deny)
- [neo4j-cli admin privilege deny database](#neo4j-cli-admin-privilege-deny-database)
- [neo4j-cli admin privilege deny dbms](#neo4j-cli-admin-privilege-deny-dbms)
- [neo4j-cli admin privilege deny entity](#neo4j-cli-admin-privilege-deny-entity)
- [neo4j-cli admin privilege deny graph](#neo4j-cli-admin-privilege-deny-graph)
- [neo4j-cli admin privilege deny label](#neo4j-cli-admin-privilege-deny-label)
- [neo4j-cli admin privilege deny load](#neo4j-cli-admin-privilege-deny-load)
- [neo4j-cli admin privilege deny property](#neo4j-cli-admin-privilege-deny-property)
- [neo4j-cli admin privilege grant](#neo4j-cli-admin-privilege-grant)
- [neo4j-cli admin privilege grant database](#neo4j-cli-admin-privilege-grant-database)
- [neo4j-cli admin privilege grant dbms](#neo4j-cli-admin-privilege-grant-dbms)
- [neo4j-cli admin privilege grant entity](#neo4j-cli-admin-privilege-grant-entity)
- [neo4j-cli admin privilege grant graph](#neo4j-cli-admin-privilege-grant-graph)
- [neo4j-cli admin privilege grant label](#neo4j-cli-admin-privilege-grant-label)
- [neo4j-cli admin privilege grant load](#neo4j-cli-admin-privilege-grant-load)
- [neo4j-cli admin privilege grant property](#neo4j-cli-admin-privilege-grant-property)
- [neo4j-cli admin privilege list](#neo4j-cli-admin-privilege-list)
- [neo4j-cli admin privilege revoke](#neo4j-cli-admin-privilege-revoke)
- [neo4j-cli admin privilege revoke database](#neo4j-cli-admin-privilege-revoke-database)
- [neo4j-cli admin privilege revoke dbms](#neo4j-cli-admin-privilege-revoke-dbms)
- [neo4j-cli admin privilege revoke entity](#neo4j-cli-admin-privilege-revoke-entity)
- [neo4j-cli admin privilege revoke graph](#neo4j-cli-admin-privilege-revoke-graph)
- [neo4j-cli admin privilege revoke label](#neo4j-cli-admin-privilege-revoke-label)
- [neo4j-cli admin privilege revoke load](#neo4j-cli-admin-privilege-revoke-load)
- [neo4j-cli admin privilege revoke property](#neo4j-cli-admin-privilege-revoke-property)
- [neo4j-cli admin role](#neo4j-cli-admin-role)
- [neo4j-cli admin role create](#neo4j-cli-admin-role-create)
- [neo4j-cli admin role drop](#neo4j-cli-admin-role-drop)
- [neo4j-cli admin role get](#neo4j-cli-admin-role-get)
- [neo4j-cli admin role grant](#neo4j-cli-admin-role-grant)
- [neo4j-cli admin role list](#neo4j-cli-admin-role-list)
- [neo4j-cli admin role revoke](#neo4j-cli-admin-role-revoke)
- [neo4j-cli admin user](#neo4j-cli-admin-user)
- [neo4j-cli admin user activate](#neo4j-cli-admin-user-activate)
- [neo4j-cli admin user create](#neo4j-cli-admin-user-create)
- [neo4j-cli admin user drop](#neo4j-cli-admin-user-drop)
- [neo4j-cli admin user get](#neo4j-cli-admin-user-get)
- [neo4j-cli admin user list](#neo4j-cli-admin-user-list)
- [neo4j-cli admin user rename](#neo4j-cli-admin-user-rename)
- [neo4j-cli admin user set-password](#neo4j-cli-admin-user-set-password)
- [neo4j-cli admin user suspend](#neo4j-cli-admin-user-suspend)

Manage Neo4j databases, users, roles, and privileges

Manage Neo4j databases via the system database. Connects over Bolt using the supplied connection flags or a stored dbms credential (use '--credential <name>' for a named credential, '--credential desktop' for a running Neo4j Desktop 2 DBMS, or '--credential desktop-connection:<uuid>' for a saved Desktop connection). Subcommands: `database` (list, get, create, drop, start, stop), `user` (list, get, create, drop, rename, set-password, suspend, activate), `role` (list, get, create, drop, grant, revoke), `privilege` (list, grant, deny, revoke).

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

Get the full record for a single database by name. Executes SHOW DATABASE $name against the system database. Renders name, type, access, current_status, requested_status, status_message, address, role, writer, default, home, and database_id columns.

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

List all databases visible from the system database. Renders an overview with name, current_status, type, and default columns. Use `get` for the full record of a single database.

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

## neo4j-cli admin privilege

Manage Neo4j privileges

Manage Neo4j Enterprise privileges via the system database. Read commands (list) do not require --rw. Write commands (grant, deny, revoke) require --rw.

Usage: `neo4j-cli admin privilege`

Examples:

```
# Show help for the privilege subcommands
neo4j-cli admin privilege --help

# List all privileges (read-only)
neo4j-cli admin privilege list --credential local --format json
```

### neo4j-cli admin privilege deny

Deny a privilege to a role

Deny a privilege to a role via DENY <privilege> TO <role> against the system database. The action is a positional argument on a per-category subcommand (property, entity, graph, label, load, database, dbms); run `deny <category> --help` to see its actions and flags. After the deny, the role's updated privileges are printed.

Usage: `neo4j-cli admin privilege deny`

Examples:

```
# Deny WRITE on all graphs to the readonly role
neo4j-cli admin privilege deny graph write --on-graph * --role readonly --credential local --rw

# Deny CREATE ROLE (a DBMS privilege) to the analyst role, output as JSON
neo4j-cli admin privilege deny dbms create-role --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege deny database

Deny a database privilege (access, all-database-privileges, constraint-management, … — 19 actions; see --help) to a role

Deny a database privilege to a role. The action is the positional argument; valid actions are: access, all-database-privileges, constraint-management, create-constraint, create-index, create-new-node-label, create-new-property-name, create-new-relationship-type, drop-constraint, drop-index, index-management, name-management, show-constraint, show-index, show-transaction, start, stop, terminate-transaction, transaction-management. Scope with --on-database (default *). --role is required.

Usage: `neo4j-cli admin privilege deny database <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--on-database` | string | - | Scope the privilege to a database (use * for all) |
| `--role` | string | - | Name of the role to deny the privilege to |

Examples:

```
# Deny a database privilege to the analyst role
neo4j-cli admin privilege deny database access --on-database neo4j --role analyst --credential local --rw

# Deny the same privilege, output as JSON
neo4j-cli admin privilege deny database access --on-database neo4j --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege deny dbms

Deny a DBMS privilege (all-dbms-privileges, alter-user, assign-role, … — 19 actions; see --help) to a role

Deny a DBMS privilege to a role. The action is the positional argument; valid actions are: all-dbms-privileges, alter-user, assign-role, create-database, create-role, create-user, database-management, drop-database, drop-role, drop-user, privilege-management, remove-role, role-management, set-user-home-database, set-user-status, show-privilege, show-role, show-user, user-management. Applies to the whole DBMS; takes no scope flag. --role is required.

Usage: `neo4j-cli admin privilege deny dbms <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--role` | string | - | Name of the role to deny the privilege to |

Examples:

```
# Deny a DBMS privilege to the analyst role
neo4j-cli admin privilege deny dbms create-role --role analyst --credential local --rw

# Deny the same privilege, output as JSON
neo4j-cli admin privilege deny dbms create-role --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege deny entity

Deny a graph entity privilege (create, delete, traverse) to a role

Deny a graph entity privilege to a role. The action is the positional argument; valid actions are: create, delete, traverse. Scope with --on-graph (default *); restrict to entities with --node-label or --relationship-type (mutually exclusive). No property qualifier. --role is required.

Usage: `neo4j-cli admin privilege deny entity <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node-label` | stringArray | [] | Restrict a graph privilege to node labels |
| `--on-graph` | string | - | Scope the privilege to a graph (use * for all) |
| `--relationship-type` | stringArray | [] | Restrict a graph privilege to relationship types |
| `--role` | string | - | Name of the role to deny the privilege to |

Examples:

```
# Deny a graph entity privilege to the analyst role
neo4j-cli admin privilege deny entity traverse --on-graph * --node-label Person --role analyst --credential local --rw

# Deny the same privilege, output as JSON
neo4j-cli admin privilege deny entity traverse --on-graph * --node-label Person --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege deny graph

Deny a whole-graph privilege (all-graph-privileges, write) to a role

Deny a whole-graph privilege to a role. The action is the positional argument; valid actions are: all-graph-privileges, write. Scope with --on-graph (default *). WRITE and ALL GRAPH PRIVILEGES accept no node-label, relationship-type, or property qualifiers. --role is required.

Usage: `neo4j-cli admin privilege deny graph <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--on-graph` | string | - | Scope the privilege to a graph (use * for all) |
| `--role` | string | - | Name of the role to deny the privilege to |

Examples:

```
# Deny a whole-graph privilege to the analyst role
neo4j-cli admin privilege deny graph write --on-graph * --role analyst --credential local --rw

# Deny the same privilege, output as JSON
neo4j-cli admin privilege deny graph write --on-graph * --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege deny label

Deny a label privilege (remove-label, set-label) to a role

Deny a label privilege to a role. The action is the positional argument; valid actions are: remove-label, set-label. Scope with --on-graph (default *); --node-label is required and may be repeated to cover multiple labels. --role is required.

Usage: `neo4j-cli admin privilege deny label <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node-label` | stringArray | [] | Restrict a graph privilege to node labels |
| `--on-graph` | string | - | Scope the privilege to a graph (use * for all) |
| `--role` | string | - | Name of the role to deny the privilege to |

Examples:

```
# Deny a label privilege to the analyst role
neo4j-cli admin privilege deny label set-label --node-label Person --on-graph neo4j --role analyst --credential local --rw

# Deny the same privilege, output as JSON
neo4j-cli admin privilege deny label set-label --node-label Person --on-graph neo4j --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege deny load

Deny a LOAD privilege to a role

Deny a LOAD privilege to a role. Takes no action argument. Defaults to ON ALL DATA; restrict to a CIDR range with --cidr. Accepts no scope or entity flags. --role is required.

Usage: `neo4j-cli admin privilege deny load [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--cidr` | string | - | Scope a LOAD privilege to a CIDR range (LOAD only; defaults to all data) |
| `--role` | string | - | Name of the role to deny the privilege to |

Examples:

```
# Deny a LOAD privilege to the analyst role
neo4j-cli admin privilege deny load --cidr 127.0.0.1/32 --role analyst --credential local --rw

# Deny the same privilege, output as JSON
neo4j-cli admin privilege deny load --cidr 127.0.0.1/32 --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege deny property

Deny a property privilege (match, merge, read, set-property) to a role

Deny a property privilege to a role. The action is the positional argument; valid actions are: match, merge, read, set-property. Scope with --on-graph (default *); restrict to properties with --property and to entities with --node-label or --relationship-type (mutually exclusive). --role is required.

Usage: `neo4j-cli admin privilege deny property <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node-label` | stringArray | [] | Restrict a graph privilege to node labels |
| `--on-graph` | string | - | Scope the privilege to a graph (use * for all) |
| `--property` | stringArray | [] | Restrict a property privilege to properties |
| `--relationship-type` | stringArray | [] | Restrict a graph privilege to relationship types |
| `--role` | string | - | Name of the role to deny the privilege to |

Examples:

```
# Deny a property privilege to the analyst role
neo4j-cli admin privilege deny property read --on-graph * --property name --role analyst --credential local --rw

# Deny the same privilege, output as JSON
neo4j-cli admin privilege deny property read --on-graph * --property name --role analyst --credential local --rw --format json
```

### neo4j-cli admin privilege grant

Grant a privilege to a role

Grant a privilege to a role via GRANT <privilege> TO <role> against the system database. The action is a positional argument on a per-category subcommand (property, entity, graph, label, load, database, dbms); run `grant <category> --help` to see its actions and flags. After the grant, the role's updated privileges are printed.

Usage: `neo4j-cli admin privilege grant`

Examples:

```
# Grant READ on all graphs to the analyst role
neo4j-cli admin privilege grant property read --on-graph * --role analyst --credential local --rw

# Grant CREATE ROLE (a DBMS privilege) to the admin role, output as JSON
neo4j-cli admin privilege grant dbms create-role --role admin --credential local --rw --format json
```

#### neo4j-cli admin privilege grant database

Grant a database privilege (access, all-database-privileges, constraint-management, … — 19 actions; see --help) to a role

Grant a database privilege to a role. The action is the positional argument; valid actions are: access, all-database-privileges, constraint-management, create-constraint, create-index, create-new-node-label, create-new-property-name, create-new-relationship-type, drop-constraint, drop-index, index-management, name-management, show-constraint, show-index, show-transaction, start, stop, terminate-transaction, transaction-management. Scope with --on-database (default *). --role is required.

Usage: `neo4j-cli admin privilege grant database <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--on-database` | string | - | Scope the privilege to a database (use * for all) |
| `--role` | string | - | Name of the role to grant the privilege to |

Examples:

```
# Grant a database privilege to the analyst role
neo4j-cli admin privilege grant database access --on-database neo4j --role analyst --credential local --rw

# Grant the same privilege, output as JSON
neo4j-cli admin privilege grant database access --on-database neo4j --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege grant dbms

Grant a DBMS privilege (all-dbms-privileges, alter-user, assign-role, … — 19 actions; see --help) to a role

Grant a DBMS privilege to a role. The action is the positional argument; valid actions are: all-dbms-privileges, alter-user, assign-role, create-database, create-role, create-user, database-management, drop-database, drop-role, drop-user, privilege-management, remove-role, role-management, set-user-home-database, set-user-status, show-privilege, show-role, show-user, user-management. Applies to the whole DBMS; takes no scope flag. --role is required.

Usage: `neo4j-cli admin privilege grant dbms <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--role` | string | - | Name of the role to grant the privilege to |

Examples:

```
# Grant a DBMS privilege to the analyst role
neo4j-cli admin privilege grant dbms create-role --role analyst --credential local --rw

# Grant the same privilege, output as JSON
neo4j-cli admin privilege grant dbms create-role --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege grant entity

Grant a graph entity privilege (create, delete, traverse) to a role

Grant a graph entity privilege to a role. The action is the positional argument; valid actions are: create, delete, traverse. Scope with --on-graph (default *); restrict to entities with --node-label or --relationship-type (mutually exclusive). No property qualifier. --role is required.

Usage: `neo4j-cli admin privilege grant entity <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node-label` | stringArray | [] | Restrict a graph privilege to node labels |
| `--on-graph` | string | - | Scope the privilege to a graph (use * for all) |
| `--relationship-type` | stringArray | [] | Restrict a graph privilege to relationship types |
| `--role` | string | - | Name of the role to grant the privilege to |

Examples:

```
# Grant a graph entity privilege to the analyst role
neo4j-cli admin privilege grant entity traverse --on-graph * --node-label Person --role analyst --credential local --rw

# Grant the same privilege, output as JSON
neo4j-cli admin privilege grant entity traverse --on-graph * --node-label Person --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege grant graph

Grant a whole-graph privilege (all-graph-privileges, write) to a role

Grant a whole-graph privilege to a role. The action is the positional argument; valid actions are: all-graph-privileges, write. Scope with --on-graph (default *). WRITE and ALL GRAPH PRIVILEGES accept no node-label, relationship-type, or property qualifiers. --role is required.

Usage: `neo4j-cli admin privilege grant graph <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--on-graph` | string | - | Scope the privilege to a graph (use * for all) |
| `--role` | string | - | Name of the role to grant the privilege to |

Examples:

```
# Grant a whole-graph privilege to the analyst role
neo4j-cli admin privilege grant graph write --on-graph * --role analyst --credential local --rw

# Grant the same privilege, output as JSON
neo4j-cli admin privilege grant graph write --on-graph * --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege grant label

Grant a label privilege (remove-label, set-label) to a role

Grant a label privilege to a role. The action is the positional argument; valid actions are: remove-label, set-label. Scope with --on-graph (default *); --node-label is required and may be repeated to cover multiple labels. --role is required.

Usage: `neo4j-cli admin privilege grant label <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node-label` | stringArray | [] | Restrict a graph privilege to node labels |
| `--on-graph` | string | - | Scope the privilege to a graph (use * for all) |
| `--role` | string | - | Name of the role to grant the privilege to |

Examples:

```
# Grant a label privilege to the analyst role
neo4j-cli admin privilege grant label set-label --node-label Person --on-graph neo4j --role analyst --credential local --rw

# Grant the same privilege, output as JSON
neo4j-cli admin privilege grant label set-label --node-label Person --on-graph neo4j --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege grant load

Grant a LOAD privilege to a role

Grant a LOAD privilege to a role. Takes no action argument. Defaults to ON ALL DATA; restrict to a CIDR range with --cidr. Accepts no scope or entity flags. --role is required.

Usage: `neo4j-cli admin privilege grant load [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--cidr` | string | - | Scope a LOAD privilege to a CIDR range (LOAD only; defaults to all data) |
| `--role` | string | - | Name of the role to grant the privilege to |

Examples:

```
# Grant a LOAD privilege to the analyst role
neo4j-cli admin privilege grant load --cidr 127.0.0.1/32 --role analyst --credential local --rw

# Grant the same privilege, output as JSON
neo4j-cli admin privilege grant load --cidr 127.0.0.1/32 --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege grant property

Grant a property privilege (match, merge, read, set-property) to a role

Grant a property privilege to a role. The action is the positional argument; valid actions are: match, merge, read, set-property. Scope with --on-graph (default *); restrict to properties with --property and to entities with --node-label or --relationship-type (mutually exclusive). --role is required.

Usage: `neo4j-cli admin privilege grant property <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node-label` | stringArray | [] | Restrict a graph privilege to node labels |
| `--on-graph` | string | - | Scope the privilege to a graph (use * for all) |
| `--property` | stringArray | [] | Restrict a property privilege to properties |
| `--relationship-type` | stringArray | [] | Restrict a graph privilege to relationship types |
| `--role` | string | - | Name of the role to grant the privilege to |

Examples:

```
# Grant a property privilege to the analyst role
neo4j-cli admin privilege grant property read --on-graph * --property name --role analyst --credential local --rw

# Grant the same privilege, output as JSON
neo4j-cli admin privilege grant property read --on-graph * --property name --role analyst --credential local --rw --format json
```

### neo4j-cli admin privilege list

List privileges

List privileges via SHOW PRIVILEGES. Use --role to scope to a single role's privileges (SHOW ROLE $name PRIVILEGES) or --user to scope to a single user's privileges (SHOW USER $name PRIVILEGES). --role and --user are mutually exclusive.

Usage: `neo4j-cli admin privilege list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--role` | string | - | Scope output to a single role's privileges |
| `--user` | string | - | Scope output to a single user's privileges |

Examples:

```
# List all privileges (read-only)
neo4j-cli admin privilege list --credential local

# List the privileges of role analyst as JSON
neo4j-cli admin privilege list --credential local --role analyst --format json

# List the privileges of user alice
neo4j-cli admin privilege list --credential local --user alice
```

### neo4j-cli admin privilege revoke

Revoke a privilege from a role

Revoke a privilege from a role via REVOKE <privilege> FROM <role> against the system database. The action is a positional argument on a per-category subcommand (property, entity, graph, label, load, database, dbms); run `revoke <category> --help` to see its actions and flags. Use --revoke-type grant or --revoke-type deny to revoke only a previously granted or denied privilege; omit it to revoke both. After the revoke, the role's updated privileges are printed.

Usage: `neo4j-cli admin privilege revoke`

Examples:

```
# Revoke READ on all graphs from the analyst role
neo4j-cli admin privilege revoke property read --on-graph * --role analyst --credential local --rw

# Revoke only a previously granted READ privilege, output as JSON
neo4j-cli admin privilege revoke property read --on-graph * --role analyst --revoke-type grant --credential local --rw --format json
```

#### neo4j-cli admin privilege revoke database

Revoke a database privilege (access, all-database-privileges, constraint-management, … — 19 actions; see --help) from a role

Revoke a database privilege from a role. The action is the positional argument; valid actions are: access, all-database-privileges, constraint-management, create-constraint, create-index, create-new-node-label, create-new-property-name, create-new-relationship-type, drop-constraint, drop-index, index-management, name-management, show-constraint, show-index, show-transaction, start, stop, terminate-transaction, transaction-management. Scope with --on-database (default *). --role is required.

Usage: `neo4j-cli admin privilege revoke database <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--on-database` | string | - | Scope the privilege to a database (use * for all) |
| `--revoke-type` | string | - | Restrict the revoke to grant or deny privileges (grant\|deny); omit to revoke both |
| `--role` | string | - | Name of the role to revoke the privilege from |

Examples:

```
# Revoke a database privilege to the analyst role
neo4j-cli admin privilege revoke database access --on-database neo4j --role analyst --credential local --rw

# Revoke the same privilege, output as JSON
neo4j-cli admin privilege revoke database access --on-database neo4j --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege revoke dbms

Revoke a DBMS privilege (all-dbms-privileges, alter-user, assign-role, … — 19 actions; see --help) from a role

Revoke a DBMS privilege from a role. The action is the positional argument; valid actions are: all-dbms-privileges, alter-user, assign-role, create-database, create-role, create-user, database-management, drop-database, drop-role, drop-user, privilege-management, remove-role, role-management, set-user-home-database, set-user-status, show-privilege, show-role, show-user, user-management. Applies to the whole DBMS; takes no scope flag. --role is required.

Usage: `neo4j-cli admin privilege revoke dbms <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--revoke-type` | string | - | Restrict the revoke to grant or deny privileges (grant\|deny); omit to revoke both |
| `--role` | string | - | Name of the role to revoke the privilege from |

Examples:

```
# Revoke a DBMS privilege to the analyst role
neo4j-cli admin privilege revoke dbms create-role --role analyst --credential local --rw

# Revoke the same privilege, output as JSON
neo4j-cli admin privilege revoke dbms create-role --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege revoke entity

Revoke a graph entity privilege (create, delete, traverse) from a role

Revoke a graph entity privilege from a role. The action is the positional argument; valid actions are: create, delete, traverse. Scope with --on-graph (default *); restrict to entities with --node-label or --relationship-type (mutually exclusive). No property qualifier. --role is required.

Usage: `neo4j-cli admin privilege revoke entity <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node-label` | stringArray | [] | Restrict a graph privilege to node labels |
| `--on-graph` | string | - | Scope the privilege to a graph (use * for all) |
| `--relationship-type` | stringArray | [] | Restrict a graph privilege to relationship types |
| `--revoke-type` | string | - | Restrict the revoke to grant or deny privileges (grant\|deny); omit to revoke both |
| `--role` | string | - | Name of the role to revoke the privilege from |

Examples:

```
# Revoke a graph entity privilege to the analyst role
neo4j-cli admin privilege revoke entity traverse --on-graph * --node-label Person --role analyst --credential local --rw

# Revoke the same privilege, output as JSON
neo4j-cli admin privilege revoke entity traverse --on-graph * --node-label Person --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege revoke graph

Revoke a whole-graph privilege (all-graph-privileges, write) from a role

Revoke a whole-graph privilege from a role. The action is the positional argument; valid actions are: all-graph-privileges, write. Scope with --on-graph (default *). WRITE and ALL GRAPH PRIVILEGES accept no node-label, relationship-type, or property qualifiers. --role is required.

Usage: `neo4j-cli admin privilege revoke graph <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--on-graph` | string | - | Scope the privilege to a graph (use * for all) |
| `--revoke-type` | string | - | Restrict the revoke to grant or deny privileges (grant\|deny); omit to revoke both |
| `--role` | string | - | Name of the role to revoke the privilege from |

Examples:

```
# Revoke a whole-graph privilege to the analyst role
neo4j-cli admin privilege revoke graph write --on-graph * --role analyst --credential local --rw

# Revoke the same privilege, output as JSON
neo4j-cli admin privilege revoke graph write --on-graph * --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege revoke label

Revoke a label privilege (remove-label, set-label) from a role

Revoke a label privilege from a role. The action is the positional argument; valid actions are: remove-label, set-label. Scope with --on-graph (default *); --node-label is required and may be repeated to cover multiple labels. --role is required.

Usage: `neo4j-cli admin privilege revoke label <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node-label` | stringArray | [] | Restrict a graph privilege to node labels |
| `--on-graph` | string | - | Scope the privilege to a graph (use * for all) |
| `--revoke-type` | string | - | Restrict the revoke to grant or deny privileges (grant\|deny); omit to revoke both |
| `--role` | string | - | Name of the role to revoke the privilege from |

Examples:

```
# Revoke a label privilege to the analyst role
neo4j-cli admin privilege revoke label set-label --node-label Person --on-graph neo4j --role analyst --credential local --rw

# Revoke the same privilege, output as JSON
neo4j-cli admin privilege revoke label set-label --node-label Person --on-graph neo4j --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege revoke load

Revoke a LOAD privilege from a role

Revoke a LOAD privilege from a role. Takes no action argument. Defaults to ON ALL DATA; restrict to a CIDR range with --cidr. Accepts no scope or entity flags. --role is required.

Usage: `neo4j-cli admin privilege revoke load [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--cidr` | string | - | Scope a LOAD privilege to a CIDR range (LOAD only; defaults to all data) |
| `--revoke-type` | string | - | Restrict the revoke to grant or deny privileges (grant\|deny); omit to revoke both |
| `--role` | string | - | Name of the role to revoke the privilege from |

Examples:

```
# Revoke a LOAD privilege to the analyst role
neo4j-cli admin privilege revoke load --cidr 127.0.0.1/32 --role analyst --credential local --rw

# Revoke the same privilege, output as JSON
neo4j-cli admin privilege revoke load --cidr 127.0.0.1/32 --role analyst --credential local --rw --format json
```

#### neo4j-cli admin privilege revoke property

Revoke a property privilege (match, merge, read, set-property) from a role

Revoke a property privilege from a role. The action is the positional argument; valid actions are: match, merge, read, set-property. Scope with --on-graph (default *); restrict to properties with --property and to entities with --node-label or --relationship-type (mutually exclusive). --role is required.

Usage: `neo4j-cli admin privilege revoke property <action> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node-label` | stringArray | [] | Restrict a graph privilege to node labels |
| `--on-graph` | string | - | Scope the privilege to a graph (use * for all) |
| `--property` | stringArray | [] | Restrict a property privilege to properties |
| `--relationship-type` | stringArray | [] | Restrict a graph privilege to relationship types |
| `--revoke-type` | string | - | Restrict the revoke to grant or deny privileges (grant\|deny); omit to revoke both |
| `--role` | string | - | Name of the role to revoke the privilege from |

Examples:

```
# Revoke a property privilege to the analyst role
neo4j-cli admin privilege revoke property read --on-graph * --property name --role analyst --credential local --rw

# Revoke the same privilege, output as JSON
neo4j-cli admin privilege revoke property read --on-graph * --property name --role analyst --credential local --rw --format json
```

## neo4j-cli admin role

Manage Neo4j roles and role membership

Manage Neo4j roles and role membership via the system database. Read commands (list, get) do not require --rw. Write commands (create, drop, grant, revoke) require --rw.

Usage: `neo4j-cli admin role`

Examples:

```
# Show help for the role subcommands
neo4j-cli admin role --help

# List all roles (read-only)
neo4j-cli admin role list --credential local --format json
```

### neo4j-cli admin role create

Create a role

Create a role via CREATE ROLE $name IF NOT EXISTS against the system database. The command is idempotent — running it twice does not return an error. After creation the current member list for the role is printed.

Usage: `neo4j-cli admin role create <name>`

Examples:

```
# Create a role named analyst
neo4j-cli admin role create analyst --credential local --rw

# Create a role and output the member list as JSON
neo4j-cli admin role create analyst --credential local --rw --format json
```

### neo4j-cli admin role drop

Drop a role

Drop a role via DROP ROLE $name against the system database. Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli admin role drop <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Drop a role, prompting for confirmation on a TTY
neo4j-cli admin role drop analyst --credential local --rw

# Drop a role without prompting (required for scripts and non-TTY callers)
neo4j-cli admin role drop analyst --credential local --rw --yes --force
```

### neo4j-cli admin role get

Get the privileges for a role

Get the privileges assigned to a role. First checks the role exists via SHOW ROLES WITH USERS WHERE role = $name; returns a not-found error if no rows are returned. Then executes SHOW ROLE $name PRIVILEGES and returns the privilege list. Returns an empty list if the role exists but has no privileges.

Usage: `neo4j-cli admin role get <name>`

Examples:

```
# Get the privileges for the admin role
neo4j-cli admin role get admin --credential local

# Get privileges as JSON for scripting
neo4j-cli admin role get admin --credential local --format json
```

### neo4j-cli admin role grant

Grant a role to a user

Grant a role to a user via GRANT ROLE $role TO $user against the system database. After the grant, the updated user record is printed with the current role membership.

Usage: `neo4j-cli admin role grant [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--role` | string | - | Name of the role to grant |
| `--user` | string | - | Name of the user to grant the role to |

Examples:

```
# Grant the analyst role to alice
neo4j-cli admin role grant --role analyst --user alice --credential local --rw

# Grant a role and output the updated user record as JSON
neo4j-cli admin role grant --role analyst --user alice --credential local --rw --format json
```

### neo4j-cli admin role list

List all roles and their members

List all roles and their members via SHOW ROLES WITH USERS. Use --user to filter results to only roles that contain a specific user.

Usage: `neo4j-cli admin role list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--user` | string | - | Filter results to only roles that contain this user |

Examples:

```
# List all roles and their members
neo4j-cli admin role list --credential local

# List all roles and members as JSON
neo4j-cli admin role list --credential local --format json

# List only the roles that user alice belongs to
neo4j-cli admin role list --credential local --user alice --format json
```

### neo4j-cli admin role revoke

Revoke a role from a user

Revoke a role from a user via REVOKE ROLE $role FROM $user against the system database. After the revoke, the updated user record is printed with the current role membership.

Usage: `neo4j-cli admin role revoke [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--role` | string | - | Name of the role to revoke |
| `--user` | string | - | Name of the user to revoke the role from |

Examples:

```
# Revoke the analyst role from alice
neo4j-cli admin role revoke --role analyst --user alice --credential local --rw

# Revoke a role and output the updated user record as JSON
neo4j-cli admin role revoke --role analyst --user alice --credential local --rw --format json
```

## neo4j-cli admin user

Manage Neo4j users via the system database

Manage Neo4j users. Read commands (list, get) do not require --rw. Write commands (create, drop, rename, set-password, suspend, activate) require --rw.

Usage: `neo4j-cli admin user`

Examples:

```
# Show help for the user subcommands
neo4j-cli admin user --help

# List all users (read-only)
neo4j-cli admin user list --credential local --format json
```

### neo4j-cli admin user activate

Activate a user

Activate a previously suspended user by setting their status to ACTIVE via ALTER USER $name SET STATUS ACTIVE against the system database. An active user can log in normally. Returns the updated user record on success.

Usage: `neo4j-cli admin user activate <name>`

Examples:

```
# Activate a user
neo4j-cli admin user activate alice --credential local --rw

# Activate a user and display the result as JSON
neo4j-cli admin user activate alice --credential local --rw --format json
```

### neo4j-cli admin user create

Create a user

Create a user via CREATE USER $name SET PASSWORD $password SET PASSWORD CHANGE [NOT] REQUIRED against the system database. Pass --set-password to supply the password non-interactively; omit it on a TTY to be prompted. Pass --password-change-required=false to allow the new user to log in without changing password.

Usage: `neo4j-cli admin user create <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--password-change-required` | bool | true | Require the user to change password on first login |
| `--set-password` | string | - | Password for the new user (prompted on TTY if omitted) |

Examples:

```
# Create a user (prompted for password on a TTY)
neo4j-cli admin user create alice --credential local --rw

# Create a user with an explicit password and no change-on-login requirement
neo4j-cli admin user create alice --set-password s3cr3t --password-change-required=false --credential local --rw --format json
```

### neo4j-cli admin user drop

Drop a user

Drop a user via DROP USER <name> against the system database. Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli admin user drop <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Drop a user, prompting for confirmation on a TTY
neo4j-cli admin user drop alice --credential local --rw

# Drop a user without prompting (required for scripts and non-TTY callers)
neo4j-cli admin user drop alice --credential local --rw --yes --force
```

### neo4j-cli admin user get

Get details of a user

Get the full record for a single user by name. Executes SHOW USERS WHERE user = $name against the system database. Renders user, roles, password_change_required, and suspended columns.

Usage: `neo4j-cli admin user get <name>`

Examples:

```
# Get a user record as a table
neo4j-cli admin user get neo4j --credential local

# Get a user record as JSON for scripting
neo4j-cli admin user get neo4j --credential local --format json
```

### neo4j-cli admin user list

List all users

List all users visible from the system database. Renders an overview with user, roles, password_change_required, and suspended columns. Use `get` for the full record of a single user.

Usage: `neo4j-cli admin user list`

Examples:

```
# List all users as a table
neo4j-cli admin user list --credential local

# List all users as JSON for scripting
neo4j-cli admin user list --credential local --format json
```

### neo4j-cli admin user rename

Rename a user

Rename a user via RENAME USER $oldName TO $newName against the system database. On success, emits the updated user record for the new name. On Aura, renaming any user is not supported (Aura uses a non-native authentication provider globally).

Usage: `neo4j-cli admin user rename <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--new-name` | string | - | New name for the user (required) |

Examples:

```
# Rename a user
neo4j-cli admin user rename alice --new-name bob --credential local --rw

# Rename a user and view the result as JSON
neo4j-cli admin user rename alice --new-name bob --credential local --rw --format json
```

### neo4j-cli admin user set-password

Set the password for a user

Set the password for an existing user via ALTER USER $name SET PASSWORD against the system database. Use --password-change-required to control whether the user must change their password on next login (defaults to false).

Usage: `neo4j-cli admin user set-password <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--new-password` | string | - | New password for the user (prompted if absent in interactive mode) |
| `--password-change-required` | bool | false | Require the user to change their password on next login |

Examples:

```
# Set a password explicitly
neo4j-cli admin user set-password alice --new-password s3cr3t --credential local --rw

# Set a password and require the user to change it on next login
neo4j-cli admin user set-password alice --new-password s3cr3t --password-change-required --credential local --rw
```

### neo4j-cli admin user suspend

Suspend a user

Suspend a user by setting their status to SUSPENDED via ALTER USER $name SET STATUS SUSPENDED against the system database. A suspended user cannot log in. Returns the updated user record on success.

Usage: `neo4j-cli admin user suspend <name>`

Examples:

```
# Suspend a user
neo4j-cli admin user suspend alice --credential local --rw

# Suspend a user and display the result as JSON
neo4j-cli admin user suspend alice --credential local --rw --format json
```

