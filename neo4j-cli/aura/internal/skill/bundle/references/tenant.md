# aura-cli tenant

Relates to an Aura Tenant

Usage: `aura-cli tenant`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |

## aura-cli tenant get

Returns tenant details

This subcommand returns details about a specific Aura Tenant.

Usage: `aura-cli tenant get <id>`

Examples:

```
# Get details of a tenant by ID
neo4j-cli aura tenant get 00000000-0000-0000-0000-000000000000

# Get tenant details and emit JSON for scripting
neo4j-cli aura tenant get 00000000-0000-0000-0000-000000000000 --format json

# Pipe details through jq to extract the tenant name
neo4j-cli aura tenant get 00000000-0000-0000-0000-000000000000 --format json | jq -r '.data.name'
```

## aura-cli tenant list

Returns a list of tenants

This subcommand returns a list containing a summary of each of your Aura Tenants.

When --organization-id is provided (or a default organization ID is stored on the active credential),
the request is scoped to that organization via GET v2beta1/organizations/{orgId}/projects.
Otherwise GET v1/tenants is used, which returns all tenants visible to the credential.

To find out more about a specific Tenant, retrieve the details using the get subcommand.

Usage: `aura-cli tenant list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--organization-id` | string | - | Organization ID to scope the tenant list; falls back to the default stored on the active credential |

Examples:

```
# List all tenants the current user has access to
neo4j-cli aura tenant list

# List tenants in a specific organization
neo4j-cli aura tenant list --organization-id 3d6481bf-2df1-47cf-8392-0288b1ac215f

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura tenant list --format json

# Pipe tenant ids through jq for a follow-up command
neo4j-cli aura tenant list --format json | jq -r '.data[].id'
```

