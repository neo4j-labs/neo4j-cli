# aura-cli credential

Manage and view credential values

Usage: `aura-cli credential`

## aura-cli credential add

Adds a credential

Usage: `aura-cli credential add [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--client-id` | string | - | (required) Client ID |
| `--client-secret` | string | - | (required) Client secret |
| `--name` | string | - | (required) Name |

Examples:

```
# Add an Aura Console API credential (becomes the default if it is the first one)
neo4j-cli aura credential add --name my-creds --client-id <client-id> --client-secret <client-secret> --rw

# Add a second credential alongside an existing default
neo4j-cli aura credential add --name staging --client-id <client-id> --client-secret <client-secret> --rw

# Add a credential and emit the response as JSON
neo4j-cli aura credential add --name my-creds --client-id <client-id> --client-secret <client-secret> --rw --format json
```

## aura-cli credential list

list credentials

Usage: `aura-cli credential list`

Examples:

```
# List all stored Aura credentials (the default column flags the active one)
neo4j-cli aura credential list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura credential list --format json

# Pipe through jq to print just the default credential's name
neo4j-cli aura credential list --format json | jq -r '.data[] | select(.default == true) | .name'
```

## aura-cli credential remove

Removes a credential

Usage: `aura-cli credential remove <name>`

Examples:

```
# Remove a stored credential by name
neo4j-cli aura credential remove my-creds --rw

# Remove a staging credential
neo4j-cli aura credential remove staging --rw

# Remove and confirm by listing remaining credentials as JSON
neo4j-cli aura credential remove my-creds --rw && neo4j-cli aura credential list --format json
```

## aura-cli credential use

Sets the default credential to be used

Usage: `aura-cli credential use <name>`

Examples:

```
# Switch the default credential used by subsequent aura commands
neo4j-cli aura credential use my-creds --rw

# Switch to a staging credential before running write operations
neo4j-cli aura credential use staging --rw

# Switch and verify the default has been set
neo4j-cli aura credential use my-creds --rw && neo4j-cli aura credential list --format json
```

