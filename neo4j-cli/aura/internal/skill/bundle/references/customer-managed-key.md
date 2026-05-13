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
| `--tenant-id` | string | - | The Aura tenant/project ID |
| `--type` | type | - | (required) The type of the instance. |
| `--wait` | bool | false | Waits until created customer managed key is ready. |

Examples:

```
# Create a customer managed key (AWS-hosted instance)
neo4j-cli aura customer-managed-key create --name my-key --region us-east-1 --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-east-1:000000000000:key/00000000-0000-0000-0000-000000000000 --tenant-id 00000000-0000-0000-0000-000000000000 --rw

# Create a key and wait until it is ready before returning
neo4j-cli aura customer-managed-key create --name my-key --region us-east-1 --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-east-1:000000000000:key/00000000-0000-0000-0000-000000000000 --tenant-id 00000000-0000-0000-0000-000000000000 --wait --rw

# Create a key and emit JSON for scripting
neo4j-cli aura customer-managed-key create --name my-key --region us-east-1 --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-east-1:000000000000:key/00000000-0000-0000-0000-000000000000 --tenant-id 00000000-0000-0000-0000-000000000000 --rw --format json
```

## aura-cli customer-managed-key delete

Deletes a customer managed key

Deletes a Customer Managed Key from Aura.

Note that you can only delete a Key if it is not being used by any instances, otherwise you will get an error with the reason field set to encryption-key-is-active.

Usage: `aura-cli customer-managed-key delete <id>`

Examples:

```
# Delete a customer managed key by ID
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --rw

# Delete a key and emit JSON for scripting
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --rw --format json

# Delete and confirm by piping the response through jq
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --rw --format json | jq -r '.data.deleted'
```

## aura-cli customer-managed-key get

Returns a customer managed key details

This subcommand returns details about a specific Customer Managed Key.

Usage: `aura-cli customer-managed-key get <id>`

Examples:

```
# Get details of a customer managed key by ID
neo4j-cli aura customer-managed-key get 00000000-0000-0000-0000-000000000000

# Get details and emit JSON for scripting
neo4j-cli aura customer-managed-key get 00000000-0000-0000-0000-000000000000 --format json

# Pipe details through jq to extract the key status
neo4j-cli aura customer-managed-key get 00000000-0000-0000-0000-000000000000 --format json | jq -r '.data.status'
```

## aura-cli customer-managed-key list

Returns a list of customer managed keys

This subcommand returns a list containing a summary of each of your customer managed keys. To find out more about a specific key, retrieve the details using the get subcommand.

You can filter keys in a particular tenant using --tenant-id. If the tenant flag is not specified, this endpoint lists all keys a user has access to across all tenants.

Usage: `aura-cli customer-managed-key list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--tenant-id` | string | - | An optional Tenant ID to filter customer managed keys in a tenant |

Examples:

```
# List all customer managed keys the current user has access to
neo4j-cli aura customer-managed-key list

# List keys in a specific tenant
neo4j-cli aura customer-managed-key list --tenant-id 00000000-0000-0000-0000-000000000000

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura customer-managed-key list --format json
```

