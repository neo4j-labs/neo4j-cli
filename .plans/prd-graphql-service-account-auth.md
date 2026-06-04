# PRD: GraphQL Data API Service Account Authentication

## Overview

The Aura GraphQL Data API backend no longer requires a database username and password for instance authentication. Instead, it uses a service account model with two permission levels: `read_only` and `read_write`. The CLI must drop the `--instance-username` / `--instance-password` flags from `graphql create` and `graphql update`, and replace them with a single `--service-account` flag that accepts `read_only` or `read_write`.

## Goals

- Remove `--instance-username` and `--instance-password` from `graphql create` and `graphql update`.
- Add `--service-account` flag (values: `read_only` | `read_write`) to both commands.
- On `create`, `--service-account` is optional and defaults to `read_write`.
- On `update`, `--service-account` is optional and omitting it leaves the service account unchanged.
- Keep all other flags, request/response behaviour, and polling logic intact.

## Non-Goals

- No changes to `graphql get`, `graphql list`, `graphql delete`, `graphql pause`, `graphql resume`.
- No changes to `auth-provider` or `cors-policy` subcommands.
- No changes to API endpoint paths or response field names.
- No support for username/password as a fallback or hidden flag.

## Requirements

### Functional Requirements

- REQ-F-001: `graphql create` MUST NOT accept `--instance-username` or `--instance-password`. Passing either flag must produce an "unknown flag" error.
- REQ-F-002: `graphql create` MUST accept `--service-account <value>` where `<value>` is `read_only` or `read_write`. Any other value must produce a validation error.
- REQ-F-003: On `graphql create`, `--service-account` is optional and defaults to `read_write` when omitted.
- REQ-F-004: The `create` request body must include `"aura_instance": {"service_account": "<value>"}` instead of the old `{"username": ..., "password": ...}` form.
- REQ-F-005: `graphql update` MUST NOT accept `--instance-username` or `--instance-password`.
- REQ-F-006: `graphql update` MUST accept `--service-account <value>` (same valid values). When provided, send `"aura_instance": {"service_account": "<value>"}` in the PATCH body. When omitted, the `aura_instance` key must be absent from the body entirely (do not send an empty object).
- REQ-F-007: All `Example:` fields in `create.go` and `update.go` must be updated to reflect the new flag surface (no username/password, include `--service-account` in at least one example).

### Non-Functional Requirements

- REQ-NF-001: The skill bundle must be regenerated (`go generate ./neo4j-cli/internal/skill/...`) so `TestGenerator_RoundTrip` passes.
- REQ-NF-002: All existing tests must be updated to match the new flag/body shape; no test should reference `instance-username` or `instance-password`.
- REQ-NF-003: A changelog entry (kind `Minor`) is required — this is a user-facing breaking change (flag removal).

## Technical Considerations

### `create.go` changes

Remove:
- `instanceUsernameFlag`, `instancePasswordFlag` constants
- `instanceUsername`, `instancePassword` variables
- `cmd.Flags().StringVar(...)` registrations for both flags
- `cmd.MarkFlagRequired(instanceUsernameFlag)` and `cmd.MarkFlagRequired(instancePasswordFlag)`
- The `"username"` / `"password"` entries in the `aura_instance` map in the request body

Add:
- `serviceAccountFlag = "service-account"` constant
- `serviceAccount string` variable, default `"read_write"`
- `cmd.Flags().StringVar(&serviceAccount, serviceAccountFlag, "read_write", "...")`
- Enum validation: reject values other than `read_only` / `read_write` (use `cobra.OnlyValidArgs` or a manual check in `RunE` before `SilenceUsage = true`)
- In the request body: `"aura_instance": map[string]string{"service_account": serviceAccount}`

### `update.go` changes

Remove:
- `instanceUsernameFlag`, `instancePasswordFlag` constants
- `instanceUsername`, `instancePassword` variables
- `cmd.Flags().StringVar(...)` registrations for both
- The `if instanceUsername != "" || instancePassword != ""` block that built `aura_instance`

Add:
- `serviceAccountFlag = "service-account"` constant
- `serviceAccount string` variable (empty default)
- `cmd.Flags().StringVar(&serviceAccount, serviceAccountFlag, "", "...")`
- Enum validation for non-empty value (same `read_only` / `read_write` check)
- In `RunE`: `if serviceAccount != "" { body["aura_instance"] = map[string]string{"service_account": serviceAccount} }`

### Test file changes

`create_test.go`:
- Remove all `instanceUsername` / `instancePassword` variables and references
- Update flag-validation test cases: remove tests for missing username/password, add a test for invalid `--service-account` value
- Update `executeCommand` strings: remove `--instance-username`/`--instance-password`, add `--service-account read_write` (or omit to use default)
- Update `expectedRequestBody` strings: replace `"username":"neo4j","password":"dfjglhssdopfrow"` with `"service_account":"read_write"`

`update_test.go`:
- Remove `instanceUsername` / `instancePassword` variables and references
- Remove the test case for updating instance credentials via username/password
- Add a test case for updating with `--service-account read_only`
- Update request body assertions accordingly

### Enum validation approach

Cobra doesn't enforce enum values for `StringVar` flags natively. Use a manual check at the top of `RunE` (before `SilenceUsage = true`) for `update`. For `create`, the default means the flag is always non-empty, so validate unconditionally. A shared constant or small helper is reasonable if it avoids duplication between the two files, but a local check in each `RunE` is also acceptable given the project pattern.

## Acceptance Criteria

- [ ] `graphql create --instance-id X --name N --type-definitions T --rw` succeeds with default `read_write` service account sent in body.
- [ ] `graphql create ... --service-account read_only --rw` sends `"service_account":"read_only"` in body.
- [ ] `graphql create ... --service-account invalid` returns a validation error before making any API call.
- [ ] `graphql create ... --instance-username neo4j` returns "unknown flag" error.
- [ ] `graphql update <id> --instance-id X --service-account read_only --rw` sends `"aura_instance":{"service_account":"read_only"}` in PATCH body.
- [ ] `graphql update <id> --instance-id X --name new-name --rw` sends no `aura_instance` key in body.
- [ ] `graphql update <id> --instance-id X --instance-username neo4j` returns "unknown flag" error.
- [ ] `make test` passes with zero failures.
- [ ] `make fmt-check`, `make lint`, `make license-check` all pass.
- [ ] `TestGenerator_RoundTrip` passes (skill bundle regenerated).
- [ ] Changelog entry present under `.changes/unreleased/`.

## Out of Scope

- Supporting username/password as a hidden or deprecated flag.
- Adding `--service-account` to any command other than `create` and `update`.
- Changing the `--data-api-id` flag or any other flag on sub-tree commands.
- Any API response field changes.

## Open Questions

None — requirements are fully resolved.
