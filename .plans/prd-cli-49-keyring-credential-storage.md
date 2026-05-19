# PRD: Keyring-Based Credential Secret Storage (CLI-49)

## Overview

Replace plaintext secret storage in `credentials.json` with OS-native keyring storage (macOS Keychain, Windows Credential Manager, Linux Secret Service). Sensitive credential fields are extracted from the JSON file and stored in the system keyring, while non-sensitive metadata remains in the existing JSON file. Users can opt out via `credential-storage: insecure` to retain the current JSON-only behaviour.

## Goals

- Eliminate plaintext secrets from `credentials.json` on user machines by default.
- Use OS-native secure storage for all three credential types (Aura, Dbms, Embed).
- Provide a clean escape hatch (`credential-storage: insecure`) for environments without keyring access.
- Auto-migrate secrets bidirectionally when the storage mode changes via an explicit `config set` command.
- Surface actionable errors when keyring mode is configured but the keyring is unavailable (e.g., headless CI).
- Preserve existing users' credential behaviour without disruption on upgrade; offer a clear opt-in path to keyring storage.

## Non-Goals

- `credential-storage: env` (env-var-based credential injection) — deferred to a follow-up (see CLI env-var follow-up).
- 1Password `op://` secret reference support — deferred to a follow-up.
- Encryption of the non-sensitive JSON metadata file.
- Per-credential-type storage mode (all types use the same configured mode).

## Requirements

### Functional Requirements

- REQ-F-001: The global config key `credential-storage` accepts exactly two values: `keyring` (default) and `insecure`. All other values are rejected with a validation error listing the valid options.
- REQ-F-002: When `credential-storage: keyring`, the following sensitive fields are stored in the OS keyring and omitted/blank in `credentials.json`:
  - Aura: `ClientSecret`, `AccessToken`
  - Dbms: `Password`
  - Embed: `APIKey`
- REQ-F-003: When `credential-storage: insecure`, all fields including secrets are stored in `credentials.json` exactly as today. No keyring reads or writes are performed.
- REQ-F-004: **Forward migration** — `neo4j-cli config set credential-storage keyring` triggers migration before saving the config value. All existing credential secrets are written to the keyring and scrubbed from the JSON file. The config value is only persisted if all migrations succeed. If there are no credentials to migrate, the command succeeds normally.
- REQ-F-005: **Reverse migration** — `neo4j-cli config set credential-storage insecure` triggers migration before saving the config value. All existing credential secrets are read from the keyring and written back to the JSON file, then deleted from the keyring. The config value is only persisted if all migrations succeed.
- REQ-F-010: **Atomic migration rollback** — if migration fails part-way through (e.g., keyring write succeeds for 3 of 5 credentials then errors), all already-migrated entries are rolled back (keyring entries deleted for forward migration; JSON fields re-zeroed for reverse migration) and the config value is left unchanged. The error message identifies which credential caused the failure.
- REQ-F-006: If `credential-storage: keyring` and a keyring operation fails (keyring unavailable, daemon not running, permission denied, etc.), the CLI must return a clear error explaining that the keyring is unavailable and instructing the user to run `neo4j-cli config set credential-storage insecure`.
- REQ-F-007: Keyring entries use a consistent naming scheme: service `neo4j-cli`, user key `<type>/<credential-name>/<field>` (e.g., `aura/my-cred/client-secret`, `dbms/mydb/password`, `embed/openai/api-key`).
- REQ-F-008: `credential <type> remove` must also delete the associated keyring entries when `credential-storage: keyring`.
- REQ-F-009: `credential list` and `credential get` output is unchanged — secrets are never printed regardless of storage mode.
- REQ-F-011: **First-run default detection** — on startup, in `PersistentPreRunE` on the root command, if `credential-storage` is absent from `config.json`:
  - If any credentials exist in `credentials.json`: write `credential-storage: insecure` to config silently (preserves existing behaviour for upgrading users).
  - If no credentials exist: write `credential-storage: keyring` to config silently (secure default for new installs).
  This check fires exactly once; after the first run `credential-storage` is always present.
- REQ-F-012: **One-time upgrade notice** — when the first-run detection writes `credential-storage: insecure` (i.e., an existing user is upgrading), emit a single notice to stderr informing the user that their credentials remain in plaintext and providing the command to migrate: `neo4j-cli config set credential-storage keyring`. The notice fires only on the run that writes the default (naturally one-time since the absence check does not repeat).
- REQ-F-013: **Missing-secret behaviour during migration** — applied symmetrically in both directions (forward: empty value in JSON; reverse: `ErrNotFound` from keyring):
  - **Aura `ClientSecret`** and **Dbms `Password`**: treated as required — a missing value is a hard error. Migration aborts, rolls back, config unchanged. Error message names the credential and suggests removing it with `neo4j-cli credential <type> remove <name>` before retrying.
  - **Aura `AccessToken`** and **Embed `APIKey`**: treated as optional/cacheable — a missing value is silently skipped (no keyring entry written for forward; empty value written to JSON for reverse). A keyring error *other than* `ErrNotFound` (e.g., daemon unavailable, permission denied) is still a hard error for all field types.

### Non-Functional Requirements

- REQ-NF-001: A pure-Go keyring library (e.g., `github.com/zalando/go-keyring`) must be used — no CGo dependencies.
- REQ-NF-002: Unit tests must use the library's mock/in-memory provider so that tests run without a real keyring daemon on any platform.
- REQ-NF-003: Migration during `config set credential-storage` must complete within a few seconds for users with fewer than 20 credentials.
- REQ-NF-004: If a credential's keyring entry is missing during a `keyring`-mode read (e.g., the OS keyring was cleared externally), the CLI must return a clear error naming the missing credential rather than silently returning an empty secret.

## Technical Considerations

### Library Choice

`github.com/zalando/go-keyring` is the recommended library. It is pure Go, uses OS-native backends (`/usr/bin/security` on macOS, Win32 Credential Manager on Windows, dbus Secret Service on Linux), has a minimal three-function API (`Get` / `Set` / `Delete`), and provides `MockInit()` for hermetic testing. No CGo required.

### Storage Split

`credentials.json` continues to hold all non-sensitive fields. Sensitive fields become separate keyring entries per credential. In keyring mode the JSON serialisation of sensitive fields is blank or omitted; `load()` populates them from the keyring after unmarshalling JSON.

### Credentials Load/Save Seam

`load()` in `common/clicfg/credentials/credentials.go` handles the normal read path only — no migration logic:
1. Unmarshal JSON as today.
2. If `credential-storage: keyring`: for each credential call `keyring.Get()` to populate sensitive fields.
3. Wire `onUpdate` callbacks as today.

`save()` in keyring mode:
1. Before writing JSON: zero out sensitive fields in the struct copies written to disk.
2. Call `keyring.Set()` for each sensitive field.

### Migration Methods on Credentials

Two new methods on the `Credentials` struct, called from `config set` RunE:

**`MigrateToKeyring() error`**:
1. Iterate all credential types (Aura, Dbms, Embed).
2. For each credential, per field:
   - Required fields (`ClientSecret`, `Password`): if empty, hard error — roll back all entries written so far and return.
   - Optional fields (`AccessToken`, `APIKey`): if empty, skip (no keyring entry written).
   - Non-empty fields: call `keyring.Set()`. Track all entries written; on any error roll back and return.
3. On full success: scrub secrets from the in-memory structs and call `save()`.

**`MigrateToInsecure() error`**:
1. Iterate all credential types.
2. For each credential, per field:
   - Call `keyring.Get()` for each sensitive field.
   - Required fields (`ClientSecret`, `Password`): `ErrNotFound` is a hard error — re-zero all in-memory fields already populated and return. Any other keyring error is also a hard error.
   - Optional fields (`AccessToken`, `APIKey`): `ErrNotFound` is silently skipped (write empty value to JSON). Any other keyring error is a hard error.
3. On full success: call `save()` to persist secrets to JSON, then call `keyring.Delete()` for all non-empty entries.

### Config Set RunE Integration

The `config set credential-storage` RunE in `neo4j-cli/internal/subcommands/config/set.go`:
1. Validates the new value as today.
2. If the new value differs from the current value and credentials exist:
   - `keyring`: call `credentials.MigrateToKeyring()`; return error on failure (config not saved).
   - `insecure`: call `credentials.MigrateToInsecure()`; return error on failure (config not saved).
3. Call `GlobalConfig.Set(...)` only after successful migration.

### Config Key Registration

Add `credential-storage` to `GlobalConfig.ValidConfigKeys` in `common/clicfg/clicfg.go`. The existing `Set()` validation pattern applies; add an additional value check (only `keyring` / `insecure` accepted).

### First-Run Default Detection

A `initCredentialStorageDefault(cfg *Config, creds *Credentials)` function (or equivalent) is called from `PersistentPreRunE` on the root cobra command in `neo4j-cli/app/app.go`. It:

1. Checks whether `credential-storage` is set in `config.json` via `GlobalConfig` — if present, returns immediately (no-op).
2. If absent: loads credentials to check if any exist across all three types.
3. Writes `credential-storage: insecure` if credentials exist, `credential-storage: keyring` if not, via `GlobalConfig.Set(...)`.
4. If writing `insecure`: prints the one-time upgrade notice to stderr before returning.

This runs on every invocation but exits immediately after the key-presence check on all subsequent runs, adding negligible overhead.

### Linux / Headless CI

On Linux without a running Secret Service daemon, `keyring.Get()` / `keyring.Set()` return an error. This surfaces REQ-F-006 — a clear error with instructions to switch to `insecure` mode. CI users should set `credential-storage: insecure` in their config file or equivalent. A follow-up issue will address env-var-based injection as the idiomatic CI pattern.

### AccessToken Storage

`AccessToken` is stored in the keyring — not in `credentials.json`. It is a short-lived OAuth token refreshed by `UpdateAccessToken()` after expiry; every token refresh triggers a `keyring.Set()`. On macOS/Windows this is sub-millisecond; on Linux it involves a dbus round-trip, which is acceptable for CLI use.

`AccessToken` may be legitimately empty on a brand-new credential before first authentication. During migration it is therefore classified as skip-if-empty (REQ-F-013) — not because it is optional in the security sense, but because it starts life empty and will be written to the keyring on first token acquisition.

## Acceptance Criteria

- [ ] `neo4j-cli config set credential-storage keyring` succeeds; any other value fails with a clear validation error.
- [ ] After `credential aura-client add` in keyring mode, `credentials.json` contains no `client_secret` or `access_token` values (fields absent or empty string).
- [ ] After `credential aura-client add` in insecure mode, `credentials.json` contains the secrets as today.
- [ ] `neo4j-cli config set credential-storage keyring` migrates existing secrets to the keyring and only persists the config change on success.
- [ ] `neo4j-cli config set credential-storage insecure` migrates secrets back to the JSON file and only persists the config change on success.
- [ ] If migration fails part-way through, already-migrated entries are rolled back and the config value is unchanged.
- [ ] On a headless machine (no keyring daemon), `neo4j-cli config set credential-storage keyring` returns a clear error and leaves `credential-storage` unchanged.
- [ ] `credential <type> remove` deletes the associated keyring entries when in keyring mode.
- [ ] If a keyring entry is unexpectedly missing, the CLI returns a clear error naming the affected credential.
- [ ] Forward migration with a missing required secret (empty `ClientSecret` or `Password` in JSON) returns an error naming the credential, rolls back, and leaves config unchanged.
- [ ] Reverse migration with a required secret not found in the keyring (`ErrNotFound` for `ClientSecret` or `Password`) returns an error naming the credential, rolls back, and leaves config unchanged.
- [ ] Forward/reverse migration with a missing optional secret (`AccessToken`, `APIKey`) skips that field silently and completes successfully.
- [ ] A keyring error other than `ErrNotFound` on an optional field is still a hard error.
- [ ] On first run after upgrade with existing credentials, `credential-storage: insecure` is written to config and a notice pointing to `neo4j-cli config set credential-storage keyring` is printed to stderr.
- [ ] On first run with no existing credentials, `credential-storage: keyring` is written to config with no notice.
- [ ] On all subsequent runs, the first-run detection is a no-op (key already present).
- [ ] All credential unit tests pass using the mock keyring provider.
- [ ] `make test`, `make lint`, and `make fmt-check` all pass clean.

## Out of Scope

- `credential-storage: env` — follow-up issue to be created.
- 1Password `op://` reference support — follow-up.
- Keyring storage for non-credential config values.
- Per-credential-type storage mode.

## Open Questions

None.
