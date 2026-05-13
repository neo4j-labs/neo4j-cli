# aura-cli graph-analytics

## Contents

- [aura-cli graph-analytics session](#aura-cli-graph-analytics-session)
- [aura-cli graph-analytics session create](#aura-cli-graph-analytics-session-create)
- [aura-cli graph-analytics session delete](#aura-cli-graph-analytics-session-delete)
- [aura-cli graph-analytics session get](#aura-cli-graph-analytics-session-get)
- [aura-cli graph-analytics session list](#aura-cli-graph-analytics-session-list)

Relates to Aura Graph Analytics

Usage: `aura-cli graph-analytics`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |

## aura-cli graph-analytics session

Relates to Aura Graph Analytics

Usage: `aura-cli graph-analytics session`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |

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

Examples:

```
# Create a standalone session in a specific project/tenant on AWS
neo4j-cli aura graph-analytics session create --rw --name my-session --memory 8GB --tenant-id 00000000-0000-0000-0000-000000000000 --cloud-provider aws --region us-east-1

# Create a session attached to an existing Aura instance and wait until ready
neo4j-cli aura graph-analytics session create --rw --name attached-session --memory 8GB --instance-id 00000000 --await

# Create a session with a TTL and emit JSON for scripting
neo4j-cli aura graph-analytics session create --rw --name scripted-session --memory 4GB --instance-id 00000000 --ttl 1h --format json
```

### aura-cli graph-analytics session delete

Delete a Graph Analytics Serverless session

This subcommand deletes a Graph Analytics Serverless session by id.

Usage: `aura-cli graph-analytics session delete <id>`

Examples:

```
# Delete a session by ID
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --rw

# Delete a session and emit JSON for scripting
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --rw --format json

# Delete a session, suppressing all stdout output
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --rw > /dev/null
```

### aura-cli graph-analytics session get

Get a Graph Analytics Serverless session

This subcommand returns the details of a Graph Analytics Serverless session.

Usage: `aura-cli graph-analytics session get <id>`

Examples:

```
# Get a session by ID
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000

# Render the session as a TOON table
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000 --format toon

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000 --format json
```

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

Examples:

```
# List all Graph Analytics sessions the current user has access to
neo4j-cli aura graph-analytics session list

# List sessions in a specific project/tenant
neo4j-cli aura graph-analytics session list --tenant-id 00000000-0000-0000-0000-000000000000

# List sessions attached to a specific instance and emit JSON for scripting
neo4j-cli aura graph-analytics session list --instance-id 00000000 --format json
```

