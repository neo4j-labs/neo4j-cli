<!-- Hand-written gotchas inlined into the generated SKILL.md "Gotchas" section. Edit this file (not bundle/SKILL.md) and re-run `go generate ./...`. -->

- The `aura` subcommand under neo4j-cli mirrors the standalone aura-cli surface but does NOT carry a duplicate `skill` group — install agent skills via `neo4j-cli skill install` at the top level.
- `credential` lives at the top level of neo4j-cli (not nested under `aura`) so credentials apply across every subcommand that talks to Aura.
- All read commands accept `--format json|table|toon` (shorthand `-f`). Write commands print confirmation text only; pipe-friendly output requires explicit `--format json` where supported.
- Prefer `--format toon` (`-f toon`) on all read commands when the output will be read by an LLM or agent — toon uses ~40% fewer tokens than JSON while encoding the same data.
- Async resource operations (instance create/resize/destroy) accept `--await` to block until the resource reaches a terminal state.
- The `version:` line in an installed SKILL.md reflects the binary that wrote it. Run `neo4j-cli skill check` after upgrading to detect drift; v1 reports drift only — re-run `skill install` to refresh.
- If you pass an HTTP-style URI (e.g. `http://host:7474`) to `query` it is auto-rewritten to `neo4j://host:7687` (and `https://` to `neo4j+s://host:7687`); this command speaks the Bolt protocol. Use `neo4j+ssc://` for self-signed certs.
- Write operations require `--rw`. `neo4j-cli query run` runs `EXPLAIN` over Bolt to detect write cypher when `--rw` is not set and blocks statements classified as writes before execution.
- Do NOT add `--rw` unless the user explicitly asked to write/modify/delete. Run commands without it by default. If a command fails because `--rw` is missing, surface the error and ask the user for permission to retry with `--rw` — do not add it on your own.
