# PRD: Consistent Env Var Credential Override (`accept-env-vars`)

## Overview

Add a global `accept-env-vars` config option (default: `false`) that controls whether
neo4j-cli reads credentials from well-known environment variables. When enabled, env vars act
as an override layer following a consistent precedence: explicit CLI flags > env vars >
stored credential. The config can also be activated by setting the env var
`NEO4J_CLI_ACCEPT_ENV_VARS=1`, enabling zero-setup CI/CD usage without any prior config
command.

This replaces the `credential-storage: env` approach from PR #197, which was abandoned
because tying a no-op storage mode to credential injection was confusing, blocked
migration back to real storage, and created ambiguity around what happened to pre-existing
stored credentials.

## Goals

- Uniform behavior: env vars are either consulted for ALL credential types (Aura, DBMS,
  Embed) or for none — no per-type surprises.
- Predictable: env vars never accidentally override stored credentials unless explicitly
  opted in.
- CI/CD-native: setting `NEO4J_CLI_ACCEPT_ENV_VARS=1` alongside the secret vars is all a
  runner needs — no setup command, no committed config file required.
- Non-destructive: stored credentials are untouched; env var credentials are ephemeral and
  never persisted.

## Non-Goals

- A new credential storage mode (`credential-storage: env` is abandoned).
- Auto-detecting CI environments (e.g. `GITHUB_ACTIONS`, `CI`) — explicit opt-in only.
- Partial env var overrides for DBMS (the existing "all-or-nothing" completeness rule is
  preserved).
- Changing how credentials are stored, migrated, or managed in any other respect.

## Requirements

### Functional Requirements

- **REQ-F-001**: Add a global config key `accept-env-vars` (boolean, default `false`).
  Settable via `neo4j-cli config set accept-env-vars true`.

- **REQ-F-002**: Bind the env var `NEO4J_CLI_ACCEPT_ENV_VARS=1` (via
  `Viper.BindEnv("accept-env-vars", ...)`) so the config can be activated without writing
  to disk — the canonical CI bootstrap path.

- **REQ-F-003**: When `accept-env-vars` is `false` (the default), ALL existing env var
  reads for credential data are suppressed. This applies to:
  - DBMS: `NEO4J_URI`, `NEO4J_USERNAME`, `NEO4J_PASSWORD`, `NEO4J_DATABASE` — including the
    `NEO4J_DATABASE` override applied alongside `--credential` (the CLI-212 path; see
    REQ-F-013).
  - Embed: `NEO4J_EMBED_PROVIDER`, `NEO4J_EMBED_MODEL`, `NEO4J_EMBED_BASE_URL`,
    `NEO4J_EMBED_DIMENSIONS`, `NEO4J_EMBED_API_KEY`, `OPENAI_API_KEY`, `HF_TOKEN`,
    `GEMINI_API_KEY`, `GOOGLE_API_KEY`
  - Aura: `NEO4J_AURA_CLIENT_ID`, `NEO4J_AURA_CLIENT_SECRET` (currently unimplemented —
    this is the primary gap being closed)

- **REQ-F-004**: When `accept-env-vars` is `true`, the credential resolution precedence for
  every credential type is:
  1. Explicit CLI flag (e.g. `--uri`, `--username`, `--embed-provider`)
  2. Environment variable
  3. Stored default credential

- **REQ-F-005**: Aura credential injection (new capability): when `accept-env-vars` is
  `true` and both `NEO4J_AURA_CLIENT_ID` and `NEO4J_AURA_CLIENT_SECRET` are set, synthesize
  an ephemeral in-memory `AuraCredential` and use it as the active credential for the
  invocation. Secrets are never persisted to disk or the keyring.

- **REQ-F-006**: Aura JWT token cache (rate-limit avoidance): in env-var mode, the Aura
  client secret triggers a fresh OAuth JWT mint on every process start. To avoid
  API rate limits on repeated CLI invocations in CI, cache the **derived JWT** (not the
  client secret) to an OS-temp file (`os.TempDir()/neo4j-cli-aura-token-<hash>.json`,
  mode `0600`). The cache is identity-bound: filename and in-file hash include
  `sha256(clientID ∥ clientSecret ∥ auth-url)`. A mismatch or expired cache falls back to
  a fresh mint. This cache is only active when env vars provide the Aura credential; the
  normal keyring/insecure modes already persist the token via credentials.json.

- **REQ-F-007**: When `accept-env-vars` has **never been explicitly set** in config
  (`Viper.IsSet("accept-env-vars")` is false — i.e. the user has never run
  `config set accept-env-vars <value>` and has not set `NEO4J_CLI_ACCEPT_ENV_VARS`) and one
  or more of the credential-type env vars are present in the process environment, emit a
  single-line hint to stderr:
  `hint: credential env vars detected but accept-env-vars is not set — run 'neo4j-cli config set accept-env-vars true' or set NEO4J_CLI_ACCEPT_ENV_VARS=1`
  Once the user has explicitly set the key to any value (including `false`), the hint is
  permanently suppressed — it is a one-time discovery prompt, not an ongoing warning. The
  hint fires at most once per invocation and only when a relevant env var is actually set.

- **REQ-F-008**: The `config set` command accepts and validates `accept-env-vars` as a
  valid key with values `true` / `false`.

- **REQ-F-009**: `neo4j-cli config get accept-env-vars` and the full config listing include
  the key.

- **REQ-F-010**: When `accept-env-vars` is `true` and a **partial** (incomplete) set of env
  vars for any credential type is detected, fail immediately with an explicit error
  identifying which vars are missing. The resolution MUST NOT silently fall back to a stored
  credential when any env vars for that type were set. Per type:
  - **Aura**: `NEO4J_AURA_CLIENT_ID` present without `NEO4J_AURA_CLIENT_SECRET` (or vice
    versa) → `"NEO4J_AURA_CLIENT_ID and NEO4J_AURA_CLIENT_SECRET must both be set"`
  - **DBMS**: if any of `NEO4J_URI`, `NEO4J_USERNAME`, `NEO4J_PASSWORD` is present, all
    three must be set → error listing the missing variables (e.g.
    `"NEO4J_USERNAME and NEO4J_PASSWORD must be set when NEO4J_URI is provided"`).
    `NEO4J_DATABASE` remains optional.
  - **Embed**: if `NEO4J_EMBED_PROVIDER` is set and the chosen provider requires an API key
    (all providers except Ollama), at least one of the applicable key vars
    (`NEO4J_EMBED_API_KEY`, `OPENAI_API_KEY`, `HF_TOKEN`, `GEMINI_API_KEY`,
    `GOOGLE_API_KEY`) must be present → error naming the missing key var(s).

- **REQ-F-011**: DBMS env-var gating is implemented at the single shared resolution
  chokepoint `dbconn.ResolveConn` (`neo4j-cli/internal/dbconn/conn.go`), NOT in
  `neo4j-cli/query/connect.go` (which no longer reads connection env vars — that logic was
  centralised into `dbconn` after this PRD was first written). Because the gate lives at
  the shared resolution layer, it applies uniformly and automatically to every command that
  resolves a DBMS connection through `dbconn.ResolveConn`. There is no per-command env-var
  handling and no per-command gate.

- **REQ-F-012**: The `admin` command tree added since this PRD was written — `admin database`,
  `admin user`, `admin role`, `admin privilege` (and any future admin leaf) — resolves its
  connection via `dbconn.ResolveConn` (with `skipDatabase=true`, admin mode). It therefore
  inherits the gated DBMS env-var behaviour from REQ-F-011 with no admin-specific code:
  with `accept-env-vars` false, admin commands ignore `NEO4J_URI`/`NEO4J_USERNAME`/
  `NEO4J_PASSWORD` and use the stored default credential; with `accept-env-vars` true they
  honour those env vars at the documented precedence (REQ-F-004) and apply the same
  partial-set completeness error (REQ-F-010, DBMS group). `NEO4J_DATABASE` is never consulted
  in admin mode (`skipDatabase=true`), independent of `accept-env-vars`.

- **REQ-F-013**: The `NEO4J_DATABASE` override in `applyDBOverride` (the CLI-212 capability
  letting `--database`/`NEO4J_DATABASE` override a credential-supplied database when
  `--credential` is used) is ALSO gated behind `accept-env-vars`. When `accept-env-vars` is
  false, `NEO4J_DATABASE` no longer overrides the credential's database; the explicit
  `--database` flag continues to override unconditionally (flags are never gated). This is a
  behavioural change to CLI-212 and must be called out in the changelog/README (see
  REQ-NF-003).

### Non-Functional Requirements

- **REQ-NF-001**: Changing `accept-env-vars` has no side effects on credential storage.
  Stored credentials are never moved, deleted, or modified. The option controls only the
  runtime resolution path.

- **REQ-NF-002**: The JWT token cache file contains only the derived OAuth JWT, never the
  raw client secret. The file is written at `0600` permissions.

- **REQ-NF-003**: The breaking change to DBMS/embed env var reads (gating behind
  `accept-env-vars`) must be documented in the changelog and README. See REQ-NF-007 for the
  changelog kind.

- **REQ-NF-004**: All three credential types must behave symmetrically: same gate, same
  precedence rule, same hint message.

- **REQ-NF-006**: Because the DBMS gate lives once in `dbconn.ResolveConn`, query and the
  admin command tree behave identically with no duplicated gating logic, and any future
  DBMS-connecting command inherits the gate for free. No env reads for connection params may
  remain outside `dbconn.ResolveConn` (i.e. none may be reintroduced into `query/connect.go`
  or an admin leaf).

- **REQ-NF-007**: Despite the breaking changes (REQ-F-011/F-012/F-013 and the embed gate),
  the changelog entry MUST be a **`Minor`** changie kind, NOT `Major`. `neo4j-cli` is an
  experimental project and this breaking change is being accepted under a minor version bump
  by explicit decision. Create the entry with
  `changie new --projects neo4j-cli --kind Minor --body <body>`; the body must still clearly
  describe the breaking behaviour so users reading the changelog are warned, even though the
  version increment is minor.

- **REQ-NF-005**: Hint detection and completeness validation MUST NOT be implemented
  per-type in each resolution path. Instead, define a shared `EnvCredentialSpec` metadata
  type in `common/clicfg/credentials/` that encodes, per credential type: (a) sentinel
  env vars for hint detection, (b) required groups (sets of vars that must all be present
  if any one is set), and (c) optional vars. Package-level functions
  `HasAnyCredentialEnvVar(getenv)` and `ValidateEnvCredentialSet(spec, getenv)` operate
  on these specs. This mirrors the existing `sensitiveField` / `keyringCredential` pattern:
  metadata declared once per type, shared logic consuming the metadata. A new credential
  type MUST add an `EnvCredentialSpec` var before its env vars are reachable — absence is
  a compile-time gap, not a runtime one.

## Technical Considerations

### Config layer (`common/clicfg/clicfg.go`)

- Add `"accept-env-vars"` to `GlobalConfig.ValidConfigKeys`.
- Add `bindEnvironmentVariables`: `Viper.BindEnv("accept-env-vars", "NEO4J_CLI_ACCEPT_ENV_VARS")`.
- Add `GlobalConfig.AcceptEnvVars() bool` accessor (`Viper.GetBool("accept-env-vars")`).
- Add validation in `GlobalConfig.Set()` for `accept-env-vars` (true/false only).

### DBMS env var gate (`neo4j-cli/internal/dbconn/conn.go`)

NOTE: this moved since the PRD was first drafted. DBMS connection env-var reads no longer
live in `query/connect.go`; they were centralised into `dbconn.ResolveConn`
(`neo4j-cli/internal/dbconn/conn.go`), which `query/connect.go`, the `admin` command tree
(`admin.go`), and desktop resolution (`desktop.go`) all call. Gating here is what makes the
behaviour consistent across query AND admin (REQ-F-011/REQ-F-012) with no per-command code.

The reads to gate (all currently unconditional, `cfg` is already a parameter of
`ResolveConn`):

- `conn.go` ~145-150: `uri/username/password/database := Overlay(dotenvVals[...], os.Getenv(...))`.
  Gate ONLY the `os.Getenv(...)` argument behind `cfg.Global.AcceptEnvVars()` — when the flag
  is false, pass `""` for the OS-env half so resolution falls back to the dotenv value (if
  any) then the stored credential. The dotenv (`dotenvVals`, the `--env` walk-up) half is NOT
  gated — it is the separate dotenv mechanism (see Out of Scope). Practically: introduce a
  small helper (e.g. `gatedGetenv(cfg, name)` returning `""` when `AcceptEnvVars()` is false)
  and use it in place of the raw `os.Getenv` calls.
- `conn.go` ~294 (`applyDBOverride`): `os.Getenv(EnvDatabase)` — gate behind
  `AcceptEnvVars()` per REQ-F-013. The `--database` flag branch above it is unchanged.

The existing "all-or-nothing" partial-params completeness rule (the `explicitCount` switch)
is unchanged; only the env-var participation in that resolution is gated. With the gate off,
env vars contribute nothing to `explicitCount`, so a stored credential is used cleanly.

The DBMS completeness error required by REQ-F-010 (`ValidateEnvCredentialSet(DBMSEnvSpec, …)`)
runs inside `ResolveConn` (gated to `AcceptEnvVars()`), so it too covers query and admin
uniformly. Note `ResolveConn` already emits a partial-params error of its own; reconcile the
two so the env-mode message names the missing `NEO4J_*` variables (REQ-F-010) rather than the
`--flag/ENV` dual form.

### Embed env var gate (`neo4j-cli/query/embed/embed.go`)

The `.env` file walk-up and `os.Getenv` calls for embed provider params (lines ~150-289)
are currently unconditional. Gate all `os.Getenv` calls (but not the `.env` file walk-up,
which is controlled by the `--env` flag / dotenv mechanism) behind `AcceptEnvVars()`.
Note: provider-specific key lookups (`OPENAI_API_KEY`, `HF_TOKEN`, etc.) are also gated.

The `.env` file mechanism is SEPARATE from this feature — dotenv files are controlled by
the `--env` flag and the walk-up logic, not by `accept-env-vars`. Only OS env reads are
gated.

### Aura credential injection (new, `neo4j-cli/aura/internal/api/` area)

When `accept-env-vars` is true:
- Read `NEO4J_AURA_CLIENT_ID` + `NEO4J_AURA_CLIENT_SECRET` (both required; one alone is
  an error / no-op).
- Construct an ephemeral `*credentials.AuraCredential` and call
  `cfg.Aura.SetActiveCredential(...)`. The existing `api.go` credential resolution already
  checks `cfg.Aura.ActiveCredential()` first (line ~99), so no further plumbing needed.
- Synthesise the credential before `getToken` is called (in the Aura root
  `PersistentPreRunE` or in `getToken` itself).

### Aura JWT token cache (`neo4j-cli/aura/internal/api/token_cache.go`)

Reuse the design from PR #197:
- Cache file: `filepath.Join(os.TempDir(), fmt.Sprintf("neo4j-cli-aura-token-%s.json", shortHash))`
- `shortHash` = first 16 hex chars of `sha256(clientID + "|" + clientSecret + "|" + authURL)`
- Cache file contains: `{"token": "<jwt>", "expiry": "<rfc3339>", "hash": "<fullHash>"}`
- On read: verify full hash, check expiry with a 60-second buffer.
- On failure (missing / corrupt / expired / hash-mismatch): mint fresh, write back.
- Cache is only consulted when Aura credentials came from env vars (not keyring/insecure).

### Shared env var spec (`common/clicfg/credentials/env_spec.go`)

This is the key extensibility hook. Define in `common/clicfg/credentials/`:

```go
// EnvCredentialSpec describes the env vars for one credential type.
type EnvCredentialSpec struct {
    // Sentinel is the single env var whose presence triggers the hint.
    // Convention: the primary identifying var for the type.
    Sentinel string
    // RequiredGroups: each inner slice is a set of vars that must ALL be present
    // if ANY one of them is set. Multiple groups are validated independently.
    RequiredGroups [][]string
    // OptionalVars are vars that may be set independently (no completeness check).
    OptionalVars []string
}

// Package-level specs — one per credential type.
var AuraEnvSpec = EnvCredentialSpec{
    Sentinel:       "NEO4J_AURA_CLIENT_ID",
    RequiredGroups: [][]string{{"NEO4J_AURA_CLIENT_ID", "NEO4J_AURA_CLIENT_SECRET"}},
}

var DBMSEnvSpec = EnvCredentialSpec{
    Sentinel:       "NEO4J_URI",
    RequiredGroups: [][]string{{"NEO4J_URI", "NEO4J_USERNAME", "NEO4J_PASSWORD"}},
    OptionalVars:   []string{"NEO4J_DATABASE"},
}

var EmbedEnvSpec = EnvCredentialSpec{
    Sentinel:       "NEO4J_EMBED_PROVIDER",
    RequiredGroups: [][]string{}, // API key completeness is provider-specific; see ValidateEmbedEnvSet
    OptionalVars:   []string{"NEO4J_EMBED_MODEL", "NEO4J_EMBED_BASE_URL",
        "NEO4J_EMBED_DIMENSIONS", "NEO4J_EMBED_API_KEY",
        "OPENAI_API_KEY", "HF_TOKEN", "GEMINI_API_KEY", "GOOGLE_API_KEY"},
}

// HasAnyCredentialEnvVar returns true if any sentinel env var across all specs is set.
// Used for hint detection.
func HasAnyCredentialEnvVar(getenv func(string) string) bool

// ValidateEnvCredentialSet checks that if any var in a RequiredGroup is set, all are set.
// Returns a clierr.UsageError listing missing vars, or nil if the set is complete.
func ValidateEnvCredentialSet(spec EnvCredentialSpec, getenv func(string) string) error
```

Embed API-key completeness is provider-specific (Ollama needs no key) so it gets its own
`ValidateEmbedEnvSet(getenv)` function rather than squeezing into the generic
`RequiredGroups` mechanism. All other completeness checks use `ValidateEnvCredentialSet`.

A new credential type wires in by: (1) declaring an `XxxEnvSpec` var, (2) adding its
sentinel to `HasAnyCredentialEnvVar`, (3) calling `ValidateEnvCredentialSet` in its
resolution path. Steps 1 and 3 are the mandatory gates; missing step 1 is immediately
visible as a compiler error if the sentinel is referenced from the function.

### Hint message

Detect at startup (early in `PersistentPreRunE` or `NewConfig`) whether:
- `Viper.IsSet("accept-env-vars")` is **false** (key has never been explicitly written), AND
- `HasAnyCredentialEnvVar(os.Getenv)` returns true.

If both conditions hold, emit to `os.Stderr` once per process. Once the user has
set the key to any value, `Viper.IsSet` returns true and the hint is never shown again —
even if they later set it to `false`. Add an `AcceptEnvVarsIsSet() bool` accessor to
`GlobalConfig` (mirrors the existing `CredentialStorageIsSet()` pattern).

### Incomplete env var validation

When `accept-env-vars=true`, call `ValidateEnvCredentialSet(spec, os.Getenv)` for Aura
and DBMS, and `ValidateEmbedEnvSet(os.Getenv)` for Embed, before resolution. Run the Aura
check in the Aura root `PersistentPreRunE` and the DBMS/Embed checks in `connect.go` /
`embed.go`. All functions return a `clierr.NewUsageError` so the CLI surfaces a clean
usage-style message. Validation order: check for any type-specific env var first; if any
are found but the set is incomplete, error immediately rather than falling through.

### Breaking change handling

`DBMS` and embed env var reads are currently unconditional. Gating them is a breaking
change for users who already rely on `NEO4J_URI` etc. without opt-in. Two specific breaks:
- DBMS connection env vars (`NEO4J_URI`/`USERNAME`/`PASSWORD`) for both `query` and the new
  `admin` commands now require `accept-env-vars` (REQ-F-011/F-012).
- The CLI-212 `NEO4J_DATABASE`-with-`--credential` override now requires `accept-env-vars`
  (REQ-F-013); `--database` is unaffected.

Mitigations:
- The hint message (REQ-F-007) prompts existing users to opt in. The hint is emitted once
  per process at startup (in the root `PersistentPreRunE` / `NewConfig`), so it surfaces
  regardless of which command — query, admin, embed — is being run.
- Changelog entry documents both breaks (DBMS/embed gate and the CLI-212 behaviour change).
- README "Environment variables" section updated to describe the gate and that it covers the
  admin command tree.

## Acceptance Criteria

- [ ] `neo4j-cli config set accept-env-vars true` succeeds; `config get accept-env-vars`
  returns `true`.
- [ ] `neo4j-cli config set accept-env-vars false` succeeds; invalid values return a usage
  error.
- [ ] With `accept-env-vars` unset (default), setting `NEO4J_URI` has no effect on query
  connection — stored credential is used.
- [ ] With `accept-env-vars=true` (via config or `NEO4J_CLI_ACCEPT_ENV_VARS=1`),
  `NEO4J_URI/USERNAME/PASSWORD/DATABASE` override the stored DBMS credential.
- [ ] Admin commands behave identically to query: with `accept-env-vars=true`,
  `neo4j-cli admin user list` (and other admin leaves) honour `NEO4J_URI/USERNAME/PASSWORD`;
  with `accept-env-vars` unset/false they ignore them and use the stored default credential.
- [ ] The env gate is implemented once in `dbconn.ResolveConn`; no connection env reads
  remain in `query/connect.go` or any admin leaf (REQ-NF-006).
- [ ] With `accept-env-vars` false, `NEO4J_DATABASE` no longer overrides a `--credential`
  credential's database (CLI-212 path gated); with `accept-env-vars=true` it does; the
  explicit `--database` flag overrides in both cases.
- [ ] With `accept-env-vars=true` and a partial DBMS env set, both `query` and `admin`
  surface the same missing-variable error.
- [ ] With `accept-env-vars=true` and `NEO4J_AURA_CLIENT_ID` + `NEO4J_AURA_CLIENT_SECRET`
  set, Aura commands succeed without any stored credential.
- [ ] With `accept-env-vars=true` and `NEO4J_EMBED_API_KEY` set, embed commands use that
  key over the stored embed credential.
- [ ] With `accept-env-vars` never set (default) and `NEO4J_URI` present, stderr includes
  the hint text.
- [ ] With `accept-env-vars` explicitly set to `false` and `NEO4J_URI` present, the hint is
  NOT shown (user has consciously opted out).
- [ ] With `accept-env-vars=true`, setting only `NEO4J_AURA_CLIENT_ID` (without SECRET)
  returns a usage error naming the missing variable.
- [ ] With `accept-env-vars=true`, setting only `NEO4J_URI` (without `NEO4J_USERNAME` /
  `NEO4J_PASSWORD`) returns a usage error naming the missing variables.
- [ ] With `accept-env-vars=true`, setting `NEO4J_EMBED_PROVIDER=openai` without any API
  key variable returns a usage error.
- [ ] `NEO4J_CLI_ACCEPT_ENV_VARS=1` (no config file change) enables env var reads.
- [ ] Aura JWT cache: two sequential Aura invocations with the same client ID/secret result
  in one OAuth token mint (second invocation uses cache).
- [ ] Aura JWT cache: changing `NEO4J_AURA_CLIENT_SECRET` causes a cache miss and a fresh
  mint.
- [ ] `AuraEnvSpec`, `DBMSEnvSpec`, and `EmbedEnvSpec` are defined in
  `common/clicfg/credentials/`; hint detection and completeness validation use them rather
  than hardcoded var lists in each resolution path.
- [ ] `HasAnyCredentialEnvVar` and `ValidateEnvCredentialSet` have unit tests in
  `common/clicfg/credentials/env_spec_test.go`.
- [ ] Stored credentials are unmodified before and after enabling `accept-env-vars`.
- [ ] `make test`, `make fmt-check`, `make lint` all pass.
- [ ] `go generate ./neo4j-cli/internal/skill/...` produces no bundle drift.
- [ ] Changelog and README document the new config key, the env var bootstrap, and the
  breaking change to existing env var behaviour.
- [ ] The changelog entry is created with `--kind Minor` (NOT `Major`), per the explicit
  decision to accept this breaking change as a minor for this experimental project
  (REQ-NF-007), while still describing the breaking behaviour in the body.

## Out of Scope

- `credential-storage: env` as a storage mode (explicitly abandoned in favour of this
  approach).
- Auto-detecting CI environments.
- Fine-grained per-credential-type gates (single boolean covers all types).
- Changing the dotenv (`.env` file) walk-up mechanism — that is controlled by `--env` /
  the existing dotenv logic and is unaffected by this feature.

## Open Questions

None — all questions resolved during PRD refinement:
- Hint fires only when `accept-env-vars` has never been explicitly set; once set to any
  value the hint is permanently suppressed (not an ongoing warning).
- Partial env var sets for any credential type (Aura, DBMS, Embed) are a hard error, not a
  silent fallback.
- The DBMS env gate lives in the shared `dbconn.ResolveConn`, so the `admin` command tree
  (added to main after this PRD was drafted) inherits the behaviour with no per-command code
  (REQ-F-011/F-012).
- The CLI-212 `NEO4J_DATABASE`-with-`--credential` override is gated behind `accept-env-vars`
  for consistency, accepting the behavioural change to CLI-212 (REQ-F-013).
