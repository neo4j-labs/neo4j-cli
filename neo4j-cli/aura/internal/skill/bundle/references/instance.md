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

## aura-cli instance create

Creates a new instance

This subcommand starts the creation process of an Aura instance.

Creating an instance is an asynchronous operation that can be awaited with --await. Supported instance configurations for your tenant can be obtained by calling the tenant get subcommand.

You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand. Once the status transitions from "creating" to "running" you may begin to use your instance.

This subcommand returns your instance ID, initial credentials, connection URL along with your tenant id, cloud provider, region, instance type, and the instance name for you to use once the instance is running. It is important to store these initial credentials until you have the chance to login to your running instance and change them.

You must also provide a --cloud-provider flag with the subcommand, which specifies which cloud provider the instances will be hosted in. The acceptable values for this field are gcp, aws, or azure.

For Enterprise instances you can specify a --customer-managed-key-id flag to use a Customer Managed Key for encryption.

Usage: `aura-cli instance create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--await` | bool | false | Waits until created instance is ready. |
| `--cloud-provider` | cloud-provider | - | The cloud provider hosting the instance. |
| `--customer-managed-key-id` | string | - | An optional customer managed key to be used for instance creation. |
| `--graph-analytics-plugin` | bool | false | An optional graph analytics plugin configuration to be set during instance creation |
| `--memory` | memory | - | The size of the instance memory in GB. |
| `--name` | string | - | (required) The name of the instance (any UTF-8 characters with no trailing or leading whitespace). |
| `--region` | string | - | The region where the instance is hosted. |
| `--tenant-id` | string | - | The Aura tenant/project ID |
| `--type` | type | - | (required) The type of the instance. |
| `--vector-optimized` | bool | false | An optional vector optimization configuration to be set during instance creation |
| `--version` | string | 5 | The Neo4j version of the instance. |

## aura-cli instance delete

Deletes an instance

Starts the deletion process of an Aura instance.

Deleting an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

If another operation is being performed on the instance you are trying to delete, an error will be returned that indicates that deletion cannot be performed.

Usage: `aura-cli instance delete <id>`

## aura-cli instance get

Returns instance details

This endpoint returns details about a specific Aura Instance.

Usage: `aura-cli instance get <id>`

## aura-cli instance list

Returns a list of instances

This subcommand returns a list containing a summary of each of your Aura instances. To find out more about a specific instance, retrieve the details using the get subcommand.

You can filter instances in a particular tenant using --tenant-id. If the tenant flag is not specified, this subcommand lists all instances a user has access to across all tenants.

Usage: `aura-cli instance list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--tenant-id` | string | - | An optional Tenant ID to filter instances in a tenant |

## aura-cli instance overwrite

Starts the process of overwriting the specified instance with data from the source instance provided

Starts the process of overwriting the specified instance with data from the source instance provided.

The overwrite process mimics the 'Clone to existing' functionality of the Aura Console.

If only --source-instance-id is provided, a new snapshot of that instance is created and used for overwriting. Alternatively, you can specify an additional --source-snapshot-id to use a specific snapshot for overwriting, from --source-instance-id provided, otherwise as a snapshot of the instance being overwritten. The snapshot specified must be exportable.

Usage: `aura-cli instance overwrite <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--await` | bool | false | Waits until created snapshot is ready |
| `--source-instance-id` | string | - | The ID of the instance to overwrite with, from the source snapshot ID if provided, otherwise takes a new snapshot and overwrites |
| `--source-snapshot-id` | string | - | The ID of the snapshot to overwrite with, which must be exportable, from the source instance ID if provided, otherwise the argument provided instance |

## aura-cli instance pause

Pauses an instance

Starts the pause process of an Aura instance.

Pausing an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

The pause time depends on the amount of data stored in the instance; larger quantities of data will take longer. The exact time this will take is dependent on the size of your data store.

If another operation is being performed on the instance you are trying to pause, an error will be returned that indicates that the pause operation cannot be performed.

Usage: `aura-cli instance pause <id>`

## aura-cli instance resume

Resumes an instance

Starts the resume process of an Aura instance.

Resuming an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

If another operation is being performed on the instance you are trying to resume, an error will be returned that indicates that resume cannot be performed.

Usage: `aura-cli instance resume <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--await` | bool | false | Waits until resumed instance is ready. |

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
| `--await` | bool | false | Waits until created snapshot is ready. |
| `--instance-id` | string | - | (required) The ID of the instance to create a snapshot of |

### aura-cli instance snapshot get

Get details of a snapshot

This endpoint returns details about a specific snapshot.

Usage: `aura-cli instance snapshot get <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | The ID of the instance to get the snapshot details of |

### aura-cli instance snapshot list

Returns a list of snapshots

This subcommand returns a list of available snapshots from the current day.

Usage: `aura-cli instance snapshot list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--date` | string | - | An optional date to list snapshots for a given day, defaults to today. Must be formatted with an ISO formatted date string (YYYY-MM-DD) |
| `--instance-id` | string | - | The ID of the instance to list the snapshots of |

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

