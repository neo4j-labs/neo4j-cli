# linter — offline Cypher linting via the bundled lintCypherQuery

`cypherLint.js` is an esbuild bundle of `lintCypherQuery` (and the
`isNotParamError` helper) from
[`cypher-language-support`](https://github.com/neo4j/cypher-language-support)'s
`@neo4j-cypher/language-support` package. The bundle contains the antlr4-based
Cypher parser, the TypeScript-side schema-aware checks (unknown
label/relationship-type warnings, path-directionality warnings), and the
TeaVM-compiled semantic-analysis engine (Neo4j's parser + semantic checks,
written in Java/Scala, compiled to JavaScript). It runs here inside
[goja](https://github.com/dop251/goja), a pure-Go JavaScript engine — no CGo,
no node dependency, cross-compiles with the normal release matrix.

A nil/empty schema lints schema-less (syntax errors, undefined variables,
scoping/aggregation/type problems). Supplying a `DbSchema` additionally
activates the schema-aware checks each populated key enables — see the
`DbSchema` doc comment in `linter.go`.

## Artifact provenance & refresh

Built from the published npm package
`@neo4j-cypher/language-support@2.0.0-next.34` (includes #681/#685
schema-based linting and #688 `isNotParamError`) with esbuild 0.27.1 — pin
both when rebuilding: minified identifier names (which `exportMarker`
depends on) can change across esbuild versions. Reproducible anywhere, no
monorepo checkout needed:

```sh
mkdir lint-bundle && cd lint-bundle && npm init -y
npm install @neo4j-cypher/language-support@2.0.0-next.34
printf "export { lintCypherQuery, isNotParamError } from '@neo4j-cypher/language-support';\n" > entry.ts
npx esbuild@0.27.1 entry.ts --bundle --platform=browser --format=esm \
    --minify --target=es2017 \
    --outfile=<neo4j-cli>/neo4j-cli/query/linter/cypherLint.js
```

Flag rationale (all three matter for goja):

- `--platform=browser` — `--platform=node` injects
  `import{createRequire}from"node:module"`, and goja evaluates scripts only
  (no `import` at all).
- `--target=es2017` — goja's parser rejects newer *syntax*; esbuild lowers it.
  Library *methods* newer than that are NOT lowered by esbuild — the prelude
  in `linter.go` polyfills the ES2025 `Set.prototype.union`/`intersection`
  and iterator `forEach` that `labelTreeWalking.ts` uses. A refresh that
  trips a `TypeError: Object has no member '...'` at lint time means upstream
  started using another post-ES2017 builtin: add it to the prelude.
- `--format=esm` — the trailing `export{...}` statement is rewritten into
  `globalThis` assignments by `rewriteExports`.

After copying (keep it LF — it is `go:embed`-ed and LF-pinned in
`.gitattributes`):

1. Update `exportMarker`/`exportReplacement` in `linter.go`: the minified
   identifier names in the final `export{...}` statement change on every
   rebuild. `rewriteExports` fails loudly if you forget.
2. Run `go test ./neo4j-cli/query/...`.
3. Record the new upstream commit hash above.

## Measurements (macOS arm64, 2026-06-11)

- Artifact: 5.9 MB minified (old TeaVM-only artifact: 5.3 MB); binary
  34.1 → 34.8 MB.
- Engine init (goja compile + evaluate): ~0.9 s. First lint pays the
  analyzer's lazy setup on top: ~3–5 s total wall for a cold
  `query :lint` invocation (old artifact: ~2.5 s). Subsequent lints in the
  same process: ~0.3–0.6 s each (antlr parsing in an interpreted engine).

## Parameters

`lintCypherQuery` errors on every `$param` not present in
`dbSchema.parameters` — that check is NOT schema-gated upstream. `:lint`
gates it on declarations instead: `--param` entries populate
`DbSchema.Parameters` (a `key:embed=` entry declares the key with a nil
value — the embedding is never computed for linting), and with no `--param`
at all the glue filters the parameter-not-defined diagnostics with the
upstream-exported `isNotParamError` predicate, exactly like the language
server and react-codemirror do when not connected.

## Open question: a common schema model across the CLI (discuss before extending)

The CLI now has **two unrelated schema representations**, and any future
schema feature (a `--schema-file`, a schema cache, schema-aware
autocomplete, …) forces the question of whether to unify them:

| | `query :schema` (`schemaResult`) | `:lint --fetch-schema` (`linter.DbSchema`) |
|---|---|---|
| consumer | humans/agents (rendered output) | `lintCypherQuery` (wire input) |
| labels/relTypes | implicit in property rows | flat name lists, capped at 1000 |
| properties | names + types + mandatory per label-set | names only (no diagnostic uses them yet) |
| graph shape | exact per-relType `MATCH` scans, **multi-label arrays** | one sampled `db.schema.visualization()` call, **single-label triples**, skipped at ≥200 labels+relTypes |
| default language | DBMS-wide `SHOW SETTINGS` probe | per-database `SHOW DATABASES` column |
| indexes/constraints | yes | no |
| key casing | snake_case output rule | camelCase (pinned to upstream's `DbSchema` wire shape) |

The lint side deliberately mirrors cypher-language-support's metadata poller
rather than reusing `:schema`'s machinery: `findPathIssues` upstream was
designed around the single-label visualization triples, and `:schema`'s
exact per-relType scans are too expensive to run implicitly before a lint.
The cost is divergence.

Issues to resolve before adding `--schema-file` or a shared schema model:

- **Which format does a schema file use?** Accepting saved
  `:schema --format json` output is the user-obvious choice but is lossy to
  convert (multi-label path arrays don't map cleanly onto single-label
  triples; the capped name lists aren't present at all). Accepting raw
  `linter.DbSchema` JSON is trivial here but invents a second user-facing
  schema artifact that only `:lint` understands.
- **Unification option**: extend `schemaResult` to a superset that embeds the
  CLS-shaped fields (name lists, triples, per-db default language), so one
  saved `:schema` payload can feed both consumers. That drags the 1000-cap /
  200-threshold heuristics and the camelCase-vs-snake_case conflict into
  `:schema`'s public output — needs a deliberate decision, not a side effect
  of a lint PR.
- **Staleness/caching**: any persisted schema needs an identity (URI +
  database) and an invalidation story; the CLI has so far avoided caches.
- **Upstream drift**: `linter.DbSchema` is pinned to upstream's wire shape
  and will grow (`parameters` from `--param`, `procedures`/`functions`
  registries for unknown-procedure errors — deliberately not fetched yet;
  absent keys are skipped upstream). A CLI-common model would need a mapping
  layer that tracks that evolution.

Until that thinking happens, keep the two representations separate and keep
`lint_schema.go` as the only place that knows how to build a
`linter.DbSchema` from a live connection — a file source slots in beside it
as a second constructor.

## Open question: binary size (discuss with maintainers)

Embedding the linter roughly **doubles the CLI binary**: 17.2 MB → 34.8 MB
(macOS arm64, measured 2026-06). The increase splits into ~11.5 MB of compiled
goja (ES parser + interpreter + regexp2) and 5.9 MB of embedded artifact
(~1.5 MB gzipped in release archives). If that's too steep, options:

1. Accept it (simplest; archives are compressed in distribution).
2. Build-tag the linter out of size-sensitive distribution channels.
3. Ship `:lint` as a separate optional binary.

Note: WebAssembly would not help here on current evidence — the wasm-gc build
of the same semantic-analysis engine is 28.6 MB raw / 4.6 MB gzipped (vs
5.3 MB / 1.4 MB for its JS form). That wasm build is dev-configured
(minification off), so it's possible a fully minified/optimized wasm build
comes out smaller than the JS route — but unlikely to land under ~10–15 MB
total added: a wasm runtime like wazero contributes ~4–8 MB of compiled code,
and the sheer size of the transpiled analyzer sets the floor. Running wasm-gc
in Go is also blocked twice over today: no Go-embeddable runtime supports the
wasm-gc + (legacy) exception-handling proposals (wazero tracks GC in
[wazero#1860](https://github.com/wazero/wazero/issues/1860); wasmtime-go
lacks the exceptions/GC host API), and the host would still need TeaVM's
JS-interop import surface. Revisit when a Go runtime ships wasm-gc. Full
research notes: `cypher-lint-runtime-findings.md` (kept outside this repo).
