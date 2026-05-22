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
- REQ-F-014: **First-run keyring availability probe** — when first-run default detection (REQ-F-011) would write `credential-storage: keyring` (i.e., no existing credentials), the keyring is first probed for availability. If the probe fails (e.g., `dbus-launch` not found on Linux), a warning is emitted to stderr and `credential-storage: insecure` is written instead. The warning names the failure and provides the command to retry once the keyring daemon is available.
- REQ-F-015: **Explicit keyring probe on `config set credential-storage keyring`** — before accepting `keyring` as the new mode (regardless of whether credentials exist), the keyring is probed. If unavailable, the command returns a `UsageError` identifying the keyring failure and instructing the user to either fix the keyring daemon or run `neo4j-cli config set credential-storage insecure --rw`. The config key is not written.
- REQ-F-016: **Silent JSON fallback for missing keyring entries during credential load** — when `credential-storage: keyring` is configured and `keyring.Get()` returns `ErrNotFound` for a sensitive field during `load()`, if the JSON value for that field is non-empty, silently use the JSON value without warning or error. Hard-error only when both the keyring entry and the JSON value are absent/empty for a required field (`ClientSecret`, `Password`). Optional fields (`AccessToken`, `APIKey`) remain silently skipped when both are absent. This makes the CLI resilient to out-of-sync states where a credential was written to JSON while keyring mode was already configured (e.g., a new credential added via direct JSON edit, or an incomplete migration).
- REQ-F-017: **Repair migration — `config set credential-storage keyring` is idempotent** — `config set credential-storage keyring --rw` runs `MigrateToKeyring()` even when `credential-storage` is already `keyring`. Since `load()` now populates in-memory credential fields from JSON fallback (REQ-F-016), the repair pass has access to the JSON-resident secrets and writes them into the keyring, scrubbing them from `credentials.json`. Credentials already correctly stored in the keyring are unaffected (their in-memory values, loaded from the keyring, are written back — effectively a no-op). This allows users to explicitly repair an out-of-sync state by re-running the same command.
- REQ-F-018: **Reverse migration handles JSON-only credentials** — during `MigrateToInsecure()`, if `keyring.Get()` returns `ErrNotFound` for a sensitive field but the in-memory credential struct already has a non-empty value for that field (populated via the REQ-F-016 JSON fallback during `load()`), the field is already present in JSON — treat it as a no-op (no keyring read needed, no error). This allows `config set credential-storage insecure --rw` to succeed when transitioning away from a partially-migrated or out-of-sync keyring state.
- REQ-F-019: **Auto-migration on load** — when `load()` uses the REQ-F-016 JSON fallback for any sensitive field (keyring `ErrNotFound` but JSON value present), immediately attempt to write that value to the keyring and scrub it from `credentials.json`. This auto-migration fires on every command that loads credentials, without requiring `--rw`. If the keyring write fails (e.g., the daemon is temporarily unavailable), the attempt is silently abandoned for this invocation and the JSON value continues to be used — the next command will retry automatically. Auto-migration is per-field and per-credential: a failure on one does not block others. A successful auto-migration leaves the credential fully in keyring mode with no trace in `credentials.json`, without any user action.

### Non-Functional Requirements

- REQ-NF-001: A pure-Go keyring library (e.g., `github.com/zalando/go-keyring`) must be used — no CGo dependencies.
- REQ-NF-002: Unit tests must use the library's mock/in-memory provider so that tests run without a real keyring daemon on any platform.
- REQ-NF-003: Migration during `config set credential-storage` must complete within a few seconds for users with fewer than 20 credentials.
- REQ-NF-004: If a credential's keyring entry is missing during a `keyring`-mode read (e.g., the OS keyring was cleared externally), the CLI must return a clear error naming the missing credential rather than silently returning an empty secret. Exception: if the JSON value for that field is non-empty (out-of-sync state), REQ-F-016 applies — the JSON value is used silently and no error is raised. Hard errors fire only when both keyring and JSON are absent/empty for a required field.
- REQ-NF-005: Keyring availability is probed by issuing a `Get()` for a sentinel key (`__neo4j-cli-probe__`). `ErrNotFound` indicates the keyring daemon is reachable (the key simply does not exist). Any other error indicates the keyring is unavailable. The probe uses the same `KeyringProvider` test seam so tests can inject errors without a real daemon.
- REQ-NF-006: Linux keyring behaviour must be covered by subprocess-driven e2e smoke tests (build tag `keyring_smoke && linux`) in `test/e2e/keyring/keyring_linux_test.go`. Tests drive the real `bin/neo4j-cli` binary with a temp `HOME` for credential isolation. Two groups:
  - **No-daemon tests** (always run on Linux CI): subprocess env strips `DBUS_SESSION_BUS_ADDRESS`; verifies graceful degradation (REQ-F-014, REQ-F-015) — warning + insecure fallback on first run, hard error on explicit `config set keyring`.
  - **With-daemon tests** (must always run in CI; skip gracefully only in local dev): verifies full credential lifecycle in keyring mode — add, migration forward/reverse, remove. Skip guard: if `DBUS_SESSION_BUS_ADDRESS` is absent AND `CI` env var is not `"true"`, call `t.Skip`. If `DBUS_SESSION_BUS_ADDRESS` is absent AND `CI=true`, call `t.Fatal` — a missing session bus in CI means the `dbus-run-session` + `gnome-keyring` setup step has failed, and the test suite must not silently pass.
- REQ-NF-007: Windows keyring behaviour must be covered by subprocess-driven e2e smoke tests (build tag `keyring_smoke && windows`) in `test/e2e/keyring/keyring_windows_test.go`. Tests drive the real `bin/neo4j-cli.exe` binary with an isolated temp `USERPROFILE`/`APPDATA`. Tests always run on `windows-latest` CI (Windows Credential Manager is a built-in Win32 service with no daemon; `ErrUnsupportedPlatform` is not raised on Windows). Test groups:
  - **Happy-path tests** (always run — Credential Manager is always available on standard Windows): verifies the full credential lifecycle in keyring mode — add, migration forward/reverse, remove. Mirrors the Linux with-daemon group.
  - **Graceful degradation tests**: Credential Manager unavailability on Windows (containers, service accounts) cannot be reliably simulated in standard CI. This path is validated by the platform-agnostic unit tests in REQ-NF-005 using `MockInitWithError()`; no separate Windows e2e group is needed.
- REQ-NF-008: macOS Keychain behaviour must be covered by subprocess-driven e2e smoke tests (build tag `keyring_smoke && darwin`) in `test/e2e/keyring/keyring_darwin_test.go`. Tests drive the real `bin/neo4j-cli` binary with a temp `HOME` for credential isolation. Three groups:
  - **Happy-path tests** (always run on macos-latest — Keychain is always available on standard macOS): verifies the full credential lifecycle in keyring mode — add, migration forward/reverse, remove. Mirrors the Windows happy-path group.
  - **Locked-Keychain tests** (graceful degradation via custom Keychain): create a temporary Keychain (`security create-keychain`), make it the default, lock it (`security lock-keychain`), run neo4j-cli, verify the probe triggers the expected graceful-degradation behavior (REQ-F-014 warning+insecure fallback on first run; REQ-F-015 hard error on `config set keyring`). Restore the original default Keychain in `t.Cleanup`.
  - **Missing-security tests** (graceful degradation via PATH override): set subprocess PATH to a temp dir containing a stub `security` executable that always exits non-zero, prepended before `/usr/bin/`. Verifies graceful degradation when the `security` binary is absent or broken. Note: this group only works if go-keyring invokes `security` via PATH lookup; if it hardcodes `/usr/bin/security`, this group falls back to unit-test coverage and the test must be skipped or adapted during implementation.

## Technical Considerations

### CI Failures — Ubuntu Unit Test and Windows Smoke Binary

Two pre-existing CI failures need repair before the branch can merge:

**Ubuntu — `TestConfigSet/set_credential-storage_to_keyring_with_rw_succeeds`** (`neo4j-cli/internal/subcommands/config/set_test.go`)

The table-driven `TestConfigSet` includes a case that runs `config set --rw credential-storage keyring` without initialising the mock keyring. When executed under plain `make test` on ubuntu-latest (no D-Bus session), `ProbeKeyringAvailability()` (REQ-F-015) hits the real Secret Service and returns `"The name org.freedesktop.secrets was not provided by any .service files"`, making the command fail rather than succeed.

Fix: call `gokeyring.MockInit()` at the top of `TestConfigSet` (before the range loop) and register a `t.Cleanup(gokeyring.MockInit)` to reset state after all subtests. The mock makes `ProbeKeyringAvailability()` return `ErrNotFound` (→ keyring available) instead of the real D-Bus error. Other subtests in the table that don't touch credentials are unaffected.

**Windows — Smoke test binary not found** (`test/e2e/keyring/keyring_windows_test.go`)

The Windows keyring smoke CI step assumes the matrix `make build` step already produced `bin/neo4j-cli.exe`. Empirically, `go build -o bin/neo4j-cli ./neo4j-cli` on the windows-latest runner does not create `bin/neo4j-cli.exe` at the path the test helper expects (confirmed by CI log: all four smoke tests fail immediately with `GetFileAttributesEx ... neo4j-cli.exe: The system cannot find the file specified`).

Fix: add a Windows-specific CI step immediately before "Keyring smoke (Windows)" that explicitly produces the binary with the correct name:
```yaml
- name: Build neo4j-cli.exe (Windows keyring smoke)
  if: matrix.os == 'windows-latest'
  run: go build -o bin/neo4j-cli.exe ./neo4j-cli
```

### Library Choice

`github.com/zalando/go-keyring` is the recommended library. It is pure Go, uses OS-native backends (`/usr/bin/security` on macOS, Win32 Credential Manager on Windows, dbus Secret Service on Linux), has a minimal three-function API (`Get` / `Set` / `Delete`), and provides `MockInit()` for hermetic testing. No CGo required.

### Storage Split

`credentials.json` continues to hold all non-sensitive fields. Sensitive fields become separate keyring entries per credential. In keyring mode the JSON serialisation of sensitive fields is blank or omitted; `load()` populates them from the keyring after unmarshalling JSON.

### Credentials Load/Save Seam

`load()` in `common/clicfg/credentials/credentials.go` — updated for REQ-F-016 and REQ-F-019:
1. Unmarshal JSON as today.
2. If `credential-storage: keyring`: for each credential and sensitive field:
   a. Call `keyring.Get()`.
   b. If the value is returned: use it (normal path).
   c. If `ErrNotFound` and the JSON value is non-empty (REQ-F-016 fallback): use the JSON value, then immediately call `keyring.Set()` + scrub from the in-memory JSON struct to trigger auto-migration (REQ-F-019). If `keyring.Set()` fails, leave `credentials.json` unchanged for this invocation — do not abort.
   d. If `ErrNotFound` and the JSON value is also empty/absent: hard error for required fields (`ClientSecret`, `Password`); silent skip for optional fields (`AccessToken`, `APIKey`).
3. Wire `onUpdate` callbacks as today.

The auto-migration in step 2c must call `save()` (or the equivalent partial-write path) to persist the scrub to `credentials.json`. This write does not require `--rw`; it is treated as internal housekeeping, not a user-initiated write.

`save()` in keyring mode (unchanged):
1. Before writing JSON: zero out sensitive fields in the struct copies written to disk.
2. Call `keyring.Set()` for each sensitive field.

### Auto-Migration Design Notes

- **Best-effort and silent**: auto-migration (REQ-F-019) never fails a command. If `keyring.Set()` returns an error, the migration attempt is dropped for that invocation. The credential remains readable from JSON on the next run, which will retry the migration.
- **Per-field atomicity**: each sensitive field is migrated independently. A `keyring.Set()` failure for one field does not roll back fields that already succeeded.
- **No `--rw` required**: auto-migration bypasses the `--rw` flag because it is transparent housekeeping, not a user-visible write. The user's intent is preserved; the credential value is unchanged, only its storage location moves.
- **`save()` is the JSON scrub path**: after a successful `keyring.Set()` for a field, the corresponding JSON field is zeroed and `credentials.json` is rewritten. This reuses the existing `save()` call at the end of `load()` rather than introducing a second write path.

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

### Why Not the Config Migration Engine

`common/configmigrate` (CLI-134) was considered and rejected for both migration concerns in this feature:

- **REQ-F-011 (first-run detection)**: the engine's `Apply` function receives only raw `config.json` bytes — it has no access to `credentials.json` or the keyring. The `credential-storage` default depends on whether credentials already exist, so the engine cannot make this decision.
- **REQ-F-004/005 (explicit migration)**: the engine uses a warn-and-continue error policy and runs automatically on startup. Our explicit migrations must be user-triggered, must hard-error on failure, must roll back partial state, and must gate the config write on success — the opposite of the engine's posture.

The engine is the right tool for config key renames and graduated-flag cleanup. It is the wrong tool here.

### First-Run Default Detection

A `initCredentialStorageDefault(cfg *Config, creds *Credentials)` function (or equivalent) is called from `PersistentPreRunE` on the root cobra command in `neo4j-cli/app/app.go`. It:

1. Checks whether `credential-storage` is set in `config.json` via `GlobalConfig` — if present, returns immediately (no-op).
2. If absent: loads credentials to check if any exist across all three types.
3. Writes `credential-storage: insecure` if credentials exist, `credential-storage: keyring` if not, via `GlobalConfig.Set(...)`.
4. If writing `insecure`: prints the one-time upgrade notice to stderr before returning.

This runs on every invocation but exits immediately after the key-presence check on all subsequent runs, adding negligible overhead.

### Linux / Headless CI

On Linux without a running Secret Service daemon (e.g., `dbus-launch` not found in `$PATH`), `keyring.Get()` / `keyring.Set()` return an error such as `exec: "dbus-launch": executable file not found in $PATH`.

**Probe function** — `ProbeKeyringAvailability() error` in `common/clicfg/credentials/keyring.go` calls `defaultKeyring.Get(ServiceName, "__neo4j-cli-probe__")`. `ErrNotFound` → nil (keyring reachable); any other error → that error (keyring unavailable). Uses the same `defaultKeyring` test seam as the rest of the package.

**First-run path** (REQ-F-014) — `initCredentialStorageDefault` in `neo4j-cli/app/app.go` calls `ProbeKeyringAvailability()` before writing `credential-storage: keyring` when no credentials exist. On failure it warns to stderr and writes `insecure` instead.

**Explicit migration path** (REQ-F-015) — `MigrateToKeyring()` in `common/clicfg/credentials/credentials.go` calls `ProbeKeyringAvailability()` at the top, before iterating any credentials. On failure it returns a `clierr.UsageError` immediately. This covers both the has-creds case (where the existing error surfacing was already correct but message is improved) and the no-creds case (where no `keyring.Set()` calls would otherwise have been made).

CI users should set `credential-storage: insecure` in their config file. A follow-up issue will address env-var-based injection as the idiomatic CI pattern.

### macOS Keychain

go-keyring on macOS spawns `/usr/bin/security` as a subprocess (no CGo, no daemon). The `security` binary is part of every standard macOS install and is protected by SIP, so it cannot be deleted. It is always present on `macos-latest` CI runners.

**Failure modes:**
- **Locked Keychain**: if the default Keychain is locked and `security` cannot prompt the user (headless CI), the command exits non-zero. go-keyring surfaces this as a generic `exec.ExitError`, which `ProbeKeyringAvailability()` classifies as unavailable.
- **Missing/broken `security` binary**: if the binary is replaced by a failing stub (PATH override), `exec.Command` returns an `*exec.ExitError` (or `exec.ErrNotFound` if PATH lookup fails). Same classification path.
- **Data too large**: `ErrSetDataTooBig` if the combined command string exceeds ~4096 bytes. Not a graceful-degradation scenario; surface as a regular credential error.

**GitHub Actions macOS runners**: start with an unlocked login Keychain. Happy-path tests should work without any setup. The locked-Keychain test creates and manages its own temporary Keychain to avoid mutating the runner's login Keychain.

**Probe coverage**: `ProbeKeyringAvailability()` (REQ-NF-005) catches all non-ErrNotFound errors from the `security` binary, including locked Keychain and broken-binary failures, triggering the same graceful degradation as Linux.

**Home directory isolation on macOS**: subprocess env sets `HOME` (neo4j-cli uses `os.UserHomeDir()` to locate config on macOS).

**Locked-Keychain test setup**:
```
security create-keychain -p "" /tmp/neo4j-cli-smoke-<pid>.keychain-db
security list-keychains -d user -s /tmp/neo4j-cli-smoke-<pid>.keychain-db <original-keychains...>
security default-keychain -s /tmp/neo4j-cli-smoke-<pid>.keychain-db
security lock-keychain /tmp/neo4j-cli-smoke-<pid>.keychain-db
```
Cleanup (in `t.Cleanup`): restore original default Keychain, restore search list, `security delete-keychain /tmp/neo4j-cli-smoke-<pid>.keychain-db`.

**PATH-override test**: create a temp dir with a `security` shell script that `exit 1`; prepend to PATH in subprocess env. Effective only if go-keyring uses `exec.Command("security", ...)` (PATH-resolved). If it uses the full path `/usr/bin/security`, this test must be skipped (document via `t.Skip` with an explanation) — the unit tests with `MockInitWithError()` cover this path instead.

### Windows Credential Manager

`advapi32.dll` (always present on standard Windows — part of LSASS, no external daemon). Unlike Linux, there is no external process dependency; `ErrUnsupportedPlatform` from go-keyring only fires on truly unsupported platforms (not Windows, macOS, or Linux).

**Known size limit**: `wincred` caps the credential blob at 2560 bytes. All current sensitive fields (client secrets, OAuth tokens, passwords, API keys) are well under this limit; future fields should be checked.

**Edge cases where Credential Manager can fail** (analogous to Linux's missing dbus-launch, but rarer):
- **Windows containers** (process isolation mode): Credential Manager may be unavailable under the container service account.
- **Service account without user profile**: DPAPI encryption of credentials requires a loaded user profile; running as SYSTEM or a headless service account without `LoadUserProfile` can fail.

**Probe coverage**: The platform-agnostic `ProbeKeyringAvailability()` (REQ-NF-005) catches these failures and triggers the same graceful degradation as Linux (REQ-F-014, REQ-F-015). No Windows-specific probe code is needed.

**E2E simulation gap**: Credential Manager unavailability cannot be reliably simulated on `windows-latest` CI without running inside a Windows container or service account context. The graceful degradation path is therefore validated solely by unit tests using `MockInitWithError()`. Happy-path e2e tests cover the standard case (REQ-NF-007).

**Home directory isolation on Windows**: Tests use `t.TempDir()` and set both `USERPROFILE` and `APPDATA` env vars in the subprocess (neo4j-cli constructs the config path from `APPDATA` on Windows via `os.UserConfigDir()`).

### Linux Keyring E2E Smoke Tests

Subprocess-driven smoke tests in `test/e2e/keyring/` using a split-file layout:
- `helpers_test.go` (`//go:build keyring_smoke`): shared helpers — repo-root walk-up, binary path resolution, HOME/USERPROFILE isolation, credentials.json reader.
- `keyring_linux_test.go` (`//go:build keyring_smoke && linux`): Linux-specific tests.
- `keyring_windows_test.go` (`//go:build keyring_smoke && windows`): Windows-specific tests (see REQ-NF-007).

Linux tests: `test/e2e/keyring/keyring_linux_test.go` Each test creates a temp dir and sets `HOME` to it so credential files are isolated. The repo-root helper (`runtime.Caller(0)` walk-up) locates `bin/neo4j-cli` (built by `make build`).

**No-daemon group** (always run on Linux CI, no dbus-run-session needed):
- Strip `DBUS_SESSION_BUS_ADDRESS` and `DBUS_LAUNCHD_SESSION_BUS_SOCKET` from subprocess env.
- `TestKeyring_NoDaemon_FirstRun_WritesInsecureAndWarns`: run any neo4j-cli command on a fresh install; verify `credential-storage: insecure` written to config.json, warning on stderr.
- `TestKeyring_NoDaemon_ConfigSet_FailsWithNoCreds`: run `config set credential-storage keyring --rw` with no credentials; assert non-zero exit and keyring error in stderr.
- `TestKeyring_NoDaemon_ConfigSet_FailsWithExistingCreds`: pre-seed an insecure credential, then run `config set credential-storage keyring --rw`; assert non-zero exit and keyring error in stderr.

**With-daemon group** (mandatory in CI; graceful skip in local dev without a session bus):
- Inherit parent env (which has the session bus address from `dbus-run-session`).
- Guard at top of each with-daemon test:
  ```go
  if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
      if os.Getenv("CI") == "true" {
          t.Fatal("DBUS_SESSION_BUS_ADDRESS not set in CI — dbus-run-session/gnome-keyring setup failed")
      }
      t.Skip("DBUS_SESSION_BUS_ADDRESS not set; run inside dbus-run-session for with-daemon tests")
  }
  ```
- `TestKeyring_WithDaemon_CredentialAddDoesNotStoreSecretInJSON`: add a dbms credential in keyring mode; assert password absent from credentials.json.
- `TestKeyring_WithDaemon_ForwardMigration`: start in insecure mode with a seeded dbms credential; run `config set credential-storage keyring --rw`; assert password absent from credentials.json afterwards.
- `TestKeyring_WithDaemon_ReverseMigration`: start in keyring mode with a seeded keyring entry; run `config set credential-storage insecure --rw`; assert password present in credentials.json afterwards.
- `TestKeyring_WithDaemon_RemoveCleansKeyring`: add a credential in keyring mode; remove it; assert the keyring entry is gone (probe via `secret-tool lookup`).

**CI steps** added to `.github/workflows/test.yml` under `ubuntu-latest`:
1. Build: `make build` (provides `bin/neo4j-cli`).
2. No-daemon step: `go test -tags=keyring_smoke -count=1 -v ./test/e2e/keyring/...` (no dbus-run-session; with-daemon tests self-skip).
3. Install gnome-keyring: `sudo apt-get install -y gnome-keyring libsecret-tools`.
4. With-daemon step: `dbus-run-session -- go test -tags=keyring_smoke -count=1 -v ./test/e2e/keyring/...` (gnome-keyring auto-activates via D-Bus when first keyring call is made; both groups run).

The exact `dbus-run-session` + gnome-keyring unlock incantation (empty passphrase via `echo "" | gnome-keyring-daemon --unlock --components=secrets`) should be validated during implementation — CI headless environments may require explicit daemon unlock before auto-activation works reliably.

### AccessToken Storage

`AccessToken` is stored in the keyring — not in `credentials.json`. It is a short-lived OAuth token refreshed by `UpdateAccessToken()` after expiry; every token refresh triggers a `keyring.Set()`. On macOS/Windows this is sub-millisecond; on Linux it involves a dbus round-trip, which is acceptable for CLI use.

`AccessToken` may be legitimately empty on a brand-new credential before first authentication. During migration it is therefore classified as skip-if-empty (REQ-F-013) — not because it is optional in the security sense, but because it starts life empty and will be written to the keyring on first token acquisition.

## Acceptance Criteria

- [ ] `neo4j-cli config set credential-storage keyring` and `neo4j-cli config set credential-storage insecure` both succeed; any other value fails with a clear validation error listing the valid options.
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
- [ ] On first run with no existing credentials and keyring unavailable (e.g., Linux without dbus-launch), a warning is emitted to stderr and `credential-storage: insecure` is written instead of `keyring`.
- [ ] `neo4j-cli config set credential-storage keyring` on a machine with unavailable keyring (with or without existing credentials) returns a UsageError naming the keyring failure and leaves `credential-storage` unchanged.
- [ ] Both keyring-unavailable behaviours are covered by unit tests using mock keyring errors.
- [ ] `go test -tags=keyring_smoke -count=1 -v ./test/e2e/keyring/...` passes on ubuntu-latest (no-daemon group runs; with-daemon group self-skips).
- [ ] `dbus-run-session -- go test -tags=keyring_smoke -count=1 -v ./test/e2e/keyring/...` passes on ubuntu-latest with `gnome-keyring` installed (both groups run).
- [ ] `go test -tags=keyring_smoke -count=1 -v ./test/e2e/keyring/...` passes on windows-latest (happy-path group runs against real Windows Credential Manager).
- [ ] Keyring_smoke CI steps present in `.github/workflows/test.yml` for `ubuntu-latest`, `windows-latest`, and `macos-latest`.
- [ ] `go test -tags=keyring_smoke -count=1 -v ./test/e2e/keyring/...` passes on macos-latest (happy-path, locked-Keychain, and missing-security groups all run or are appropriately skipped).
- [ ] In keyring mode, a command run against a credential whose keyring entry is missing but whose secret is present in `credentials.json` completes successfully without error or warning, and the secret is automatically moved to the keyring and scrubbed from `credentials.json` by the time the command finishes.
- [ ] In keyring mode, if the keyring write fails during auto-migration (e.g., daemon temporarily unavailable), the command still succeeds using the JSON value and `credentials.json` is left unchanged — the auto-migration will be retried on the next invocation.
- [ ] In keyring mode, a command run against a credential whose keyring entry is missing AND whose JSON value is also absent/empty returns a clear hard error naming the credential (REQ-NF-004 unchanged for fully-missing case).
- [ ] `neo4j-cli config set credential-storage keyring --rw` when `credential-storage` is already `keyring` moves any remaining JSON-resident secrets into the keyring and scrubs them from `credentials.json` (explicit repair pass, complements auto-migration).
- [ ] `neo4j-cli config set credential-storage insecure --rw` succeeds when credentials are in JSON only (no keyring entries) — JSON values are retained and no `ErrNotFound` error is raised.
- [ ] All new load-fallback, auto-migration, and repair-migration scenarios are covered by unit tests using the mock keyring provider.
- [ ] `make test` passes on ubuntu-latest without a D-Bus session (`TestConfigSet/set_credential-storage_to_keyring_with_rw_succeeds` no longer hits the real Secret Service).
- [ ] `go test -tags=keyring_smoke -count=1 -v ./test/e2e/keyring/...` on windows-latest finds `bin/neo4j-cli.exe` and all four smoke tests run (no "binary not found" failure).

## Out of Scope

- `credential-storage: env` — follow-up issue to be created.
- 1Password `op://` reference support — follow-up.
- Keyring storage for non-credential config values.
- Per-credential-type storage mode.

## Open Questions

None.
