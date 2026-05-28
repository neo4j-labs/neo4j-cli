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
- Unify the remaining triple loops in `saveWithKeyring` and `MigrateToKeyring` behind a `keyringCredential` interface and a single `allCredentials()` iterator, so adding a new credential type requires only implementing the interface — not touching `credentials.go`.

## Non-Goals

- No changes to the keyring feature's observable behaviour (migration flows, error messages, exit codes).
- No new credential types or new keyring fields.
- No changes to `common/confirm/` or any cobra command logic beyond threading the `io.Writer`.
- The `keyringCredential` interface is package-private (unexported); no new exported API surface.

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

- **REQ-F-008**: Define an unexported `keyringCredential` interface in `common/clicfg/credentials/keyring.go`. The interface must contain exactly the methods that `credentials.go` needs to call on any individual credential, regardless of its concrete type:
  - `writeToKeyring(provider KeyringProvider, written *[]string) error` — writes non-empty sensitive fields to the keyring, appending each successfully-written key to `written` when `written != nil`. `saveWithKeyring` passes `nil`; `MigrateToKeyring` passes a `*[]string` to track exact written keys for rollback.
  - `loadFromKeyring(provider KeyringProvider, warnW io.Writer) (migrated bool)` — startup path (warn-and-continue).
  - `migrateFromKeyring(provider KeyringProvider, filled *[]migratedField) error` — explicit migration path (strict).
  - `validateForMigration() error` — returns a `clierr.UsageError` if a required field is empty, naming the credential and suggesting removal. Used by `MigrateToKeyring` before writing any keyring entries.
  - `zeroSensitiveFields()` — zeros all sensitive fields in place.
  - `saveSensitiveFields() []string` — returns current sensitive field values in a type-specific fixed-length slice. The ordering must match `restoreSensitiveFields`.
  - `restoreSensitiveFields([]string)` — restores sensitive field values from a slice returned by `saveSensitiveFields`.

  All three concrete types (`*AuraCredential`, `*DbmsCredential`, `*EmbedCredential`) must satisfy this interface. The existing `writeToKeyring(provider KeyringProvider) error` signatures on the concrete types must be updated to the new `(provider KeyringProvider, written *[]string) error` signature.

- **REQ-F-009**: Add an unexported `allCredentials() []keyringCredential` method on `Credentials` that returns a single flattened slice containing all credentials across all three types in deterministic order: Aura entries first (in slice order), then Dbms, then Embed. This is the canonical way to iterate all credentials in `credentials.go`; all remaining triple loops must be replaced with a single loop over `c.allCredentials()`.

- **REQ-F-010**: Rewrite `saveWithKeyring` and `MigrateToKeyring` in `credentials.go` to use `c.allCredentials()` instead of separate per-type iterations:
  - `saveWithKeyring`: one loop over `c.allCredentials()` for `writeToKeyring(defaultKeyring, nil)`, one loop to save fields + zero via `saveSensitiveFields()`/`zeroSensitiveFields()`, one call to `saveToJSON()`, one loop to restore via `restoreSensitiveFields()`.
  - `MigrateToKeyring`: one loop over `c.allCredentials()` that calls `validateForMigration()` then `writeToKeyring(defaultKeyring, &writtenKeys)` per credential; rollback iterates `writtenKeys` (exact key tracking preserved); one subsequent loop to zero via `zeroSensitiveFields()`.

### Non-Functional Requirements

- **REQ-NF-001**: `make test`, `make lint`, and `make fmt-check` must all pass after the refactor.
- **REQ-NF-002**: No new exported symbols except the per-type keyring methods in REQ-F-001 (which must be exported since they are called from `credentials.go` in the same package — they may remain unexported).
- **REQ-NF-003**: All existing tests in `credentials_keyring_test.go` must continue to pass. Any test that previously captured `os.Stderr` output for warning assertions must be updated to assert against the `io.Writer` passed to `SetStorageMode`.

## Technical Considerations

- **`writeToKeyring` signature change**: The existing per-type `writeToKeyring(provider KeyringProvider) error` must become `writeToKeyring(provider KeyringProvider, written *[]string) error`. The `written` pointer is nil-safe: implementations append to `*written` only when `written != nil`. `saveWithKeyring` passes `nil`; `MigrateToKeyring` passes `&writtenKeys`. All existing call sites (`saveWithKeyring` loop in task-005 output) must be updated to pass the second argument.

- **`saveSensitiveFields`/`restoreSensitiveFields` ordering invariant**: Each type's `saveSensitiveFields` returns a fixed-length slice (Aura: 2 elements [ClientSecret, AccessToken]; Dbms: 1 [Password]; Embed: 1 [APIKey]). `restoreSensitiveFields` restores in the same order. The slice length and field order must not change between calls; `allCredentials()` must return credentials in the same order on both calls (save and restore pass).

- **`allCredentials()` must be stable**: Since `saveWithKeyring` calls it twice (once to save/zero, once to restore), the order and length must be identical between calls. Since no credential is added or removed between the two calls (within a single `saveWithKeyring` invocation), this is naturally guaranteed.

- **`validateForMigration()` error messages**: Each type's implementation must reproduce the same `clierr.UsageError` message format currently inlined in `MigrateToKeyring` — naming the credential and suggesting `remove`+re-add.

- **Compile-time interface check**: Add a blank `var _ keyringCredential` assertion for each concrete type in their respective test files (or in `export_test.go`) to catch interface drift.

- **Per-type method signatures**: The `readFromKeyring` method on each type needs different required/optional semantics depending on which function is calling it (`loadSensitiveFieldsFromKeyring` does warn-and-continue on missing required fields; `MigrateToInsecure` errors on missing required fields). Consider separate `loadFromKeyring(provider, warnW)` (for startup) and `migrateFromKeyring(provider)` (for explicit migration) if the semantics diverge enough to make a shared method's signature awkward. Alternatively, pass an `onMissing` function or a boolean `strict bool` to a single method. Choose whichever keeps the per-type files readable.

- **Zero-restore in `saveWithKeyring`**: The zero-restore pattern (`defer func() { cred.Field = saved }()` or explicit pre/post) must ensure the restore happens even if `saveToJSON()` panics (which it does on marshal errors). Use explicit save-zero-restore without defer if the panic path is acceptable to leave fields zeroed (consistent with current behaviour on marshal panic). Using `defer` for restore is safer but adds complexity; the current code also panics on marshal error in both paths so not restoring on panic is acceptable.

- **`config/set.go` simplification**: After REQ-F-007, the `credentialStorageModeChanged` boolean and the `if credentialStorageModeChanged { cfg.Credentials.SetStorageMode(value) }` block can be removed for the `MigrateToInsecure` path. The `MigrateToKeyring` path still needs a `SetStorageMode` call after persisting the config key (to flip the in-process mode to keyring so subsequent saves go to the keyring). Keep only that call.

- **Test updates**: `TestSetStorageMode_KeyringMode_MissingRequired_Warn` in `credentials_keyring_test.go` (and any other test exercising warning paths) must pass a `*bytes.Buffer` as `warnW` and assert `strings.Contains(buf.String(), "Warning:")`. Tests that previously used `os.Stderr` capture can be simplified.

## Acceptance Criteria

- [ ] An unexported `keyringCredential` interface exists in `keyring.go` with all seven methods from REQ-F-008.
- [ ] `*AuraCredential`, `*DbmsCredential`, and `*EmbedCredential` all satisfy `keyringCredential`; compile-time assertions exist.
- [ ] `Credentials.allCredentials()` exists and returns all credentials in deterministic Aura→Dbms→Embed order.
- [ ] `saveWithKeyring` contains a single loop over `c.allCredentials()` for the keyring-write phase and a single loop for the save/zero/restore phase (no per-type inline loops remain).
- [ ] `MigrateToKeyring` contains a single loop over `c.allCredentials()` for validate+write with exact `writtenKeys` tracking (no per-type inline loops remain).
- [ ] `writeToKeyring` on all three types has the `(provider KeyringProvider, written *[]string) error` signature; `saveSensitiveFields`/`restoreSensitiveFields`/`validateForMigration` exist on all three.
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
