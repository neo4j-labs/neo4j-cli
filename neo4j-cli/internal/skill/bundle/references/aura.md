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
- [neo4j-cli aura graphql](#neo4j-cli-aura-graphql)
- [neo4j-cli aura graphql auth-provider](#neo4j-cli-aura-graphql-auth-provider)
- [neo4j-cli aura graphql auth-provider create](#neo4j-cli-aura-graphql-auth-provider-create)
- [neo4j-cli aura graphql auth-provider delete](#neo4j-cli-aura-graphql-auth-provider-delete)
- [neo4j-cli aura graphql auth-provider get](#neo4j-cli-aura-graphql-auth-provider-get)
- [neo4j-cli aura graphql auth-provider list](#neo4j-cli-aura-graphql-auth-provider-list)
- [neo4j-cli aura graphql cors-policy](#neo4j-cli-aura-graphql-cors-policy)
- [neo4j-cli aura graphql cors-policy allowed-origin](#neo4j-cli-aura-graphql-cors-policy-allowed-origin)
- [neo4j-cli aura graphql cors-policy allowed-origin add](#neo4j-cli-aura-graphql-cors-policy-allowed-origin-add)
- [neo4j-cli aura graphql cors-policy allowed-origin remove](#neo4j-cli-aura-graphql-cors-policy-allowed-origin-remove)
- [neo4j-cli aura graphql create](#neo4j-cli-aura-graphql-create)
- [neo4j-cli aura graphql delete](#neo4j-cli-aura-graphql-delete)
- [neo4j-cli aura graphql get](#neo4j-cli-aura-graphql-get)
- [neo4j-cli aura graphql list](#neo4j-cli-aura-graphql-list)
- [neo4j-cli aura graphql pause](#neo4j-cli-aura-graphql-pause)
- [neo4j-cli aura graphql resume](#neo4j-cli-aura-graphql-resume)
- [neo4j-cli aura graphql update](#neo4j-cli-aura-graphql-update)
- [neo4j-cli aura instance](#neo4j-cli-aura-instance)
- [neo4j-cli aura instance create](#neo4j-cli-aura-instance-create)
- [neo4j-cli aura instance delete](#neo4j-cli-aura-instance-delete)
- [neo4j-cli aura instance deploy](#neo4j-cli-aura-instance-deploy)
- [neo4j-cli aura instance get](#neo4j-cli-aura-instance-get)
- [neo4j-cli aura instance list](#neo4j-cli-aura-instance-list)
- [neo4j-cli aura instance load](#neo4j-cli-aura-instance-load)
- [neo4j-cli aura instance overwrite](#neo4j-cli-aura-instance-overwrite)
- [neo4j-cli aura instance pause](#neo4j-cli-aura-instance-pause)
- [neo4j-cli aura instance resume](#neo4j-cli-aura-instance-resume)
- [neo4j-cli aura instance snapshot](#neo4j-cli-aura-instance-snapshot)
- [neo4j-cli aura instance snapshot create](#neo4j-cli-aura-instance-snapshot-create)
- [neo4j-cli aura instance snapshot get](#neo4j-cli-aura-instance-snapshot-get)
- [neo4j-cli aura instance snapshot list](#neo4j-cli-aura-instance-snapshot-list)
- [neo4j-cli aura instance update](#neo4j-cli-aura-instance-update)
- [neo4j-cli aura login](#neo4j-cli-aura-login)
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

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--debug` | bool | false | Route Aura API activity (HTTP request/response wire, token acquisition, polling) to stderr; stdout is unaffected. Output may include the (best-effort-redacted) request/response bodies [env: NEO4J_DEBUG (set to 1 to enable)] |

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
| `--organization-id` | string | - | Organization ID |
| `--project-id` | string | - | Project ID |
| `--system-prompt` | string | - | Optional system prompt for the agent |
| `--tools` | string | - | (required) Tools configuration as a JSON array |

Examples:

```
# Create an agent with a text2cypher tool
neo4j-cli aura agent create --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --rw

# Create an agent with a system prompt
neo4j-cli aura agent create --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --system-prompt "you are helpful" --rw

# Create an agent and emit the response as JSON
neo4j-cli aura agent create --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --rw --format json
```

### neo4j-cli aura agent delete

Deletes an agent

Deletes an agent by its ID.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli aura agent delete <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--organization-id` | string | - | Organization ID |
| `--project-id` | string | - | Project ID |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Delete an agent by ID
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --rw --yes --force

# Delete an agent in a specific organization and project
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000 --rw --yes --force

# Delete an agent and emit the response as JSON
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --rw --yes --force --format json
```

### neo4j-cli aura agent get

Returns agent details

Returns the details of a specific agent.

Usage: `neo4j-cli aura agent get <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--organization-id` | string | - | Organization ID |
| `--project-id` | string | - | Project ID |

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
| `--organization-id` | string | - | Organization ID |
| `--project-id` | string | - | Project ID |

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
| `--organization-id` | string | - | Organization ID |
| `--project-id` | string | - | Project ID |

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
| `--organization-id` | string | - | Organization ID |
| `--project-id` | string | - | Project ID |
| `--system-prompt` | string | - | System prompt for the agent |
| `--tools` | string | - | (required) Tools configuration as a JSON array |

Examples:

```
# Replace an agent's full definition with a text2cypher tool
neo4j-cli aura agent replace 00000000-0000-0000-0000-000000000000 --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --rw

# Replace an agent with a system prompt
neo4j-cli aura agent replace 00000000-0000-0000-0000-000000000000 --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --system-prompt "you are helpful" --rw

# Replace an agent and emit the response as JSON
neo4j-cli aura agent replace 00000000-0000-0000-0000-000000000000 --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --rw --format json
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
| `--organization-id` | string | - | Organization ID |
| `--project-id` | string | - | Project ID |
| `--system-prompt` | string | - | System prompt for the agent |
| `--tools` | string | - | Tools configuration as a JSON array |

Examples:

```
# Rename an agent
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --name my-renamed-agent --rw

# Disable an agent
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --enabled=false --rw

# Update an agent's tools with a text2cypher tool
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --rw

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

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli aura customer-managed-key delete <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Delete a customer managed key by ID
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Delete a key and emit JSON for scripting
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json

# Delete and confirm by piping the response through jq
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json | jq -r '.data.deleted'
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

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli aura graph-analytics session delete <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Delete a session by ID
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Delete a session and emit JSON for scripting
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json

# Delete a session, suppressing all stdout output
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force > /dev/null
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

## neo4j-cli aura graphql

Allows you to programmatically provision and manage your GraphQL Data APIs

Usage: `neo4j-cli aura graphql`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |
| `--organization-id` | string | - | ID of the Aura organization |
| `--project-id` | string | - | ID of the Aura project |

### neo4j-cli aura graphql auth-provider

Allows you to programmatically manage Authentication providers for a specific GraphQL Data API

Usage: `neo4j-cli aura graphql auth-provider`

#### neo4j-cli aura graphql auth-provider create

Creates a new GraphQL Data API authentication provider

This command creates a new GraphQL Data API authentication provider.

Creating a GraphQL Data API authentication provider is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be ready. Once the status transitions from "updating" to "ready" you may begin to use your GraphQL Data API.

If you create an 'api-key' Authentication provider, an API key will be created. It is important to store the API key as it is not currently possible to get it or update it.

If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.

Usage: `neo4j-cli aura graphql auth-provider create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data-api-id` | string | - | (required) The ID of the GraphQL Data API to create the authentication provider for |
| `--disabled` | bool | false | Whether or not the Authentication provider is disabled |
| `--instance-id` | string | - | (required) The ID of the instance to create the GraphQL Data API for |
| `--name` | string | - | (required) The name of the Authentication provider |
| `--type` | type | - | (required) The type of the Authentication provider, one of 'api-key' or 'jwks' |
| `--url` | string | - | The JWKS URL that you want the bearer tokens in incoming GraphQL requests to be validated against. NOTE: only applicable for Authentication provider type 'jwks' |
| `--wait` | bool | false | Waits until created Authentication provider is ready. |

Examples:

```
# Create an api-key authentication provider (using flags)
neo4j-cli aura graphql auth-provider create --instance-id 00000000 --data-api-id 11111111 --type api-key --name my-api-key --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Create an api-key authentication provider using a configured default workspace
neo4j-cli aura graphql auth-provider create --instance-id 00000000 --data-api-id 11111111 --type api-key --name my-api-key --rw

# Create a JWKS authentication provider with a validation URL
neo4j-cli aura graphql auth-provider create --instance-id 00000000 --data-api-id 11111111 --type jwks --name my-jwks --url https://example.com/.well-known/jwks.json --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw
```

#### neo4j-cli aura graphql auth-provider delete

Delete a GraphQL Data API authentication provider

Deletes a GraphQL Data API authentication provider. This action can not be undone.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli aura graphql auth-provider delete <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data-api-id` | string | - | (required) The ID of the GraphQL Data API to delete the Authentication provider for |
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--instance-id` | string | - | (required) The ID of the instance to delete the Data API for |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Delete an authentication provider (using flags)
neo4j-cli aura graphql auth-provider delete 22222222 --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Delete an authentication provider using a configured default workspace
neo4j-cli aura graphql auth-provider delete 22222222 --instance-id 00000000 --data-api-id 11111111 --rw --yes --force

# Delete an authentication provider and capture the response as JSON
neo4j-cli aura graphql auth-provider delete 22222222 --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json
```

#### neo4j-cli aura graphql auth-provider get

Get details of a GraphQL Data API authentication provider

This endpoint returns details of a specific GraphQL Data API authentication provider.

Usage: `neo4j-cli aura graphql auth-provider get <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data-api-id` | string | - | (required) The ID of the GraphQL Data API to get the authentication provider of |
| `--instance-id` | string | - | (required) The ID of the instance the GraphQL Data API is connected to |

Examples:

```
# Get details of an authentication provider (using flags)
neo4j-cli aura graphql auth-provider get 22222222 --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get details of an authentication provider using a configured default workspace
neo4j-cli aura graphql auth-provider get 22222222 --instance-id 00000000 --data-api-id 11111111

# Get details of an authentication provider as JSON
neo4j-cli aura graphql auth-provider get 22222222 --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json
```

#### neo4j-cli aura graphql auth-provider list

Returns a list of authentication providers of a specific GraphQL Data API

Usage: `neo4j-cli aura graphql auth-provider list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data-api-id` | string | - | (required) The ID of the GraphQL Data API to list the authentication providers of |
| `--instance-id` | string | - | (required) The ID of the instance the GraphQL Data API is connected to |

Examples:

```
# List authentication providers of a GraphQL Data API (using flags)
neo4j-cli aura graphql auth-provider list --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List authentication providers using a configured default workspace
neo4j-cli aura graphql auth-provider list --instance-id 00000000 --data-api-id 11111111

# List authentication providers as JSON for scripting
neo4j-cli aura graphql auth-provider list --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json
```

### neo4j-cli aura graphql cors-policy

Allows you to manage the Cross-Origin Resource Sharing (CORS) policy for a specific GraphQL Data API

Usage: `neo4j-cli aura graphql cors-policy`

#### neo4j-cli aura graphql cors-policy allowed-origin

Allows you to manage Cross-Origin Resource Sharing (CORS) allowed origins for a specific GraphQL Data API

Usage: `neo4j-cli aura graphql cors-policy allowed-origin`

##### neo4j-cli aura graphql cors-policy allowed-origin add

Adds a new allowed origin to the CORS policy

This command adds a new allowed origin to the Cross-Origin Resource Sharing (CORS) policy of a GraphQL Data API.

Updating the CORS policy of a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be ready. Once the status transitions from "updating" to "ready" you may begin to use your GraphQL Data API.

Adding a new allowed origin to the CORS policy of a GraphQL Data API allows browsers to make requests to the GraphQL Data API from a web app that is served from the specified origin.

Usage: `neo4j-cli aura graphql cors-policy allowed-origin add <origin> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data-api-id` | string | - | (required) The ID of the GraphQL Data API to add the CORS allowed origin for |
| `--instance-id` | string | - | (required) The ID of the instance the GraphQL Data API is connected to |
| `--wait` | bool | false | Waits until updated GraphQL Data API is ready. |

Examples:

```
# Add an allowed origin to the CORS policy (using flags)
neo4j-cli aura graphql cors-policy allowed-origin add https://app.example.com --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Add an allowed origin using a configured default workspace
neo4j-cli aura graphql cors-policy allowed-origin add https://app.example.com --instance-id 00000000 --data-api-id 11111111 --rw

# Add an allowed origin and wait until the GraphQL Data API is ready
neo4j-cli aura graphql cors-policy allowed-origin add https://app.example.com --instance-id 00000000 --data-api-id 11111111 --wait --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw
```

##### neo4j-cli aura graphql cors-policy allowed-origin remove

Removes an allowed origin from the CORS policy

This command removes an allowed origin from the Cross-Origin Resource Sharing (CORS) policy of a GraphQL Data API.

Updating the CORS policy of a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be ready. Once the status transitions from "updating" to "ready" you may begin to use your GraphQL Data API.

Removing an allowed origin from the CORS policy of a GraphQL Data API means that most browsers are no longer able to make requests to the GraphQL Data API from a web app that is served from the specified origin.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli aura graphql cors-policy allowed-origin remove <origin> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data-api-id` | string | - | (required) The ID of the GraphQL Data API to remove the CORS allowed origin for |
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--instance-id` | string | - | (required) The ID of the instance the GraphQL Data API is connected to |
| `--wait` | bool | false | Waits until updated GraphQL Data API is ready. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Remove an allowed origin from the CORS policy (using flags)
neo4j-cli aura graphql cors-policy allowed-origin remove https://app.example.com --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Remove an allowed origin using a configured default workspace
neo4j-cli aura graphql cors-policy allowed-origin remove https://app.example.com --instance-id 00000000 --data-api-id 11111111 --rw --yes --force

# Remove an allowed origin and wait until the GraphQL Data API is ready
neo4j-cli aura graphql cors-policy allowed-origin remove https://app.example.com --instance-id 00000000 --data-api-id 11111111 --wait --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force
```

### neo4j-cli aura graphql create

Creates a new GraphQL Data API

This command starts the creation process of a GraphQL Data API.

Creating a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be ready. Once the status transitions from "creating" to "ready" you may begin to use your GraphQL Data API.

This command returns your GraphQL Data API ID, API key, and connection URL for you to use once the GraphQL Data API is running. It is important to store the API key as it is not currently possible to get this or update it.

If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.

Usage: `neo4j-cli aura graphql create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | (required) The ID of the instance to create the GraphQL Data API for |
| `--memory` | string | - | (required) Memory allocated to the GraphQL Data API, must be one of: 256MB, 512MB, 1024MB, 2048MB, 4096MB |
| `--name` | string | - | The name of the GraphQL Data API (auto-generated if not specified) |
| `--service-account` | string | read_write | The service account type for the instance connection, must be one of: read_only, read_write |
| `--type-definitions` | string | - | The GraphQL type definitions, NOTE: must be base64 encoded |
| `--type-definitions-file` | string | - | Path to a local GraphQL type definitions file, e.g. path/to/typeDefs.graphql. Must be of file type .graphql |
| `--wait` | bool | false | Waits until created GraphQL Data API is ready. |

Examples:

```
# Create a GraphQL Data API (using flags)
neo4j-cli aura graphql create --instance-id 00000000 --name my-api --memory 256MB --type-definitions dHlwZSBNb3ZpZSB7IHRpdGxlOiBTdHJpbmcgfQ== --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Create a GraphQL Data API using a configured default workspace (auto-generated name)
neo4j-cli aura graphql create --instance-id 00000000 --memory 256MB --type-definitions-file ./typeDefs.graphql --rw

# Create a GraphQL Data API from a local type definitions file
neo4j-cli aura graphql create --instance-id 00000000 --name my-api --memory 512MB --type-definitions-file ./typeDefs.graphql --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Create a GraphQL Data API and wait until it is ready
neo4j-cli aura graphql create --instance-id 00000000 --name my-api --memory 256MB --type-definitions-file ./typeDefs.graphql --wait --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw
```

### neo4j-cli aura graphql delete

Delete a GraphQL Data API

Deletes a GraphQL Data API. This action can not be undone.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli aura graphql delete <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--instance-id` | string | - | (required) The ID of the instance to delete the Data API for |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Delete a GraphQL Data API (using flags)
neo4j-cli aura graphql delete 11111111 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Delete a GraphQL Data API using a configured default workspace
neo4j-cli aura graphql delete 11111111 --instance-id 00000000 --rw --yes --force

# Delete a GraphQL Data API and capture the response as JSON
neo4j-cli aura graphql delete 11111111 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json
```

### neo4j-cli aura graphql get

Get details of a GraphQL Data API

This endpoint returns details of a specific GraphQL Data API.

Usage: `neo4j-cli aura graphql get <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | (required) The ID of the instance to get the GraphQL Data API details for |

Examples:

```
# Get details of a GraphQL Data API (using flags)
neo4j-cli aura graphql get 11111111 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get details of a GraphQL Data API using a configured default workspace
neo4j-cli aura graphql get 11111111 --instance-id 00000000

# Get details of a GraphQL Data API as JSON
neo4j-cli aura graphql get 11111111 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json
```

### neo4j-cli aura graphql list

Returns a list of GraphQL Data APIs

Usage: `neo4j-cli aura graphql list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | (required) The ID of the instance to list the GraphQL Data APIs of |

Examples:

```
# List GraphQL Data APIs of an instance (using flags)
neo4j-cli aura graphql list --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List GraphQL Data APIs using a configured default workspace
neo4j-cli aura graphql list --instance-id 00000000

# List GraphQL Data APIs as JSON for scripting
neo4j-cli aura graphql list --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json
```

### neo4j-cli aura graphql pause

Pause a GraphQL Data API

This command starts the pausing process of an existing GraphQL Data API.

Pausing a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be paused. The GraphQL Data API will only be paused once the status transitions from "pausing" to "paused".

Usage: `neo4j-cli aura graphql pause <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | (required) The ID of the instance to pause the Data API for |
| `--wait` | bool | false | Waits until GraphQL Data API is paused. |

Examples:

```
# Pause a GraphQL Data API (using flags)
neo4j-cli aura graphql pause 11111111 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Pause a GraphQL Data API using a configured default workspace
neo4j-cli aura graphql pause 11111111 --instance-id 00000000 --rw

# Pause a GraphQL Data API and wait until it is paused
neo4j-cli aura graphql pause 11111111 --instance-id 00000000 --wait --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw
```

### neo4j-cli aura graphql resume

Resume a GraphQL Data API

This command starts the resuming process of an existing GraphQL Data API.

Resuming a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be ready. Once the status transitions from "resuming" to "ready" you may begin to use your GraphQL Data API.

Usage: `neo4j-cli aura graphql resume <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | (required) The ID of the instance to resume the Data API for |
| `--wait` | bool | false | Waits until GraphQL Data API is resumed. |

Examples:

```
# Resume a paused GraphQL Data API (using flags)
neo4j-cli aura graphql resume 11111111 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Resume a GraphQL Data API using a configured default workspace
neo4j-cli aura graphql resume 11111111 --instance-id 00000000 --rw

# Resume a GraphQL Data API and wait until it is ready
neo4j-cli aura graphql resume 11111111 --instance-id 00000000 --wait --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw
```

### neo4j-cli aura graphql update

Edit a GraphQL Data API

This endpoint edits a specific GraphQL Data API.

Updating a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be ready again. Once the status transitions from "updating" to "ready" you may continue to use your GraphQL Data API.

Usage: `neo4j-cli aura graphql update <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--instance-id` | string | - | (required) The ID of the instance to update the Data API for |
| `--name` | string | - | The name of the GraphQL Data API |
| `--service-account` | string | - | The service account permission for the instance this GraphQL Data API will be connected to (read_only or read_write) |
| `--type-definitions` | string | - | The GraphQL type definitions, NOTE: must be base64 encoded |
| `--type-definitions-file` | string | - | Path to a local GraphQL type definitions file, e.g. path/to/typeDefs.graphql |
| `--wait` | bool | false | Waits until updated GraphQL Data API is ready again. |

Examples:

```
# Rename a GraphQL Data API (using flags)
neo4j-cli aura graphql update 11111111 --instance-id 00000000 --name renamed-api --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Rename a GraphQL Data API using a configured default workspace
neo4j-cli aura graphql update 11111111 --instance-id 00000000 --name renamed-api --rw

# Update the service account permission and wait for the API to be ready
neo4j-cli aura graphql update 11111111 --instance-id 00000000 --service-account read_only --wait --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw
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

If you're unsure of possible configurations, run 'project get' to discover the full list of supported configurations for your project. The output lists every valid combination of --cloud-provider, --region, --type, and --memory.

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
| `--region` | string | - | The region where the instance is hosted. Values follow each cloud provider's naming convention (e.g. us-east-1 for AWS, eastus for Azure, europe-west1 for GCP). Run 'project get' to see the full list of supported regions for your project. |
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

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

Usage: `neo4j-cli aura instance delete <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Delete an instance by ID
neo4j-cli aura instance delete 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Delete an instance and emit the response as JSON
neo4j-cli aura instance delete 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json

# Delete and pipe the response status through jq
neo4j-cli aura instance delete 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json | jq -r '.data.status'
```

### neo4j-cli aura instance deploy

Creates a new Aura instance and clones a local database into it

This subcommand creates a new Aura instance and clones a local Neo4j database into it.

The source database can come from a local Neo4j Docker container managed by 'neo4j-cli docker' (--from-docker) or from a DBMS managed by a local Neo4j Desktop 2 install (--from-desktop). Exactly one source must be specified.

deploy operates on Enterprise Neo4j sources only: Neo4j Desktop 2 manages only enterprise DBMSs, and the --from-docker path requires an enterprise container (the dump relies on the enterprise-only STOP DATABASE command).

A new Aura instance is provisioned using the same flags as 'instance create', then the named --database (default "neo4j") is dumped from the source and uploaded into the new instance, overwriting its contents. The "system" database cannot be cloned.

The command waits for the instance to be ready and for the data load to finish before returning. On success the structured output reports the instance connection details plus deploy_status=succeeded. If the data load fails after the instance was created, the instance is left in place (it is not deleted), deploy_status=failed is reported, and the instance id is printed so you can retry or delete it manually.

Usage: `neo4j-cli aura instance deploy [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--cloud-provider` | cloud-provider | - | The cloud provider hosting the instance. Must be one of "aws", "azure", or "gcp". |
| `--credential-name` | string | - | The name to use when storing the credentials locally. Defaults to <instance-id>-default. |
| `--database` | string | neo4j | The name of the source database to clone. The system database cannot be cloned. |
| `--desktop-port` | int | 0 | Pin the Neo4j Desktop 2 relate API to a specific port instead of probing 44222..44232 (used only with --from-desktop). |
| `--from-desktop` | string | - | ID of a DBMS managed by a local Neo4j Desktop 2 install to clone the database from. |
| `--from-docker` | string | - | Name of a local Neo4j Docker container (managed by `neo4j-cli docker`) to clone the database from. |
| `--memory` | memory | - | The size of the instance memory (e.g. 2GB, 8GB, 64GB). Run with an invalid value to see all accepted sizes. |
| `--name` | string | - | The name of the instance (any UTF-8 characters with no trailing or leading whitespace). If omitted, a default name is generated automatically (e.g. Instance01). |
| `--no-credential-print` | bool | false | Omit the password from the command output. |
| `--no-credential-storage` | bool | false | Skip storing the instance credentials locally after creation. |
| `--region` | string | - | The region where the instance is hosted. Values follow each cloud provider's naming convention (e.g. us-east-1 for AWS, eastus for Azure, europe-west1 for GCP). Run 'project get' to see the full list of supported regions for your project. |
| `--type` | type | - | (required) The type of the instance. Must be one of "free-db", "professional-db", "business-critical", "enterprise-db", "professional-ds", or "enterprise-ds". |
| `--vector-optimized` | bool | false | An optional vector optimization configuration to be set during instance creation |
| `--version` | string | 5 | The Neo4j version of the instance. |

Examples:

```
# Deploy a local Docker container's neo4j database into a new free-db Aura instance
neo4j-cli aura instance deploy --rw --from-docker my-local-neo4j --type free-db --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Deploy a Neo4j Desktop 2 DBMS database into a new professional-db instance on AWS
neo4j-cli aura instance deploy --rw --from-desktop dbms-1234 --database movies --type professional-db --cloud-provider aws --region us-east-1 --memory 2GB --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111
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

### neo4j-cli aura instance load

Creates a new Aura instance pre-loaded with an example dataset

This subcommand creates a new Aura instance and loads an example Neo4j dataset into it.

A dataset is a '.dump' published by a GitHub repo carrying a 'relate.project-install.json' manifest (e.g. 'neo4j-graph-examples/movies'). The manifest is resolved for the requested --version, the matching dump is downloaded from the Git-LFS media host, and the data is loaded into the --database (default "neo4j") of a new Aura instance provisioned with the same flags as 'instance create'.

Aura has no dump-upload API, so the dump is staged through an ephemeral local Neo4j Docker container and then uploaded into the new instance over Bolt. A local Docker daemon is therefore REQUIRED; the command errors early (before creating any Aura instance) if Docker is unavailable.

Datasets requiring the graph-data-science plugin cannot be loaded into Aura (GDS is not installable there); such a dataset is rejected before any work is done. The apoc plugin is allowed.

If the data load fails after the instance was created, the instance is left in place (it is not deleted), load_status=failed is reported, and the instance id is printed so you can retry or delete it manually.

Usage: `neo4j-cli aura instance load <owner/repo> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--cloud-provider` | cloud-provider | - | The cloud provider hosting the instance. Must be one of "aws", "azure", or "gcp". |
| `--credential-name` | string | - | The name to use when storing the credentials locally. Defaults to <instance-id>-default. |
| `--database` | string | neo4j | The target database the dataset is loaded into. The system database cannot be loaded into. |
| `--max-size` | int64 | 2147483648 | Maximum dump download size in bytes; the download is refused if exceeded. |
| `--memory` | memory | - | The size of the instance memory (e.g. 2GB, 8GB, 64GB). Run with an invalid value to see all accepted sizes. |
| `--name` | string | - | The name of the instance (any UTF-8 characters with no trailing or leading whitespace). If omitted, a default name is generated automatically (e.g. Instance01). |
| `--no-credential-print` | bool | false | Omit the password from the command output. |
| `--no-credential-storage` | bool | false | Skip storing the instance credentials locally after creation. |
| `--region` | string | - | The region where the instance is hosted. Values follow each cloud provider's naming convention (e.g. us-east-1 for AWS, eastus for Azure, europe-west1 for GCP). Run 'project get' to see the full list of supported regions for your project. |
| `--type` | type | - | (required) The type of the instance. Must be one of "free-db", "professional-db", "business-critical", "enterprise-db", "professional-ds", or "enterprise-ds". |
| `--vector-optimized` | bool | false | An optional vector optimization configuration to be set during instance creation |
| `--version` | string | 5 | The Neo4j version of the instance. Also used to resolve the dataset manifest. |

Examples:

```
# Load the movies dataset into a new free-db Aura instance
neo4j-cli aura instance load neo4j-graph-examples/movies --rw --name movies-demo --type free-db --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Load the recommendations dataset into a new professional-db instance on AWS and emit JSON
neo4j-cli aura instance load neo4j-graph-examples/recommendations --rw --name recs --type professional-db --cloud-provider aws --region us-east-1 --memory 2GB --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json
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

## neo4j-cli aura login

Authenticate with Aura using the device authorization flow

Authenticate with Aura using the OAuth 2.0 Device Authorization Grant (RFC 8628).
On success, the credential is stored under the name "login" and set as the default
when no other default is configured.

The following environment variables must be set before running:
  NEO4J_AURA_LOGIN_DEVICE_ENDPOINT  Device authorization endpoint URL
  NEO4J_AURA_LOGIN_TOKEN_ENDPOINT    Token endpoint URL
  NEO4J_AURA_LOGIN_CLIENT_ID         Public OAuth client ID
  NEO4J_AURA_LOGIN_AUDIENCE          OAuth audience

Usage: `neo4j-cli aura login`

Examples:

```
# Log in interactively; the command prints a URL to open in your browser
neo4j-cli aura login

# Source the example env file first, then log in
source .env.aura-login-spike && neo4j-cli aura login

# Log in after setting required environment variables in the current shell
export NEO4J_AURA_LOGIN_CLIENT_ID=my-client && neo4j-cli aura login
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

