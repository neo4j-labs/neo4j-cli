# neo4j-cli agent-context

Emit the full CLI shape as JSON for AI-agent discovery

Emit a stable JSON envelope describing the neo4j-cli command tree, exit codes, error categories, supported output formats, and the canonical async flag — intended for AI agents discovering the CLI's surface.

The envelope (schema_version 1) carries: schema_version, cli_version, binary, commands (recursive tree of every visible subcommand with use/short/long/example/aliases/deprecated/flags/subcommands), exit_codes, error_codes, output_formats, and async_flag. The commands tree is reflected from the live cobra tree at every invocation — adding a new subcommand, flag, or alias auto-surfaces with no regen step.

JSON is the canonical machine view. On a TTY, --format defaults to a degraded flat command-list table. The same envelope is also available via --format toon. See AGENTS.md "Agent Context Notes" for the schema-versioning rules and the hand-coded constants that live in build.go.

Usage: `neo4j-cli agent-context`

Examples:

```
neo4j-cli agent-context
neo4j-cli agent-context --format json | jq '.commands | keys'
neo4j-cli agent-context --format json | jq -e '.commands.aura.subcommands.instance.subcommands.list.flags'
```

