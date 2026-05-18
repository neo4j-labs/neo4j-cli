# aura-cli workspace

Manage the active organization and project workspace

Usage: `aura-cli workspace`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |

## aura-cli workspace list

Returns a flat list of all accessible organization/project workspaces

This subcommand lists all organization/project pairs accessible to the current user.
Each entry includes the workspace slug ({organizationId}/{projectId}), the organization and
project IDs and names, and whether this entry is the currently active default workspace.

Usage: `aura-cli workspace list`

Examples:

```
# List all accessible workspaces in table format
neo4j-cli aura workspace list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura workspace list --format json

# Find the active workspace via jq
neo4j-cli aura workspace list --format json | jq -r '.data[] | select(.default == true) | .workspace'
```

## aura-cli workspace use

Sets the active organization and project workspace

This subcommand sets the active organization and project workspace used by default
in subsequent commands. Accepts either a positional {organizationId}/{projectId} slug
or the --organization-id and --project-id flags (but not both).

The workspace is validated against the Aura API before being persisted.

Usage: `aura-cli workspace use [organizationId/projectId] [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--organization-id` | string | - | Organization ID |
| `--project-id` | string | - | Project ID |

Examples:

```
# Set workspace using positional slug
neo4j-cli aura workspace use 00000000-0000-0000-0000-000000000000/11111111-1111-1111-1111-111111111111 --rw

# Set workspace using flags
neo4j-cli aura workspace use --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Verify the workspace was set after switching
neo4j-cli aura workspace use 00000000-0000-0000-0000-000000000000/11111111-1111-1111-1111-111111111111 --rw && neo4j-cli aura workspace list --format json
```

