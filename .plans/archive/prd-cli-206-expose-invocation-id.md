# PRD: Expose agent invocation id in `aura agent invoke`

## Overview

Every Aura Agent invocation is tagged with an `invocationId` that threads through all
backend components and is logged in `neo4j-cloud`. The public Aura API returns it on
**every** response — success and error — via the `X-Agent-Invocation-Id` HTTP header.
It is **not** present in the JSON body. The CLI never reads response headers, so the id
is silently dropped: on failure the user sees `Error: agent invocation forbidden…` with
no traceable id to hand to support.

This feature is a purely client-side display gap fix in `neo4j-cli aura agent invoke`:
read the header and surface the id on both the error and success paths.

## Goals

- Print the invocation id on **all** failure paths of `aura agent invoke` (403
  forbidden, `type:"error"` application error, other non-2xx errors).
- Print the invocation id on **successful** invocations — critical because a failing
  *tool* (broken cypherTemplate/text2cypher) returns HTTP `200` `status: SUCCESS` with
  the failure nested in `content[]`, so error-only surfacing would miss the most common
  real-world failure.
- Keep `--format json` stdout machine-parseable (pure JSON); route the id to stderr.
- Leave `api.MakeRequest`'s signature and behaviour unchanged for its other 44 callers.

## Non-Goals

- Changing the `MakeRequest` signature or any of its other call sites.
- Adding the id to the JSON response **body** / error envelope (that is a backend
  `neo4j-cloud` change, tracked separately).
- Any change to cobra `Long` / `Example` / flag help (no skill-bundle regen needed).

## Requirements

### Functional Requirements

- REQ-F-001: Add an opt-in `ResponseHeader *http.Header` field to
  `api.RequestConfig`. When non-nil, `MakeRequest` populates it with the upstream
  `res.Header` on both success and error responses (set right after
  `defer res.Body.Close()`, above every return path).
- REQ-F-002: `invoke` passes `ResponseHeader: &respHeader` and reads
  `invocationID := respHeader.Get("X-Agent-Invocation-Id")` (nil-safe → `""`).
- REQ-F-003: A `withInvocationID(err error, id string) error` helper wraps an error as
  `%w (invocation id: <id>)`, returning the original error unchanged when `err == nil`
  or `id == ""`.
- REQ-F-004: All failure paths surface the id via `withInvocationID`: the 403
  forbidden message, the generic `return err` path, and the `invokeApplicationError`
  (`type:"error"`, HTTP 200) return at its call site.
- REQ-F-005: On success with text/table output, the stats line gains
  ` | Invocation ID: <id>` **only when** id is non-empty.
- REQ-F-006: On success with `--format json`, stdout stays pure JSON (via
  `output.PrintRawBody`); when id is non-empty, `Invocation ID: <id>` is written to
  **stderr** via `cmd.PrintErrln(...)`.
- REQ-F-007: When the header is absent, nothing extra is printed — no empty
  `(invocation id: )`, no stderr line, no trailing stats-line segment.
- REQ-F-008: The test mock (`requesthandlermock.go` / `auratesthelper.go`) gains a
  per-response header capability: a `headers map[string]string` field on `response`, a
  fluent `WithResponseHeader(key, value string) *requestHandlerMock` setter targeting
  the most-recently-added response, and handler logic that calls `res.Header().Set(k,v)`
  before `res.WriteHeader(...)`.

### Non-Functional Requirements

- REQ-NF-001: `--format json` stdout must remain valid, parseable JSON (no narration
  mixed in). Mirrors the existing CLI-82 invariant.
- REQ-NF-002: No behavioural change for the 44 callers of `MakeRequest` that do not set
  `ResponseHeader`.
- REQ-NF-003: Code passes `make test`, `make fmt-check`, and `make lint`.

## Technical Considerations

- Files: `neo4j-cli/aura/internal/api/api.go` (RequestConfig + capture),
  `neo4j-cli/aura/internal/subcommands/agent/invoke.go` (read/print + helper),
  `neo4j-cli/aura/internal/test/testutils/requesthandlermock.go` +
  `auratesthelper.go` (mock header support),
  `neo4j-cli/aura/internal/subcommands/agent/invoke_test.go` (tests).
- `printInvokeResult` gains an `invocationID string` parameter; output mode is resolved
  via the existing `commonoutput.ResolveOutput(cmd, cfg)`.
- Transport errors panic inside `MakeRequest` (existing behaviour) rather than
  returning; the `withInvocationID` generic-error path covers non-2xx HTTP errors and
  the embedded-error (`clierr.NewNotFoundError`) 2xx case.
- `http.Header(nil).Get("…")` returns `""` safely, so a nil/never-populated header
  needs no guard.
- A user-facing changelog entry is required:
  `changie new --projects neo4j-cli --kind Minor --body "aura agent invoke now prints the agent invocation id on success and failure for support/tracing"`
  (confirm kind against `.changie.yaml`; repo uses Major/Minor/Patch).
- No `go generate ./neo4j-cli/internal/skill/...` needed — no help text changes — so
  `TestGenerator_RoundTrip` stays green.

## Acceptance Criteria

- [ ] Invocation id printed on: 403 forbidden, `type:"error"` application error, other
      non-2xx errors, and HTTP-200 tool-failure responses.
- [ ] Id shown on a fully-successful invocation (stats line for text/table; stderr for
      `--format json`, with stdout still valid JSON).
- [ ] Nothing printed when the header is absent (no empty `()`, no stray stderr line).
- [ ] `MakeRequest` signature/behaviour unchanged for the other 44 callers.
- [ ] Tests cover: 403 forbidden, application error, **HTTP-200 tool-failure**, success
      (text + json-to-stderr), and header-absent.
- [ ] Changelog entry added; `make test`, `make fmt-check`, `make lint` pass.

## Out of Scope

- Backend (`agent-api`) change to include `invocation_id` in the JSON response body /
  error envelope so every client gets it without inspecting headers — separate
  `neo4j-cloud` follow-up.

## Open Questions

None. (JSON-output behaviour resolved: id → stderr, stdout stays pure JSON.)

## Manual Verification (optional, live)

Build, point at prod, invoke the disabled repro agent
`8b2eff1d-647e-43e3-840f-6b2bf18a4630` (org `4b6ec1dd-86e1-4f1e-9bfd-eaea7bc9e823`,
project `fbe8a2fc-cf8d-4c3d-9d8e-f5c1c1a0f06e`):

```
neo4j-cli aura agent invoke 8b2eff1d-647e-43e3-840f-6b2bf18a4630 --input hi --rw
# expect: Error: agent invocation forbidden ... (invocation id: <uuid>)
```

Confirm the printed uuid is traceable in `neo4j-cloud` agent-api logs (see
`HANDOFF-expose-invocation-id.md` for the gcloud command).
