# PRD: Raw Aura API passthrough command `neo4j-cli aura api` (CLI-225)

## Overview

Add `neo4j-cli aura api <endpoint>`, a `gh api`-style escape hatch that issues an
authenticated HTTP request to an arbitrary Aura API endpoint and prints the response. It
lets users reach Aura features before the CLI wraps them in a dedicated command.

The request came from Dan Mayo in `#feature-labs-neo4j-cli`, explicitly modelled on
[`gh api`](https://cli.github.com/manual/gh_api). The concrete driver is **Aura Multi-DB
going Feature GA on Aura API v2beta1** — nine `.../instances/{instance_id}/databases`
operations that the CLI has no commands for. ([CLI-225](https://linear.app/neo4j/issue/CLI-225))

Today there is no escape hatch. Every Aura call goes through a hand-written cobra leaf, and
`api.MakeRequest` composes the version prefix from a closed enum (`AuraApiVersion1|2|Beta1`
→ `v1`/`v2beta1`/`v1beta5`, `neo4j-cli/aura/internal/api/api.go:159-170`), where
`getVersionPath` panics on an unknown value. An unreleased API version is therefore
unreachable without a code change and a release.

The feature reuses the CLI's existing credential resolution, token caching, `--base-url`
override, `--debug` tracing, redaction, and exit-code contract — only the request
composition and response rendering are new.

### Verified against the live v2beta1 spec

The Aura docs Swagger UI loads its v2beta1 definition from a live endpoint,
`https://api.neo4j.io/v2beta1/spec.json` (per `swagger-initializer.js`): OpenAPI 3.0.0,
"Aura API v2beta1", server `https://api.neo4j.io/v2beta1`, **43 paths / 70 operations**
(34 GET, 17 POST, 11 DELETE, 7 PATCH, 1 PUT). Six findings from that audit constrain the
design and are cited throughout this PRD:

1. **The motivating Multi-DB surface is entirely unwrapped** — list/create databases,
   get/delete a database, list/take backups, get a backup, and
   `POST .../databases/{database_id}/restore` (body `{"id": "<backup-id>"}`). Also unwrapped:
   `fleet-manager/deployments`, `ip-filters`, `invites`, `users`, `billing/ledger`,
   `billing/usage`, `activity-feed`, `import/jobs`.
2. **Path placeholders in the spec are `{org_id}` / `{project_id}`** (snake_case). Users copy
   paths verbatim from the docs, so those are the primary substitution tokens.
3. **`handleResponseError` panics on statuses the spec documents** — `422` (2 ops, incl.
   `POST .../databases`), `413` (2 ops) both hit the `default:` arm
   (`response.go:200`); `415` (2 ops) hits an explicit `panic(reportIssueFatal)`
   (`response.go:180`). Most 4xx responses on the database endpoints declare **no `content`
   schema**, so an empty or non-JSON body fails `json.Unmarshal` → panic.
4. **Error bodies are not one shape** — `BillingErrorResponse` uses
   `errors[].{error,message}`, `GDSError` uses `{id,message,reason}`, `InvokeAgentError` uses
   `{message,type,status_code}`; none match the `{message,reason,field}` that `api.Error`
   models.
5. **Success envelopes are heterogeneous** — 34 ops return `{"data":…}`, **7 return
   `{"data","errors"}`**, 3 return `{"data","links"}`, 6 return **204 with an empty body**,
   and several return a **bare object** with no envelope (`agents`, `ip-filters`,
   `{"token":…}`, `{"id":…}`).
6. **Two pagination styles coexist** — cursor `page_token`/`page_limit` (3 ops) and offset
   `page`/`page_size` (2 ops), plus `start`/`end` and `start_date`/`end_date` ranges.

## Goals

- Reach **any** current or future Aura endpoint and HTTP method without a CLI release.
- Reuse the existing auth stack unchanged: credential resolution (`--credential`, default
  credential, env-var-synthesised ephemeral credential), OAuth token minting, token cache,
  and 401 token-clearing.
- Honour the CLI's cross-cutting contracts: `--format json|table|toon`, `--rw` write gating,
  destructive-action confirmation, `--debug` wire tracing, secret redaction, and the
  closed exit-code enum in `common/clierr`.
- Emit machine-consumable output: with `--format json`, stdout is the response body
  **byte-for-byte**, so `| jq` works and nothing is reordered or re-enveloped.
- **Never panic** on any HTTP status or response body, however malformed — the escape hatch
  will be pointed at pre-GA endpoints whose error shapes are undocumented.
- Leave `MakeRequest` and every existing command's observable behaviour untouched.

## Non-Goals

- Fixing the pre-existing `handleResponseError` panics for existing commands. Filed
  separately as [CLI-227](https://linear.app/neo4j/issue/CLI-227). This feature must not
  inherit the bug, and must not modify `handleResponseError`.
- Wrapping Multi-DB (or any other endpoint) in dedicated first-class commands. The
  passthrough is the deliverable; native commands are separate future work.
- Client-side response filtering (`--jq`, `--template`). The README already teaches
  `--format json | jq`.
- `--paginate`. See REQ-NF-006.
- `gh`'s nested/array field syntax (`key[sub]=`, `key[]=`).

## Requirements

### Functional Requirements

**Command surface**

- REQ-F-001: A new command `neo4j-cli aura api <endpoint>` is registered on the aura root
  (`neo4j-cli/aura/aura.go`). Exactly one positional argument (the endpoint).
- REQ-F-002: The command lives at `neo4j-cli/aura/internal/subcommands/api/`, package `api`,
  split one-concern-per-file per AGENTS.md: `api.go` (`NewCmd`, flags, `RunE`, ≤ ~120 lines),
  `endpoint.go` (endpoint parsing/validation/substitution), `params.go` (fields, body,
  headers, method inference), with colocated `*_test.go`. The Aura client is imported under
  an alias (e.g. `auraapi "github.com/neo4j/cli/neo4j-cli/aura/internal/api"`) to avoid the
  package-name collision.
- REQ-F-003: Because the aura root registers only `--debug`, the command replicates the
  per-group block every other aura subtree has (cf.
  `subcommands/organization/organization.go:16-29`): `--auth-url` / `--base-url` flags bound
  via `cfg.Aura.BindBaseUrl` / `BindAuthUrl` in `PersistentPreRunE`,
  `flags.RegisterAuraCredentialFlag`, and `auraflags.RegisterOrgProjectFlags`. Omitting the
  binds would leave `--base-url` silently inert while tests still pass.

**Endpoint and version**

- REQ-F-004: The endpoint argument is the full path **after the base host, including the API
  version segment** (`aura api v2beta1/organizations/…`, `aura api v1/instances/abc`). A
  leading `/` is accepted. This is what makes an unreleased version reachable day one.
- REQ-F-005: The endpoint may carry an inline query string (`v1/instances?include_deleted=true`);
  it is split off and merged with `--field`-derived query params.
- REQ-F-006: `{org_id}` and `{project_id}` in the endpoint are substituted from
  `--organization-id` / `--project-id` or `aura.default-workspace`. `{org}` and `{project}`
  are accepted aliases. Substitution happens **only when a placeholder is present**, so an
  unscoped path (e.g. `v1/instances`) never requires a configured workspace.
- REQ-F-007: Substitution reuses the existing resolution precedence in the private
  `resolveIDs` (`subcommands/utils/resolve.go:69-98`) via a new thin **non-validating**
  exported `utils.ResolveOrgProject(cmd, cfg)`. It must **not** use
  `ResolveAndValidateOrgProject`, which spends an extra `GET /organizations/{org}/projects`
  round trip. Each substituted value passes through `utils.ValidateResourceID`.

**Request composition**

- REQ-F-008: `--method` / `-X` selects the HTTP method from the allowlist
  `GET HEAD POST PUT PATCH DELETE OPTIONS`, case-insensitively, upper-cased before use. An
  off-list value is a usage error.
- REQ-F-009: Method defaults to `GET`, and is inferred as `POST` when `--field`,
  `--raw-field`, or `--input` is present and `--method` was not given explicitly (matches
  `gh api`).
- REQ-F-010: `--field` / `-F` takes repeatable `key=value` with **type inference**: `true`,
  `false`, `null` and integer-looking values become JSON literals; a value prefixed `@`
  reads from that file, `@-` from stdin; anything else is a string.
- REQ-F-011: `--raw-field` / `-f` takes repeatable `key=value` whose value is **always** a
  string. Shorthands match `gh api` exactly: `-F` typed, `-f` raw.
- REQ-F-012: Fields become **query parameters** for `GET`, `HEAD`, and `DELETE`, and a
  **JSON object request body** for every other method. (The spec's 32 query params —
  `include_deleted`, `list_only_owned`, `database_username`, `page_limit`, … — make the GET
  mapping necessary, not optional.)
- REQ-F-013: `--input` reads the request body **verbatim** from a file, or from stdin when
  the value is `-`. It supports any JSON shape, including a top-level array. It is mutually
  exclusive with `--field` / `--raw-field`; combining them is a usage error.
- REQ-F-014: `--header` / `-H` takes repeatable `Name: value` headers, overlaid on the
  generated auth headers (so a user may deliberately override e.g. `Accept` — two v2beta1
  operations declare an `Accept` header param). Header names must be valid HTTP tokens and
  values must not contain CR or LF; violations are usage errors.

**Response rendering**

- REQ-F-015: A new `output.PrintPassthrough(cmd, cfg, body []byte)` in
  `common/output/output.go` renders the body per the existing `output.ResolveOutput`
  precedence. It must live in that package because it needs the unexported
  `stripControlDeep` and `printTable`.
- REQ-F-016: `--format json` writes the body **byte-for-byte** to `cmd.OutOrStdout()` with a
  single trailing newline — nothing reordered, reindented, or re-enveloped. An empty body
  (HTTP 204) writes nothing.
- REQ-F-017: `--format toon` unmarshals to `any`, applies `stripControlDeep`, and
  `toon.Marshal`s, falling back to the verbatim body on any error (mirroring
  `printToonValue`'s existing non-panicking fallback).
- REQ-F-018: `--format table` derives rows — a top-level object with a `data` array of
  objects uses that array; a bare array of objects uses itself; a bare object is one row.
  Columns are the union of row keys in **first-seen order** (deterministic). Any other shape
  falls back to the verbatim body. Rows are fed to the existing `printTable` through a small
  unexported `rawRows []map[string]any` implementing `AsArray()`.
- REQ-F-019: Rendering must **not** route through `api.ParseBody` or `api.ParseRawBody`, both
  of which panic on JSON that is not an object or array (`response.go:533`) — a bare string,
  number, `null`, or `[1,2,3]` response would crash. All six envelope shapes from the spec
  audit must render without panicking.
- REQ-F-020: `--include` / `-i` writes the HTTP status line and the response headers (sorted,
  each passed through `output.StripControl`) to stdout before the body. Documented as making
  stdout non-JSON.
- REQ-F-021: `--silent` suppresses the response body; combined with `-i` it emits headers
  only, and alone it yields the exit code only.

**Request path and error mapping**

- REQ-F-022: A new `neo4j-cli/aura/internal/api/raw.go` provides `MakeRawRequest(cfg,
  *RawRequestConfig) (*RawResponse, error)` and `RawStatusError(*RawResponse) error`.
  `RawRequestConfig` carries `Method`, `VersionPath` (literal first segment; `""` means
  none), `Path`, `Body []byte`, `QueryParams url.Values`, `Headers http.Header`.
  `RawResponse` carries `StatusCode`, `Status`, `Proto`, `Header`, and `Body` — the body
  populated on **every** status.
- REQ-F-023: The shared prologue of `MakeRequest` (base-URL parse +
  `urlcheck.ValidateRemoteURL`, active-vs-default credential resolution at `api.go:99-107`,
  `getHeaders`, debug emit) is extracted into an unexported helper used by **both**
  `MakeRequest` and `MakeRawRequest`, so auth and SSRF gating are single-sourced.
- REQ-F-024: `MakeRawRequest` never calls `handleResponseError` and skips the
  2xx-embedded-errors trap at `api.go:142-145` — a genuine 200 carrying `{"data","errors"}`
  (7 spec operations) must stay a success, not become exit 3.
- REQ-F-025: `MakeRawRequest` **never panics**. Transport (`client.Do`) and `io.ReadAll`
  failures return `clierr.NewUpstreamError`, in contrast to `api.go:120-123` which panics.
- REQ-F-026: On HTTP 401, `MakeRawRequest` still clears the stale access token via the
  existing unexported `formatAuthorizationError`, so a retry works as it does for every
  other command.
- REQ-F-027: `RawStatusError` returns `nil` for 2xx and otherwise maps the status to the
  **same `clierr` codes** `handleResponseError` uses — 400→6 validation, 401/403→4 auth,
  404→3 not-found, 402/409→5 conflict, 429→7 rate-limited (carrying `Retry-After`), 5xx→8
  upstream — with `clierr.NewUpstreamError` as the fallback for anything unmapped
  (413/415/422/…). It never panics and never depends on the body parsing.
- REQ-F-028: The upstream response body is folded into the returned error's `ce.Message`,
  `StripControl`ed and truncated (~4 KB), rather than written to stdout. `clierr.Render`
  already writes the JSON error envelope to stdout (`render.go:85-87`), so echoing the body
  there too would put two JSON documents on stdout and break the `AssertOutIsValidJSON` /
  CLI-82 single-envelope invariant.

**Gating**

- REQ-F-029: The precedence logic of `flags.EnforceWriteGate`
  (`common/flags/flags.go:94-119`) is extracted into an exported
  `flags.RequireWriteAccess(cmd) error`; `EnforceWriteGate` delegates to it after its
  annotation check, with **no behaviour change** for the ~35 existing `write:true` commands.
- REQ-F-030: `RunE` calls `flags.RequireWriteAccess` when the resolved method is **not**
  `GET` or `HEAD`. The command therefore carries **no** `write:true` annotation, and its
  `Example` must contain at least one `--format json` invocation to satisfy
  `TestAllLeafCommands_HaveExamples` (`agentcontext_test.go:248-251`).
- REQ-F-031: `DELETE` is additionally gated by `common/confirm`: `confirm.Register(cmd)` in
  `NewCmd`, and the confirm check in `RunE` **after** `RequireWriteAccess` so a missing
  `--rw` is reported first. `RunE` returns the error unchanged; `main.go:168-169` maps
  `confirm.ErrCancelled` to exit 0.
- REQ-F-032: `confirm.Require` derives its noun from `cmd.Parent().Name()`
  (`confirm.go:72-82`), which here would read `Delete aura "v1/instances/abc"?`. Add
  `confirm.RequireTyped(cmd, resourceType, resourceID string) error` and have the existing
  `Require` delegate to it with the derived type (no behaviour change for current callers),
  then call `RequireTyped(cmd, "endpoint", endpoint)` so the prompt reads
  `Delete endpoint "v1/instances/abc"? This action is irreversible. [y/N]`.

**Security**

- REQ-F-033: The endpoint must be rejected as a usage error (exit 2, request never issued)
  when it is empty, contains `://`, yields a non-empty `Scheme` or `Host` from `url.Parse`,
  or begins with `//`. Without this, `aura api https://evil.com/x` would exfiltrate the
  bearer token to an arbitrary host.
- REQ-F-034: The endpoint must be rejected when any path segment is `.` or `..`.
  `url.JoinPath` **resolves** those segments, the same hazard `utils.ValidateResourceID`
  guards against (`resolve.go:18-31`).
- REQ-F-035: `common/clievents/redact.go` — add `"header"` to `secretFlags` so
  `-H 'Authorization: Bearer …'` is never written to telemetry, panic text, or on-disk
  history. Fails closed, same rationale as the existing `-p` entry.
- REQ-F-036: `common/clievents/redact.go` — generalise `isParamFlag` (line 282) to also
  match `field` and `raw-field`, so the existing `redactParamValue` scrubs
  `--field password=hunter2` → `password=***` while leaving `--field name=my-db` intact.
  This reuses the selective `key=value` machinery already built for `--param`.

**Docs and generated artefacts**

- REQ-F-037: A flush-left `Example` with at least three `#`-comment-headed invocations, each
  prefixed `neo4j-cli`, `--rw` on the writes and at least one `--format json`, using the real
  Multi-DB paths so the example doubles as the CLI-225 answer.
- REQ-F-038: Run `go generate ./neo4j-cli/internal/skill/...` and commit the regenerated
  bundle alongside the source. A new command drifts `bundle/references/aura.md`, and
  `TestGenerator_RoundTrip` plus `make generate-check` fail otherwise.
- REQ-F-039: Add a README section for `aura api`: the endpoint-carries-the-version contract,
  the `{org_id}`/`{project_id}` placeholders, `--rw` on non-GET plus `--yes --force` on
  DELETE, and that `--format json` is verbatim so `| jq` works.
- REQ-F-040: Add a changie entry —
  `changie new --projects neo4j-cli --kind Minor --body …` — describing only the observable
  user-facing surface (the new command, its flags, its output behaviour), not the internal
  `raw.go` mechanics.

### Non-Functional Requirements

- REQ-NF-001: **No regression in `MakeRequest`.** `response.go`'s status→exit-code matrix
  (`response_test.go`) and `stdout_invariance_debug_test.go` must pass untouched, and
  `handleResponseError` must not be modified.
- REQ-NF-002: `--debug` traces the passthrough request/response through the existing
  package-global `debugW` seam using the `[aura-debug] > ` / `[aura-debug] < ` prefixes, with
  every line passed through `scrub` (`RedactText` then `StripControl`), and guarded by
  `cfg.Aura.Debug()` so the off-path is untouched. stdout must be byte-identical with and
  without `--debug`.
- REQ-NF-003: Flag long names are kebab-case per
  `agentcontext/casing_input_gate_test.go`. Path placeholder tokens (`{org_id}`) are wire
  shapes, not CLI input identifiers, and are outside that gate. No new json-tagged output
  struct is introduced, so `common/output/casing_gate_test.go`'s `outputStructAllowlist`
  needs no entry; because table columns are derived at runtime there are no `fields` string
  literals for the `printFuncs` arm to check.
- REQ-NF-004: `--base-url` may point at any https host. `urlcheck.ValidateRemoteURL` still
  blocks non-https, private, link-local, and cloud-metadata targets. This freedom is
  **intended**: aiming the CLI at a dev environment
  (`config set aura.base-url https://api-devdan.neo4j-dev.io`) is an established workflow, and
  an escape hatch that only reached production would be useless for the pre-GA features
  CLI-225 exists to serve. No extra host gate.
- REQ-NF-005: All new `.go` files carry the Neo4j copyright header (CI `addlicense`).
- REQ-NF-006: `--paginate` is deliberately absent. v2beta1 uses two pagination styles
  (cursor `page_token`/`page_limit` and offset `page`/`page_size`), so a flag built on
  `api.NextPageToken` would silently mispaginate the offset endpoints. Users can pass
  `--field page_token=…` and follow `links.next` manually.
- REQ-NF-007: `make test`, `make fmt-check`, and `make lint` all pass.

## Technical Considerations

**Why a second request path rather than extending `MakeRequest`.** `MakeRequest` is
unusable for a passthrough on five counts, every one of which is load-bearing: (1) it always
injects a version prefix from a closed enum whose lookup panics on an unknown value;
(2) `RequestConfig.PostBody` is `map[string]any`, so a top-level JSON array body is
inexpressible; (3) there is no custom-header field — headers are hard-coded in `getHeaders`;
(4) on non-2xx the body is consumed by `handleResponseError`, which panics in its `default:`
branch and on any unmarshal failure, and returns a nil body so the API's own error JSON never
reaches the user; (5) the 2xx trap converts a genuine 200 carrying `{"errors":[…]}` into exit
3. Rather than destabilise a function every Aura command depends on, extract the shared
prologue and add a parallel entrypoint. Spec finding 3 makes the panic risk concrete rather
than theoretical.

**Single-sourcing.** Four extract-and-delegate refactors keep behaviour identical while
opening a seam: `MakeRequest`'s prologue (REQ-F-023), `flags.EnforceWriteGate` →
`RequireWriteAccess` (REQ-F-029), `confirm.Require` → `RequireTyped` (REQ-F-032), and
`isParamFlag`'s kv-redaction list (REQ-F-036). Each is a pure refactor with existing tests as
the safety net; none changes an existing caller's behaviour.

**stdout discipline.** The CLI-82 invariant is that stdout carries exactly one machine-
readable document. That drives two decisions: the upstream error body goes into
`ce.Message` rather than stdout (REQ-F-028), and `--include` is documented as explicitly
opting out of JSON-parseable stdout (REQ-F-020).

**Testing seams already available.** `testutils.NewAuraTestHelper` + `NewRequestHandlerMock`
drive a full command against an httptest server with a canned `/oauth/token` handler; mock
paths include the version prefix. `api.SetDebugWriterForTest` captures `--debug` output
(cobra-captured stderr stays empty — the trace goes to the package global).
`confirm.SetStdinIsTerminal` drives both confirm branches without a PTY. A package
`main_test.go` must seed `commonoutput.IsAgent = func() bool { return false }`, since
`CLAUDECODE` in the dev/CI environment would otherwise flip every format assertion to toon.

**Risks.** The `table` column-derivation heuristic is the least-specified part; keeping it
deterministic (first-seen key order) and always falling back to verbatim JSON bounds the
blast radius. The `--field` type-inference rules are a compatibility surface with `gh` and
should be table-tested exhaustively.

## Acceptance Criteria

- [ ] `neo4j-cli aura api v1/instances` issues an authenticated `GET` and prints the response.
- [ ] `--format json` reproduces the response body byte-for-byte (asserted against the exact
      mock string, not a re-marshalled one).
- [ ] All six spec envelope shapes render under `json`, `toon`, and `table` without panicking:
      `{"data":[…]}`, `{"data":{…}}`, `{"data","errors"}` (stays a success), `{"data","links"}`,
      a bare object, a bare array, a 204 empty body, and a `null`/scalar/invalid-JSON body.
- [ ] An endpoint whose first segment is an unreleased version reaches the server unmodified
      (e.g. `aura api v2beta1/spec.json` returns the spec).
- [ ] `{org_id}`/`{project_id}` and the `{org}`/`{project}` aliases substitute from both
      `aura.default-workspace` and `--organization-id`/`--project-id`; a placeholder-free path
      succeeds with **no** workspace configured.
- [ ] `--method POST` without `--rw` exits 2 with `this command writes; pass --rw to allow it`
      and issues **no** request; a plain `GET` needs no `--rw`.
- [ ] `DELETE` without `--yes --force` on a non-TTY exits 2 and issues no request; declining
      the TTY prompt exits 0 with `cancelled.` on stderr and issues no request; both flags
      together issue the request.
- [ ] `--field`/`--raw-field` type inference is correct; fields land as query params on GET
      and as a JSON body otherwise; `--input -` reads stdin; `--input` plus `--field` is a
      usage error.
- [ ] `--header` is applied; CR/LF and malformed names are usage errors.
- [ ] `https://evil.com/x`, `//evil.com/x`, and `v1/../../x` each exit 2 with no request issued.
- [ ] Every status the spec documents maps to the right `clierr` code — 200/201/202/204 and
      400/401/403/404/405/409/**413**/**415**/**422**/429/500 — with **no panic**, including a
      400 whose body is empty or non-JSON. The upstream body appears in `ce.Message` and
      stdout holds exactly one JSON document.
- [ ] `-i` prints the status line and headers; `--silent` prints no body.
- [ ] `--debug` emits `[aura-debug] >`/`<` lines to the `debugW` seam with a
      `-H 'Authorization: …'` value redacted, and stdout is unchanged versus a non-debug run.
- [ ] `--header` and `--field password=…` are redacted by `clievents.RedactArgs`;
      `--field name=my-db` is not.
- [ ] The Multi-DB flow works against the real v2beta1 endpoints: list databases, create one
      from `--input`, and delete one with `--rw --yes --force`.
- [ ] Provoking a real 422 from `POST .../databases` yields a clean error envelope, not a panic.
- [ ] `make test`, `make fmt-check`, `make lint` pass; `go generate
      ./neo4j-cli/internal/skill/...` leaves the tree clean with the bundle committed;
      README section and changie `Minor` entry present.

## Out of Scope

- Fixing `handleResponseError`'s panics on 413/415/422 and schema-less 4xx bodies for existing
  commands — [CLI-227](https://linear.app/neo4j/issue/CLI-227). `handleResponseError` is not
  touched here, and `response_test.go`'s existing matrix stays green and unmodified.
- Native first-class commands for Multi-DB or any other currently-unwrapped resource.
- `--paginate` (REQ-NF-006), `--jq` / `--template`, and nested `key[sub]=` / `key[]=` field
  syntax.
- Any additional restriction on `--base-url` (REQ-NF-004).

## Open Questions

None. Every design choice was settled with the user before this PRD was written: the endpoint
carries the version, `--rw` gates only non-GET/HEAD, `--format` is honoured with verbatim
JSON, the extra flags are `{org_id}`/`{project_id}` placeholders plus `-i`/`--silent`,
`DELETE` also requires confirm, `--base-url` stays unrestricted, and the changelog kind is
`Minor`.
