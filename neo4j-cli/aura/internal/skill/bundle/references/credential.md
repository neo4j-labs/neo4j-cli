# aura-cli credential

Manage and view credential values

Usage: `aura-cli credential`

## aura-cli credential add

Adds a credential

Add an Aura API client credential. Pass `--file <path>` to import an Aura console–exported aura-client credentials file (recognised keys: CLIENT_ID, CLIENT_SECRET, CLIENT_NAME); explicit flags override file values.

Usage: `aura-cli credential add [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--client-id` | string | - | (required) Client ID |
| `--client-secret` | string | - | (required) Client secret |
| `--file` | string | - | Path to an Aura console–exported aura-client credentials file. Recognised keys: CLIENT_ID, CLIENT_SECRET, CLIENT_NAME. Explicit flags override file values. |
| `--name` | string | - | (required) Name |

## aura-cli credential list

list credentials

Usage: `aura-cli credential list`

## aura-cli credential remove

Removes a credential

Usage: `aura-cli credential remove <name>`

## aura-cli credential use

Sets the default credential to be used

Usage: `aura-cli credential use <name>`

