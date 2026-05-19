# neo4j-cli aura

## Contents

- [neo4j-cli aura agent](#neo4j-cli-aura-agent)
- [neo4j-cli aura agent create](#neo4j-cli-aura-agent-create)
- [neo4j-cli aura agent delete](#neo4j-cli-aura-agent-delete)
- [neo4j-cli aura agent get](#neo4j-cli-aura-agent-get)
- [neo4j-cli aura agent invoke](#neo4j-cli-aura-agent-invoke)
- [neo4j-cli aura agent list](#neo4j-cli-aura-agent-list)
- [neo4j-cli aura agent replace](#neo4j-cli-aura-agent-replace)
- [neo4j-cli aura agent update](#neo4j-cli-aura-agent-update)
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
- [neo4j-cli aura organization](#neo4j-cli-aura-organization)
- [neo4j-cli aura organization get](#neo4j-cli-aura-organization-get)
- [neo4j-cli aura organization list](#neo4j-cli-aura-organization-list)
- [neo4j-cli aura project](#neo4j-cli-aura-project)
- [neo4j-cli aura project get](#neo4j-cli-aura-project-get)
- [neo4j-cli aura project list](#neo4j-cli-aura-project-list)
- [neo4j-cli aura workspace](#neo4j-cli-aura-workspace)
- [neo4j-cli aura workspace list](#neo4j-cli-aura-workspace-list)
- [neo4j-cli aura workspace use](#neo4j-cli-aura-workspace-use)

Allows you to programmatically provision and manage your Aura resources

Allows you to programmatically provision and manage your Aura resources. Write operations require --rw.

Usage: `neo4j-cli aura`

## neo4j-cli aura agent

Relates to Aura Agents

Usage: `neo4j-cli aura agent`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |

### neo4j-cli aura agent create

Creates a new agent

Creates a new agent for the specified project.

Usage: `neo4j-cli aura agent create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dbid` | string | - | (required) Aura database instance ID the agent connects to |
| `--description` | string | - | (required) Agent description |
| `--enabled` | bool | true | Whether the agent is enabled |
| `--is-mcp-enabled` | bool | false | Whether MCP is enabled for the agent |
| `--is-private` | bool | false | Whether the agent is private |
| `--name` | string | - | (required) Agent name |
| `--organization-id` | string | - | (required) Organization ID |
| `--project-id` | string | - | (required) Project/tenant ID |
| `--system-prompt` | string | - | Optional system prompt for the agent |
| `--tools` | string | - | (required) Tools configuration as a JSON array |

Examples:

```
# Create an agent in the default project
neo4j-cli aura agent create --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools "[]" --rw

# Create an agent with a system prompt
neo4j-cli aura agent create --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools "[]" --system-prompt "you are helpful" --rw

# Create an agent and emit the response as JSON
neo4j-cli aura agent create --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools "[]" --rw --format json
```

### neo4j-cli aura agent delete

Deletes an agent

Deletes an agent by its ID.

Usage: `neo4j-cli aura agent delete <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--organization-id` | string | - | (required) Organization ID |
| `--project-id` | string | - | (required) Project/tenant ID |

Examples:

```
# Delete an agent by ID
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --rw

# Delete an agent in a specific organization and project
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000 --rw

# Delete an agent and emit the response as JSON
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --rw --format json
```

### neo4j-cli aura agent get

Returns agent details

Returns the details of a specific agent.

Usage: `neo4j-cli aura agent get <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--organization-id` | string | - | (required) Organization ID |
| `--project-id` | string | - | (required) Project/tenant ID |

Examples:

```
# Get details for an agent
neo4j-cli aura agent get 00000000-0000-0000-0000-000000000000

# Get an agent in a specific organization and project
neo4j-cli aura agent get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000

# Get agent details as JSON for scripting
neo4j-cli aura agent get 00000000-0000-0000-0000-000000000000 --format json
```

### neo4j-cli aura agent invoke

Invoke an agent with an input prompt

Invokes an agent with the provided input string. Use --format json for the full response including content blocks.

Usage: `neo4j-cli aura agent invoke <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--input` | string | - | (required) Input message to send to the agent |
| `--organization-id` | string | - | (required) Organization ID |
| `--project-id` | string | - | (required) Project/tenant ID |

Examples:

```
# Invoke an agent with a prompt
neo4j-cli aura agent invoke 00000000-0000-0000-0000-000000000000 --input "hello" --rw

# Invoke an agent in a specific organization and project
neo4j-cli aura agent invoke 00000000-0000-0000-0000-000000000000 --input "hello" --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000 --rw

# Invoke an agent and emit the response as JSON
neo4j-cli aura agent invoke 00000000-0000-0000-0000-000000000000 --input "hello" --rw --format json
```

### neo4j-cli aura agent list

Returns a list of agents

Returns a list of agents for the specified project.

Usage: `neo4j-cli aura agent list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--organization-id` | string | - | (required) Organization ID |
| `--project-id` | string | - | (required) Project/tenant ID |

Examples:

```
# List all agents in the default project
neo4j-cli aura agent list

# List agents in a specific organization and project
neo4j-cli aura agent list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000

# List agents as JSON for scripting
neo4j-cli aura agent list --format json
```

### neo4j-cli aura agent replace

Fully replaces an existing agent

Fully replaces an existing agent's configuration. All fields are required (PUT semantics).

Usage: `neo4j-cli aura agent replace <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dbid` | string | - | (required) Aura database instance ID the agent connects to |
| `--description` | string | - | (required) Agent description |
| `--enabled` | bool | true | Whether the agent is enabled |
| `--is-mcp-enabled` | bool | false | Whether MCP is enabled for the agent |
| `--is-private` | bool | false | Whether the agent is private |
| `--name` | string | - | (required) Agent name |
| `--organization-id` | string | - | (required) Organization ID |
| `--project-id` | string | - | (required) Project/tenant ID |
| `--system-prompt` | string | - | System prompt for the agent |
| `--tools` | string | - | (required) Tools configuration as a JSON array |

Examples:

```
# Replace an agent's full definition
neo4j-cli aura agent replace 00000000-0000-0000-0000-000000000000 --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools "[]" --rw

# Replace an agent with a system prompt
neo4j-cli aura agent replace 00000000-0000-0000-0000-000000000000 --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools "[]" --system-prompt "you are helpful" --rw

# Replace an agent and emit the response as JSON
neo4j-cli aura agent replace 00000000-0000-0000-0000-000000000000 --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools "[]" --rw --format json
```

### neo4j-cli aura agent update

Partially updates an existing agent

Partially updates an existing agent's configuration. Only provided fields are updated (PATCH semantics).

Usage: `neo4j-cli aura agent update <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dbid` | string | - | Aura database instance ID the agent connects to |
| `--description` | string | - | Agent description |
| `--enabled` | bool | true | Whether the agent is enabled |
| `--is-mcp-enabled` | bool | false | Whether MCP is enabled for the agent |
| `--is-private` | bool | false | Whether the agent is private |
| `--name` | string | - | Agent name |
| `--organization-id` | string | - | (required) Organization ID |
| `--project-id` | string | - | (required) Project/tenant ID |
| `--system-prompt` | string | - | System prompt for the agent |
| `--tools` | string | - | Tools configuration as a JSON array |

Examples:

```
# Rename an agent
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --name my-renamed-agent --rw

# Disable an agent
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --enabled=false --rw

# Update an agent and emit the response as JSON
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --description "updated" --rw --format json
```

## neo4j-cli aura customer-managed-key

Relates to Customer Managed Keys

Usage: `neo4j-cli aura customer-managed-key`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |
| `--organization-id` | string | - | ID of the Aura organization |
| `--project-id` | string | - | ID of the Aura project |

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
| `--cloud-provider` | cloud-provider | - | (required) The cloud provider hosting the instance. |
| `--key-id` | string | - | (required) Encryption Key ARN |
| `--name` | string | - | (required) The name of the customer managed key (any UTF-8 characters with no trailing or leading whitespace). |
| `--region` | string | - | (required) The region where the instance is hosted. |
| `--type` | type | - | (required) The type of the instance. |
| `--wait` | bool | false | Waits until created customer managed key is ready. |

Examples:

```
# Create a customer managed key (AWS-hosted instance)
neo4j-cli aura customer-managed-key create --name my-key --region us-east-1 --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-east-1:000000000000:key/00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Create a key and wait until it is ready before returning
neo4j-cli aura customer-managed-key create --name my-key --region us-east-1 --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-east-1:000000000000:key/00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --wait --rw

# Create a key and emit JSON for scripting
neo4j-cli aura customer-managed-key create --name my-key --region us-east-1 --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-east-1:000000000000:key/00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json
```

### neo4j-cli aura customer-managed-key delete

Deletes a customer managed key

Deletes a Customer Managed Key from Aura.

Note that you can only delete a Key if it is not being used by any instances, otherwise you will get an error with the reason field set to encryption-key-is-active.

Usage: `neo4j-cli aura customer-managed-key delete <id>`

Examples:

```
# Delete a customer managed key by ID
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Delete a key and emit JSON for scripting
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json

# Delete and confirm by piping the response through jq
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json | jq -r '.data.deleted'
```

### neo4j-cli aura customer-managed-key get

Returns a customer managed key details

This subcommand returns details about a specific Customer Managed Key.

Usage: `neo4j-cli aura customer-managed-key get <id>`

Examples:

```
# Get details of a customer managed key by ID
neo4j-cli aura customer-managed-key get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get details and emit JSON for scripting
neo4j-cli aura customer-managed-key get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json

# Pipe details through jq to extract the key status
neo4j-cli aura customer-managed-key get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json | jq -r '.data.status'
```

### neo4j-cli aura customer-managed-key list

Returns a list of customer managed keys

This subcommand returns a list containing a summary of each of your customer managed keys in the specified project. To find out more about a specific key, retrieve the details using the get subcommand.

Use --organization-id and --project-id to specify which project's keys to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.

Usage: `neo4j-cli aura customer-managed-key list`

Examples:

```
# List all customer managed keys in a project
neo4j-cli aura customer-managed-key list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List keys using a configured default workspace
neo4j-cli aura customer-managed-key list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura customer-managed-key list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json
```

## neo4j-cli aura graph-analytics

Relates to Aura Graph Analytics

Usage: `neo4j-cli aura graph-analytics`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |

### neo4j-cli aura graph-analytics session

Relates to Aura Graph Analytics

Usage: `neo4j-cli aura graph-analytics session`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `--organization-id` | string | - | ID of the Aura organization |
| `--project-id` | string | - | ID of the Aura project |

#### neo4j-cli aura graph-analytics session create

Creates a new Aura Graph Analytics Serverless session

This subcommand gets or creates a Aura Graph Analytics Serverless session. If no Session with a matching name and project is found, one will be created. A Session is either attached to an AuraDB, or standalone.
Creating a session is an asynchronous operation that can be waited for with --wait.

Usage: `neo4j-cli aura graph-analytics session create [flags]`

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

#### neo4j-cli aura graph-analytics session delete

Delete a Graph Analytics Serverless session

This subcommand deletes a Graph Analytics Serverless session by id.

Usage: `neo4j-cli aura graph-analytics session delete <id>`

Examples:

```
# Delete a session by ID
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Delete a session and emit JSON for scripting
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json

# Delete a session, suppressing all stdout output
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw > /dev/null
```

#### neo4j-cli aura graph-analytics session get

Get a Graph Analytics Serverless session

This subcommand returns the details of a Graph Analytics Serverless session.

Usage: `neo4j-cli aura graph-analytics session get <id>`

Examples:

```
# Get a session by ID
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Render the session as a TOON table
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format toon

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json
```

#### neo4j-cli aura graph-analytics session list

Returns a list of Graph Analytics Serverless sessions

This subcommand returns a list containing a summary of each of your Graph Analytics Serverless sessions in the specified project.

Use --organization-id and --project-id to specify which project's sessions to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.

Usage: `neo4j-cli aura graph-analytics session list [flags]`

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

## neo4j-cli aura instance

Relates to AuraDB or AuraDS instances

Usage: `neo4j-cli aura instance`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |
| `--organization-id` | string | - | ID of the Aura organization |
| `--project-id` | string | - | ID of the Aura project |

### neo4j-cli aura instance create

Creates a new instance

This subcommand starts the creation process of an Aura instance.

Region identifiers follow each cloud provider's own naming convention: AWS uses identifiers such as us-east-1, Azure uses identifiers such as eastus, and GCP uses identifiers such as us-central1.

If you're unsure of possible configurations, run 'tenant get' to discover the full list of supported configurations for your tenant. The output lists every valid combination of --cloud-provider, --region, --type, and --memory.

Creating an instance is an asynchronous operation that can be waited for with --wait. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand. Once the status transitions from "creating" to "running" you may begin to use your instance.

This subcommand returns your instance ID, initial credentials, connection URL along with your project id, cloud provider, region, instance type, and the instance name for you to use once the instance is running. It is important to store these initial credentials until you have the chance to login to your running instance and change them.

For Enterprise instances you can specify a --customer-managed-key-id flag to use a Customer Managed Key for encryption.

Usage: `neo4j-cli aura instance create [flags]`

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

### neo4j-cli aura instance delete

Deletes an instance

Starts the deletion process of an Aura instance.

Deleting an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

If another operation is being performed on the instance you are trying to delete, an error will be returned that indicates that deletion cannot be performed.

Usage: `neo4j-cli aura instance delete <id>`

Examples:

```
# Delete an instance by ID
neo4j-cli aura instance delete 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Delete an instance and emit the response as JSON
neo4j-cli aura instance delete 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json

# Delete and pipe the response status through jq
neo4j-cli aura instance delete 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json | jq -r '.data.status'
```

### neo4j-cli aura instance get

Returns instance details

This endpoint returns details about a specific Aura Instance.

Usage: `neo4j-cli aura instance get <id>`

Examples:

```
# Get details of an instance by ID
neo4j-cli aura instance get 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get details and emit JSON for scripting
neo4j-cli aura instance get 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json

# Pipe details through jq to extract the connection URL
neo4j-cli aura instance get 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json | jq -r '.data.connection_url'
```

### neo4j-cli aura instance list

Returns a list of instances

This subcommand returns a list containing a summary of each of your Aura instances in the specified project. To find out more about a specific instance, retrieve the details using the get subcommand.

Use --organization-id and --project-id to specify which project's instances to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.

Usage: `neo4j-cli aura instance list`

Examples:

```
# List instances in a project (using flags)
neo4j-cli aura instance list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List instances using a configured default workspace
neo4j-cli aura instance list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura instance list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json
```

### neo4j-cli aura instance overwrite

Starts the process of overwriting the specified instance with data from the source instance provided

Starts the process of overwriting the specified instance with data from the source instance provided.

The overwrite process mimics the 'Clone to existing' functionality of the Aura Console.

If only --source-instance-id is provided, a new snapshot of that instance is created and used for overwriting. Alternatively, you can specify an additional --source-snapshot-id to use a specific snapshot for overwriting, from --source-instance-id provided, otherwise as a snapshot of the instance being overwritten. The snapshot specified must be exportable.

Usage: `neo4j-cli aura instance overwrite <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--source-instance-id` | string | - | The ID of the instance to overwrite with, from the source snapshot ID if provided, otherwise takes a new snapshot and overwrites |
| `--source-snapshot-id` | string | - | The ID of the snapshot to overwrite with, which must be exportable, from the source instance ID if provided, otherwise the argument provided instance |
| `--wait` | bool | false | Waits until created snapshot is ready |

Examples:

```
# Overwrite an instance with a fresh snapshot of a source instance
neo4j-cli aura instance overwrite 00000000 --source-instance-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Overwrite using a specific exportable snapshot and wait until ready
neo4j-cli aura instance overwrite 00000000 --source-instance-id 11111111 --source-snapshot-id 22222222-2222-2222-2222-222222222222 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --wait --rw

# Overwrite and emit JSON for scripting
neo4j-cli aura instance overwrite 00000000 --source-instance-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json
```

### neo4j-cli aura instance pause

Pauses an instance

Starts the pause process of an Aura instance.

Pausing an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

The pause time depends on the amount of data stored in the instance; larger quantities of data will take longer. The exact time this will take is dependent on the size of your data store.

If another operation is being performed on the instance you are trying to pause, an error will be returned that indicates that the pause operation cannot be performed.

Usage: `neo4j-cli aura instance pause <id>`

Examples:

```
# Pause an Aura instance
neo4j-cli aura instance pause 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Pause an instance and emit the response as JSON
neo4j-cli aura instance pause 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json

# Pause and pipe the response status through jq
neo4j-cli aura instance pause 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json | jq -r '.data.status'
```

### neo4j-cli aura instance resume

Resumes an instance

Starts the resume process of an Aura instance.

Resuming an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

If another operation is being performed on the instance you are trying to resume, an error will be returned that indicates that resume cannot be performed.

Usage: `neo4j-cli aura instance resume <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--wait` | bool | false | Waits until resumed instance is ready. |

Examples:

```
# Resume a paused Aura instance
neo4j-cli aura instance resume 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Resume and wait until the instance is ready
neo4j-cli aura instance resume 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --wait --rw

# Resume and emit the response as JSON for scripting
neo4j-cli aura instance resume 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json
```

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
| `--instance-id` | string | - | (required) The ID of the instance to create a snapshot of |
| `--wait` | bool | false | Waits until created snapshot is ready. |

Examples:

```
# Take an on-demand snapshot of an instance
neo4j-cli aura instance snapshot create --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Take a snapshot and wait until it is ready
neo4j-cli aura instance snapshot create --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --wait --rw

# Take a snapshot and emit JSON for scripting
neo4j-cli aura instance snapshot create --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json
```

#### neo4j-cli aura instance snapshot get

Get details of a snapshot

This endpoint returns details about a specific snapshot.

Usage: `neo4j-cli aura instance snapshot get <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | The ID of the instance to get the snapshot details of |

Examples:

```
# Get details of a snapshot by its ID
neo4j-cli aura instance snapshot get 22222222-2222-2222-2222-222222222222 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get details and emit JSON for scripting
neo4j-cli aura instance snapshot get 22222222-2222-2222-2222-222222222222 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json

# Check whether a snapshot is exportable via jq
neo4j-cli aura instance snapshot get 22222222-2222-2222-2222-222222222222 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json | jq -r '.data.exportable'
```

#### neo4j-cli aura instance snapshot list

Returns a list of snapshots

This subcommand returns a list of available snapshots from the current day.

Usage: `neo4j-cli aura instance snapshot list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--date` | string | - | An optional date to list snapshots for a given day, defaults to today. Must be formatted with an ISO formatted date string (YYYY-MM-DD) |
| `--instance-id` | string | - | The ID of the instance to list the snapshots of |

Examples:

```
# List today's snapshots for an instance
neo4j-cli aura instance snapshot list --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List snapshots for a specific date
neo4j-cli aura instance snapshot list --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --date 2025-01-15

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura instance snapshot list --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json
```

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

Examples:

```
# Rename an Aura instance
neo4j-cli aura instance update 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --name my-renamed-instance --rw

# Resize an Aura instance to 8GB of memory
neo4j-cli aura instance update 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --memory 8GB --rw

# Rename and resize, emitting JSON for scripting
neo4j-cli aura instance update 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --name my-renamed-instance --memory 8GB --rw --format json
```

## neo4j-cli aura organization

Manage Aura organizations

Usage: `neo4j-cli aura organization`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |

### neo4j-cli aura organization get

Returns organization details

This subcommand returns details about a specific Aura organization.

Usage: `neo4j-cli aura organization get <id>`

Examples:

```
# Get details of an organization by ID
neo4j-cli aura organization get 00000000-0000-0000-0000-000000000000

# Emit JSON for scripting
neo4j-cli aura organization get 00000000-0000-0000-0000-000000000000 --format json

# Pipe details through jq to extract the organization name
neo4j-cli aura organization get 00000000-0000-0000-0000-000000000000 --format json | jq -r '.data.name'
```

### neo4j-cli aura organization list

Returns a list of organizations

This subcommand returns a list of Aura organizations accessible to the current user.

Usage: `neo4j-cli aura organization list`

Examples:

```
# List all organizations the current user has access to
neo4j-cli aura organization list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura organization list --format json

# Pipe organization ids through jq for a follow-up command
neo4j-cli aura organization list --format json | jq -r '.data[].id'
```

## neo4j-cli aura project

Manage Aura projects

Usage: `neo4j-cli aura project`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |

### neo4j-cli aura project get

Returns project details

This subcommand returns details about a specific Aura project.

Usage: `neo4j-cli aura project get <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--organization-id` | string | - | Organization ID (defaults to org portion of aura.default-workspace) |

Examples:

```
# Get project details by ID (uses org from aura.default-workspace)
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000

# Get project details in a specific organization
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000 --organization-id 11111111-1111-1111-1111-111111111111

# Emit JSON for scripting
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000 --format json
```

### neo4j-cli aura project list

Returns a list of projects

This subcommand returns a list of Aura projects within the given organization.

Usage: `neo4j-cli aura project list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--organization-id` | string | - | Organization ID (defaults to org portion of aura.default-workspace) |

Examples:

```
# List all projects in the default organization (from aura.default-workspace)
neo4j-cli aura project list

# List projects in a specific organization
neo4j-cli aura project list --organization-id 00000000-0000-0000-0000-000000000000

# Emit JSON for scripting
neo4j-cli aura project list --organization-id 00000000-0000-0000-0000-000000000000 --format json
```

## neo4j-cli aura workspace

Manage the active organization and project workspace

Usage: `neo4j-cli aura workspace`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |

### neo4j-cli aura workspace list

Returns a flat list of all accessible organization/project workspaces

This subcommand lists all organization/project pairs accessible to the current user.
Each entry includes the workspace slug ({organizationId}/{projectId}), the organization and
project IDs and names, and whether this entry is the currently active default workspace.

Usage: `neo4j-cli aura workspace list`

Examples:

```
# List all accessible workspaces in table format
neo4j-cli aura workspace list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura workspace list --format json

# Find the active workspace via jq
neo4j-cli aura workspace list --format json | jq -r '.data[] | select(.default == true) | .workspace'
```

### neo4j-cli aura workspace use

Sets the active organization and project workspace

This subcommand sets the active organization and project workspace used by default
in subsequent commands. Accepts either a positional {organizationId}/{projectId} slug
or the --organization-id and --project-id flags (but not both).

The workspace is validated against the Aura API before being persisted.

Usage: `neo4j-cli aura workspace use [organizationId/projectId] [flags]`

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

