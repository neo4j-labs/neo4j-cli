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

