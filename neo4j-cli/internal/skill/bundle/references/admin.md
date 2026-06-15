# neo4j-cli admin

## Contents

- [neo4j-cli admin database](#neo4j-cli-admin-database)
- [neo4j-cli admin database create](#neo4j-cli-admin-database-create)
- [neo4j-cli admin database drop](#neo4j-cli-admin-database-drop)
- [neo4j-cli admin database get](#neo4j-cli-admin-database-get)
- [neo4j-cli admin database list](#neo4j-cli-admin-database-list)
- [neo4j-cli admin database start](#neo4j-cli-admin-database-start)
- [neo4j-cli admin database stop](#neo4j-cli-admin-database-stop)
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

Manage Neo4j databases via the system database. Connects over Bolt using the supplied connection flags or a stored dbms credential (use '--credential <name>' for a named credential, '--credential desktop' for a running Neo4j Desktop 2 DBMS, or '--credential desktop-connection:<uuid>' for a saved Desktop connection). Subcommands: `database` (list, get, create, drop, start, stop), `user` (list, get, create, drop, rename, set-password, suspend, activate).

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

Rename a user via RENAME USER $oldName TO $newName against the system database. On success, emits the updated user record for the new name. On Aura, renaming a non-native user is rejected by the server.

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

