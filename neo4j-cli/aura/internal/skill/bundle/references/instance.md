# aura-cli instance

## Contents

- [aura-cli instance create](#aura-cli-instance-create)
- [aura-cli instance delete](#aura-cli-instance-delete)
- [aura-cli instance get](#aura-cli-instance-get)
- [aura-cli instance list](#aura-cli-instance-list)
- [aura-cli instance overwrite](#aura-cli-instance-overwrite)
- [aura-cli instance pause](#aura-cli-instance-pause)
- [aura-cli instance resume](#aura-cli-instance-resume)
- [aura-cli instance snapshot](#aura-cli-instance-snapshot)
- [aura-cli instance snapshot create](#aura-cli-instance-snapshot-create)
- [aura-cli instance snapshot get](#aura-cli-instance-snapshot-get)
- [aura-cli instance snapshot list](#aura-cli-instance-snapshot-list)
- [aura-cli instance update](#aura-cli-instance-update)

Relates to AuraDB or AuraDS instances

Usage: `aura-cli instance`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |
| `--organization-id` | string | - | ID of the Aura organization |
| `--project-id` | string | - | ID of the Aura project |

## aura-cli instance create

Creates a new instance

This subcommand starts the creation process of an Aura instance.

Region identifiers follow each cloud provider's own naming convention: AWS uses identifiers such as us-east-1, Azure uses identifiers such as eastus, and GCP uses identifiers such as us-central1.

If you're unsure of possible configurations, run 'tenant get' to discover the full list of supported configurations for your tenant. The output lists every valid combination of --cloud-provider, --region, --type, and --memory.

Creating an instance is an asynchronous operation that can be waited for with --wait. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand. Once the status transitions from "creating" to "running" you may begin to use your instance.

This subcommand returns your instance ID, initial credentials, connection URL along with your project id, cloud provider, region, instance type, and the instance name for you to use once the instance is running. It is important to store these initial credentials until you have the chance to login to your running instance and change them.

For Enterprise instances you can specify a --customer-managed-key-id flag to use a Customer Managed Key for encryption.

Usage: `aura-cli instance create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--cloud-provider` | cloud-provider | - | The cloud provider hosting the instance. Must be one of "aws", "azure", or "gcp". |
| `--credential-name` | string | - | The name to use when storing the credentials locally. Defaults to <instance-id>-default. |
| `--customer-managed-key-id` | string | - | An optional customer managed key to be used for instance creation. |
| `--graph-analytics-plugin` | bool | false | An optional graph analytics plugin configuration to be set during instance creation |
| `--memory` | memory | - | The size of the instance memory (e.g. 2GB, 8GB, 64GB). Run with an invalid value to see all accepted sizes. |
| `--name` | string | - | The name of the instance (any UTF-8 characters with no trailing or leading whitespace). If omitted, a default name is generated automatically (e.g. Instance01). |
| `--no-credential-print` | bool | false | Omit the password from the command output. |
| `--no-credential-storage` | bool | false | Skip storing the instance credentials locally after creation. |
| `--region` | string | - | The region where the instance is hosted. Values follow each cloud provider's naming convention (e.g. us-east-1 for AWS, eastus for Azure, europe-west1 for GCP). Run 'tenant get' to see the full list of supported regions for your tenant. |
| `--type` | type | - | (required) The type of the instance. Must be one of "free-db", "professional-db", "business-critical", "enterprise-db", "professional-ds", or "enterprise-ds". |
| `--vector-optimized` | bool | false | An optional vector optimization configuration to be set during instance creation |
| `--version` | string | 5 | The Neo4j version of the instance. |
| `--wait` | bool | false | Waits until created instance is ready. |

Examples:

```
# Create a free-db instance (no cloud provider, region, or memory required)
neo4j-cli aura instance create --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --type free-db --wait --rw

# Create a professional-db instance on AWS (us-east-1, N. Virginia)
neo4j-cli aura instance create --rw --name my-aws-instance --type professional-db --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --cloud-provider aws --region us-east-1 --memory 1GB

# Create a professional-db instance on GCP and emit JSON for scripting
neo4j-cli aura instance create --rw --name my-gcp-instance --type professional-db --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --cloud-provider gcp --region europe-west1 --memory 8GB --format json
```

## aura-cli instance delete

Deletes an instance

Starts the deletion process of an Aura instance.

Deleting an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

If another operation is being performed on the instance you are trying to delete, an error will be returned that indicates that deletion cannot be performed.

Usage: `aura-cli instance delete <id>`

Examples:

```
# Delete an instance by ID
neo4j-cli aura instance delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Delete an instance and emit the response as JSON
neo4j-cli aura instance delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json

# Delete and pipe the response status through jq
neo4j-cli aura instance delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json | jq -r '.data.status'
```

## aura-cli instance get

Returns instance details

This endpoint returns details about a specific Aura Instance.

Usage: `aura-cli instance get <id>`

Examples:

```
# Get details of an instance by ID
neo4j-cli aura instance get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get details and emit JSON for scripting
neo4j-cli aura instance get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json

# Pipe details through jq to extract the connection URL
neo4j-cli aura instance get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json | jq -r '.data.connection_url'
```

## aura-cli instance list

Returns a list of instances

This subcommand returns a list containing a summary of each of your Aura instances in the specified project. To find out more about a specific instance, retrieve the details using the get subcommand.

Use --organization-id and --project-id to specify which project's instances to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.

Usage: `aura-cli instance list`

Examples:

```
# List instances in a project (using flags)
neo4j-cli aura instance list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List instances using a configured default workspace
neo4j-cli aura instance list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura instance list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json
```

## aura-cli instance overwrite

Starts the process of overwriting the specified instance with data from the source instance provided

Starts the process of overwriting the specified instance with data from the source instance provided.

The overwrite process mimics the 'Clone to existing' functionality of the Aura Console.

If only --source-instance-id is provided, a new snapshot of that instance is created and used for overwriting. Alternatively, you can specify an additional --source-snapshot-id to use a specific snapshot for overwriting, from --source-instance-id provided, otherwise as a snapshot of the instance being overwritten. The snapshot specified must be exportable.

Usage: `aura-cli instance overwrite <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--source-instance-id` | string | - | The ID of the instance to overwrite with, from the source snapshot ID if provided, otherwise takes a new snapshot and overwrites |
| `--source-snapshot-id` | string | - | The ID of the snapshot to overwrite with, which must be exportable, from the source instance ID if provided, otherwise the argument provided instance |
| `--wait` | bool | false | Waits until created snapshot is ready |

Examples:

```
# Overwrite an instance with a fresh snapshot of a source instance
neo4j-cli aura instance overwrite 00000000-0000-0000-0000-000000000000 --source-instance-id 11111111-1111-1111-1111-111111111111 --rw

# Overwrite using a specific exportable snapshot and wait until ready
neo4j-cli aura instance overwrite 00000000-0000-0000-0000-000000000000 --source-instance-id 11111111-1111-1111-1111-111111111111 --source-snapshot-id 22222222-2222-2222-2222-222222222222 --wait --rw

# Overwrite and emit JSON for scripting
neo4j-cli aura instance overwrite 00000000-0000-0000-0000-000000000000 --source-instance-id 11111111-1111-1111-1111-111111111111 --rw --format json
```

## aura-cli instance pause

Pauses an instance

Starts the pause process of an Aura instance.

Pausing an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

The pause time depends on the amount of data stored in the instance; larger quantities of data will take longer. The exact time this will take is dependent on the size of your data store.

If another operation is being performed on the instance you are trying to pause, an error will be returned that indicates that the pause operation cannot be performed.

Usage: `aura-cli instance pause <id>`

Examples:

```
# Pause an Aura instance
neo4j-cli aura instance pause 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Pause an instance and emit the response as JSON
neo4j-cli aura instance pause 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json

# Pause and pipe the response status through jq
neo4j-cli aura instance pause 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json | jq -r '.data.status'
```

## aura-cli instance resume

Resumes an instance

Starts the resume process of an Aura instance.

Resuming an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

If another operation is being performed on the instance you are trying to resume, an error will be returned that indicates that resume cannot be performed.

Usage: `aura-cli instance resume <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--wait` | bool | false | Waits until resumed instance is ready. |

Examples:

```
# Resume a paused Aura instance
neo4j-cli aura instance resume 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Resume and wait until the instance is ready
neo4j-cli aura instance resume 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --wait --rw

# Resume and emit the response as JSON for scripting
neo4j-cli aura instance resume 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json
```

## aura-cli instance snapshot

Relates to an instance snapshots

Usage: `aura-cli instance snapshot`

### aura-cli instance snapshot create

Takes an on-demand snapshot

This subcommand starts the on-demand snapshot creation process for an Aura instance.
Creating a snapshot is an asynchronous operation. You can poll the current status of this operation by periodically getting the snapshots details for the instance ID using the get subcommand.
The time taken to complete a snapshot depends on the amount of data stored in the instance; larger quantities of data will take longer. The exact time this will take is dependent on the size of your data store.

Usage: `aura-cli instance snapshot create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | (required) The ID of the instance to create a snapshot of |
| `--wait` | bool | false | Waits until created snapshot is ready. |

Examples:

```
# Take an on-demand snapshot of an instance
neo4j-cli aura instance snapshot create --instance-id 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Take a snapshot and wait until it is ready
neo4j-cli aura instance snapshot create --instance-id 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --wait --rw

# Take a snapshot and emit JSON for scripting
neo4j-cli aura instance snapshot create --instance-id 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json
```

### aura-cli instance snapshot get

Get details of a snapshot

This endpoint returns details about a specific snapshot.

Usage: `aura-cli instance snapshot get <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | The ID of the instance to get the snapshot details of |

Examples:

```
# Get details of a snapshot by its ID
neo4j-cli aura instance snapshot get 22222222-2222-2222-2222-222222222222 --instance-id 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get details and emit JSON for scripting
neo4j-cli aura instance snapshot get 22222222-2222-2222-2222-222222222222 --instance-id 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json

# Check whether a snapshot is exportable via jq
neo4j-cli aura instance snapshot get 22222222-2222-2222-2222-222222222222 --instance-id 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json | jq -r '.data.exportable'
```

### aura-cli instance snapshot list

Returns a list of snapshots

This subcommand returns a list of available snapshots from the current day.

Usage: `aura-cli instance snapshot list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--date` | string | - | An optional date to list snapshots for a given day, defaults to today. Must be formatted with an ISO formatted date string (YYYY-MM-DD) |
| `--instance-id` | string | - | The ID of the instance to list the snapshots of |

Examples:

```
# List today's snapshots for an instance
neo4j-cli aura instance snapshot list --instance-id 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List snapshots for a specific date
neo4j-cli aura instance snapshot list --instance-id 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --date 2025-01-15

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura instance snapshot list --instance-id 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json
```

## aura-cli instance update

Updates an instance

This command allows you to rename and/or resize an Aura instance.

Resizing an instance is an asynchronous operation. The instance remains available throughout.

Usage: `aura-cli instance update <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--memory` | string | - | The size of the instance memory in GB. |
| `--name` | string | - | The name of the instance (any UTF-8 characters with no trailing or leading whitespace). |

Examples:

```
# Rename an Aura instance
neo4j-cli aura instance update 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --name my-renamed-instance --rw

# Resize an Aura instance to 8GB of memory
neo4j-cli aura instance update 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --memory 8GB --rw

# Rename and resize, emitting JSON for scripting
neo4j-cli aura instance update 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --name my-renamed-instance --memory 8GB --rw --format json
```

