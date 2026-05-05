---
name: aura-cli
description: Use this skill when the user asks to manage Neo4j Aura resources from the command line via aura-cli — provisioning instances, listing tenants, configuring deployments, or managing client credentials. aura-cli is the standalone Aura CLI (also wrapped by neo4j-cli under the `aura` subcommand) and exposes top-level groups for `instance`, `tenant`, `customer-managed-key`, `graph-analytics`, `config`, `credential`, and `skill` (for installing the embedded agent-skill bundle). Commands follow `<resource> <action>` form (e.g. `instance list`), take at most one positional argument, and support `--format json|table|toon` (shorthand `-f`) on read commands.
version: {{VERSION}}
---

# aura-cli

Allows you to programmatically provision and manage your Aura resources

## Subcommands

| Command | Description |
|---------|-------------|
| [`config`](references/config.md) | Manage and view configuration values |
| [`credential`](references/credential.md) | Manage and view credential values |
| [`customer-managed-key`](references/customer-managed-key.md) | Relates to Customer Managed Keys |
| [`graph-analytics`](references/graph-analytics.md) | Relates to Aura Graph Analytics |
| [`instance`](references/instance.md) | Relates to AuraDB or AuraDS instances |
| [`skill`](references/skill.md) | Install agent skills for this CLI into supported AI agents |
| [`tenant`](references/tenant.md) | Relates to an Aura Tenant |

## Gotchas

<!-- Hand-written gotchas inlined into the generated SKILL.md "Gotchas" section. Edit this file (not bundle/SKILL.md) and re-run `go generate ./...`. -->

- `aura-cli` is the standalone binary; the same surface is also reachable via `neo4j-cli aura ...` (the super-CLI re-mounts this tree under `aura`). Use whichever binary the user has installed.
- `credential` lives at the top level of aura-cli; client credentials are required before any command that talks to Aura's API.
- All read commands accept `--format json|table|toon` (shorthand `-f`). Write commands print confirmation text only; pipe-friendly output requires explicit `--format json` where supported.
- Prefer `--format toon` (`-f toon`) on all read commands when the output will be read by an LLM or agent — toon uses ~40% fewer tokens than JSON while encoding the same data.
- Async resource operations (instance create/resize/destroy) accept `--await` to block until the resource reaches a terminal state.
- The `version:` line in an installed SKILL.md reflects the binary that wrote it. Run `aura-cli skill check` after upgrading to detect drift; v1 reports drift only — re-run `skill install` to refresh.
