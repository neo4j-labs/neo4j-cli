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
| `--organization-id` | string | - | ID of the Aura organization |
| `--project-id` | string | - | ID of the Aura project |

### aura-cli graph-analytics session create

Creates a new Aura Graph Analytics Serverless session

This subcommand gets or creates a Aura Graph Analytics Serverless session. If no Session with a matching name and project is found, one will be created. A Session is either attached to an AuraDB, or standalone.
Creating a session is an asynchronous operation that can be waited for with --wait.

Usage: `aura-cli graph-analytics session create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--cloud-provider` | string | - | The cloud provider hosting the session. |
| `--instance-id` | string | - | The ID of the instance to create the session for. |
| `--memory` | string | - | (required) The size of the session memory in GB. |
| `--name` | string | - | (required) The name of the session. |
| `--region` | string | - | The region where the session is hosted. |
| `--ttl` | string | - | This optional parameter specifies the time-to-live of the session. The session will be marked as expired if the session was unused for the provided duration. |
| `--wait` | bool | false | Waits until created session is ready. |

Examples:

```
# Create a standalone session in a specific project on AWS
neo4j-cli aura graph-analytics session create --rw --name my-session --memory 8GB --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --cloud-provider aws --region us-east-1

# Create a session attached to an existing Aura instance and wait until ready
neo4j-cli aura graph-analytics session create --rw --name attached-session --memory 8GB --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --wait

# Create a session with a TTL and emit JSON for scripting
neo4j-cli aura graph-analytics session create --rw --name scripted-session --memory 4GB --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --ttl 1h --format json
```

### aura-cli graph-analytics session delete

Delete a Graph Analytics Serverless session

This subcommand deletes a Graph Analytics Serverless session by id.

Usage: `aura-cli graph-analytics session delete <id>`

Examples:

```
# Delete a session by ID
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Delete a session and emit JSON for scripting
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json

# Delete a session, suppressing all stdout output
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw > /dev/null
```

### aura-cli graph-analytics session get

Get a Graph Analytics Serverless session

This subcommand returns the details of a Graph Analytics Serverless session.

Usage: `aura-cli graph-analytics session get <id>`

Examples:

```
# Get a session by ID
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Render the session as a TOON table
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format toon

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json
```

### aura-cli graph-analytics session list

Returns a list of Graph Analytics Serverless sessions

This subcommand returns a list containing a summary of each of your Graph Analytics Serverless sessions in the specified project.

Use --organization-id and --project-id to specify which project's sessions to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.

Usage: `aura-cli graph-analytics session list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | An optional Instance ID to filter for sessions attached to an instance |

Examples:

```
# List all Graph Analytics sessions in a project
neo4j-cli aura graph-analytics session list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List sessions using a configured default workspace
neo4j-cli aura graph-analytics session list

# List sessions attached to a specific instance and emit JSON for scripting
neo4j-cli aura graph-analytics session list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --instance-id 00000000 --format json
```

