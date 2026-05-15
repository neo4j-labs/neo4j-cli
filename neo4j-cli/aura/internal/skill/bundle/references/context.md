# aura-cli context

Manage the active organization and project context

Usage: `aura-cli context`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |

## aura-cli context list

Returns a flat list of all accessible organization/project contexts

This subcommand lists all organization/project pairs accessible to the current user.
Each entry includes the context slug ({organizationId}/{projectId}), the organization and
project IDs and names, and whether this entry is the currently active default context.

Usage: `aura-cli context list`

Examples:

```
# List all accessible contexts in table format
neo4j-cli aura context list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura context list --format json

# Find the active context via jq
neo4j-cli aura context list --format json | jq -r '.data[] | select(.default == true) | .context'
```

## aura-cli context use

Sets the active organization and project context

This subcommand sets the active organization and project context used by default
in subsequent commands. Accepts either a positional {organizationId}/{projectId} slug
or the --organization-id and --project-id flags (but not both).

The context is validated against the Aura API before being persisted.

Usage: `aura-cli context use [organizationId/projectId] [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--organization-id` | string | - | Organization ID |
| `--project-id` | string | - | Project ID |

Examples:

```
# Set context using positional slug
neo4j-cli aura context use 00000000-0000-0000-0000-000000000000/11111111-1111-1111-1111-111111111111 --rw

# Set context using flags
neo4j-cli aura context use --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Verify the context was set after switching
neo4j-cli aura context use 00000000-0000-0000-0000-000000000000/11111111-1111-1111-1111-111111111111 --rw && neo4j-cli aura context list --format json
```

