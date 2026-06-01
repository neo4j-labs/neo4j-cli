# PRD: `credential-storage: env` mode (CLI-152)

## Overview

Add a third `credential-storage` config value, `env`, alongside the existing `keyring` and `insecure`. In env mode, sensitive credential fields are sourced from well-known environment variables at runtime and **nothing is ever persisted** to disk or the OS keyring (`save()` becomes a global no-op). Credentials are ephemeral per process.

This makes `neo4j-cli` composable with standard CI/CD secret-injection mechanisms (GitHub Secrets, Vault, etc.) without committing plaintext secrets to `credentials.json` (the current `insecure` escape hatch) on headless runners that have no keyring daemon.

## Goals

- Let CI/CD runners authenticate **Aura** commands purely from env vars — the genuine gap today, since `Aura.GetDefault()` hard-errors with no stored default and no Aura env-var path exists.
- Provide a single, documented, symmetric `credential-storage: env` mode covering all three credential types (Aura, Dbms, Embed) for consistent `credential list` visibility and `GetDefault()` behavior.
- Guarantee zero on-disk / keyring writes while in env mode (token refresh, `instance create` auto-store, and credential mutations all stay in-memory).

## Non-Goals

- Changing how `query` already overlays `NEO4J_URI/USERNAME/PASSWORD/DATABASE` (dbms) and `NEO4J_EMBED_*` (embed) — that behavior is independent of storage mode and remains unchanged.
- Per-credential-name env vars. Env mode sources a single ephemeral default credential per type (well-known fixed var names).
- Making `env` an automatically-selected first-run default. It is explicitly settable only.
- A `MigrateToEnv` path / migrating ephemeral env secrets into keyring or JSON.

## Requirements

### Functional Requirements

- REQ-F-001: Add storage-mode constant `StorageModeEnv = "env"` in `common/clicfg/credentials/credentials.go`.
- REQ-F-002: `GlobalConfig.Set` (`common/clicfg/clicfg.go`) accepts `env` for the `credential-storage` key; the invalid-value error message lists all three valid values (`keyring`, `insecure`, `env`).
- REQ-F-003: When storage mode is `env`, synthesize an ephemeral default credential per type from env vars, mutating in-memory structs directly (never via `Add`/`SetDefault`, which trigger `onUpdate → save`):
  - Aura: requires BOTH `NEO4J_AURA_CLIENT_ID` and `NEO4J_AURA_CLIENT_SECRET`. Synthesize `&AuraCredential{Name:"env", ClientId, ClientSecret}` and set it as default. `AccessToken`/`TokenExpiry` left empty so the first call performs a normal OAuth fetch (in-memory only). If only one of the pair is present, skip synthesis and warn to stderr.
  - Dbms: synthesize when `NEO4J_PASSWORD` present; populate `URI/Username/DatabaseName` from `NEO4J_URI/NEO4J_USERNAME/NEO4J_DATABASE` as available; set default `"env"`.
  - Embed: synthesize **only when `NEO4J_EMBED_PROVIDER` present**; populate `Provider/Model/BaseURL/Dimensions/APIKey` from `NEO4J_EMBED_PROVIDER/MODEL/BASE_URL/DIMENSIONS/API_KEY`; set default `"env"`.
- REQ-F-004: `Credentials.save()` is a no-op when storage mode is `env` (returns nil without writing to disk or keyring).
- REQ-F-005: Reserve the credential name `env` across all three types — reject user attempts to create a credential named `env` (mirror the existing `desktop` reservation in `dbms.go`).
- REQ-F-006: Block switching away from env: `config set credential-storage keyring|insecure` while current mode is `env` returns a `clierr.NewUsageError` (env creds are ephemeral; nothing to migrate) and leaves the config value unchanged.
- REQ-F-007: Credential-mutating commands (`credential aura-client add`; `credential dbms add|use|remove|set-embed`; `credential embed add|use|remove`) in env mode warn-but-allow: mutate in-memory, print a stderr warning that nothing is persisted in env mode. The warning is user-command-scoped only — it must NOT fire on automatic token refresh or `instance create` auto-store.
- REQ-F-008: env reading in the credentials package goes through a test seam (`var getenv = os.Getenv` + `SetGetenvForTest` in `export_test.go`).

### Non-Functional Requirements

- REQ-NF-001: While in env mode, no write occurs to `credentials.json` or the OS keyring under any command path (verified by tests asserting an unchanged MemMapFs and zero mock-keyring `Set` calls).
- REQ-NF-002: Switching **to** env is non-destructive — existing on-disk / keyring secrets are left untouched.
- REQ-NF-003: Follow repo conventions: colocated table-driven `*_test.go`, copyright headers, `common/*` packages must not import `neo4j-cli/internal/*`.

## Technical Considerations

- **Dispatch points** (single chokepoints): `SetStorageMode` (`credentials.go:90`) gains an `env` case calling a new `loadFromEnv(warnW)`; `save()` (`credentials.go:191`) gains an `env` no-op case. No change needed at the `clicfg.NewConfig` call site (`clicfg.go:117`).
- **New file** `common/clicfg/credentials/env.go` holds the three synthesis helpers + `loadFromEnv`.
- **Double-source is benign**: `query`'s own dbms/embed env overlay (`connect.go:304-307`, `query/embed/embed.go` Resolve) still runs and takes precedence with identical values; synthesized dbms/embed creds exist for `credential list` and non-query `GetDefault()` consumers. `connect.go`/`embed.go` read real `os.Getenv`, so command/e2e tests use `t.Setenv`; the credentials package uses the `getenv` seam.
- **Load ordering**: `NewCredentials → load()` runs in insecure mode first (reads/normalizes `credentials.json`), then `SetStorageMode(env)` overlays env in-memory — non-destructive.
- **Migration gate** lives in `neo4j-cli/internal/subcommands/config/set.go:71-86`; add the `currentMode == StorageModeEnv` block (REQ-F-006).
- **Skill bundle**: any `Long`/help text change on config or credential commands requires `go generate ./neo4j-cli/internal/skill/...` (gate: `TestGenerator_RoundTrip`).
- **Env var names**: Aura new pair `NEO4J_AURA_CLIENT_ID`/`NEO4J_AURA_CLIENT_SECRET`; dbms/embed reuse existing constants (`connect.go:33-36`, `query/embed/embed.go:65-69`).

## Acceptance Criteria

- [ ] `config set credential-storage env --rw` succeeds and persists the config key; runs no migration.
- [ ] With `credential-storage=env` and `NEO4J_AURA_CLIENT_ID`/`NEO4J_AURA_CLIENT_SECRET` set, an Aura command authenticates (no "default credential not set" error) and writes nothing to `credentials.json` or keyring.
- [ ] Aura with only one of the pair set → no synthesis + stderr warning.
- [ ] Dbms synthesis on `NEO4J_PASSWORD`; embed synthesis only on `NEO4J_EMBED_PROVIDER`.
- [ ] `save()` is a no-op in env mode: mutating creds (incl. Aura token refresh) leaves the MemMapFs file and mock keyring untouched.
- [ ] `credential aura-client|dbms|embed add` in env mode prints the not-persisted warning and does not write.
- [ ] `credential <type> add --name env` is rejected as reserved.
- [ ] `config set credential-storage keyring|insecure` while in env mode is blocked with a usage error; config value unchanged.
- [ ] Invalid-value error message for `credential-storage` lists `keyring, insecure, env`.
- [ ] `env` is never auto-selected by first-run default logic.
- [ ] `make test`, `make fmt-check`, `make lint` pass; `go generate ./neo4j-cli/internal/skill/...` clean.

## Out of Scope

- Modifying the existing `query` env-var overlay behavior.
- 1Password / other backends (tracked separately in CLI-153).
- Migrating ephemeral env secrets into keyring/JSON.

## Open Questions

None — all design decisions confirmed (synthesize all three types; global `save()` no-op; warn-but-allow on writes; reserved name `env`; block env→other switch; embed requires `NEO4J_EMBED_PROVIDER`).
