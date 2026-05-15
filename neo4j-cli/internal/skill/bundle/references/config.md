# neo4j-cli config

Manage and view global configuration values

Usage: `neo4j-cli config`

## neo4j-cli config get

Displays the specified configuration value

Usage: `neo4j-cli config get <key>`

Examples:

```
# Get the active output format
neo4j-cli config get format

# Get the active output format as JSON
neo4j-cli config get format --format json

# Get an aura-scoped key via dot-notation
neo4j-cli config get aura.default-context --format json
```

## neo4j-cli config list

Lists the current global configuration values

Usage: `neo4j-cli config list`

Examples:

```
# List all configuration values as a table
neo4j-cli config list

# List all configuration values as JSON (machine-readable)
neo4j-cli config list --format json

# List all configuration values in toon format
neo4j-cli config list --format toon
```

## neo4j-cli config set

Sets the specified configuration value to the provided value

Usage: `neo4j-cli config set <key> <value>`

Examples:

```
# Set the default output format to JSON
neo4j-cli config set format json --rw

# Disable telemetry
neo4j-cli config set telemetry false --rw

# Set the default Aura context via dot-notation
neo4j-cli config set aura.default-context my-org-id/my-project-id --rw
```

