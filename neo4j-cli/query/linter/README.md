# linter — offline Cypher linting via the TeaVM JS artifact

`semanticAnalysis.js` is the Cypher semantic-analysis engine (Neo4j's parser +
semantic checks, written in Java/Scala) compiled to JavaScript with TeaVM. It
runs here inside [goja](https://github.com/dop251/goja), a pure-Go JavaScript
engine — no CGo, no node dependency, cross-compiles with the normal release
matrix.

## Artifact provenance & refresh

The vendored file is a verbatim copy of
`cypher-language-support/packages/language-support/src/syntaxValidation/semanticAnalysis.js`
(the same artifact that powers the Neo4j language server and react-codemirror
via `packages/lint-worker`). To refresh:

1. Copy the new artifact over `semanticAnalysis.js` (keep it byte-identical to
   upstream; it is LF-pinned in `.gitattributes` because it is `go:embed`-ed).
2. Run `go test ./neo4j-cli/query/linter/...`.
3. If `rewriteExports` fails, the minified export identifiers changed — update
   `exportMarker`/`exportReplacement` in `linter.go` to match the artifact's
   final `export{...}` statement.

## Open question: schema-aware linting (discuss with maintainers)

`:lint` currently runs **schema-less**: it catches syntax errors, undefined
variables, and scoping/aggregation/type problems, but does not validate
procedure/function calls against a real server. The artifact supports it — it
exports `updateSignatureResolver(procedureRegistry, cypherVersion)`, which is
how the language server feeds procedure/function signatures in — but the CLI
has nothing to feed it from: there is no schema cache anywhere (`:schema`
queries the database live on every invocation and persists nothing).

If schema-aware linting is wanted, a standardised way to supply a schema to
offline commands needs deciding first. Options, roughly in order of how well
they preserve `:lint`'s offline guarantee:

1. **`--schema-file` flag** — feed a saved `:schema --format json` output;
   fully offline, fits scripting/CI.
2. **A CLI-level schema cache** — e.g. `:schema` optionally persisting its
   result keyed by connection; useful beyond linting, but introduces staleness
   and cache-invalidation questions the CLI has avoided so far.
3. **Optional online mode** — `:lint --credential ...` fetching the schema
   live; dilutes the no-connection contract.

## Open question: binary size (discuss with maintainers)

Embedding the linter roughly **doubles the CLI binary**: 17.2 MB → 34.1 MB
(macOS arm64, measured 2026-06). The increase splits into ~11.5 MB of compiled
goja (ES parser + interpreter + regexp2) and 5.3 MB of embedded artifact
(1.4 MB gzipped in release archives). If that's too steep, options:

1. Accept it (simplest; archives are compressed in distribution).
2. Build-tag the linter out of size-sensitive distribution channels.
3. Ship `:lint` as a separate optional binary.

Note: WebAssembly would not help here on current evidence — the wasm-gc build
of the same engine is 28.6 MB raw / 4.6 MB gzipped (vs 5.3 MB / 1.4 MB for the
JS artifact). That wasm build is dev-configured (minification off), so it's
possible a fully minified/optimized wasm build comes out smaller than the JS
route — but unlikely to land under ~10–15 MB total added: a wasm runtime like
wazero contributes ~4–8 MB of compiled code, and the sheer size of the
transpiled analyzer (the JS form is already 5.3 MB minified) sets the floor
for the artifact.

## Why not WebAssembly?

A TeaVM **wasm-gc** build of the same engine exists, but running it in Go is
blocked twice over today: the module requires the wasm-gc and (legacy)
exception-handling proposals, which no Go-embeddable runtime supports (wazero
tracks GC in [wazero#1860](https://github.com/wazero/wazero/issues/1860);
wasmtime-go lacks the exceptions/GC host API), and the host would still need to
provide TeaVM's JS-interop import surface. The TeaVM wasm-gc backend itself is
also still unstable. Interesting long-term — revisit when a Go runtime ships
wasm-gc — but not now. Full research notes: `cypher-lint-runtime-findings.md`
(kept outside this repo).
