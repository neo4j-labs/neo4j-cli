# aura-cli graph-analytics

Relates to Aura Graph Analytics

Usage: `aura-cli graph-analytics`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `--output` | string | - | Format to print console output in, from a choice of [default, json, table] |

## aura-cli graph-analytics session

Relates to Aura Graph Analytics

Usage: `aura-cli graph-analytics session`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `--output` | string | - | Format to print console output in, from a choice of [default, json, table] |

### aura-cli graph-analytics session create

Creates a new Aura Graph Analytics Serverless session

This subcommand gets or creates a Aura Graph Analytics Serverless session. If no Session with a matching name and project/tenant is found, one will be created. A Session is either attached to an AuraDB, or standalone.
				Creating a session is an asynchronous operation that can be awaited with --await.

Usage: `aura-cli graph-analytics session create [flags]`

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

### aura-cli graph-analytics session delete

Delete a Graph Analytics Serverless session

This subcommand deletes a Graph Analytics Serverless session by id.

Usage: `aura-cli graph-analytics session delete <id>`

### aura-cli graph-analytics session get

Get a Graph Analytics Serverless session

This subcommand returns the details of a Graph Analytics Serverless session.

Usage: `aura-cli graph-analytics session get <id>`

### aura-cli graph-analytics session list

Returns a list of Graph Analytics Serverless sessions

This subcommand returns a list containing a summary of each of your Graph Analytics Serverless session
				By default, this subcommand lists all sessions a user has access to across all projects.
				You can filter sessions in a particular project/tenant using:
				--organization-id <organization-id>
				--tenant-id <tenant-id>
				--instance-id <instance-id>

Usage: `aura-cli graph-analytics session list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | An optional Instance ID to filter for sessions attached to an instance |
| `--organization-id` | string | - | An optional Organization ID to filter sessions in an organization |
| `--tenant-id` | string | - | An optional Project ID to filter sessions in a project/tenant |

