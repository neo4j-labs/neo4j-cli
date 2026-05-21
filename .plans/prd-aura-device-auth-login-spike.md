# PRD: Aura Device Auth Flow Spike (neo4j-cli aura login)

## Overview

Spike that implements OAuth 2.0 Device Authorization Grant (RFC 8628) as a new `neo4j-cli aura login` command. The goal is to validate the end-to-end device flow against the Aura auth server. Success criterion: the command prints the received access token to stdout. No credential storage, no integration with the existing `AuraCredential` workflow.

## Goals

- Validate device auth flow works against the Aura Auth0 tenant.
- Establish the HTTP polling pattern and user-facing UX (display URL + code, poll, print token) for a future full implementation.
- Persist the obtained token into the credential store so all existing `aura` subcommands authenticate automatically without requiring a client ID / secret.

## Non-Goals

- Refresh token support (re-login required on expiry).
- `--name` flag for the stored credential name (fixed as `"login"` for this spike).
- `--format json` output support.
- Configuration of auth server URL, client ID, or audience via CLI flags or persistent config.
- Production hardening (retries, error-message polish, telemetry).
- Changelog entry (internal spike).

## Requirements

### Functional Requirements

- **REQ-F-001**: Add a new leaf command `neo4j-cli aura login` mounted under `aura.NewCmd` in `neo4j-cli/aura/aura.go`.
- **REQ-F-002**: On invocation, POST to the device authorization endpoint with the `client_id` and `audience` (read from env vars — see REQ-F-007) to obtain a device code response (`device_code`, `user_code`, `verification_uri`, `verification_uri_complete`, `expires_in`, `interval`).
- **REQ-F-003**: Print the `verification_uri_complete` (or `verification_uri` + `user_code` if complete is absent) and instruct the user to open it in their browser to authenticate.
- **REQ-F-004**: Poll the token endpoint at the interval returned in the device code response using grant type `urn:ietf:params:oauth:grant-type:device_code`, handling the following error codes:
  - `authorization_pending` — continue polling.
  - `slow_down` — increase poll interval by 5 seconds and continue.
  - `expired_token` — return a clear error.
  - `access_denied` — return a clear error.
- **REQ-F-005**: Respect the `expires_in` from the device code response; if polling times out before authorization, return a clear error.
- **REQ-F-006**: On success, print a confirmation message to stderr (e.g. `Logged in successfully. Run 'neo4j-cli aura instance list' to verify.`). Do not print the raw token to stdout.
- **REQ-F-008**: After a successful token exchange, persist the token into the credential store under the fixed name `"login"` via a new `AuraCredentials.AddOrUpdateFromToken(name, clientId, accessToken string, expiresIn int64)` method. If a credential named `"login"` already exists, update its `AccessToken` and `TokenExpiry` in-place. Set `"login"` as the default credential if no default is currently set.
- **REQ-F-009**: Update `pollForToken` (or its caller) to return the full token response including `expires_in` so `AddOrUpdateFromToken` can compute the correct expiry.
- **REQ-F-010**: In `neo4j-cli/aura/internal/api/token.go`, when the resolved credential has an empty `ClientSecret` and no valid access token, return a `clierr.AuthError` telling the user to re-run `neo4j-cli aura login` rather than attempting a client-credentials grant that would fail.
- **REQ-F-011**: `AddOrUpdateFromToken` must **always** set the named credential as the default (not only when `DefaultCredential == ""`), so that running `neo4j-cli aura login` makes subsequent commands use the new device-auth token regardless of any previously stored default credential.
- **REQ-F-012**: Update the login confirmation message to indicate the credential has been set as the active default (e.g. `Login successful. Credential "login" stored and set as default.`).
- **REQ-F-007**: Read all auth configuration from environment variables at runtime (via `os.Getenv`). Required variables:
  - `NEO4J_AURA_LOGIN_DEVICE_ENDPOINT` — device authorization endpoint URL
  - `NEO4J_AURA_LOGIN_TOKEN_ENDPOINT` — token endpoint URL
  - `NEO4J_AURA_LOGIN_CLIENT_ID` — public OAuth client ID
  - `NEO4J_AURA_LOGIN_AUDIENCE` — OAuth audience
  If any variable is unset or empty, the command must return a clear error naming the missing variable.

### Non-Functional Requirements

- **REQ-NF-001**: No new external dependencies — use stdlib `net/http`, `encoding/json`, `time`, `context` only.
- **REQ-NF-002**: Command must have a non-empty flush-left `Example:` field with `≥3` invocations (required by `TestAllLeafCommands_HaveExamples`).
- **REQ-NF-003**: All new `.go` files must carry the Neo4j copyright header (enforced by `make license-check`).
- **REQ-NF-004**: Skill bundle must be regenerated after adding the command (`go generate ./neo4j-cli/internal/skill/...`), otherwise `TestGenerator_RoundTrip` fails.
- **REQ-NF-005**: A committed `.env.aura-login-spike.example` file at the repo root documents the required env var names (no values). Add `.env.aura-login-spike` to `.gitignore` so real values are never committed.
- **REQ-NF-006**: `AddOrUpdateFromToken` lives in `common/clicfg/credentials/aura.go` (not under `neo4j-cli/internal/`) so it is reachable from both the login command and future callers in `common/`. Token expiry is computed with the same 60-second tolerance already used by `UpdateAccessToken`.

## Technical Considerations

**File layout** — single-file leaf (no children), following the one-file-per-leaf convention:
```
neo4j-cli/aura/internal/subcommands/login/
  login.go        # newLoginCmd(cfg) *cobra.Command
  login_test.go   # table-driven tests
```
Mount in `neo4j-cli/aura/aura.go` `NewCmd`:
```go
cmd.AddCommand(login.NewCmd(cfg))
```

**Auth configuration — env vars** (no values committed; share `.env.aura-login-spike` out-of-band):
| Env var | Purpose |
|---|---|
| `NEO4J_AURA_LOGIN_DEVICE_ENDPOINT` | Device authorization endpoint URL |
| `NEO4J_AURA_LOGIN_TOKEN_ENDPOINT` | Token endpoint URL |
| `NEO4J_AURA_LOGIN_CLIENT_ID` | Public OAuth client ID |
| `NEO4J_AURA_LOGIN_AUDIENCE` | OAuth audience |

**Usage** — populate `.env.aura-login-spike` (gitignored) from the shared env file, then:
```bash
source .env.aura-login-spike
neo4j-cli aura login
```

**Manual test config** (to point the rest of the CLI at the matching dev environment — values in shared env file):
```bash
neo4j-cli config set aura.base-url <dev-base-url>
neo4j-cli config set aura.auth-url <dev-token-endpoint>
```

**Polling loop** — use a `context.WithTimeout` derived from `expires_in`; `time.Sleep` between polls using a mutable interval variable (start from response `interval`, increment by 5 on `slow_down`).

**`--rw` not required** — this command does not mutate credential store state, so the write-guard should not apply.

**Do not** use `cfg.Aura.AuthUrl()` or `api.MakeRequest` for the device auth HTTP calls — the login command is intentionally standalone for those. However, after storing the credential, the existing `api.MakeRequest` / `getToken` path is used by all other aura commands as normal.

**Token integration path:**
1. `pollForToken` returns `tokenResponse` (struct with `AccessToken` + `ExpiresIn`) instead of just a `string`.
2. `login` `RunE` calls `cfg.Credentials.Aura.AddOrUpdateFromToken("login", cfg.ClientId, token.AccessToken, token.ExpiresIn)`.
3. Existing `getToken` in `api/token.go` already checks `HasValidAccessToken()` first — the stored token is returned without a client-credentials round-trip while it is valid.
4. On expiry, add a guard: `if credential.ClientSecret == "" { return "", clierr.NewAuthError(...) }`.

## Acceptance Criteria

- [ ] `neo4j-cli aura login` initiates device auth, prints a verification URL + code, polls, and on success prints a confirmation message (not the raw token).
- [ ] After login, `neo4j-cli aura instance list` (or any other aura read command) succeeds without specifying a credential, **even if another credential was previously the default**.
- [ ] Re-running `neo4j-cli aura login` updates the stored token in-place (no duplicate credential created).
- [ ] When the stored token is expired and `ClientSecret` is empty, the error message directs the user to re-run `neo4j-cli aura login`.
- [ ] `authorization_pending` is silently swallowed and polling continues.
- [ ] `slow_down` increases the poll interval by 5 s and polling continues.
- [ ] `expired_token` produces a clear user-facing error.
- [ ] `access_denied` produces a clear user-facing error.
- [ ] Missing or empty env var produces a clear error naming the unset variable.
- [ ] `.env.aura-login-spike.example` is committed with all four variable names and no values.
- [ ] `.env.aura-login-spike` is listed in `.gitignore`.
- [ ] All new source files carry the Neo4j copyright header.
- [ ] `make fmt-check` passes.
- [ ] `make lint` passes.
- [ ] `make test` passes (including `TestAllLeafCommands_HaveExamples` and `TestGenerator_RoundTrip`).
- [ ] Skill bundle regenerated (`go generate ./neo4j-cli/internal/skill/...`) and committed alongside source changes.

## Out of Scope

- Refresh token support.
- Config-driven auth server URL / client ID / audience.
- Production Auth0 client registration and production audience / scope determination.
- Changelog entry.
