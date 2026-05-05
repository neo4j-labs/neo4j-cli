# aura-cli customer-managed-key

Relates to Customer Managed Keys

Usage: `aura-cli customer-managed-key`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auth-url` | string | - |  |
| `--base-url` | string | - |  |
| `--output` | string | - | Format to print console output in, from a choice of [default, json, table] |

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
| `--await` | bool | false | Waits until created customer managed key is ready. |
| `--cloud-provider` | cloud-provider | - | (required) The cloud provider hosting the instance. |
| `--key-id` | string | - | (required) Encryption Key ARN |
| `--name` | string | - | (required) The name of the customer managed key (any UTF-8 characters with no trailing or leading whitespace). |
| `--region` | string | - | (required) The region where the instance is hosted. |
| `--tenant-id` | string | - | The Aura tenant/project ID |
| `--type` | type | - | (required) The type of the instance. |

## aura-cli customer-managed-key delete

Deletes a customer managed key

Deletes a Customer Managed Key from Aura.

Note that you can only delete a Key if it is not being used by any instances, otherwise you will get an error with the reason field set to encryption-key-is-active.

Usage: `aura-cli customer-managed-key delete <id>`

## aura-cli customer-managed-key get

Returns a customer managed key details

This subcommand returns details about a specific Customer Managed Key.

Usage: `aura-cli customer-managed-key get <id>`

## aura-cli customer-managed-key list

Returns a list of customer managed keys

This subcommand returns a list containing a summary of each of your customer managed keys. To find out more about a specific key, retrieve the details using the get subcommand.

You can filter keys in a particular tenant using --tenant-id. If the tenant flag is not specified, this endpoint lists all keys a user has access to across all tenants.

Usage: `aura-cli customer-managed-key list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--tenant-id` | string | - | An optional Tenant ID to filter customer managed keys in a tenant |

