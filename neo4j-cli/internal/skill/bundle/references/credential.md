# neo4j-cli credential

Manage and view credential values

Usage: `neo4j-cli credential`

## neo4j-cli credential aura-client

Manage and view aura-client credential values

Usage: `neo4j-cli credential aura-client`

### neo4j-cli credential aura-client add

Adds a credential

Usage: `neo4j-cli credential aura-client add [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--client-id` | string | - | (required) Client ID |
| `--client-secret` | string | - | (required) Client secret |
| `--name` | string | - | (required) Name |

### neo4j-cli credential aura-client list

List credentials

Usage: `neo4j-cli credential aura-client list`

### neo4j-cli credential aura-client remove

Removes a credential

Usage: `neo4j-cli credential aura-client remove`

### neo4j-cli credential aura-client use

Sets the default credential to be used

Usage: `neo4j-cli credential aura-client use`

## neo4j-cli credential database

Manage and view database credential values

Usage: `neo4j-cli credential database`

### neo4j-cli credential database add

Adds a database credential

Usage: `neo4j-cli credential database add [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--database-name` | string | neo4j | Database name |
| `--insecure` | bool | false | Disable TLS verification |
| `--name` | string | - | (required) Name |
| `--password` | string | - | (required) Password |
| `--uri` | string | - | (required) URI |
| `--username` | string | - | (required) Username |

### neo4j-cli credential database list

Lists database credentials

Usage: `neo4j-cli credential database list`

### neo4j-cli credential database remove

Removes a database credential

Usage: `neo4j-cli credential database remove <name>`

### neo4j-cli credential database use

Sets the default database credential to be used

Usage: `neo4j-cli credential database use <name>`

