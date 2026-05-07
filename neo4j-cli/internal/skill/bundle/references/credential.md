# neo4j-cli credential

## Contents

- [neo4j-cli credential aura-client](#neo4j-cli-credential-aura-client)
- [neo4j-cli credential aura-client add](#neo4j-cli-credential-aura-client-add)
- [neo4j-cli credential aura-client list](#neo4j-cli-credential-aura-client-list)
- [neo4j-cli credential aura-client remove](#neo4j-cli-credential-aura-client-remove)
- [neo4j-cli credential aura-client use](#neo4j-cli-credential-aura-client-use)
- [neo4j-cli credential dbms](#neo4j-cli-credential-dbms)
- [neo4j-cli credential dbms add](#neo4j-cli-credential-dbms-add)
- [neo4j-cli credential dbms list](#neo4j-cli-credential-dbms-list)
- [neo4j-cli credential dbms remove](#neo4j-cli-credential-dbms-remove)
- [neo4j-cli credential dbms use](#neo4j-cli-credential-dbms-use)
- [neo4j-cli credential embed](#neo4j-cli-credential-embed)
- [neo4j-cli credential embed add](#neo4j-cli-credential-embed-add)
- [neo4j-cli credential embed list](#neo4j-cli-credential-embed-list)
- [neo4j-cli credential embed remove](#neo4j-cli-credential-embed-remove)
- [neo4j-cli credential embed use](#neo4j-cli-credential-embed-use)

Manage and view credential values

Usage: `neo4j-cli credential`

## neo4j-cli credential aura-client

Manage and view aura-client credential values

Usage: `neo4j-cli credential aura-client`

### neo4j-cli credential aura-client add

Adds a credential

Usage: `neo4j-cli credential aura-client add [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--client-id` | string | - | (required) Client ID |
| `--client-secret` | string | - | (required) Client secret |
| `--name` | string | - | (required) Name |

### neo4j-cli credential aura-client list

List credentials

Usage: `neo4j-cli credential aura-client list`

### neo4j-cli credential aura-client remove

Removes a credential

Usage: `neo4j-cli credential aura-client remove`

### neo4j-cli credential aura-client use

Sets the default credential to be used

Usage: `neo4j-cli credential aura-client use`

## neo4j-cli credential dbms

Manage and view dbms credential values

Usage: `neo4j-cli credential dbms`

### neo4j-cli credential dbms add

Adds a dbms credential

Usage: `neo4j-cli credential dbms add [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--database-name` | string | neo4j | Database name |
| `--name` | string | - | (required) Name |
| `--password` | string | - | (required) Password |
| `--uri` | string | - | (required) URI |
| `--username` | string | - | (required) Username |

### neo4j-cli credential dbms list

Lists dbms credentials

Usage: `neo4j-cli credential dbms list`

### neo4j-cli credential dbms remove

Removes a dbms credential

Usage: `neo4j-cli credential dbms remove <name>`

### neo4j-cli credential dbms use

Sets the default dbms credential to be used

Usage: `neo4j-cli credential dbms use <name>`

## neo4j-cli credential embed

Manage and view embed credential values

Usage: `neo4j-cli credential embed`

### neo4j-cli credential embed add

Adds an embed credential

Usage: `neo4j-cli credential embed add [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--api-key` | string | - | API key for the provider |
| `--base-url` | string | - | Base URL for the provider (overrides provider default) |
| `--dimensions` | int | 0 | Embedding dimensions (provider-specific; 0 means provider default) |
| `--model` | string | - | (required) Model |
| `--name` | string | - | (required) Name |
| `--provider` | string | - | (required) Provider (one of: openai, ollama, huggingface) |

### neo4j-cli credential embed list

Lists embed credentials

Usage: `neo4j-cli credential embed list`

### neo4j-cli credential embed remove

Removes an embed credential

Usage: `neo4j-cli credential embed remove <name>`

### neo4j-cli credential embed use

Sets the default embed credential to be used

Usage: `neo4j-cli credential embed use <name>`

