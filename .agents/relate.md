# Relate (embedded engine)

`neo4j-cli query --relate <name-or-path>` and the `relate` command tree run the Relate embedded openCypher engine in-process. No server, ports or credentials.

## Layout

- `internal/relate/` — service package (no cobra): the `Store` interface, the `OpenStore`/`OpenStoreReadOnly` seams, store helpers (`stores.go`), pull (`pull.go`, identity sidecar), push (`push.go`), `--relate-opt` mapping (`openopts.go`), `QuoteIdent` (`cypher.go`). `engine.go` is the ONLY file importing the cgo client; it is built under `cgo && relate` and installs the real hooks via `init`. Untagged, the seams are nil and callers return `relate.NotCompiledError(...)` (a usage error).
- `internal/commands/relate/` — cobra surface, one file per leaf (`create`/`list`/`info`/`delete`/`pull`/`push`). `list`/`delete` are pure filesystem and work untagged.
- `internal/commands/query/relate_backend.go` — the embedded backend behind `query --relate`: reuses the query loops through the `runStatementsResponseFn` seam; rows go through `coerceDriverValue` so output matches the Bolt path.
- `internal/dbconn/bolterrors.go` — the shared Bolt error classifier (`CategorizeBoltError`); pull/push use it for remote failures so a Cypher mistake is exit 6, not 8.

## Build and test

- Get the native library: `eval "$(scripts/relate-ffi.sh <triple> <dir>)"` (downloads the pinned `relate-ffi-<triple>.tar.gz`, verifies sha256, validates `<dest>`, prints `CGO_CFLAGS`/`CGO_LDFLAGS`; link is static). Then `CGO_ENABLED=1 go build -tags relate ./cmd/neo4j-cli`. Windows needs clang in MSVC mode + lld (see the script header and `.github/workflows/relate.yml`).
- Bump Relate by changing `RELATE_VERSION` and all five sha256s in `scripts/relate-ffi.sh` together with the `clients/go` version in `go.mod`. The client and the FFI archive must move together: a newer client can call C symbols the older header lacks. Compute digests from the downloaded archives (the release `SHA256SUMS` does not list them).
- `make test` does NOT cover the engine. Run `CGO_ENABLED=1 go test -tags relate -count=1 ./...` with the cgo env set; CI runs it in the `relate` lane (linux/macOS/windows). Real-engine tests carry `//go:build cgo && relate`; everything else uses fake stores (`fakeStore` in `internal/commands/relate/helpers_test.go`, `fakeRelateStore` in the query package).
- Pull/push are Bolt-only, so their remote side is faked in CI. Verify against a real Neo4j by hand: start `neo4j:latest` (docker or podman), then `relate pull` / `relate push` against it.

## Gotchas

- **Open options travel as environment variables.** The Go client's `Open(uri)` takes only a path; the engine reads `RELATE_*` at open. `--relate-opt key=value` maps an allow-list of keys onto those variables in `ApplyOpenOpts`, which MUST run before `OpenStore`. Validate values against the engine's real grammar (memory sizes: whole number + optional mb/mib/gb/gib, no decimals, no kb). `snapshot-format` applies to a new/empty store only.
- **Store detection** (`IsStore`): a parquet/json store keeps `manifest.json` at its root, a Markdown vault under `.relate/manifest.json`. Accept both.
- **Typed parameters.** Pull binds property values as typed parameters (`UNWIND $rows`), including temporals, points and vectors: `toClientParams` (`params_convert.go`, tagged) maps driver `dbtype` values and `[]float32` embeddings onto the Go client's typed kinds, which the client binds through the engine's typed putters (relatedb/relate#901, REL8-929). That needs a client + FFI release that contains #901; an older client binds those kinds as maps and the engine rejects them (`InvalidPropertyType`).
- **Identity sidecar.** Pull keeps `pull-identity.json` beside the store (per source URI+database). Persist it even when a pull fails part-way, and never treat an unreadable sidecar as empty — either loses identities and the next pull duplicates.
- Write leaves (`create`, `delete`, `pull`, `push`) carry the `write` annotation, so scripted callers and agents need `--rw`; every `Example:` for them includes it. Registering the tree in `app.go` also drives `internal/commands/mcp/server/testdata/policy.golden` and the skill bundle — regenerate both.
- Output field names stay snake_case; `relate info` rows are keyed `relationship_types` etc.

## Release status

No released binary includes the engine yet: `.goreleaser.yaml` builds with `CGO_ENABLED=0` and no `relate` tag, so brew/npm/PyPI/archives report "not compiled into this build". Shipping it needs per-platform native builds (cgo does not cross-compile cleanly here: darwin needs the macOS SDK for CoreFoundation, Windows needs clang+lld for the MSVC archive, 386 has no FFI archive) and a decision on binary size (about 20 MB without the engine, 55 MB with it on darwin/arm64) versus a download-on-demand engine.
