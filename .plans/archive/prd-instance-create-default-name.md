# PRD: Instance Create Default Name Generation

## Overview

Make `--name` an optional flag for `instance create`. When omitted, the CLI generates a default name following the pattern `Instance01`, `Instance02`, etc., incrementing until a name not already used within the target tenant is found. This mirrors the behaviour of the Aura web UI.

## Goals

- Remove the friction of having to supply `--name` for every `instance create` invocation.
- Generate deterministic, human-readable default names that avoid collisions with existing instances in the same tenant.
- Keep the generated name fully visible to the user via the existing `name` output field — no extra UI work required.

## Non-Goals

- Server-side name uniqueness enforcement — this is a client-side convenience feature only.
- Changing the name format for non-default names supplied via `--name`.
- Checking name uniqueness across tenants — only the resolved tenant is checked.
- Retrying after a race condition where two concurrent creates pick the same default name simultaneously.

## Requirements

### Functional Requirements

- REQ-F-001: `--name` MUST become optional. The flag description should be updated to remove the `(required)` prefix and clarify it is optional with auto-generation behaviour.
- REQ-F-002: When `--name` is not provided, the CLI MUST list all instances in the resolved tenant before submitting the create request.
- REQ-F-003: The CLI MUST select the lowest positive integer `N` such that `InstanceNN` does not match the name of any existing instance in that tenant (case-insensitive comparison).
- REQ-F-004: The generated name MUST be zero-padded to two digits for `N` in the range 1–99 (e.g., `Instance01`, `Instance09`, `Instance10`, `Instance99`). For `N >= 100` the decimal representation is used without additional padding (e.g., `Instance100`, `Instance101`).
- REQ-F-005: The tenant used for the uniqueness check MUST be resolved using the same logic as the rest of the command — `--tenant-id` flag value if provided, otherwise the default tenant from configuration.
- REQ-F-006: If the list-instances API call fails during name generation, the command MUST return an error and abort, with an error message consistent in style with a failed create request (i.e., propagate the API error).
- REQ-F-007: The resolved (or user-supplied) name MUST appear in the command output via the existing `name` output field — no additional output changes are required.

### Non-Functional Requirements

- REQ-NF-001: The name generation logic MUST be covered by unit tests, including: no existing instances, some names taken (non-contiguous gaps), all two-digit names taken (rolls to three digits), and list API failure.
- REQ-NF-002: The additional list API call MUST only be made when `--name` is not supplied; it MUST NOT occur when the user provides an explicit name.
- REQ-NF-003: Existing tests for `instance create` MUST continue to pass. All new tests MUST follow the table-driven pattern used in `create_test.go`.

## Technical Considerations

### Current implementation

- `--name` is declared with `cmd.MarkFlagRequired(nameFlag)` in `create.go` (line 234). Removing this mark makes the flag optional.
- The `RunE` body already resolves `tenantID` early in execution; the name-generation step slots in immediately after that resolution, before the body map is assembled.
- The list-instances endpoint is `GET /instances` with an optional `tenantId` query parameter — already used by `list.go`. Reuse `api.MakeRequest` with `Method: http.MethodGet` and `QueryParams: map[string]string{"tenantId": tenantID}`.
- The response is a `ListResponseData` with each element containing at minimum a `name` field (string). Parse with `api.ParseBody` as already done in `list.go`.

### Name generation algorithm

```
func defaultInstanceName(existingNames []string) string {
    taken := make(map[string]bool, len(existingNames))
    for _, n := range existingNames {
        taken[strings.ToLower(n)] = true
    }
    for i := 1; ; i++ {
        var candidate string
        if i < 100 {
            candidate = fmt.Sprintf("Instance%02d", i)
        } else {
            candidate = fmt.Sprintf("Instance%d", i)
        }
        if !taken[strings.ToLower(candidate)] {
            return candidate
        }
    }
}
```

This function can live in a new file `name_helpers.go` (alongside `credential_helpers.go`) with a corresponding `name_helpers_test.go`.

### Tenant resolution ordering

The tenant resolution block already exists in `RunE`. The name-generation step reads the already-resolved `tenantID` value — no changes to tenant resolution logic are needed.

### Flag description update

Change the `--name` flag usage string from:

> `(required) The name of the instance (any UTF-8 characters with no trailing or leading whitespace).`

to:

> `The name of the instance (any UTF-8 characters with no trailing or leading whitespace). Defaults to the next available InstanceNN name within the tenant.`

## Acceptance Criteria

- [ ] `neo4j-cli aura instance create --type free-db` (no `--name`) succeeds and outputs a name matching `Instance\d+`.
- [ ] When `Instance01` already exists, the auto-generated name is `Instance02`.
- [ ] When `Instance01`–`Instance99` all exist, the auto-generated name is `Instance100`.
- [ ] When `--name my-instance` is provided, no list-instances API call is made.
- [ ] If the list-instances call fails, the command exits with a non-zero status and a meaningful error message.
- [ ] `make test`, `make fmt-check`, and `make lint` all pass with no regressions.
- [ ] `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...` is re-run and the resulting bundle changes (updated flag description) are committed alongside the implementation.

## Out of Scope

- Server-side deduplication or reserving names.
- Handling race conditions where two concurrent CLI invocations pick the same default name.
- Changing the auto-naming format (e.g., using tenant name as prefix).
- Any changes to `instance update` or other commands.

## Open Questions

None — all clarifying questions resolved.
