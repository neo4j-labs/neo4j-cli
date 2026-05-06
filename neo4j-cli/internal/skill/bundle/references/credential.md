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

## neo4j-cli credential dbms

Manage and view dbms credential values

Usage: `neo4j-cli credential dbms`

### neo4j-cli credential dbms add

Adds a dbms credential

Usage: `neo4j-cli credential dbms add [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--database-name` | string | neo4j | Database name |
| `--insecure` | bool | false | Disable TLS verification |
| `--name` | string | - | (required) Name |
| `--password` | string | - | (required) Password |
| `--uri` | string | - | (required) URI |
| `--username` | string | - | (required) Username |

### neo4j-cli credential dbms list

Lists dbms credentials

Usage: `neo4j-cli credential dbms list`

### neo4j-cli credential dbms remove

Removes a dbms credential

Usage: `neo4j-cli credential dbms remove <name>`

### neo4j-cli credential dbms use

Sets the default dbms credential to be used

Usage: `neo4j-cli credential dbms use <name>`

