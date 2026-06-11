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
- [neo4j-cli admin privilege grant](#neo4j-cli-admin-privilege-grant)
- [neo4j-cli admin privilege list](#neo4j-cli-admin-privilege-list)
- [neo4j-cli admin privilege revoke](#neo4j-cli-admin-privilege-revoke)
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

Manage Neo4j databases, users, and roles

Manage Neo4j databases, users, roles, and privileges via the system database. Connects over Bolt using the supplied connection flags or a stored dbms credential (use '--credential <name>' for a named credential, '--credential desktop' for a running Neo4j Desktop 2 DBMS, or '--credential desktop-connection:<uuid>' for a saved Desktop connection). Subcommands: `database` (list, get, create, drop, start, stop), `user` (list, get, create, drop, rename, set-password, suspend, activate), `role` (list, get, create, drop, grant, revoke — Enterprise only), `privilege` (list — Enterprise only).

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

## neo4j-cli admin privilege

Show Neo4j privileges (Enterprise only)

Show Neo4j privileges (Enterprise edition only). Read commands (list) do not require --rw.

Usage: `neo4j-cli admin privilege`

Examples:

```
# Show help for the privilege subcommands
neo4j-cli admin privilege --help

# List all privileges as JSON for scripting
neo4j-cli admin privilege list --credential local --format json
```

### neo4j-cli admin privilege deny

Deny a privilege to a role (Enterprise only)

Deny a privilege to a role in the system database. Executes DENY <action> ON <resource> TO <role>. Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error. The --action flag accepts Neo4j privilege keywords (case-insensitive; use _ or space as word separator). Resource target is controlled by --on-graph (default), --on-database, or --on-dbms (mutually exclusive). Graph-scoped qualifiers --node-label, --relationship-type, and --property are only valid with --on-graph.

Usage: `neo4j-cli admin privilege deny [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--action` | string | - | Privilege action keyword (required; e.g. read, write, access, create_role) |
| `--node-label` | stringArray | [] | Node label qualifier (repeatable; only valid with --on-graph) |
| `--on-database` | string | - | Target a specific database by name |
| `--on-dbms` | bool | false | Target the DBMS (for DBMS-level privileges) |
| `--on-graph` | string | - | Target a specific graph by name (default: * when no resource flag is set) |
| `--property` | stringArray | [] | Property name qualifier (repeatable; only valid with --on-graph; default: all properties) |
| `--relationship-type` | stringArray | [] | Relationship type qualifier (repeatable; only valid with --on-graph) |
| `--role` | string | - | Role name to deny the privilege to (required) |

Examples:

```
# Deny WRITE on all graphs to the readonly role
neo4j-cli admin privilege deny --action write --on-graph '*' --role readonly --credential local --rw

# Deny ACCESS on a specific database to a role
neo4j-cli admin privilege deny --action access --on-database restricted --role readonly --credential local --rw

# Deny a DBMS-level privilege to a role
neo4j-cli admin privilege deny --action create_user --on-dbms --role limited --credential local --rw

# Deny READ on specific nodes to a role
neo4j-cli admin privilege deny --action read --on-graph neo4j --node-label Secret --role analyst --credential local --rw
```

### neo4j-cli admin privilege grant

Grant a privilege to a role (Enterprise only)

Grant a privilege to a role in the system database. Executes GRANT <action> ON <resource> TO <role>. Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error. The --action flag accepts Neo4j privilege keywords (case-insensitive; use _ or space as word separator). Resource target is controlled by --on-graph (default), --on-database, or --on-dbms (mutually exclusive). Graph-scoped qualifiers --node-label, --relationship-type, and --property are only valid with --on-graph.

Usage: `neo4j-cli admin privilege grant [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--action` | string | - | Privilege action keyword (required; e.g. read, write, access, create_role) |
| `--node-label` | stringArray | [] | Node label qualifier (repeatable; only valid with --on-graph) |
| `--on-database` | string | - | Target a specific database by name |
| `--on-dbms` | bool | false | Target the DBMS (for DBMS-level privileges) |
| `--on-graph` | string | - | Target a specific graph by name (default: * when no resource flag is set) |
| `--property` | stringArray | [] | Property name qualifier (repeatable; only valid with --on-graph; default: all properties) |
| `--relationship-type` | stringArray | [] | Relationship type qualifier (repeatable; only valid with --on-graph) |
| `--role` | string | - | Role name to grant the privilege to (required) |

Examples:

```
# Grant READ on all graphs to the analyst role
neo4j-cli admin privilege grant --action read --on-graph '*' --role analyst --credential local --rw

# Grant ACCESS on a specific database to a role
neo4j-cli admin privilege grant --action access --on-database neo4j --role analyst --credential local --rw

# Grant a DBMS-level privilege to a role
neo4j-cli admin privilege grant --action create_role --on-dbms --role admin --credential local --rw

# Grant READ on specific nodes and property to a role
neo4j-cli admin privilege grant --action read --on-graph neo4j --node-label Person --property name --role analyst --credential local --rw
```

### neo4j-cli admin privilege list

List privileges (Enterprise only)

List privileges from the system database (Enterprise edition only). Executes SHOW PRIVILEGES by default. Use --role to show privileges for a specific role or --user to show privileges for a specific user. --role and --user are mutually exclusive.

Usage: `neo4j-cli admin privilege list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--role` | string | - | Show privileges for the specified role name (mutually exclusive with --user) |
| `--user` | string | - | Show privileges for the specified user name (mutually exclusive with --role) |

Examples:

```
# List all privileges as a table
neo4j-cli admin privilege list --credential local

# List all privileges as JSON for scripting
neo4j-cli admin privilege list --credential local --format json

# List privileges for a specific role
neo4j-cli admin privilege list --credential local --role analyst --format json

# List privileges for a specific user
neo4j-cli admin privilege list --credential local --user alice
```

### neo4j-cli admin privilege revoke

Revoke a privilege from a role (Enterprise only)

Revoke a privilege from a role in the system database. Executes REVOKE [GRANT|DENY] <action> ON <resource> FROM <role>. Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error. Use --revoke-type grant to revoke only a GRANT, --revoke-type deny to revoke only a DENY, or omit --revoke-type to revoke both. The --action flag accepts Neo4j privilege keywords (case-insensitive; use _ or space as word separator). Resource target is controlled by --on-graph (default), --on-database, or --on-dbms (mutually exclusive). Graph-scoped qualifiers --node-label, --relationship-type, and --property are only valid with --on-graph.

Usage: `neo4j-cli admin privilege revoke [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--action` | string | - | Privilege action keyword (required; e.g. read, write, access, create_role) |
| `--node-label` | stringArray | [] | Node label qualifier (repeatable; only valid with --on-graph) |
| `--on-database` | string | - | Target a specific database by name |
| `--on-dbms` | bool | false | Target the DBMS (for DBMS-level privileges) |
| `--on-graph` | string | - | Target a specific graph by name (default: * when no resource flag is set) |
| `--property` | stringArray | [] | Property name qualifier (repeatable; only valid with --on-graph; default: all properties) |
| `--relationship-type` | stringArray | [] | Relationship type qualifier (repeatable; only valid with --on-graph) |
| `--revoke-type` | string | - | Revoke only a GRANT or DENY: grant\|deny (default: revoke both) |
| `--role` | string | - | Role name to revoke the privilege from (required) |

Examples:

```
# Revoke READ on all graphs from the analyst role (revokes both GRANT and DENY)
neo4j-cli admin privilege revoke --action read --on-graph '*' --role analyst --credential local --rw

# Revoke only a GRANT of WRITE from a role
neo4j-cli admin privilege revoke --action write --on-graph '*' --role analyst --revoke-type grant --credential local --rw

# Revoke only a DENY of ACCESS from a role on a specific database
neo4j-cli admin privilege revoke --action access --on-database neo4j --role readonly --revoke-type deny --credential local --rw

# Revoke a DBMS-level privilege from a role
neo4j-cli admin privilege revoke --action create_role --on-dbms --role limited --credential local --rw
```

## neo4j-cli admin role

Manage Neo4j roles via the system database

Manage Neo4j roles (Enterprise edition only). Read commands (list, get) do not require --rw. Write commands (create, drop, grant, revoke) require --rw.

Usage: `neo4j-cli admin role`

Examples:

```
# Show help for the role subcommands
neo4j-cli admin role --help

# List all roles (read-only)
neo4j-cli admin role list --credential local --format json
```

### neo4j-cli admin role create

Create a role (Enterprise only)

Create a new role in the system database. Executes CREATE ROLE $name against the system database. Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error.

Usage: `neo4j-cli admin role create <name>`

Examples:

```
# Create a new role
neo4j-cli admin role create analyst --credential local --rw

# Create a role and confirm it exists
neo4j-cli admin role create analyst --credential local --rw && neo4j-cli admin role list --credential local --format json
```

### neo4j-cli admin role drop

Drop a role (Enterprise only)

Drop an existing role from the system database. Executes DROP ROLE $name against the system database. Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error. Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli admin role drop <name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Drop a role (prompts on a TTY)
neo4j-cli admin role drop analyst --credential local --rw

# Drop a role without prompting (for scripts and non-TTY callers)
neo4j-cli admin role drop analyst --credential local --rw --yes --force
```

### neo4j-cli admin role get

Get privileges for a role

Get the full privileges record for a single role by name. Executes SHOW ROLE $name PRIVILEGES against the system database.

Usage: `neo4j-cli admin role get <role>`

Examples:

```
# Get privileges for a role as a table
neo4j-cli admin role get admin --credential local

# Get privileges for a role as JSON for scripting
neo4j-cli admin role get admin --credential local --format json
```

### neo4j-cli admin role grant

Grant a role to a user (Enterprise only)

Grant a role to a user in the system database. Executes GRANT ROLE $role TO $user against the system database. Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error.

Usage: `neo4j-cli admin role grant [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--role` | string | - | Role name to grant (required) |
| `--user` | string | - | Username to grant the role to (required) |

Examples:

```
# Grant the analyst role to a user
neo4j-cli admin role grant --role analyst --user alice --credential local --rw

# Grant the reader role to a user
neo4j-cli admin role grant --role reader --user bob --credential local --rw
```

### neo4j-cli admin role list

List all roles and their members

List all roles and their member users. Executes SHOW ROLES WITH USERS against the system database. Use --role to filter by role name or --user to filter by user name.

Usage: `neo4j-cli admin role list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--role` | string | - | Filter output to the specified role name |
| `--user` | string | - | Filter output to rows where the member matches the specified user name |

Examples:

```
# List all roles and their members as a table
neo4j-cli admin role list --credential local

# List all roles and members as JSON for scripting
neo4j-cli admin role list --credential local --format json

# Filter to a specific role
neo4j-cli admin role list --credential local --role admin --format json

# Filter to roles a specific user belongs to
neo4j-cli admin role list --credential local --user alice
```

### neo4j-cli admin role revoke

Revoke a role from a user (Enterprise only)

Revoke a role from a user in the system database. Executes REVOKE ROLE $role FROM $user against the system database. Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error.

Usage: `neo4j-cli admin role revoke [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--role` | string | - | Role name to revoke (required) |
| `--user` | string | - | Username to revoke the role from (required) |

Examples:

```
# Revoke the analyst role from a user
neo4j-cli admin role revoke --role analyst --user alice --credential local --rw

# Revoke the reader role from a user
neo4j-cli admin role revoke --role reader --user bob --credential local --rw
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

Activate (unsuspend) a Neo4j user (Enterprise edition only)

Activate (unsuspend) an existing user in the system database, allowing them to log in again. Requires Enterprise edition (Community edition returns an error).

Usage: `neo4j-cli admin user activate <username>`

Examples:

```
# Activate a previously suspended user
neo4j-cli admin user activate alice --credential local --rw

# Activate a user and verify status
neo4j-cli admin user activate bob --credential local --rw && neo4j-cli admin user get bob --credential local --format json
```

### neo4j-cli admin user create

Create a Neo4j user

Create a new user in the system database. If --password is not supplied, prompts on a TTY or returns a usage error on non-TTY. --password-change-required (default true) controls whether the user must change their password on first login. --home-database sets the user's default database (Enterprise edition only).

Usage: `neo4j-cli admin user create <username> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--home-database` | string | - | Set the user's home database (Enterprise edition only) |
| `--password` | string | - | Password for the new user (prompted if not supplied on a TTY) |
| `--password-change-required` | bool | true | Require the user to change their password on first login |

Examples:

```
# Create a user interactively (password will be prompted)
neo4j-cli admin user create alice --credential local --rw

# Create a user with a password and no change required
neo4j-cli admin user create bob --password secret --password-change-required=false --credential local --rw

# Create a user with a home database (Enterprise)
neo4j-cli admin user create carol --password secret --home-database mydb --credential local --rw
```

### neo4j-cli admin user drop

Drop a Neo4j user

Drop (delete) a user from the system database. Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli admin user drop <username> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Drop a user with confirmation prompt (TTY only)
neo4j-cli admin user drop alice --credential local --rw

# Drop a user without prompting (required for scripts)
neo4j-cli admin user drop alice --credential local --rw --yes --force
```

### neo4j-cli admin user get

Get details of a user

Get the full record for a single user by name. Executes 'SHOW USERS WHERE user = $name' against the system database.

Usage: `neo4j-cli admin user get <username>`

Examples:

```
# Get a user record as a table
neo4j-cli admin user get neo4j --credential local

# Get a user record as JSON for scripting
neo4j-cli admin user get neo4j --credential local --format json
```

### neo4j-cli admin user list

List all users

List all users visible from the system database. Renders user, roles, passwordChangeRequired, and suspended columns.

Usage: `neo4j-cli admin user list`

Examples:

```
# List all users as a table
neo4j-cli admin user list --credential local

# List all users as JSON for scripting
neo4j-cli admin user list --credential local --format json
```

### neo4j-cli admin user rename

Rename a Neo4j user

Rename an existing user in the system database. Not supported on Aura connections (Aura uses a non-native authentication provider).

Usage: `neo4j-cli admin user rename <old-name> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--new-name` | string | - | The new name for the user |

Examples:

```
# Rename a user
neo4j-cli admin user rename alice --new-name alice2 --credential local --rw

# Rename a user and verify the change
neo4j-cli admin user rename bob --new-name bob-renamed --credential local --rw && neo4j-cli admin user get bob-renamed --credential local --format json
```

### neo4j-cli admin user set-password

Set the password for a Neo4j user

Set the password for an existing user in the system database. If --password is not supplied, prompts on a TTY or returns a usage error on non-TTY. --password-change-required (default false) controls whether the user must change their password on next login.

Usage: `neo4j-cli admin user set-password <username> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--password` | string | - | New password (prompted if not supplied on a TTY) |
| `--password-change-required` | bool | false | Require the user to change their password on next login |

Examples:

```
# Set a user's password interactively (password will be prompted)
neo4j-cli admin user set-password alice --credential local --rw

# Set a user's password with a flag and require change on next login
neo4j-cli admin user set-password bob --password newsecret --password-change-required --credential local --rw
```

### neo4j-cli admin user suspend

Suspend a Neo4j user (Enterprise edition only)

Suspend an existing user in the system database, preventing them from logging in. Requires Enterprise edition (Community edition returns an error).

Usage: `neo4j-cli admin user suspend <username>`

Examples:

```
# Suspend a user
neo4j-cli admin user suspend alice --credential local --rw

# Suspend a user and verify status
neo4j-cli admin user suspend bob --credential local --rw && neo4j-cli admin user get bob --credential local --format json
```

