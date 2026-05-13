# Agent Context Notes

`neo4j-cli agent-context` emits the full CLI shape as JSON for AI-agent discovery (Layer 2 per `agent-cli-auditor.md` §7.2). Reflected from the live cobra tree at runtime — no static artifact to keep in sync.

## What's reflected vs hand-coded

- Adding a new command/flag automatically surfaces in the next `agent-context` invocation. No regen step, no `make generate-check` involvement for the JSON itself. (Skill-bundle `references/<cmd>.md` still needs `go generate` per the existing rules.)
- Hand-coded constants live in `neo4j-cli/internal/subcommands/agentcontext/build.go`: `schemaVersion`, `exitCodes`, `errorCodes`, `asyncFlag`. Update these when adding a new error category, exit code, or async-flag convention.
- `output_formats` is sourced from `clicfg.ValidFormatValues` — do NOT duplicate the list in agent-context.

## Schema versioning

- Bump `schemaVersion` on breaking JSON-shape changes (rename a top-level key, change a field type, drop a documented code).

## Tests

- `agentcontext_test.go` locks the envelope shape, output-format parity, and tree coverage. Adding a new top-level command will trip the coverage test until the JSON includes it — the failure message tells you what's missing.
