# PRD: CLI-49 Keyring Structural Fixes

## Overview

The keyring credential storage implementation (`common/clicfg/credentials/`) is functionally correct but has six structural issues identified in a post-merge review. This PRD covers refactoring `credentials.go` and related call sites to eliminate temporal-coupling hacks, duplicated iteration patterns, direct `os.Stderr` writes, and a misleading default value — without changing observable behaviour.

## Goals

- Eliminate the temporary `storageMode` mutation in `MigrateToInsecure`.
- Replace 30-line scrubbed-snapshot boilerplate in `saveWithKeyring` with a zero-restore pattern.
- Extract per-type keyring methods on `AuraCredentials`, `DbmsCredentials`, `EmbedCredentials` so that the five triple-iteration groups in `credentials.go` become three-line call chains.
- Thread `io.Writer` through `SetStorageMode` / `loadSensitiveFieldsFromKeyring` so warnings go to `cmd.ErrOrStderr()` at cobra call sites and `os.Stderr` at initialization.
- Align `GlobalConfig.CredentialStorage()` default with the actual on-disk default (`"insecure"`).
- Make `MigrateToInsecure` own its own `storageMode` transition so callers don't need a follow-up `SetStorageMode` call.

## Non-Goals

- No changes to the keyring feature's observable behaviour (migration flows, error messages, exit codes).
- No new credential types or new keyring fields.
- No changes to `common/confirm/` or any cobra command logic beyond threading the `io.Writer`.

## Requirements

### Functional Requirements

- **REQ-F-001**: Extract `writeToKeyring(provider KeyringProvider) error`, `readFromKeyring(provider KeyringProvider) error`, and `zeroSensitiveFields()` methods on `AuraCredentials`, `DbmsCredentials`, and `EmbedCredentials`. Each method encapsulates all fields for that type (e.g., `AuraCredentials.writeToKeyring` handles `client-secret` and `access-token`; `AuraCredentials.readFromKeyring` handles both with the appropriate required/optional semantics for `loadSensitiveFieldsFromKeyring`'s auto-migrate path). The `defaultKeyring` package var is passed in so tests can inject a mock without package-level mutation.

- **REQ-F-002**: Replace the five groups of `for _, cred := range c.{Aura,Dbms,Embed}.Credentials` loops in `credentials.go` with calls to the per-type methods introduced in REQ-F-001. Each function's body shrinks to three calls (one per credential type). The `MigrateToKeyring` rollback logic stays in `credentials.go` since it is cross-type.

- **REQ-F-003**: Extract a private `saveToJSON() error` method on `Credentials` containing the insecure branch of `save()` (the `json.Marshal` + `fileutils.WriteFile` + return path). `save()` calls `saveToJSON()` for the insecure path. `MigrateToInsecure` calls `saveToJSON()` directly instead of the current temporary `storageMode` mutation.

- **REQ-F-004**: Replace the scrubbed-snapshot boilerplate in `saveWithKeyring` (lines 232–262: three struct copies with zeroed fields) with a zero-restore pattern: zero sensitive fields in-place on the in-memory structs, call `saveToJSON()`, then restore the original values. This collapses ~30 lines to ~10 and removes the per-type snapshot structs.

- **REQ-F-005**: Add an `io.Writer` parameter to `SetStorageMode(mode string, warnW io.Writer) error` and `loadSensitiveFieldsFromKeyring(warnW io.Writer) error`. All `fmt.Fprintf(os.Stderr, ...)` calls in `loadSensitiveFieldsFromKeyring` are replaced with writes to `warnW`. Call sites:
  - `common/clicfg/clicfg.go` (`NewConfig`): pass `os.Stderr`.
  - `neo4j-cli/app/app.go` (`initCredentialStorageDefault`): pass the existing `stderr io.Writer` param.
  - `neo4j-cli/internal/subcommands/config/set.go`: pass `cmd.ErrOrStderr()`.
  - All test call sites: pass `&bytes.Buffer{}` and assert warning text when exercising warning paths.

- **REQ-F-006**: Change `GlobalConfig.CredentialStorage()` to return `StorageModeInsecure` as the default (instead of `"keyring"`) when `credential-storage` is not set in viper. This matches the actual boot-time default set in `NewCredentials()`. Add a comment explaining that the real first-run default decision (keyring vs. insecure) lives in `initCredentialStorageDefault`; `CredentialStorage()` is only the read accessor for the persisted value.

- **REQ-F-007**: `MigrateToInsecure` sets `c.storageMode = StorageModeInsecure` immediately before calling `saveToJSON()` (after Phase 1 completes). Remove the doc comment that transfers this responsibility to callers. The caller in `config/set.go` no longer needs to call `SetStorageMode` after `MigrateToInsecure`; the `credentialStorageModeChanged` flag and the post-migration `SetStorageMode` call in `set.go` can be simplified accordingly.

### Non-Functional Requirements

- **REQ-NF-001**: `make test`, `make lint`, and `make fmt-check` must all pass after the refactor.
- **REQ-NF-002**: No new exported symbols except the per-type keyring methods in REQ-F-001 (which must be exported since they are called from `credentials.go` in the same package — they may remain unexported).
- **REQ-NF-003**: All existing tests in `credentials_keyring_test.go` must continue to pass. Any test that previously captured `os.Stderr` output for warning assertions must be updated to assert against the `io.Writer` passed to `SetStorageMode`.

## Technical Considerations

- **Per-type method signatures**: The `readFromKeyring` method on each type needs different required/optional semantics depending on which function is calling it (`loadSensitiveFieldsFromKeyring` does warn-and-continue on missing required fields; `MigrateToInsecure` errors on missing required fields). Consider separate `loadFromKeyring(provider, warnW)` (for startup) and `migrateFromKeyring(provider)` (for explicit migration) if the semantics diverge enough to make a shared method's signature awkward. Alternatively, pass an `onMissing` function or a boolean `strict bool` to a single method. Choose whichever keeps the per-type files readable.

- **Zero-restore in `saveWithKeyring`**: The zero-restore pattern (`defer func() { cred.Field = saved }()` or explicit pre/post) must ensure the restore happens even if `saveToJSON()` panics (which it does on marshal errors). Use explicit save-zero-restore without defer if the panic path is acceptable to leave fields zeroed (consistent with current behaviour on marshal panic). Using `defer` for restore is safer but adds complexity; the current code also panics on marshal error in both paths so not restoring on panic is acceptable.

- **`config/set.go` simplification**: After REQ-F-007, the `credentialStorageModeChanged` boolean and the `if credentialStorageModeChanged { cfg.Credentials.SetStorageMode(value) }` block can be removed for the `MigrateToInsecure` path. The `MigrateToKeyring` path still needs a `SetStorageMode` call after persisting the config key (to flip the in-process mode to keyring so subsequent saves go to the keyring). Keep only that call.

- **Test updates**: `TestSetStorageMode_KeyringMode_MissingRequired_Warn` in `credentials_keyring_test.go` (and any other test exercising warning paths) must pass a `*bytes.Buffer` as `warnW` and assert `strings.Contains(buf.String(), "Warning:")`. Tests that previously used `os.Stderr` capture can be simplified.

## Acceptance Criteria

- [ ] `AuraCredentials`, `DbmsCredentials`, and `EmbedCredentials` each have unexported keyring methods (`writeToKeyring`, `readFromKeyring` or equivalent) that encapsulate their sensitive fields.
- [ ] `credentials.go` has no group of three consecutive `for _, cred := range c.{Aura,Dbms,Embed}.Credentials` loops; each is replaced by a three-call chain to per-type methods.
- [ ] A private `saveToJSON()` method exists on `Credentials`; `save()` and `MigrateToInsecure` both call it.
- [ ] `saveWithKeyring` contains no snapshot-struct construction (`auraSnap`, `dbmsSnap`, `embedSnap` variables are gone).
- [ ] `MigrateToInsecure` contains no temporary `storageMode` mutation (`prevMode`/restore pattern is gone).
- [ ] `SetStorageMode` and `loadSensitiveFieldsFromKeyring` accept an `io.Writer`; no `os.Stderr` reference remains in `credentials.go`.
- [ ] `GlobalConfig.CredentialStorage()` returns `StorageModeInsecure` as the default.
- [ ] `MigrateToInsecure` sets `c.storageMode = StorageModeInsecure` before saving; `config/set.go`'s post-migration `SetStorageMode` call for the insecure path is removed.
- [ ] All call sites pass `cmd.ErrOrStderr()` (cobra commands) or `os.Stderr` (initialization) to `SetStorageMode`.
- [ ] `make test`, `make lint`, `make fmt-check` pass.

## Out of Scope

- Changes to `common/confirm/`, `keyring_hint.go`, `keyring.go`, or any e2e test.
- Changing error messages, exit codes, or migration semantics.
- Adding or removing credential types or sensitive fields.

## Open Questions

None.
