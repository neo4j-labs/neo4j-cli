# PRD: Add --debug to Aura commands

Linear: CLI-197 (child of CLI-169 "Add --debug flag for more commands"). CLI-168 added `--debug` locally to the `query`/driver subcommand for driver-level wire info. This issue extends the same idea to the Aura command tree.

## Overview

Add a persistent `--debug` flag to the Aura command tree (`aura` root → all subcommands: `instance`, `project`, `organization`, `credential`, `agent`, `customermanagedkey`, `dataapi/graphql`, `graphanalytics`, `workspace`). When enabled, the CLI emits diagnostic information about every Aura REST API interaction — HTTP request/response wire activity, credential/token acquisition, and async poll/retry steps — to **stderr**, leaving stdout (the JSON/table/TOON result) byte-unchanged. This mirrors the existing `query --debug` behaviour and gives users (and bug reports) "all kinds of useful info when something's gone wrong" against the Aura API.

## Goals

- Give Aura command users a single `--debug` switch that surfaces what the CLI sent to and received from the Aura API, why a request failed, and what happened during polling — without changing program output.
- Match the established `query --debug` contract: flag + `NEO4J_DEBUG=1` env toggle, stderr-only output, identical stdout under `--debug=true` vs `--debug=false`.
- Make the debug output safe to paste into a bug report: every secret (Bearer/access token, client id/secret basic-auth, credential and password body fields) is fully redacted.

## Non-Goals

- A global neo4j-cli root `--debug` covering non-Aura subcommands (docker, dataset, etc.). That is the broader parent CLI-169; this issue is scoped to the Aura tree only. `query` keeps its own existing `--debug` flag unchanged.
- Changing the existing `query`/driver `--debug` behaviour or its `stderrLogger`.
- A new verbosity-level system (`-v`/`-vv`), structured/JSON debug logs, or writing debug output to a file. Plaintext to stderr only.
- Partial secret reveal (e.g. token last-4). Secrets are fully redacted.

## Requirements

### Functional Requirements

- REQ-F-001: Register a persistent `--debug` boolean flag (default `false`) on the Aura root command (`neo4j-cli/aura/aura.go` `NewCmd`), so every Aura subcommand inherits it. Help text mirrors query's: routes Aura API activity to stderr; stdout unaffected; mentions `[env: NEO4J_DEBUG (set to 1 to enable)]`.
- REQ-F-002: Debug-enabled resolution must match `query`'s `resolveDebug` semantics exactly: if `--debug` was explicitly set on the command line (flag `Changed`), its boolean value wins (so `--debug=false` overrides `NEO4J_DEBUG=1`); otherwise debug is ON iff `os.Getenv("NEO4J_DEBUG") == "1"` (strict — any other value leaves it OFF). Factor this into a shared resolver reused by both query and aura rather than duplicating the env-var string, OR replicate the identical logic with a test that locks parity.
- REQ-F-003: The resolved debug state must reach `api.MakeRequest` / `api.getToken` / `api.Poll` (which take `*clicfg.Config`, not `*cobra.Command`). Resolve `--debug` once at Aura command startup (e.g. a `PersistentPreRunE` on the Aura root) and carry the boolean on `cfg` (or an equivalent already-threaded state object) so none of the ~45 `MakeRequest` call sites change. Honor the existing `flags`/cobra precedence gotchas in `.agents/cobra.md`.
- REQ-F-004: When debug is on, `MakeRequest` emits to stderr for every request: HTTP method, full request URL, request headers, request body (if any), then the response status code, response headers, response body, and the request duration. Group/label the lines so request and response are distinguishable (e.g. an `> ` / `< ` prefix or `[aura-debug]` tag).
- REQ-F-005: When debug is on, `getToken` (token.go) emits the credential source / token-acquisition step: that a token fetch is happening, the auth URL, the request/response status (NOT the client secret or returned token), and whether a cached valid token was reused vs a new one fetched.
- REQ-F-006: When debug is on, `Poll` (poll.go) emits one line per poll attempt: attempt index / max retries, the polled path, the observed status, and the sleep/interval — so a hanging or slow async operation is diagnosable. (The underlying per-attempt `MakeRequest` wire dump already prints via REQ-F-004; the poll line adds loop-level context.)
- REQ-F-007: All debug output goes to **stderr**, never stdout. stdout under `--debug` must be byte-identical to stdout without it, in every `--format` (json/table/toon). This is the analog of `query`'s `TestRunQuery_DebugFlagDoesNotChangeStdout` and the project's "writer wrapping must not flip TTY/format resolution" invariant.

### Non-Functional Requirements

- REQ-NF-001: **Full secret redaction.** Before any header/body is written to the debug stream it must pass through `common/clievents.RedactText` (the shape-based redactor covering `key=value`, JSON fields, connection URIs, and `Authorization`/auth headers). The `Authorization: Bearer <token>` header, the basic-auth client id/secret in `getToken`, and any password/secret body fields must render as `***`. Add any Aura-specific secret-bearing field not already covered to the single-sourced `secretWords` vocabulary in `redact.go`, not at the call site. Verify with a test that asserts no raw token/secret substring appears in captured debug output.
- REQ-NF-002: Debug writing must be testable via a package-level writer seam (default `os.Stderr`), following the existing `var`-seam pattern (e.g. `driverOpener`, `clientFactory`). Tests capture the buffer and assert on content; production defaults to stderr. No `afero.NewOsFs()` in tests touching the query/credential packages (per AGENTS.md).
- REQ-NF-003: Zero measurable overhead when debug is off — the redact+format work must be guarded by the debug boolean so it is skipped entirely in the common (off) path.
- REQ-NF-004: Cross-platform: stderr line formatting must not depend on a TTY and must produce LF-consistent output for golden/string assertions; no ANSI color in debug lines.

## Technical Considerations

- **Plumbing seam.** `MakeRequest`, `getToken`, `Poll` all receive `*clicfg.Config`. Carrying the resolved debug bool on `cfg` (set by a `PersistentPreRunE` on the Aura root) is the lowest-churn path — the alternative of threading a new parameter through ~45 call sites is rejected. Confirm `clicfg.Config` is the right home and that the `PersistentPreRunE` runs for the mounted `neo4j-cli aura ...` surface, not only the standalone tree.
- **Resolver reuse.** `resolveDebug` currently lives in `neo4j-cli/query/connect.go`. Either lift a shared helper to `common/` (preferred — single source for the `NEO4J_DEBUG==1` semantics) or replicate with a parity-locking test. Watch the `common/*` cannot import `neo4j-cli/internal/*` rule.
- **Redaction is the safety boundary.** Reuse `clievents.RedactText` rather than writing a new redactor. Note its documented limitation: it is shape-based and does NOT redact table-formatted columns — but debug output here is raw headers/JSON bodies, which it does cover. The Aura access token and client id/secret are the critical values; confirm `RedactText` scrubs `Authorization` and basic-auth headers, and add `secretWords` entries if a field slips through.
- **Output destination.** Prefer `cmd.ErrOrStderr()` where a `*cobra.Command` is in scope; in the api package (no cmd) use the package writer seam defaulting to `os.Stderr`, consistent with the existing `WarnW`/`os.Stderr` default in `MakeRequest`.
- **Skill bundle / examples.** Adding a persistent flag to the aura tree may affect generated skill-bundle reference docs and the `TestAllLeafCommands_HaveExamples` gate. Run `go generate ./neo4j-cli/internal/skill/...` after the flag is added and verify `TestGenerator_RoundTrip`. The aura tree no longer drives a separate bundle, but check for drift.
- **Changelog.** User-facing — add a changie entry (`Minor`) via the project's changie workflow.

## Acceptance Criteria

- [ ] `neo4j-cli aura instance list --debug` prints HTTP request/response wire activity + timing to stderr; stdout is the unchanged result.
- [ ] `NEO4J_DEBUG=1 neo4j-cli aura instance list` produces the same debug output; `--debug=false` with `NEO4J_DEBUG=1` set suppresses it.
- [ ] `--debug` is available on every Aura subcommand (instance/project/organization/credential/agent/cmk/dataapi/graphanalytics/workspace).
- [ ] Token acquisition (`getToken`) and async polling (`Poll`) emit diagnostic lines under `--debug`.
- [ ] No raw Bearer/access token, client secret, or password appears anywhere in debug output — all redacted to `***` (test-asserted).
- [ ] stdout is byte-identical with and without `--debug`, across json/table/toon (test-asserted, analog of `TestRunQuery_DebugFlagDoesNotChangeStdout`).
- [ ] `make test`, `make fmt-check`, `make lint` all pass; skill bundle regenerated if needed (`TestGenerator_RoundTrip` green).
- [ ] Changelog entry added.

## Out of Scope

- Global `--debug` for non-Aura subcommands (CLI-169 parent).
- Verbosity levels, structured/file debug logs, secret partial-reveal.
- Any change to `query --debug` behaviour.

## Open Questions

None — clarifications resolved up front: aura-tree-only scope, HTTP wire + credential/poll diagnostics, reuse `NEO4J_DEBUG=1`, fully redact secrets.
