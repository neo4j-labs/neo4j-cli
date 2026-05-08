# PRD: Auto-Save Instance Credentials to DBMS Storage on Create

## Overview

When `aura instance create` successfully provisions an instance, automatically save the returned credentials (username, password, connection URL) to the local DBMS credentials store. Two new flags—`--no-credential-storage` and `--no-credential-print`—give users opt-out control over storage and terminal visibility of the password. A third flag—`--credential-name`—lets users override the auto-generated credential name.

## Goals

- Reduce friction for users who want to immediately use a newly created Aura instance with `neo4j query` or other DBMS commands, without a manual `neo4j credential dbms add` step.
- Provide sensible, safe defaults (auto-save, auto-display) while allowing opt-out for automation or security-sensitive workflows.
- Surface the stored credential name in command output so users and agents know exactly how to reference it.

## Non-Goals

- Modifying credential storage behaviour for any other Aura command (scope is `instance create` only).
- Adding listing or management of credentials via the Aura subcommand (users use `neo4j credential dbms` for that).
- Auto-deleting stored credentials when the corresponding Aura instance is deleted.

## Requirements

### Functional Requirements

- **REQ-F-001**: After a successful `aura instance create` response (HTTP 202 or 200), save the instance credentials to `cfg.Dbms` with:
  - `name`: derived credential name (see REQ-F-002/REQ-F-003)
  - `username`: from response `username`
  - `password`: from response `password`
  - `uri`: from response `connection_url`
  - `database-name`: `"neo4j"` (Go default; do not prompt or expose as a flag)

- **REQ-F-002**: Default credential name is `<instance-id>-default`. If that name is already taken in `DbmsCredentials`, try `<instance-id>-default-1`, `<instance-id>-default-2`, … until a unique name is found.

- **REQ-F-003**: Add a `--credential-name` flag (string) to override the auto-generated base name. Collision handling applies identically: if `<custom-name>` is taken, try `<custom-name>-1`, `<custom-name>-2`, etc.

- **REQ-F-004**: Include a `credential_name` field in the command output showing the name under which the credentials were saved. This field must appear after `username` in the field list.

- **REQ-F-005**: Add `--no-credential-storage` flag (bool, default `false`). When set:
  - Skip saving to dbms storage.
  - Omit the `credential_name` field from output entirely.
  - All other fields (including `password`) are unaffected.

- **REQ-F-006**: Add `--no-credential-print` flag (bool, default `false`). When set:
  - Omit the `password` field from command output.
  - Storage still proceeds normally; `credential_name` still appears in output.

- **REQ-F-007**: `--no-credential-print` affects only the `password` field. It does not suppress `username`, `connection_url`, or `credential_name`.

- **REQ-F-008**: If dbms credential storage fails (e.g., file write error), emit a warning to stderr and exit 0. The instance was already created; emitting an error exit code would mislead callers. The warning message must vary by context:
  - If `--no-credential-print` is **not** set: warn that storage failed and instruct the user to save the printed password before it is lost, as the Aura API does not re-expose the original password after creation.
  - If `--no-credential-print` **is** set: warn that storage failed and the password was not printed; instruct the user to reset the instance password via the Aura console, as there is no way to recover the original password.

- **REQ-F-009**: `--no-credential-storage` and `--no-credential-print` may be combined freely; each flag controls its own orthogonal concern.

- **REQ-F-012**: Using `--credential-name` together with `--no-credential-storage` is a user error. The command must validate this combination in `PreRunE` and return an error before making any API call. The error message should be clear that `--credential-name` has no effect when `--no-credential-storage` is set.

- **REQ-F-013**: If `--credential-name` is explicitly provided, it must be a non-empty string. Validate in `PreRunE` and return an error if the value is empty. No other format constraints apply.

- **REQ-F-014**: When `--no-credential-storage` is not set, validate in `PreRunE` that `cfg.Credentials.Dbms` is non-nil and return a clear error if it is. Under normal operation `Credentials.load()` always initialises `Dbms` to a non-nil pointer, so nil indicates a corrupted or manually edited credentials file. If nil is somehow encountered later (in `RunE`, as a defence-in-depth fallback), treat it as a soft failure: emit a warning to stderr and continue without storing, consistent with REQ-F-008.

- **REQ-F-010**: Credential storage occurs immediately after the successful create response, before any `--await` polling begins, so that credentials are persisted even if the user cancels during polling.

- **REQ-F-011**: The `--await` output only emits the instance status string ("Instance Status: running"), not the full credential fields; `--no-credential-print` therefore has no effect on `--await` output beyond what is already handled at step REQ-F-010.

### Non-Functional Requirements

- **REQ-NF-001**: Use `cfg.Dbms.Add(name, username, password, databaseName, uri)` for storage, consistent with `neo4j credential dbms add`.
- **REQ-NF-002**: All existing `create` command tests must continue to pass.
- **REQ-NF-003**: New tests must be table-driven and cover: default name generation, collision resolution, custom `--credential-name`, `--no-credential-storage`, `--no-credential-print`, both flags together, and storage failure warning.
- **REQ-NF-004**: `make test`, `make fmt-check`, and `make lint` must pass.

## Technical Considerations

### Config Access
`NewCreateCmd` already receives `*clicfg.Config`, so `cfg.Dbms` is directly accessible without any signature change.

### Output Field Injection
`credential_name` is not part of the API response and must be injected synthetically. The codebase already has an established pattern for this — `tenant/get.go` uses `postProcessResponseValues` to inject a computed `metrics_integration_url` field — and the same approach should be used here:

1. Parse `resBody` via `api.ParseBody(resBody)` to get a `ResponseData`.
2. Extract the single data map: `data, err := responseData.GetSingleOrError()`.
3. Compute the credential name and run storage (see REQ-F-001 through REQ-F-003).
4. Inject the synthetic field: `data["credential_name"] = resolvedName` (only when `--no-credential-storage` is not set).
5. Wrap back: `augmented := api.NewSingleValueResponseData(data)`.
6. Render: `output.PrintBodyMap(cmd, cfg, augmented, fields)` with the conditionally built `fields` slice.

This replaces the current `output.PrintBody(cmd, cfg, resBody, fields)` call and means the `credential_name` field flows correctly through all output formats (table, JSON, toon) without any raw byte re-marshalling. `MarshalJSON` does not need to be implemented on any new type — `SingleValueResponseData` uses default struct encoding and `AsArray()` already returns the injected field in the map.

### Flag Combination Validation
Add to `PreRunE` before the existing type/version checks:
```go
if cmd.Flags().Changed(credentialNameFlag) && noCredentialStorage {
    return fmt.Errorf("--credential-name cannot be used with --no-credential-storage")
}
```
Use `cmd.Flags().Changed(credentialNameFlag)` (not `credentialName != ""`) so the check only fires when the user explicitly passed the flag, not when the default empty string is in effect.

### Conditional Field List
Build the `fields []string` slice dynamically in `RunE`:
```go
fields := []string{"id", "name", "tenant_id", "connection_url", "username"}
if !noCredentialPrint {
    fields = append(fields, "password")
}
if !noCredentialStorage {
    fields = append(fields, "credential_name")
}
fields = append(fields, "cloud_provider", "region", "type")
```

### Collision Detection
```go
candidateName := baseCredentialName(instanceId, credentialName)
for {
    if _, err := cfg.Dbms.Get(candidateName); err != nil { // not found → unique
        break
    }
    candidateName = nextName(candidateName, counter)
    counter++
}
```

### Test Helper Wiring
Existing tests use `testutils.NewAuraTestHelper` which wires `*clicfg.Config`. Dbms credential assertions can call `helper.Config.Dbms.Get(name)` to verify storage. Use an in-memory FS (`testfs.GetTestFs`) so tests do not touch the real credentials file on the developer's machine (per the existing hermetic test conventions).

### Updated Output Field Order
```
id, name, tenant_id, connection_url, username, [password], [credential_name], cloud_provider, region, type
```
Fields in `[...]` are present only when the corresponding flag is not set.

## Acceptance Criteria

- [ ] Default run: credentials saved as `<id>-default`, `credential_name` appears in output, `password` visible.
- [ ] Collision: second create with same instance ID uses `<id>-default-1`.
- [ ] `--credential-name foo`: saved as `foo`; collision → `foo-1`.
- [ ] `--no-credential-storage`: no credential saved, `credential_name` absent from output, `password` visible (unless `--no-credential-print` is set).
- [ ] `--no-credential-print`: `password` absent from output, credential saved normally, `credential_name` visible (unless `--no-credential-storage` is set).
- [ ] Both `--no-credential-storage --no-credential-print`: no storage, no `credential_name`, no `password`.
- [ ] Storage failure (without `--no-credential-print`): warning emitted to stderr advising user to save the printed password; command exits 0.
- [ ] Storage failure + `--no-credential-print`: warning emitted to stderr advising user to reset the password via the Aura console; command exits 0.
- [ ] `--await`: polling behaves as before; storage has already occurred before polling starts.
- [ ] `--credential-name` + `--no-credential-storage`: command errors in `PreRunE` before any API call, with a clear message.
- [ ] `--credential-name ""` (explicit empty string): command errors in `PreRunE` before any API call.
- [ ] `cfg.Credentials.Dbms == nil` without `--no-credential-storage`: command errors in `PreRunE` before any API call.
- [ ] All existing `create` tests pass.
- [ ] New table-driven tests cover all scenarios above.
- [ ] `make test`, `make fmt-check`, `make lint` pass.
- [ ] Changelog entry added (new feature visible to CLI users).

## Out of Scope

- Auto-deleting dbms credentials when `aura instance delete` is called.
- Credential name flag on any command other than `instance create`.
- Validating the `--credential-name` value format (no known constraints on `DbmsCredential.Name`).

## Open Questions

None.
