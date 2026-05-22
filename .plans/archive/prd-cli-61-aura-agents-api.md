# PRD: CLI-61 — Port Aura Agents API to neo4j-cli

Linear: https://linear.app/neo4j/issue/CLI-61/add-aura-agents-api-to-aura-command
Source: PR `neo4j/aura-cli#167` (merged in the deprecated `aura-cli` fork).
Blocker: CLI-120 — Apply org/project args/setting to all commands — **Done** (PR neo4j-labs/neo4j-cli#133).
Original PR diff captured at `/tmp/cli-61-original-pr.diff` (2091 lines).

## Overview

Port the Aura Agents API command tree (`agent {list,get,create,update,replace,delete,invoke}`) from the deprecated `aura-cli` fork to the current `neo4j-cli` repo, gated behind the existing `flag.aura-beta` feature flag. Commands target `/v2beta1/organizations/{org}/projects/{proj}/agents` via the existing `api.AuraApiVersion2` router.

The port adapts the original PR to the modernised conventions in `neo4j-cli`:
- `--format` is registered globally on the root cobra command, not per-leaf.
- Feature flag rename: `aura.beta-enabled` → `flag.aura-beta`, checked via `cfg.Flags.Enabled("flag.aura-beta")`.
- Test seam config keys: `flag.aura-beta` (was `aura.beta-enabled`) and `format` (was `aura.output`).
- The `common/clicfg/projects/projects.go` nil-pointer fix in the original PR does **not** apply — that package does not exist in this repo. Org/project resolution lives in `neo4j-cli/aura/internal/subcommands/utils/projectflags.go`, which already returns empty strings safely.

Two shared helpers (`api.ParseRawBody`, `output.PrintRawBody`) are added to handle the agent API's bare-JSON responses (no `{"data":...}` envelope), keeping the existing `ParseBody`/`PrintBody` path untouched for envelope-wrapped APIs.

## Goals

- Ship `neo4j-cli aura agent {list,get,create,update,replace,delete,invoke}` under `flag.aura-beta`, mirroring the original PR's behaviour bit-for-bit (request paths, request bodies, error mapping, invoke stats line).
- Reuse the closest sibling beta-v2 pattern (`deployment/`) for org/project handling so the new tree is trivially reviewable against an existing checked-in template.
- Keep the agent API's bare-JSON response handling isolated to two small helpers (`ParseRawBody`, `PrintRawBody`); do not refactor `ParseBody`/`PrintBody`.
- Land the per-leaf `Example:` fields, write-annotation gating, and skill-bundle regen needed to keep `make test`, `make fmt-check`, `make lint`, `make generate-check`, and `make license-check` green.

## Non-Goals

- Migrating the new commands to the newer `utils.ResolveAndValidateOrgProject` API-preflight pattern (used by `cmek`, `graph-analytics session`). The closest analog (`deployment/`) still uses `SetProjectFlagsAsRequired` + `SetProjetDefaults`; matching that surface keeps review small.
- `--wait` support on `create`/`update`/`replace`. Agent CRUD is synchronous in the API; no async surface to poll.
- Pagination on `list`. The agent API returns a bare array today; revisit if/when the API paginates.
- `--tools @./tools.json` (file-input form). Inline JSON only, matching the original PR.
- Per-tool breakdown in `invoke`'s human stats line. Count only.
- Multi-line input via `--input -` (stdin) for `invoke`.
- Documentation beyond per-command `Example:` fields and the auto-generated skill bundle reference.

## Requirements

### Functional Requirements

#### Parent command

- REQ-F-001: `neo4j-cli/aura/internal/subcommands/agent/agent.go` exports `NewCmd(cfg *clicfg.Config) *cobra.Command` with `Use: "agent"`, `Short: "Relates to Aura Agents"`. It mounts the seven leaf commands listed below via `cmd.AddCommand(...)`. It does **not** register persistent `--output`, `--auth-url`, or `--base-url` flags — `--format` is bound on the root, and the URL flags are config-driven elsewhere.
- REQ-F-002: `neo4j-cli/aura/aura.go` adds `cmd.AddCommand(agent.NewCmd(cfg))` inside the existing `if cfg.Flags.Enabled("flag.aura-beta") { ... }` block, import path `github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/agent`. When the flag is disabled the entire `agent` subtree is hidden, matching the existing `dataapi`/`deployment`/`import` behaviour.

#### Leaf commands

Each leaf follows the deployment template: `PreRunE` calls `utils.SetProjectFlagsAsRequired(cfg, cmd)` to mark `--organization-id` / `--project-id` required when no default workspace is set; `RunE` calls `utils.SetProjetDefaults(cfg, organizationId, projectId)` (typo preserved) to fill from the default workspace. `cmd.SilenceUsage = true` is set inside `RunE`. All HTTP calls use `api.MakeRequest(cfg, path, &api.RequestConfig{Method: ..., Version: api.AuraApiVersion2, PostBody: ...})`. Responses are routed through `api.IsSuccessful(statusCode)` before printing.

- REQ-F-003: `list.go` — `GET /organizations/{org}/projects/{proj}/agents`. `Args: cobra.ExactArgs(0)`. On success: `output.PrintRawBody(cmd, cfg, resBody, []string{"id","name","description","dbid","enabled"})`. Flags: `--organization-id` (string), `--project-id` (string), both "(required) ..." in usage.
- REQ-F-004: `get.go` — `GET .../agents/{id}`. `Args: cobra.ExactArgs(1)`. On success: `PrintRawBody` with fields `[id, name, description, dbid, is_private, is_mcp_enabled, enabled]`. Same org/project flags as list.
- REQ-F-005: `create.go` — `POST .../agents`. `Args: cobra.ExactArgs(0)`. `Annotations: map[string]string{"write": "true"}`. Required flags: `--name`, `--description`, `--dbid`, `--tools` (string, inline JSON). Optional flags: `--is-private` (bool, default false), `--system-prompt` (string), `--is-mcp-enabled` (bool, default false), `--enabled` (bool, default true), plus org/project. Request body is the literal field set `{name, description, dbid, is_private, tools, system_prompt, is_mcp_enabled, enabled}` (tools is `[]any` from `json.Unmarshal`). Invalid `--tools` JSON returns `fmt.Errorf("invalid tools JSON: %w", err)`. On success: `PrintRawBody` with the same fields as `get`.
- REQ-F-006: `update.go` — `PATCH .../agents/{id}`. `Args: cobra.ExactArgs(1)`. `Annotations: write=true`. Partial body — only include fields whose flag has been set: for strings, check non-empty (or `cmd.Flags().Changed(...)` for bool flags). Specifically include `is_private`, `is_mcp_enabled`, `enabled` only when `cmd.Flags().Changed(flag)` is true so the user can explicitly send `false`. `--tools` JSON parse error returns the same error string as create. `MarkFlagsOneRequired(name, description, dbid, tools, system-prompt, is-private, is-mcp-enabled, enabled)` so empty PATCH calls fail with cobra's group error. On success: `PrintRawBody` with the same fields as `get`.
- REQ-F-007: `replace.go` — `PUT .../agents/{id}`. `Args: cobra.ExactArgs(1)`. `Annotations: write=true`. Required flags identical to create. Full body always sent (PUT semantics). On success: `PrintRawBody` with the same fields as `get`.
- REQ-F-008: `delete.go` — `DELETE .../agents/{id}`. `Args: cobra.ExactArgs(1)`. `Annotations: write=true`. On success prints `Agent deleted successfully <id>` via `cmd.Println(...)`. No response body parsing.
- REQ-F-009: `invoke.go` — `POST .../agents/{id}/invoke` with `{"input": <input>}`. `Args: cobra.ExactArgs(1)`. `Annotations: write=true`. Required flag `--input` (string). Two output modes:
  - `cfg.Aura.Output() == "json"` → `output.PrintRawBody(cmd, cfg, resBody, nil)` (full response, no field projection).
  - default / table → join `result.Content[*].text` blocks with `"\n"` and `cmd.Println(...)`, then emit a trailing stats line:
    ```
    Status: <STATUS> | End reason: <END REASON> | Tool calls: <N> | Tokens: <req> req / <res> res / <total> total
    ```
    Status and end-reason are uppercased, end-reason underscores become spaces (`end_turn` → `END TURN`). Tool-call counter sums every content block whose `Type` ends in `tool_use` (covers `text_tool_use`, `cypher_template_tool_use`, `text2cypher_tool_use`, similarity-search variants, etc.). Count only — no per-tool breakdown.
- REQ-F-010: `invoke` error mapping:
  - HTTP 403 → `fmt.Errorf("agent invocation forbidden: agent may be disabled or private")` regardless of body.
  - HTTP 200 with response body `type: "error"` → application-level failure. If `error.message` non-empty: `fmt.Errorf("agent invocation failed: %s", message)`; otherwise `fmt.Errorf("agent invocation failed")`.
  - Other non-2xx → propagated unchanged from `api.MakeRequest`.

#### Shared helpers

- REQ-F-011: `neo4j-cli/aura/internal/api/response.go` adds:
  ```go
  // ParseRawBody parses a bare JSON array or object (no {"data":...} envelope).
  func ParseRawBody(body []byte) ResponseData {
      var rawArray []map[string]any
      if err := json.Unmarshal(body, &rawArray); err == nil {
          return NewListResponseData(rawArray)
      }
      var rawObj map[string]any
      if err := json.Unmarshal(body, &rawObj); err == nil {
          return NewSingleValueResponseData(rawObj)
      }
      panic("could not parse raw response body")
  }
  ```
  Panic-on-failure mirrors the existing `ParseBody` shape at `response.go:403`. Neither `ParseBody` nor any existing call site is modified.
- REQ-F-012: `neo4j-cli/aura/internal/output/output.go` adds:
  ```go
  // PrintRawBody prints a bare JSON response (no {"data":...} envelope).
  // Use for APIs like the agent API that return unwrapped JSON.
  func PrintRawBody(cmd *cobra.Command, cfg *clicfg.Config, body []byte, fields []string) {
      if len(body) == 0 {
          return
      }
      if cfg.Aura.Output() == "json" {
          var buf bytes.Buffer
          json.Indent(&buf, body, "", "\t")
          cmd.Println(buf.String())
          return
      }
      PrintBodyMap(cmd, cfg, api.ParseRawBody(body), fields)
  }
  ```
  Existing `PrintBody` is not modified.

#### Examples (test-gate compliance)

- REQ-F-013: Every leaf carries a non-empty `Example:` field whose first line is flush-left (no two-space indent), with ≥3 invocations, `# comment` headers between blocks, `neo4j-cli` prefix on each invocation. Read commands (`list`, `get`) include at least one `--format json` invocation. Write commands (`create`, `update`, `replace`, `delete`, `invoke`) include `--rw` on every actual invocation that is annotated `write=true`. The test gate `TestAllLeafCommands_HaveExamples` at `neo4j-cli/internal/subcommands/agentcontext/agentcontext_test.go:226` enforces the non-empty + flush-left checks; the `--format json` / `--rw` convention follows AGENTS.md guidance ("Cobra Help / Skill Bundle Rendering Notes").

#### Tests

- REQ-F-014: One `_test.go` per leaf colocated in `neo4j-cli/aura/internal/subcommands/agent/`, package `agent_test`, mirroring the leaf test files from `/tmp/cli-61-original-pr.diff`:
  - `list_test.go` — table + JSON output, default-workspace resolution, missing `--organization-id` / `--project-id` errors.
  - `get_test.go` — JSON output, default-workspace resolution, 404 → `Error: [Agent not found]`.
  - `create_test.go` — happy path with all required flags + a few optional, body assertion via `AssertCalledWithBody`, default-workspace variant, missing-required-flag errors.
  - `update_test.go` — single-field PATCH (`--name`), boolean fields (`--is-private --is-mcp-enabled` → body `{"is_mcp_enabled": true, "is_private": true}`), `--tools` happy path, default-workspace variant, `TestUpdateAgentWithNoFields` asserts the `MarkFlagsOneRequired` group error, invalid-tools-JSON error, 404 error.
  - `replace_test.go` — happy path with full body, default-workspace variant, 404.
  - `delete_test.go` — happy path, default-workspace variant, 404.
  - `invoke_test.go` — happy-path JSON output, default-workspace variant, missing `--input` error, HTTP 403 → "agent may be disabled or private" error, HTTP 200 + `type: "error"` application-level error (`Error: agent invocation failed: model context length exceeded`), 404, and a table-output case that asserts the joined text + stats line.
  - `testdata_test.go` — `package agent_test` with `const testTools = `[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]``.
- REQ-F-015: All tests use `testutils.NewAuraTestHelper(t)` and call `helper.SetConfigValue("flag.aura-beta", true)` (NOT `aura.beta-enabled`). JSON-output tests call `helper.SetConfigValue("format", "json")` (NOT `aura.output`). Workspace-default tests call `helper.SetDefaultProjectInConfig(organizationId, projectId)`. Mock URLs are `/v2beta1/organizations/{org}/projects/{proj}/agents[/{id}[/invoke]]` (v2beta1 routing is automatic from `Version: api.AuraApiVersion2`).

#### Skill bundle & changelog

- REQ-F-016: After wiring `agent.NewCmd(cfg)` into `aura.go`, run `go generate ./neo4j-cli/internal/skill/...` to refresh `neo4j-cli/internal/skill/bundle/`. The generator walks `app.NewCmd(cfg)`, so the new `agent` subtree's reference docs (`references/agent.md` and per-leaf entries) are produced automatically. `TestGenerator_RoundTrip` is the gate. Committed bundle files are part of the PR.
- REQ-F-017: A new file `.changes/unreleased/neo4j-cli-Minor-<YYYYMMDD>-<HHMMSS>.yaml` is added with fields:
  ```yaml
  project: neo4j-cli
  kind: Minor
  body: Add aura agent commands (list, get, create, update, replace, delete, invoke) for the Aura Agents API (beta).
  time: <RFC3339 with timezone>
  ```
  Format matches `.changes/unreleased/neo4j-cli-Minor-20260519-093036.yaml`.

### Non-Functional Requirements

- REQ-NF-001: Every new `.go` file begins with the standard Neo4j copyright header (`// Copyright (c) "Neo4j"\n// Neo4j Sweden AB [http://neo4j.com]\n`) — enforced by `make license-check`.
- REQ-NF-002: All gates clean against a fresh checkout:
  - `make test` — including `TestGenerator_RoundTrip` and `TestAllLeafCommands_HaveExamples`.
  - `make fmt-check` — no gofmt drift.
  - `make lint` — golangci-lint v2 (gofmt formatter + linters in `.golangci.yml`).
  - `make generate-check` — no diff after `go generate ./...`.
  - `make license-check` — copyright headers present.
- REQ-NF-003: `flag.aura-beta = false` (the default) keeps the entire `agent` subtree out of `neo4j-cli aura --help`, the agent-context output, and the skill bundle's user-visible surfaces. Toggling the flag back on reveals everything without rebuild.
- REQ-NF-004: No behavioural changes to existing commands or existing tests. The shared helper additions (`ParseRawBody`, `PrintRawBody`) are additive only.

## Technical Considerations

### Sibling templates

- **`deployment/`** (`neo4j-cli/aura/internal/subcommands/deployment/list.go`, `create.go`, `delete.go`) — closest match. Beta-gated, V2 path, same org/project pattern (`SetProjectFlagsAsRequired` + `SetProjetDefaults`), same write-annotation usage. Use as the structural template for every leaf.
- **`dataapi/graphql/`** — also beta but uses instance-id rather than org/project; useful for example formatting and `--format json`/`--rw` convention reference.

### Org/project resolution choice

Two patterns coexist in the codebase:
1. **`SetProjectFlagsAsRequired` + `SetProjetDefaults`** — used by `deployment` (and the original PR).
2. **`ResolveAndValidateOrgProject`** — newer (CLI-120 PR #133), used by `cmek`, `graphanalytics/session`. Adds an API preflight to confirm the project belongs to the org and provides migration hints for the legacy `default-tenant` setting.

We mirror the deployment pattern (option 1), matching the original PR's surface and the closest sibling. Migrating to `ResolveAndValidateOrgProject` later can happen in a single follow-up that flips deployment + agent + import together if desired.

### Response shape isolation

The agent API returns bare JSON (array on `list`, object on `get`/`create`/`update`/`replace`/`invoke`) — no `{"data": ...}` envelope. Existing `output.PrintBody` would format these incorrectly (the embedded `ParseBody` only recognises envelope-wrapped responses). Adding the small additive helpers `ParseRawBody` / `PrintRawBody` keeps the wire-format adapter at the boundary instead of leaking through to every agent leaf. Both helpers consult `cfg.Aura.Output()` so they participate in the existing `--format` / table renderer plumbing without bespoke print code.

### Invoke output design

The invoke response is a structured message body containing `content` blocks (text, tool-use, tool-result), a top-level `status`, `end_reason`, `usage` (request/response/total tokens), and an optional `error` block. The implementation extracts text blocks and counts tool-use blocks (matching `strings.HasSuffix(block.Type, "tool_use")`). The stats line is deterministic and grep-friendly; the JSON-output path preserves the full server response for scripting. Application-level error mode (`type: "error"` on HTTP 200) is surfaced as a `fmt.Errorf` so the CLI exits non-zero and the message renders through the standard CLI-143 error envelope.

### File layout (final)

```
neo4j-cli/aura/internal/subcommands/agent/
  agent.go
  list.go        list_test.go
  get.go         get_test.go
  create.go      create_test.go
  update.go      update_test.go
  replace.go     replace_test.go
  delete.go      delete_test.go
  invoke.go      invoke_test.go
  testdata_test.go
```

Mirrors the one-file-per-leaf rule from AGENTS.md "Cobra Command Layout".

### Critical files to reference

- `neo4j-cli/aura/internal/subcommands/deployment/list.go` — list-leaf template.
- `neo4j-cli/aura/internal/subcommands/deployment/create.go` — write-leaf template with `Annotations: write=true`.
- `neo4j-cli/aura/internal/subcommands/utils/projectflags.go` — `SetProjectFlagsAsRequired` (line 35), `SetProjetDefaults` (line 55).
- `neo4j-cli/aura/internal/api/response.go:403` — `ParseBody` (template for `ParseRawBody`).
- `neo4j-cli/aura/internal/output/output.go:21` — `PrintBody` (template for `PrintRawBody`).
- `neo4j-cli/aura/internal/test/testutils/auratesthelper.go` — `NewAuraTestHelper`, `NewRequestHandlerMock`, `ExecuteCommand`, `AssertCalledWithBody`, `SetConfigValue`, `SetDefaultProjectInConfig`.
- `neo4j-cli/aura/aura.go` — wire-in location (the `if cfg.Flags.Enabled("flag.aura-beta")` block).
- `/tmp/cli-61-original-pr.diff` — verbatim source for every leaf body and test, ported with the adjustments listed above.

## Acceptance Criteria

- [ ] `neo4j-cli/aura/internal/subcommands/agent/` exists with the file layout above and every file carries the Neo4j copyright header.
- [ ] `neo4j-cli aura agent --help` (with `flag.aura-beta=true`) lists `list`, `get`, `create`, `update`, `replace`, `delete`, `invoke`.
- [ ] `neo4j-cli aura agent --help` (with `flag.aura-beta=false`) returns "unknown command" — the subtree is hidden.
- [ ] Every leaf has a non-empty, flush-left `Example:` with ≥3 invocations using `# comment` headers; reads include `--format json`, writes include `--rw`.
- [ ] `agent create`, `agent update`, `agent replace`, `agent delete`, `agent invoke` carry `Annotations: map[string]string{"write": "true"}` and require `--rw` at the prompt.
- [ ] `output.PrintRawBody` and `api.ParseRawBody` exist and are only referenced by the new `agent/*.go` files; no existing call sites changed.
- [ ] `make test` passes — including `TestGenerator_RoundTrip` and `TestAllLeafCommands_HaveExamples`.
- [ ] `make fmt-check`, `make lint`, `make generate-check`, `make license-check` all pass.
- [ ] A `.changes/unreleased/neo4j-cli-Minor-*.yaml` entry exists with the body string above.
- [ ] `neo4j-cli/internal/skill/bundle/references/agent.md` (and any related re-render) is committed.
- [ ] Manual smoke against a real Aura tenant: `aura agent list --format json`, `aura agent get <id> --format json`, `aura agent invoke <id> --input "hello" --rw` each produce sensible output.

## Out of Scope

- Migration to `utils.ResolveAndValidateOrgProject`.
- `--wait` flag on create/update/replace.
- `--tools @file` form.
- Per-tool breakdown in invoke's human stats line.
- Pagination on `list`.
- `--input -` stdin input for `invoke`.
- README / website / `llms.txt` updates beyond the auto-generated skill bundle.
- Touching the `aura.beta-enabled` → `flag.aura-beta` migration shim (already in place).

## Open Questions

- None blocking. Stdin input for `invoke` is a likely follow-up if users ask for it.
