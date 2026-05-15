# aura-cli organization

Manage Aura organizations

Usage: `aura-cli organization`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |

## aura-cli organization list

Returns a list of organizations

This subcommand returns a list of Aura organizations accessible to the current user.

Usage: `aura-cli organization list`

Examples:

```
# List all organizations the current user has access to
neo4j-cli aura organization list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura organization list --format json

# Pipe organization ids through jq for a follow-up command
neo4j-cli aura organization list --format json | jq -r '.data[].id'
```

