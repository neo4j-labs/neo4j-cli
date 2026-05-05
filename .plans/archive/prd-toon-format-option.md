# PRD: Toon Format Option

## Overview

Add `toon` as a first-class output format option across the entire CLI. [TOON (Token-Oriented Object Notation)](https://toonformat.dev/) is a compact, human-readable encoding format optimised for LLM consumption — it achieves ~40% fewer tokens than JSON while preserving lossless round-trips. Users piping CLI output into LLM tools gain a meaningfully cheaper and more accurate input format by setting `--format toon` or persisting it in config.

## Goals

- Add `toon` as a valid `--format` value everywhere the flag is accepted.
- Allow `aura-cli config set format toon` and `neo4j-cli config set format toon` to persist the preference.
- Keep the implementation as close to the existing JSON path as possible — same data shape, different serialiser.

## Non-Goals

- Toon will never be the auto-detected default format (the `default` TTY-detection behaviour is unchanged).
- No sub-flags or per-command options for toon encoding knobs — `WithLengthMarkers(true)` is always on.
- No toon _input_ parsing (decode/unmarshal path).
- No changes to the table rendering path.

## Requirements

### Functional Requirements

- REQ-F-001: `--format toon` (and `-f toon`) must be accepted by every command that accepts `--format json` or `--format table`.
- REQ-F-002: `aura-cli config set format toon` and `neo4j-cli config set format toon` must succeed and persist to the config file.
- REQ-F-003: When format resolves to `toon`, output is produced by `github.com/toon-format/toon-go` with `WithLengthMarkers(true)` enabled.
- REQ-F-004: The toon output must encode exactly the same data as the `json` path — the full structure produced by `json.Marshal(values)`, including any outer wrapper (e.g. `{"data": [...]}`), re-encoded in toon.
- REQ-F-005: `aura-cli config get format` and `neo4j-cli config get format` must display `toon` when it is the configured value.
- REQ-F-006: `--format` flag help text must list `toon` alongside `default`, `json`, and `table`.
- REQ-F-007: Passing an invalid format value (e.g. `--format foo`) must still produce a usage error; `toon` must not regress existing validation.

### Non-Functional Requirements

- REQ-NF-001: The toon dependency must be added via `go get` and committed in `go.mod`/`go.sum`.
- REQ-NF-002: All existing tests must continue to pass (`make test`).
- REQ-NF-003: Code must be gofmt-clean (`make fmt-check`).
- REQ-NF-004: New code must carry the Neo4j copyright header (`make license-check`).

## Technical Considerations

### Dependency

Use the official Go implementation: `github.com/toon-format/toon-go`. It is pre-release (no tagged releases, ~9 commits) — the dependency must be pinned to the current commit hash via `go get github.com/toon-format/toon-go@<commit>`. This is an accepted risk given the feature is opt-in and the format spec itself is stable.

### Code changes

1. **`common/clicfg/clicfg.go`** — Extend `ValidFormatValues` from `[3]string` to `[4]string{"default", "json", "table", "toon"}`. The flag help text and both validation sites (flag pre-run hook in `common/flags/flags.go`, config `Set` method) derive their allowed values from this array, so no other changes are needed for validation.

2. **`common/output/output.go`** — Add a `"toon"` case in `PrintBodyMap` alongside the existing `"json"` case. Implement a `printToon` helper that calls `toon.Marshal` with `toon.WithLengthMarkers(true)` and writes to `cmd.OutOrStdout()`.

3. **`common/flags/flags.go`** — The flag help string currently hard-codes `"default, json, table"`; update it to `"default, json, table, toon"` (or derive it from `ValidFormatValues` to avoid future drift).

4. **`common/output/output.go` — `ResolveOutput`** — Add `"toon"` as an explicit case so it is returned as-is and never falls through to `"table"`.

### Mirroring the JSON path exactly

The JSON case in `PrintBodyMap` is:

```go
case "json":
    bytes, err := json.MarshalIndent(values, "", "\t")
    cmd.Println(string(bytes))
```

It passes `values` (the `ResponseData` interface) directly to `json.MarshalIndent`, which calls `MarshalJSON()` on the concrete type and produces the full structure including any outer wrapper (e.g. `{"data": [...]}`).

`toon-go`'s `Marshal` does not call `MarshalJSON`, so passing `values` directly would produce a different structure. To get identical data in toon encoding, the toon path must:

1. Call `json.Marshal(values)` to produce canonical JSON bytes (respecting `MarshalJSON`).
2. Decode those bytes to `any` via `json.Unmarshal`.
3. Pass the result to `toon.Marshal` with `toon.WithLengthMarkers(true)`.

```go
func printToon(cmd *cobra.Command, values ResponseData) {
    jsonBytes, err := json.Marshal(values)
    if err != nil {
        panic(err)
    }
    var v any
    if err := json.Unmarshal(jsonBytes, &v); err != nil {
        panic(err)
    }
    toonBytes, err := toon.Marshal(v, toon.WithLengthMarkers(true))
    if err != nil {
        panic(err)
    }
    cmd.Println(string(toonBytes))
}
```

This keeps the toon case a thin, symmetrical sibling of the JSON case — same input (`values`), same data shape, different final encoding.

## Acceptance Criteria

- [ ] `aura instance list --format toon` produces valid toon-encoded output.
- [ ] `aura instance list --format toon` output differs from `--format json` (toon-encoded, not JSON).
- [ ] `aura-cli config set format toon` succeeds; subsequent `aura-cli config get format` returns `toon`.
- [ ] `neo4j-cli config set format toon` succeeds; subsequent `neo4j-cli config get format` returns `toon`.
- [ ] `aura instance list --format foo` still returns a usage error.
- [ ] `--help` for any read command lists `toon` among valid format values.
- [ ] `make test` passes with no regressions.
- [ ] `make fmt-check` passes.
- [ ] `make license-check` passes.
- [ ] Changelog entry added for both `aura-cli` and `neo4j-cli` (user-facing change, `Minor` kind).

## Out of Scope

- Toon decoding / input parsing.
- Making `toon` the auto-detected default format.
- Per-command toon encoding options (indentation, delimiter, length markers are fixed).
- Any changes to the table or JSON rendering paths.

## Open Questions

- `toon-go` is pre-release. If the library is not `go get`-able (no module proxy entry), a `replace` directive pointing at the GitHub repo may be needed. Verify during implementation.
- Once `toon-go` cuts a stable release, the pinned commit should be updated — consider a Renovate rule or a note in `CONTRIBUTING.md`.
