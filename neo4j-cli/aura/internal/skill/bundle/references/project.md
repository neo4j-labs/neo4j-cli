# aura-cli project

Manage Aura projects

Usage: `aura-cli project`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |

## aura-cli project get

Returns project details

This subcommand returns details about a specific Aura project.

Usage: `aura-cli project get <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--organization-id` | string | - | Organization ID (defaults to org portion of aura.default-context) |

Examples:

```
# Get project details (using default organization from aura.default-context)
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000

# Get project details with an explicit organization ID
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000001

# Emit JSON for scripting
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000001 --format json
```

## aura-cli project list

Returns a list of projects

This subcommand returns a list of Aura projects within the given organization.

Usage: `aura-cli project list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--organization-id` | string | - | Organization ID (defaults to org portion of aura.default-context) |

Examples:

```
# List all projects in the default organization (from aura.default-context)
neo4j-cli aura project list

# List projects in a specific organization
neo4j-cli aura project list --organization-id 00000000-0000-0000-0000-000000000000

# Emit JSON for scripting
neo4j-cli aura project list --organization-id 00000000-0000-0000-0000-000000000000 --format json
```

