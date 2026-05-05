# aura-cli credential

Manage and view credential values

Usage: `aura-cli credential`

## aura-cli credential add

Adds a credential

Usage: `aura-cli credential add [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--client-id` | string | - | (required) Client ID |
| `--client-secret` | string | - | (required) Client secret |
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

