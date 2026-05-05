---
name: fixture-cli
description: Fixture CLI for golden-file tests. Trigger when render code changes.
version: {{VERSION}}
---

# fixture-cli

Fixture CLI used by render tests

Fixture CLI for golden-file render tests.

This tree is intentionally small but covers global flags, sorted
subcommands, hidden subcommands, nested subcommands, and flag tables.

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output` | string | table | Output format (table\|json) |
| `-v, --verbose` | bool | false | Enable verbose logging |

## Subcommands

| Command | Description |
|---------|-------------|
| [`instance`](references/instance.md) | Manage instances |
| [`zebra`](references/zebra.md) | Last alphabetical sub — verifies sort order |

## Gotchas

- One gotcha line.
- Another gotcha line.
