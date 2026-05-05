# neo4j-cli aura

## Contents

- [neo4j-cli aura config](#neo4j-cli-aura-config)
- [neo4j-cli aura config get](#neo4j-cli-aura-config-get)
- [neo4j-cli aura config list](#neo4j-cli-aura-config-list)
- [neo4j-cli aura config set](#neo4j-cli-aura-config-set)
- [neo4j-cli aura customer-managed-key](#neo4j-cli-aura-customer-managed-key)
- [neo4j-cli aura customer-managed-key create](#neo4j-cli-aura-customer-managed-key-create)
- [neo4j-cli aura customer-managed-key delete](#neo4j-cli-aura-customer-managed-key-delete)
- [neo4j-cli aura customer-managed-key get](#neo4j-cli-aura-customer-managed-key-get)
- [neo4j-cli aura customer-managed-key list](#neo4j-cli-aura-customer-managed-key-list)
- [neo4j-cli aura graph-analytics](#neo4j-cli-aura-graph-analytics)
- [neo4j-cli aura graph-analytics session](#neo4j-cli-aura-graph-analytics-session)
- [neo4j-cli aura graph-analytics session create](#neo4j-cli-aura-graph-analytics-session-create)
- [neo4j-cli aura graph-analytics session delete](#neo4j-cli-aura-graph-analytics-session-delete)
- [neo4j-cli aura graph-analytics session get](#neo4j-cli-aura-graph-analytics-session-get)
- [neo4j-cli aura graph-analytics session list](#neo4j-cli-aura-graph-analytics-session-list)
- [neo4j-cli aura instance](#neo4j-cli-aura-instance)
- [neo4j-cli aura instance create](#neo4j-cli-aura-instance-create)
- [neo4j-cli aura instance delete](#neo4j-cli-aura-instance-delete)
- [neo4j-cli aura instance get](#neo4j-cli-aura-instance-get)
- [neo4j-cli aura instance list](#neo4j-cli-aura-instance-list)
- [neo4j-cli aura instance overwrite](#neo4j-cli-aura-instance-overwrite)
- [neo4j-cli aura instance pause](#neo4j-cli-aura-instance-pause)
- [neo4j-cli aura instance resume](#neo4j-cli-aura-instance-resume)
- [neo4j-cli aura instance snapshot](#neo4j-cli-aura-instance-snapshot)
- [neo4j-cli aura instance snapshot create](#neo4j-cli-aura-instance-snapshot-create)
- [neo4j-cli aura instance snapshot get](#neo4j-cli-aura-instance-snapshot-get)
- [neo4j-cli aura instance snapshot list](#neo4j-cli-aura-instance-snapshot-list)
- [neo4j-cli aura instance update](#neo4j-cli-aura-instance-update)
- [neo4j-cli aura tenant](#neo4j-cli-aura-tenant)
- [neo4j-cli aura tenant get](#neo4j-cli-aura-tenant-get)
- [neo4j-cli aura tenant list](#neo4j-cli-aura-tenant-list)

Allows you to programmatically provision and manage your Aura resources

Usage: `neo4j-cli aura`

## neo4j-cli aura config

Manage and view configuration values

Usage: `neo4j-cli aura config`

### neo4j-cli aura config get

Displays the specified configuration value

Usage: `neo4j-cli aura config get <key>`

### neo4j-cli aura config list

Lists the current configuration of the Aura CLI subcommand

Usage: `neo4j-cli aura config list`

### neo4j-cli aura config set

Sets the specified configuration value to the provided value

Usage: `neo4j-cli aura config set <key> <value>`

## neo4j-cli aura customer-managed-key

Relates to Customer Managed Keys

Usage: `neo4j-cli aura customer-managed-key`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `--output` | string | - | Format to print console output in, from a choice of [default, json, table] |

### neo4j-cli aura customer-managed-key create

Creates a new customer managed key

This subcommand creates a new Customer Managed Key in Aura. Creating a new key is an asynchronous operation.

Before you can use the key you will need to setup permissions for it. Log in to the Console, navigate to 'Customer Managed Keys' and click on the Edit icon next to the Key in order to see the instructions.

You can poll the current status of this operation by periodically getting the key details using the get subcommand.

Once the key has a status of ready you can use it for creating new instances by setting the --customer-managed-key-id flag.

Usage: `neo4j-cli aura customer-managed-key create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--await` | bool | false | Waits until created customer managed key is ready. |
| `--cloud-provider` | cloud-provider | - | (required) The cloud provider hosting the instance. |
| `--key-id` | string | - | (required) Encryption Key ARN |
| `--name` | string | - | (required) The name of the customer managed key (any UTF-8 characters with no trailing or leading whitespace). |
| `--region` | string | - | (required) The region where the instance is hosted. |
| `--tenant-id` | string | - | The Aura tenant/project ID |
| `--type` | type | - | (required) The type of the instance. |

### neo4j-cli aura customer-managed-key delete

Deletes a customer managed key

Deletes a Customer Managed Key from Aura.

Note that you can only delete a Key if it is not being used by any instances, otherwise you will get an error with the reason field set to encryption-key-is-active.

Usage: `neo4j-cli aura customer-managed-key delete <id>`

### neo4j-cli aura customer-managed-key get

Returns a customer managed key details

This subcommand returns details about a specific Customer Managed Key.

Usage: `neo4j-cli aura customer-managed-key get <id>`

### neo4j-cli aura customer-managed-key list

Returns a list of customer managed keys

This subcommand returns a list containing a summary of each of your customer managed keys. To find out more about a specific key, retrieve the details using the get subcommand.

You can filter keys in a particular tenant using --tenant-id. If the tenant flag is not specified, this endpoint lists all keys a user has access to across all tenants.

Usage: `neo4j-cli aura customer-managed-key list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--tenant-id` | string | - | An optional Tenant ID to filter customer managed keys in a tenant |

## neo4j-cli aura graph-analytics

Relates to Aura Graph Analytics

Usage: `neo4j-cli aura graph-analytics`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `--output` | string | - | Format to print console output in, from a choice of [default, json, table] |

### neo4j-cli aura graph-analytics session

Relates to Aura Graph Analytics

Usage: `neo4j-cli aura graph-analytics session`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `--output` | string | - | Format to print console output in, from a choice of [default, json, table] |

#### neo4j-cli aura graph-analytics session create

Creates a new Aura Graph Analytics Serverless session

This subcommand gets or creates a Aura Graph Analytics Serverless session. If no Session with a matching name and project/tenant is found, one will be created. A Session is either attached to an AuraDB, or standalone.
				Creating a session is an asynchronous operation that can be awaited with --await.

Usage: `neo4j-cli aura graph-analytics session create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--await` | bool | false | Waits until created session is ready. |
| `--cloud-provider` | string | - | The cloud provider hosting the session. |
| `--instance-id` | string | - | The ID of the instance to create the session for. |
| `--memory` | string | - | (required) The size of the session memory in GB. |
| `--name` | string | - | (required) The name of the session. |
| `--region` | string | - | The region where the session is hosted. |
| `--tenant-id` | string | - | The Aura project/tenant ID |
| `--ttl` | string | - | This optional parameter specifies the time-to-live of the session. The session will be marked as expired if the session was unused for the provided duration. |

#### neo4j-cli aura graph-analytics session delete

Delete a Graph Analytics Serverless session

This subcommand deletes a Graph Analytics Serverless session by id.

Usage: `neo4j-cli aura graph-analytics session delete <id>`

#### neo4j-cli aura graph-analytics session get

Get a Graph Analytics Serverless session

This subcommand returns the details of a Graph Analytics Serverless session.

Usage: `neo4j-cli aura graph-analytics session get <id>`

#### neo4j-cli aura graph-analytics session list

Returns a list of Graph Analytics Serverless sessions

This subcommand returns a list containing a summary of each of your Graph Analytics Serverless session
				By default, this subcommand lists all sessions a user has access to across all projects.
				You can filter sessions in a particular project/tenant using:
				--organization-id <organization-id>
				--tenant-id <tenant-id>
				--instance-id <instance-id>

Usage: `neo4j-cli aura graph-analytics session list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | An optional Instance ID to filter for sessions attached to an instance |
| `--organization-id` | string | - | An optional Organization ID to filter sessions in an organization |
| `--tenant-id` | string | - | An optional Project ID to filter sessions in a project/tenant |

## neo4j-cli aura instance

Relates to AuraDB or AuraDS instances

Usage: `neo4j-cli aura instance`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `--output` | string | - | Format to print console output in, from a choice of [default, json, table] |

### neo4j-cli aura instance create

Creates a new instance

This subcommand starts the creation process of an Aura instance.

Creating an instance is an asynchronous operation that can be awaited with --await. Supported instance configurations for your tenant can be obtained by calling the tenant get subcommand.

You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand. Once the status transitions from "creating" to "running" you may begin to use your instance.

This subcommand returns your instance ID, initial credentials, connection URL along with your tenant id, cloud provider, region, instance type, and the instance name for you to use once the instance is running. It is important to store these initial credentials until you have the chance to login to your running instance and change them.

You must also provide a --cloud-provider flag with the subcommand, which specifies which cloud provider the instances will be hosted in. The acceptable values for this field are gcp, aws, or azure.

For Enterprise instances you can specify a --customer-managed-key-id flag to use a Customer Managed Key for encryption.

Usage: `neo4j-cli aura instance create [flags]`

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

### neo4j-cli aura instance delete

Deletes an instance

Starts the deletion process of an Aura instance.

Deleting an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

If another operation is being performed on the instance you are trying to delete, an error will be returned that indicates that deletion cannot be performed.

Usage: `neo4j-cli aura instance delete <id>`

### neo4j-cli aura instance get

Returns instance details

This endpoint returns details about a specific Aura Instance.

Usage: `neo4j-cli aura instance get <id>`

### neo4j-cli aura instance list

Returns a list of instances

This subcommand returns a list containing a summary of each of your Aura instances. To find out more about a specific instance, retrieve the details using the get subcommand.

You can filter instances in a particular tenant using --tenant-id. If the tenant flag is not specified, this subcommand lists all instances a user has access to across all tenants.

Usage: `neo4j-cli aura instance list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--tenant-id` | string | - | An optional Tenant ID to filter instances in a tenant |

### neo4j-cli aura instance overwrite

Starts the process of overwriting the specified instance with data from the source instance provided

Starts the process of overwriting the specified instance with data from the source instance provided.

The overwrite process mimics the 'Clone to existing' functionality of the Aura Console.

If only --source-instance-id is provided, a new snapshot of that instance is created and used for overwriting. Alternatively, you can specify an additional --source-snapshot-id to use a specific snapshot for overwriting, from --source-instance-id provided, otherwise as a snapshot of the instance being overwritten. The snapshot specified must be exportable.

Usage: `neo4j-cli aura instance overwrite <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--await` | bool | false | Waits until created snapshot is ready |
| `--source-instance-id` | string | - | The ID of the instance to overwrite with, from the source snapshot ID if provided, otherwise takes a new snapshot and overwrites |
| `--source-snapshot-id` | string | - | The ID of the snapshot to overwrite with, which must be exportable, from the source instance ID if provided, otherwise the argument provided instance |

### neo4j-cli aura instance pause

Pauses an instance

Starts the pause process of an Aura instance.

Pausing an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

The pause time depends on the amount of data stored in the instance; larger quantities of data will take longer. The exact time this will take is dependent on the size of your data store.

If another operation is being performed on the instance you are trying to pause, an error will be returned that indicates that the pause operation cannot be performed.

Usage: `neo4j-cli aura instance pause <id>`

### neo4j-cli aura instance resume

Resumes an instance

Starts the resume process of an Aura instance.

Resuming an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

If another operation is being performed on the instance you are trying to resume, an error will be returned that indicates that resume cannot be performed.

Usage: `neo4j-cli aura instance resume <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--await` | bool | false | Waits until resumed instance is ready. |

### neo4j-cli aura instance snapshot

Relates to an instance snapshots

Usage: `neo4j-cli aura instance snapshot`

#### neo4j-cli aura instance snapshot create

Takes an on-demand snapshot

This subcommand starts the on-demand snapshot creation process for an Aura instance.
Creating a snapshot is an asynchronous operation. You can poll the current status of this operation by periodically getting the snapshots details for the instance ID using the get subcommand.
The time taken to complete a snapshot depends on the amount of data stored in the instance; larger quantities of data will take longer. The exact time this will take is dependent on the size of your data store.

Usage: `neo4j-cli aura instance snapshot create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--await` | bool | false | Waits until created snapshot is ready. |
| `--instance-id` | string | - | (required) The ID of the instance to create a snapshot of |

#### neo4j-cli aura instance snapshot get

Get details of a snapshot

This endpoint returns details about a specific snapshot.

Usage: `neo4j-cli aura instance snapshot get <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | The ID of the instance to get the snapshot details of |

#### neo4j-cli aura instance snapshot list

Returns a list of snapshots

This subcommand returns a list of available snapshots from the current day.

Usage: `neo4j-cli aura instance snapshot list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--date` | string | - | An optional date to list snapshots for a given day, defaults to today. Must be formatted with an ISO formatted date string (YYYY-MM-DD) |
| `--instance-id` | string | - | The ID of the instance to list the snapshots of |

### neo4j-cli aura instance update

Updates an instance

This command allows you to rename and/or resize an Aura instance.

Resizing an instance is an asynchronous operation. The instance remains available throughout.

Usage: `neo4j-cli aura instance update <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--memory` | string | - | The size of the instance memory in GB. |
| `--name` | string | - | The name of the instance (any UTF-8 characters with no trailing or leading whitespace). |

## neo4j-cli aura tenant

Relates to an Aura Tenant

Usage: `neo4j-cli aura tenant`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `--output` | string | - | Format to print console output in, from a choice of [default, json, table] |

### neo4j-cli aura tenant get

Returns tenant details

This subcommand returns details about a specific Aura Tenant.

Usage: `neo4j-cli aura tenant get <id>`

### neo4j-cli aura tenant list

Returns a list of tenants

This subcommand returns a list containing a summary of each of your Aura Tenants. To find out more about a specific Tenant, retrieve the details using the get subcommand.

Usage: `neo4j-cli aura tenant list`

