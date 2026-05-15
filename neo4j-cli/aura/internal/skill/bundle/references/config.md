# aura-cli config

Manage and view configuration values

Usage: `aura-cli config`

## aura-cli config get

Displays the specified configuration value

Usage: `aura-cli config get <key>`

Examples:

```
# Get the default context configured for the Aura CLI
neo4j-cli aura config get default-context

# Get the Aura API base URL and emit JSON for scripting
neo4j-cli aura config get base-url --format json

# Pipe the auth-url value through jq
neo4j-cli aura config get auth-url --format json | jq -r '."auth-url"'
```

## aura-cli config list

Lists the current configuration of the Aura CLI subcommand

Usage: `aura-cli config list`

Examples:

```
# List the current Aura CLI configuration
neo4j-cli aura config list

# Emit configuration as JSON for scripting
neo4j-cli aura config list --format json

# Pipe through jq to print just the default-context value
neo4j-cli aura config list --format json | jq -r '."default-context"'
```

## aura-cli config set

Sets the specified configuration value to the provided value

Usage: `aura-cli config set <key> <value>`

Examples:

```
# Set the default context used by aura commands
neo4j-cli aura config set default-context 00000000-0000-0000-0000-000000000000/11111111-1111-1111-1111-111111111111 --rw

# Override the Aura API base URL (for staging environments)
neo4j-cli aura config set base-url https://api.neo4j.io/v1 --rw

# Switch the output format default to JSON
neo4j-cli aura config set format json --rw
```

