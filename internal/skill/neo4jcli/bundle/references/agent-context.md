# neo4j-cli agent-context

Emit a command index (or one command's details) as JSON for AI-agent discovery

Emit a stable envelope describing the neo4j-cli command surface, intended for AI agents discovering it.

With no arguments the envelope (schema_version 2) is compact: schema_version, cli_version, binary, commands (a flat index of every visible command as path + short), exit_codes, error_codes, output_formats and async_flag. Pass a command path (for example "aura instance list") to get that command's long description, example, aliases, flags and subcommand index. Use --full for the complete recursive tree with every command's details (several hundred KB).

The index and details are reflected from the live cobra tree at every invocation — adding a new subcommand, flag, or alias auto-surfaces with no regen step. JSON is the canonical machine view; --format toon carries the same data. On a TTY, --format defaults to a flat command-list table. See AGENTS.md "Agent Context Notes" for the schema-versioning rules and the hand-coded constants that live in build.go.

Usage: `neo4j-cli agent-context [command-path...] [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--full` | bool | false | Emit the complete recursive command tree with all details (large) |

Examples:

```
# List every command as a compact index (default when piped)
neo4j-cli agent-context --format json

# Get the long description, example and flags of one command
neo4j-cli agent-context aura instance list --format json

# Dump the complete recursive tree (large)
neo4j-cli agent-context --full --format json | jq '.commands | keys'
```

