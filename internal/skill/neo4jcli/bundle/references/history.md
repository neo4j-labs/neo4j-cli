# neo4j-cli history

View and manage the local command history log

View and manage the local, best-effort log of neo4j-cli commands you have run. Each command is recorded as one redacted JSON line in a history file alongside config.json. Recording is controlled by the `history-enabled` and `history-limit` config keys.

Usage: `neo4j-cli history`

## neo4j-cli history clear

Empty the local command history log

Empty the local command history log. This is destructive and irreversible, so it requires --force. Recording of future commands is unaffected (controlled by the `history-enabled` config key).

Usage: `neo4j-cli history clear [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm clearing the history log |

Examples:

```
# Clearing requires --force; this errors with guidance
neo4j-cli history clear

# Empty the history log
neo4j-cli history clear --force --rw
```

## neo4j-cli history list

List recently run neo4j-cli commands, newest first

List the most recent neo4j-cli commands recorded in the local history log, newest first. Shows the last 20 entries by default; override with --limit. The default and table views render the human form `[time] <command> {invoker:...}`; --format json|toon emits the structured entries.

Usage: `neo4j-cli history list [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--limit` | int | 20 | Maximum number of entries to show, newest first (0 = all) |

Examples:

```
# Show the last 20 commands, newest first
neo4j-cli history list

# Show the last 5 commands
neo4j-cli history list --limit 5

# Emit the full structured history as JSON
neo4j-cli history list --format json
```

