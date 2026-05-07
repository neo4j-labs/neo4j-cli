# PRD: Aura --credential Flag

## Overview

Add a `--credential <name>` flag to all Aura resource commands (`instance`, `tenant`, `deployment`, `dataapi`, `graphanalytics`, `customermanagedkey`, `import`) so users with multiple stored Aura credentials can select one by name at invocation time, without changing the stored default. The implementation is centralised: a single helper function adds the flag and hook to any resource parent command, and `MakeRequest()` resolves the active credential by name at request time.

## Goals

- Let users override the active Aura credential per-invocation without changing the stored default.
- Keep the implementation centralized — one helper, one `MakeRequest` change, one line per resource parent.
- Maintain identical behaviour for users who do not pass `--credential`.
- Apply only to commands that make Aura API calls; exclude `credential` and `config` management commands.

## Non-Goals

- Changing how the default credential is selected or stored.
- Any other changes to `neo4j-cli query` beyond adding the `-c` shorthand.
- Adding `--credential` to `credential` or `config` subcommands.
- Support for credential names with spaces (shell quoting already handles this).

## Requirements

### Functional Requirements

- REQ-F-001: The following Aura resource parent commands gain a new optional persistent flag `--credential <name>` (shorthand `-c`): `instance`, `tenant`, `deployment`, `dataapi`, `graphanalytics`, `customermanagedkey`, `import`. All their leaf commands inherit it automatically.
- REQ-F-006: The existing `--credential` flag on `neo4j-cli query` gains the `-c` shorthand. The flag behaviour is otherwise unchanged.
- REQ-F-002: When `--credential <name>` is provided, `MakeRequest()` uses the named Aura credential from `credentials.json` instead of the default credential.
- REQ-F-003: When `--credential <name>` is provided and the named credential does not exist, the command must error before making any API call with a message that includes the credential name.
- REQ-F-007: The "credential not found" error hint must reference the correct binary based on the entry point: `aura-cli credential list` when running as standalone `aura-cli`; `neo4j-cli aura credential list` when running as `neo4j-cli aura`. The entry point is determined by `cmd.Root().Use` (`"aura-cli"` vs `"neo4j-cli"`).
- REQ-F-004: When `--credential` is not provided, `MakeRequest()` falls back to `GetDefault()` — existing behaviour is unchanged.
- REQ-F-005: Works identically for both `aura-cli <resource>` (standalone) and `neo4j-cli aura <resource>` entry points.

### Non-Functional Requirements

- REQ-NF-001: All new/modified Go source files carry the Neo4j copyright header.
- REQ-NF-002: `make test`, `make lint`, and `make fmt-check` pass with no failures.
- REQ-NF-003: Skill bundles are regenerated (`go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`) since command flag lists change.
- REQ-NF-004: A changelog entry is created for both `aura-cli` and `neo4j-cli` using `changie new --projects aura-cli --projects neo4j-cli`.

## Technical Considerations

### Flag helper

Add `RegisterAuraCredentialFlag(cmd *cobra.Command, cfg *clicfg.Config)` to `common/flags/flags.go` (alongside the existing `RegisterOutputFlag`). The helper:

1. Adds `--credential` / `-c` as a persistent string flag on `cmd` with an empty default.
2. Captures any existing `cmd.PersistentPreRunE` already set on `cmd`, then replaces it with a new hook that:
   a. Runs the captured hook first (if non-nil).
   b. If the `credential` flag was changed, resolves the named credential via `cfg.Credentials.Aura.Get(name)`. On failure, returns a context-aware error: inspect `cmd.Root().Use` — if `"aura-cli"` hint `aura-cli credential list`; if `"neo4j-cli"` hint `neo4j-cli aura credential list`.
   c. On success, stores the resolved `*AuraCredential` in `cfg.Aura` via `cfg.Aura.SetActiveCredential(credential)`.

Performing validation in `PersistentPreRunE` (rather than inside `MakeRequest()`) is necessary because `MakeRequest()` receives no `*cobra.Command` and therefore cannot determine the binary name for the hint message.

Each resource parent calls `RegisterAuraCredentialFlag(cmd, cfg)` at the end of its `NewCmd()` — after the command struct (and its inline `PersistentPreRunE`) has been created. This ensures the wrapper captures the already-set hook.

### AuraConfig changes

Add two methods to `AuraConfig` in `common/clicfg/clicfg.go`:

- `SetActiveCredential(cred *credentials.AuraCredential)` — stores a pointer to the already-resolved credential in a new unexported runtime field `activeCredential *credentials.AuraCredential`. Called by the `PersistentPreRunE` hook after successful lookup.
- `ActiveCredential() *credentials.AuraCredential` — returns the runtime field value (`nil` when not set).

The `activeCredential` field is never persisted to viper or disk — it is a pure runtime value set once per invocation, analogous to how `BindBaseUrl` works. Storing the resolved pointer (rather than just a name) avoids a second credential lookup in `MakeRequest()` and keeps all error-handling in one place.

### MakeRequest() changes

In `neo4j-cli/aura/internal/api/api.go`, replace the current `cfg.Credentials.Aura.GetDefault()` call with:

```go
var credential *credentials.AuraCredential
if active := cfg.Aura.ActiveCredential(); active != nil {
    credential = active
} else {
    credential, err = cfg.Credentials.Aura.GetDefault()
    if err != nil {
        return responseBody, 0, err
    }
}
```

No error handling is needed for the `ActiveCredential()` branch — validation and the context-aware error message were already handled in `PersistentPreRunE`.

### Resource parents to update

Call `RegisterAuraCredentialFlag(cmd, cfg)` at the end of `NewCmd()` in each of:

- `neo4j-cli/aura/internal/subcommands/instance/instance.go`
- `neo4j-cli/aura/internal/subcommands/tenant/tenant.go`
- `neo4j-cli/aura/internal/subcommands/deployment/deployment.go`
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/dataapi.go` (or the `dataapi` parent, whichever registers the `PersistentPreRunE`)
- `neo4j-cli/aura/internal/subcommands/graphanalytics/graphanalytics.go`
- `neo4j-cli/aura/internal/subcommands/customermanagedkey/customermanagedkey.go`
- `neo4j-cli/aura/internal/subcommands/import/import.go`

`credential/` and `config/` are deliberately excluded.

### query --credential shorthand

In `neo4j-cli/query/query.go`, change the existing `cmd.PersistentFlags().String("credential", ...)` call to `cmd.PersistentFlags().StringP("credential", "c", ...)`. No other changes to the query package are needed. The skill bundle for `neo4j-cli` must be regenerated since the flag help text changes.

### Test coverage

- Unit tests for `SetActiveCredential` / `ActiveCredential` in `common/clicfg/`.
- Unit tests for the `PersistentPreRunE` hook in `RegisterAuraCredentialFlag`: named credential found (stored on cfg), named credential not found as standalone (`aura-cli` hint in error), named credential not found as sub-command (`neo4j-cli aura` hint in error), no flag set (cfg unchanged).
- Unit tests for `MakeRequest` credential resolution: active credential set (used directly), no active credential (falls back to `GetDefault()`).
- At least one leaf command test per resource parent (e.g. `instance list`) asserting that `--credential myname` is accepted and forwarded, using the existing mock HTTP server pattern from `testutils`.

## Acceptance Criteria

- [ ] `aura-cli instance list --credential myname` uses the `myname` credential for the API call.
- [ ] `neo4j-cli aura instance list --credential myname` works identically.
- [ ] `aura-cli instance list --credential unknown` errors with a message containing `"unknown"` and a hint referencing `aura-cli credential list`.
- [ ] `neo4j-cli aura instance list --credential unknown` errors with a message containing `"unknown"` and a hint referencing `neo4j-cli aura credential list`.
- [ ] `aura-cli instance list` (no flag) behaves identically to before this change.
- [ ] All seven resource parents (`instance`, `tenant`, `deployment`, `dataapi`, `graphanalytics`, `customermanagedkey`, `import`) have the flag with `-c` shorthand; `credential` and `config` do not.
- [ ] `aura-cli instance list -c myname` is equivalent to `--credential myname`.
- [ ] `neo4j-cli query "RETURN 1" -c mydb` is accepted and equivalent to `--credential mydb`.
- [ ] `make test`, `make lint`, and `make fmt-check` pass.
- [ ] `make generate-check` exits 0 (bundles regenerated).
- [ ] Changelog entries present for both `aura-cli` and `neo4j-cli`.

## Out of Scope

- `--credential` on `credential` or `config` subcommands.
- Changes to `neo4j-cli query --credential`.
- UI for switching the stored default credential.

## Open Questions

- None.
