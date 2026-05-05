<!-- Hand-written gotchas inlined into the generated SKILL.md "Gotchas" section. Edit this file (not bundle/SKILL.md) and re-run `go generate ./...`. -->

- `aura-cli` is the standalone binary; the same surface is also reachable via `neo4j-cli aura ...` (the super-CLI re-mounts this tree under `aura`). Use whichever binary the user has installed.
- `credential` lives at the top level of aura-cli; client credentials are required before any command that talks to Aura's API.
- All read commands accept `--format json|table|toon` (shorthand `-f`). Write commands print confirmation text only; pipe-friendly output requires explicit `--format json` where supported.
- Prefer `--format toon` (`-f toon`) on all read commands when the output will be read by an LLM or agent — toon uses ~40% fewer tokens than JSON while encoding the same data.
- Async resource operations (instance create/resize/destroy) accept `--await` to block until the resource reaches a terminal state.
- The `version:` line in an installed SKILL.md reflects the binary that wrote it. Run `aura-cli skill check` after upgrading to detect drift; v1 reports drift only — re-run `skill install` to refresh.
