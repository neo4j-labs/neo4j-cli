<!-- Hand-written gotchas inlined into the generated SKILL.md "Gotchas" section. Edit this file (not bundle/SKILL.md) and re-run `go generate ./...`. -->

- The `aura` subcommand under neo4j-cli mirrors the standalone aura-cli surface but does NOT carry a duplicate `skill` group — install agent skills via `neo4j-cli skill install` at the top level.
- `credential` lives at the top level of neo4j-cli (not nested under `aura`) so credentials apply across every subcommand that talks to Aura.
- All read commands accept `--format json|table` (shorthand `-f`). Write commands print confirmation text only; pipe-friendly output requires explicit `--format json` where supported.
- Async resource operations (instance create/resize/destroy) accept `--await` to block until the resource reaches a terminal state.
- The `version:` line in an installed SKILL.md reflects the binary that wrote it. Run `neo4j-cli skill check` after upgrading to detect drift; v1 reports drift only — re-run `skill install` to refresh.
- If you pass a bolt-style URI (e.g. `neo4j+s://...:7687`) to `query` it is auto-rewritten to `https://...:7473`; this command speaks the HTTP Query API, not bolt. Aura hosts (`*.neo4j.io`) are always rewritten to `https://<host>` (port 443) regardless of the input scheme or port.
