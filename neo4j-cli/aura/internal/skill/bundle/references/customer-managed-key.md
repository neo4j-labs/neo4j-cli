# aura-cli customer-managed-key

## Contents

- [aura-cli customer-managed-key create](#aura-cli-customer-managed-key-create)
- [aura-cli customer-managed-key delete](#aura-cli-customer-managed-key-delete)
- [aura-cli customer-managed-key get](#aura-cli-customer-managed-key-get)
- [aura-cli customer-managed-key list](#aura-cli-customer-managed-key-list)

Relates to Customer Managed Keys

Usage: `aura-cli customer-managed-key`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `-c, --credential` | string | - | Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list') |
| `--organization-id` | string | - | ID of the Aura organization |
| `--project-id` | string | - | ID of the Aura project |

## aura-cli customer-managed-key create

Creates a new customer managed key

This subcommand creates a new Customer Managed Key in Aura. Creating a new key is an asynchronous operation.

Before you can use the key you will need to setup permissions for it. Log in to the Console, navigate to 'Customer Managed Keys' and click on the Edit icon next to the Key in order to see the instructions.

You can poll the current status of this operation by periodically getting the key details using the get subcommand.

Once the key has a status of ready you can use it for creating new instances by setting the --customer-managed-key-id flag.

Usage: `aura-cli customer-managed-key create [flags]`

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

## aura-cli customer-managed-key delete

Deletes a customer managed key

Deletes a Customer Managed Key from Aura.

Note that you can only delete a Key if it is not being used by any instances, otherwise you will get an error with the reason field set to encryption-key-is-active.

Usage: `aura-cli customer-managed-key delete <id>`

Examples:

```
# Delete a customer managed key by ID
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Delete a key and emit JSON for scripting
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json

# Delete and confirm by piping the response through jq
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json | jq -r '.data.deleted'
```

## aura-cli customer-managed-key get

Returns a customer managed key details

This subcommand returns details about a specific Customer Managed Key.

Usage: `aura-cli customer-managed-key get <id>`

Examples:

```
# Get details of a customer managed key by ID
neo4j-cli aura customer-managed-key get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get details and emit JSON for scripting
neo4j-cli aura customer-managed-key get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json

# Pipe details through jq to extract the key status
neo4j-cli aura customer-managed-key get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json | jq -r '.data.status'
```

## aura-cli customer-managed-key list

Returns a list of customer managed keys

This subcommand returns a list containing a summary of each of your customer managed keys in the specified project. To find out more about a specific key, retrieve the details using the get subcommand.

Use --organization-id and --project-id to specify which project's keys to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.

Usage: `aura-cli customer-managed-key list`

Examples:

```
# List all customer managed keys in a project
neo4j-cli aura customer-managed-key list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List keys using a configured default workspace
neo4j-cli aura customer-managed-key list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura customer-managed-key list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json
```

