# neo4j-cli credential

Manage and view credential values

Usage: `neo4j-cli credential`

## neo4j-cli credential add

Adds a credential

Usage: `neo4j-cli credential add`

### neo4j-cli credential add aura-client

Adds an Aura client credential

Usage: `neo4j-cli credential add aura-client [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--client-id` | string | - | (required) Client ID |
| `--client-secret` | string | - | (required) Client secret |
| `--name` | string | - | (required) Name |

## neo4j-cli credential list

List credentials

Usage: `neo4j-cli credential list`

### neo4j-cli credential list aura-client

Lists Aura client credentials

Usage: `neo4j-cli credential list aura-client`

## neo4j-cli credential remove

Removes a credential

Usage: `neo4j-cli credential remove`

### neo4j-cli credential remove aura-client

Removes an Aura client credential

Usage: `neo4j-cli credential remove aura-client <name>`

## neo4j-cli credential use

Sets the default credential to be used

Usage: `neo4j-cli credential use`

### neo4j-cli credential use aura-client

Sets the default Aura client credential to be used

Usage: `neo4j-cli credential use aura-client <name>`

