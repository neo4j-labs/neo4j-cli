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
- Self-explanatory: messages, hints, config display, AND flag `--help` must accurately
  reflect the gate's state — no error or flag-usage string advertises a variable the gate is
  ignoring, the override-completeness rule matches the documented "database optional"
  contract across flags and env vars, partial-override errors name the source the user
  actually used (flags vs env), and the discovery hint suggests a command that actually
  works in CI.

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

> The requirements below (REQ-F-014 onward and REQ-NF-008/009) were added after QA of the
> implemented feature surfaced a behavioural bug and several message/display inconsistencies.

- **REQ-F-014** (QA Finding 1 — completeness bug): On the **default resolution path** (no
  `--credential`), a *complete* connection override is the THREE required params
  `--uri`/`NEO4J_URI`, `--username`/`NEO4J_USERNAME`, `--password`/`NEO4J_PASSWORD`. The
  database (`--database`/`NEO4J_DATABASE`) is ALWAYS optional and is never required to
  complete the set — when supplied it is applied, when omitted the server resolves the
  connecting user's home database. This rule applies **uniformly to BOTH explicit flags and
  env vars** (env vars remain gated behind `accept-env-vars` per REQ-F-011); the user
  explicitly chose uniform flag+env behaviour over an env-only relaxation. Specifically:
  - When a stored default credential exists, supplying all three of uri/username/password
    (via flags or env) bypasses the stored credential cleanly; `NEO4J_DATABASE`/`--database`
    is not required.
  - Supplying a strict subset (one or two of the three) remains a rejected partial override.
  - This fixes the contradiction where `dbconn.ResolveConn`'s legacy `explicitCount` /
    `fullCount` switch treated a complete set as **four** (uri/user/pass/database) in query
    mode and so rejected the three-var override that REQ-F-010 (database optional) mandates.
  - Admin mode (`skipDatabase=true`) already counts exactly the three required params and is
    unchanged.
  - This **intentionally relaxes the pre-existing flag-only contract** (previously documented
    and enforced as "all four when a stored credential exists") down to three; it must be
    reflected in the README (REQ-NF-003). CLI-212 (`--credential` + database override,
    REQ-F-013) is a separate code path (`applyDBOverride`) and is unaffected.

- **REQ-F-015** (QA Finding 2 — misleading off-mode messages): When `accept-env-vars` is
  **off**, error messages that prompt for a missing connection or embed value MUST NOT
  advertise the gated environment variables (`NEO4J_PASSWORD`, `NEO4J_URI`,
  `NEO4J_USERNAME`, `NEO4J_EMBED_*`, etc.) as if setting them would help — the gate is
  ignoring them, so telling a user to "set `NEO4J_PASSWORD`" while it is already set is
  misleading. Off-mode messages must instead reference explicit flags / `.env` files and,
  where useful, point at `accept-env-vars` (e.g. "set `--password`, or enable
  `accept-env-vars` to read `NEO4J_PASSWORD`"). When `accept-env-vars` is **on** the
  messages may continue to name the env vars. Affected sites identified in QA:
  `neo4j-cli/query/run.go` (`promptPassword`, ~line 203), `admin/admin.go` (~line 45), and
  `neo4j-cli/internal/dbconn/helpers.go` (~line 38). The change must not regress the
  one-time discovery hint (REQ-F-007).

- **REQ-F-016** (QA Finding 3 — hint command fails in CI): The discovery hint (REQ-F-007)
  MUST suggest a command that succeeds in the non-interactive / agent / CI contexts where
  the hint most often surfaces. The `config set accept-env-vars true` suggestion MUST include
  the `--rw` flag (auto-applied in an interactive TTY, but REQUIRED under an agent harness or
  non-interactive script). The hint becomes:
  `... run 'neo4j-cli config set accept-env-vars true --rw' or set NEO4J_CLI_ACCEPT_ENV_VARS=1`.
  The `NEO4J_CLI_ACCEPT_ENV_VARS=1` alternative is retained as the zero-write CI path.

> The requirements below (REQ-F-017 onward) were added after a second round of QA
> (2026-06-26, run against live local Neo4j Enterprise and live Aura) surfaced two
> message inconsistencies and two Aura OAuth/token-cache defects not caught by the first
> QA pass. They build on REQ-F-015 (off-mode message hygiene) and the Aura token cache
> (REQ-F-006).

- **REQ-F-017** (QA Finding 1 — embed off-mode message advertises a gated env var): The
  "missing embed provider" error raised when no provider can be resolved
  (`neo4j-cli/query/embed/embed.go`, `New()` — currently the unconditional string
  `"missing embed provider: set --embed-provider, NEO4J_EMBED_PROVIDER, or pick a stored
  embed credential"`) MUST obey the same off-mode rule as REQ-F-015. When `accept-env-vars`
  is **off**, the message MUST NOT advertise `NEO4J_EMBED_PROVIDER` as if setting it would
  help (the gate is ignoring it); it must reference `--embed-provider` / `.env` / a stored
  embed credential and MAY point at `accept-env-vars` (e.g. "set `--embed-provider`, or
  enable `accept-env-vars` to read `NEO4J_EMBED_PROVIDER`"). When `accept-env-vars` is
  **on** the message MAY continue to name `NEO4J_EMBED_PROVIDER`. This closes the gap where
  REQ-F-015 was applied to the connection/password sites but missed the embed *provider*
  message (the embed *API-key* message was already correctly gated to on-mode — **CORRECTION,
  see REQ-F-023: this was inaccurate; only the empty-key check in `Resolve()` was gated, while
  the per-provider `Embed()` backstop messages still advertised the gated key env vars off-mode**).
  Note `New()`
  takes a resolved `Config` without the gate flag, so the gate state must be threaded in
  (e.g. carry an `AcceptEnvVars` bool on the embed `Config`, or raise the empty-provider
  error from `Resolve`, which already has `cfg`). The off-path (provider resolved) must be
  untouched.

- **REQ-F-018** (QA Finding 4 — partial-params DBMS error names gated env vars off-mode):
  The DBMS partial-override error in `dbconn.ResolveConn` (currently the unconditional dual
  form `"partial connection params: when any of --uri/NEO4J_URI, --username/NEO4J_USERNAME,
  or --password/NEO4J_PASSWORD is provided, all three (--uri, --username, --password) are
  required"`) MUST be consistent with REQ-F-015. When `accept-env-vars` is **off**, the
  message MUST NOT name the gated `NEO4J_URI`/`NEO4J_USERNAME`/`NEO4J_PASSWORD` env vars
  (only one or two `--flags` could have been supplied to reach this branch in off-mode, since
  env vars contribute nothing to the count when gated); it should name only the `--flags`.
  When `accept-env-vars` is **on**, the message MAY retain the dual `--flag/NEO4J_*` form (and
  in on-mode the env-completeness check in `ValidateEnvCredentialSet` already names the
  missing `NEO4J_*` vars for env-supplied subsets, per REQ-F-010/REQ-F-014). This is a
  wording-only change; the completeness decision (three required params, database optional —
  REQ-F-014) is unchanged.

- **REQ-F-019** (QA Finding 3 — OAuth mint swallows non-2xx and returns an empty token):
  `mintTokenHTTP` (`neo4j-cli/aura/internal/api/token.go`) currently special-cases only HTTP
  401 (auth error) and 404 (panic); its `case http.StatusBadRequest:` and
  `case http.StatusForbidden:` arms have empty bodies, so a 400/403 (or any other unlisted
  non-2xx, e.g. 422/500) falls through, the body is parsed into a `Grant`, and an **empty
  `AccessToken` is returned with no error**. Downstream this produces an
  `Authorization: Bearer ` (empty) request and the alarming generic
  `"unexpected status code 422 ... please report an issue"`. The mint MUST instead return a
  clear, actionable error for every non-2xx and MUST NOT return an empty access token as
  success. Required behaviour:
  - **401** → existing auth error ("the provided credentials are invalid, expired, or
    revoked").
  - **403** → a *distinct* error indicating the credentials are valid but not authorized for
    this request — NOT the 401 wording, so a 403 user is not told their credentials are
    invalid. The message MUST NOT mention a mismatched Aura environment: a wrong-environment
    credential is a niche cause that mainly affects internal users and would clutter the
    user-facing message, so keep the 403 wording to a clean "forbidden / credentials valid but
    not authorized for this request" (lacking the required permission). Suggested shape:
    `"forbidden (HTTP 403): the credentials are valid but not authorized for this request
    (insufficient permission)."`
  - **Any other non-2xx** (incl. 400/422/5xx) and any 2xx that yields an **empty
    `access_token`** → a clear error naming the status (or "empty token") rather than the
    generic "please report" panic.
  - This fix applies to **all OAuth mint paths** (the single `mintTokenHTTP` chokepoint), so
    it benefits env-var-synthesized credentials AND stored keyring/insecure credentials
    uniformly. This is a pre-existing defect being fixed here because the env-var/CI flow
    (REQ-F-005/006) makes a clean OAuth error materially more important; it is a behavioural
    improvement to Aura auth error messaging across the board, not specific to
    `accept-env-vars`.

- **REQ-F-020** (QA Finding 2 — empty token persisted to the on-disk token cache): In
  env-var mode, `getToken` calls `writeTokenCache(..., grant.AccessToken, ...)` after a mint
  regardless of whether a token was actually obtained, so a failed/empty mint writes a junk
  `{"token":"","expiry":...}` file into `os.TempDir()` on every invocation. `readTokenCache`
  already rejects empty tokens (so there is no cache *poisoning*), but the cache MUST NOT
  persist an empty token at all. `writeTokenCache` (or its caller) MUST skip the write when
  the token is empty. With REQ-F-019 in place a failed mint returns an error before the cache
  write is reached, so this is belt-and-braces; keep the guard so the cache layer is correct
  independent of caller behaviour.

> The requirements below (REQ-F-021..022) were added after a third round of QA
> (2026-06-26, run against the built binary with live local Docker/Desktop and live Aura)
> surfaced two message/help-text inconsistencies in the same family as REQ-F-015..018: the
> gate's existence is not reflected in the flag `--help` surface, and the on-mode partial
> DBMS override message misattributes flag-only mistakes to environment variables.

- **REQ-F-021** (QA Finding 1 — flag `--help` advertises gated env vars unconditionally):
  The per-flag usage strings for the gated connection and embed flags currently append a
  `[env: NEO4J_*]` clause with no indication the read is gated — `--uri` (`[env: NEO4J_URI]`),
  `--username` (`[env: NEO4J_USERNAME]`), `--password` (`[env: NEO4J_PASSWORD]`), `--database`
  (`[env: NEO4J_DATABASE]`), `--embed-provider` (`[env: NEO4J_EMBED_PROVIDER]`),
  `--embed-model`, `--embed-base-url`, `--embed-dimensions`. Because `accept-env-vars` is
  **off by default**, this is the same defect REQ-F-015/017/018 fixed for error messages —
  `--help` advertises a variable the gate is ignoring — and it is the most-read discovery
  surface. It is compounded by the sentinel-only hint (REQ-NF-009): a user who follows
  `--help` and sets only a non-sentinel var such as `NEO4J_PASSWORD` gets **no effect AND no
  hint**. Required behaviour:
  - REMOVE the `[env: NEO4J_*]` clause from each gated-flag usage string (connection +
    embed flags listed above).
  - Do NOT simply drop the discoverability: document the gated env vars ONCE, in a
    consolidated, gate-aware paragraph in the relevant top-level command `Long`
    descriptions (at minimum the `query` command and the `admin` root; the `NEO4J_EMBED_*`
    set on `query`). That paragraph MUST name the env vars AND state they are only read when
    `accept-env-vars` is enabled, pointing at `neo4j-cli config set accept-env-vars true --rw`
    / `NEO4J_CLI_ACCEPT_ENV_VARS=1` and the README "Environment variables" section.
  - The non-gated `--debug` clause (`[env: NEO4J_DEBUG (set to 1 to enable)]`) is CORRECT —
    `NEO4J_DEBUG` is resolved separately and is never gated — and MUST be left unchanged.
  - Removing the per-flag clauses and editing the `Long`s changes generated help, so the
    skill bundle drifts: run `go generate ./neo4j-cli/internal/skill/...` and commit the
    regenerated bundle. The input/output casing gates and the `Example:`/leaf-example gates
    must stay green.

- **REQ-F-022** (QA Finding 2 — on-mode partial flag override names env vars; dead dual-form
  branch): With `accept-env-vars` **on**, a flag-only partial DBMS override (e.g. only
  `--uri`, with no `NEO4J_*` set) currently produces the env-only `ValidateEnvCredentialSet`
  message — `"incomplete credential environment: NEO4J_URI, NEO4J_USERNAME, NEO4J_PASSWORD
  must be set when NEO4J_URI is provided (missing: NEO4J_USERNAME, NEO4J_PASSWORD)"` — because
  that check runs against the **resolved** values (flag-or-env) before the partial-params
  switch in `dbconn.ResolveConn`. So a user who clearly used `--flags` is told to set
  `NEO4J_*` env vars. As a consequence, the on-mode dual-form branch added for REQ-F-018
  (`conn.go` ~231-235, `"--uri/NEO4J_URI ... all three (--uri, --username, --password) are
  required"`) is **unreachable dead code**, and its preceding comment (~225-230, claiming the
  branch "only fires for explicit --flag subsets") states the **opposite** of actual
  behaviour. Required behaviour: the partial-override error MUST name the source the user
  actually used. Reconcile the env-completeness check and the partial switch so that:
  - When the incomplete pieces came **only from flags** (no `NEO4J_*` contributed any of
    uri/username/password), the message names only the `--flags` (the REQ-F-018 off-mode
    wording), in BOTH gate states.
  - When one or more pieces came from `NEO4J_*` env vars, the message names the missing
    `NEO4J_*` variable(s) (preserving REQ-F-010 / REQ-F-014). A mixed subset (e.g. `--uri`
    flag + `NEO4J_USERNAME` env, missing password) names the missing `NEO4J_PASSWORD`.
  - Implement by making the completeness check source-aware (e.g. run `ValidateEnvCredentialSet`
    only over the env-sourced half, letting a purely-flag subset fall through to the
    flag-named partial branch) and REMOVE the now-dead reconciliation branch so no dead code
    remains; correct the misleading comment.
  - The off-mode behaviour (names only `--flags`) and the three-required-params /
    database-optional completeness decision (REQ-F-014) are unchanged.

> The requirement below (REQ-F-023) was added after a fourth round of QA (2026-06-26, run
> against the built binary) found that the embed **API-key** off-mode message still advertised
> gated env vars — contradicting REQ-F-017's claim that that message "was already correctly
> gated to on-mode". REQ-F-017 only gated the empty-key check in `Resolve()`; the per-provider
> backstop messages were missed.

- **REQ-F-023** (QA Finding — embed API-key off-mode message advertises gated env vars; corrects
  REQ-F-017's inaccurate claim): The per-provider missing-API-key errors raised in each
  embedding provider's `Embed()` are **unconditional** — `openai.go` (`"missing API key for
  openai: set OPENAI_API_KEY, NEO4J_EMBED_API_KEY, or store one with ... credential embed
  add"`), `gemini.go` (`GEMINI_API_KEY, GOOGLE_API_KEY, NEO4J_EMBED_API_KEY`), `huggingface.go`
  (`HF_TOKEN, NEO4J_EMBED_API_KEY`), each with a `WithSuggestion("provide a key via an env var,
  ...")`. When `accept-env-vars` is **off** and a provider is supplied via the (never-gated)
  `--embed-provider` flag (or `.env` / a stored credential) while the only API keys present are
  in OS env vars (gated away), resolution leaves `APIKey == ""` and these messages fire,
  advertising the gated `OPENAI_API_KEY`/`NEO4J_EMBED_API_KEY`/etc. and telling the user to
  "provide a key via an env var" — exactly the off-mode defect REQ-F-015/017 set out to
  eliminate. (There is no `--embed-api-key` flag; a key can come only from an env var, a `.env`
  file, or a stored embed credential.) Required behaviour (decision: centralize **and** gate the
  provider messages):
  - **(a) Centralize a gate-aware empty-key check in `Resolve()`** that fires in BOTH gate
    states (not just on-mode) for a needs-key provider that resolved no key, producing a single
    gate-aware message. This also removes the asymmetry between the on-mode-only `Resolve()`
    check (`embed.go` ~241) and the provider backstops (audit observation). **Off** → reference
    a `.env` file and a stored embed credential, and MAY point at enabling `accept-env-vars` to
    read the key env vars; it MUST NOT present the OS env vars
    (`OPENAI_API_KEY`/`NEO4J_EMBED_API_KEY`/`HF_TOKEN`/`GEMINI_API_KEY`/`GOOGLE_API_KEY`) as a
    direct fix. **On** → MAY name the env vars (current wording).
  - **(b) Make the three provider `Embed()` backstop messages gate-aware** (openai/gemini/
    huggingface) via the `cfg.AcceptEnvVars` mirror field already on the embed `Config`, so even
    when reached directly (e.g. the standalone `query :embed` leaf, or any path bypassing the
    `Resolve()` check) they obey the same off-mode rule. Both the primary message AND the
    `WithSuggestion` text must be gated.
  - **Off-mode wording shape** (mirrors REQ-F-015/017): off → `"missing API key for <provider>:
    set NEO4J_EMBED_API_KEY in a .env file, or store one with \`neo4j-cli credential embed add\`
    (or enable accept-env-vars to read <the provider's key env vars>)"`. The provider-specific
    OS-env-var names appear only in the on-mode wording and inside the "enable accept-env-vars to
    read ..." clause.
  - The `.env` path stays valid off-mode (dotenv is never gated), so it MUST remain a suggested
    fix in BOTH modes.
  - The on-path (key resolved) and the resolution precedence (flag > env > `.env` > stored) are
    untouched. The Authorization header / key value must never appear in any message (unchanged).

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

- **REQ-NF-008** (QA Finding 4 — config display): `neo4j-cli config get accept-env-vars` and
  the full `config list` SHOULD render the value consistently with sibling boolean config
  keys (e.g. `telemetry`, which displays as an unquoted `true`/`false`) rather than as a
  quoted string, and the `NEO4J_CLI_ACCEPT_ENV_VARS=1`/`0` bootstrap MUST NOT surface as the
  raw `"1"`/`"0"` literal. This is a **pre-existing** `config set` behaviour (it stores all
  values as strings), so the fix MAY be scoped to normalising the display of boolean-typed
  keys; if a safe, low-risk global fix is not feasible, the known display quirk MUST instead
  be documented. Resolution functionality (`Viper.GetBool` already treats `"true"`/`"1"` as
  true) is correct and must not regress.

- **REQ-NF-009** (QA Finding 5 — sentinel-only hint, by design): The one-time discovery hint
  fires only when a *sentinel* env var is present (`NEO4J_URI`, `NEO4J_AURA_CLIENT_ID`,
  `NEO4J_EMBED_PROVIDER`). Setting only a non-sentinel var (e.g. `NEO4J_PASSWORD` alone) does
  not trigger it. This is intended (sentinel-based detection per REQ-NF-005) and MUST be
  documented as expected behaviour rather than changed.

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

### Default-path completeness fix (REQ-F-014)

`dbconn.ResolveConn` computes `fullCount = 4` (uri/user/pass/database) in query mode
(`skipDatabase=false`) and rejects any `explicitCount` strictly between 1 and `fullCount`
when a stored credential exists (the `default:` "partial connection params" branch). Because
the new `ValidateEnvCredentialSet(DBMSEnvSpec, …)` check already treats uri/user/pass as the
required group with database optional, the two rules disagree: a three-var override
(uri/user/pass, no database) passes the new check but is then rejected by the old switch as
"all four required". Fix: base the completeness decision on the **three required params**
only — i.e. count uri/username/password toward `explicitCount` and treat database as an
always-optional extra that is applied when present but never required and never counted toward
"completeness". Concretely, a set of the three required params (regardless of database) is
"complete" and bypasses the stored credential; one or two of them is "partial". Apply this in
query mode for both flags and env (admin's `skipDatabase=true` path already counts three and
is unchanged). After the fix the only remaining partial errors are genuine subsets (≤2), so
ensure the env-mode partial message names the missing `NEO4J_*` vars (via
`ValidateEnvCredentialSet`, REQ-F-010) and the flag-mode partial message names the `--flags`.
Add tests for: three env vars + stored cred → override succeeds; three flags + stored cred →
override succeeds; two of three (env and flag) → partial error; database supplied is applied
but never required.

### Off-mode message rewording (REQ-F-015)

Thread `cfg.Global.AcceptEnvVars()` (or a derived bool) into the message-construction sites so
the env-var clause is conditional on the gate being on. Sites: `query/run.go` `promptPassword`
(~203), `admin/admin.go` (~45), `dbconn/helpers.go` (~38). Keep the on-mode wording (which may
name the env vars) and only suppress/redirect the env-var suggestion when the gate is off.

### Hint command (REQ-F-016)

Update the hint string in `app.go` `maybeEmitEnvVarHint` to suggest
`neo4j-cli config set accept-env-vars true --rw`. Update `neo4j-cli/app/env_var_hint_test.go`
to assert the new string (including `--rw`).

### Config display (REQ-NF-008)

Inspect the `config get` / `config list` rendering in the config subcommands. Prefer rendering
known boolean keys via their bool accessor (so `accept-env-vars` shows `true`/`false`, never
`"1"`/`"0"`); if a global normalisation is risky, document the quirk in the README/known
behaviour and leave resolution untouched.

### Second-round QA fixes (REQ-F-017..020)

- **Embed off-mode message (REQ-F-017)**: `New()` in `neo4j-cli/query/embed/embed.go` raises
  the empty-provider error without the gate flag in hand. Thread the gate in — either add an
  `AcceptEnvVars bool` to the embed `Config` (set in `Resolve` from `cfg.Global.AcceptEnvVars()`)
  and branch the message on it, or move the empty-provider check into `Resolve` (which already
  has `cfg`). Mirror the wording pattern already used by `promptPassword` / `dbconn/helpers.go`
  (REQ-F-015): off → `--embed-provider` / `.env` / stored cred (+ optional "enable
  accept-env-vars to read NEO4J_EMBED_PROVIDER"); on → may name `NEO4J_EMBED_PROVIDER`. Add an
  embed test for both gate states.

- **Partial-params message (REQ-F-018)**: in `dbconn.ResolveConn`, the `default:` partial-
  override branch builds one static dual-form string. Make it gate-aware via
  `cfg.Global.AcceptEnvVars()`: off → name only `--uri`/`--username`/`--password`; on → keep the
  `--flag/NEO4J_*` dual form. Wording-only; the `requiredCount` logic is unchanged. Reconcile
  with the existing `ValidateEnvCredentialSet` env message so on-mode env subsets still surface
  the missing `NEO4J_*` vars (that path already fires before this branch for env-supplied sets).

- **Mint error handling (REQ-F-019)**: rework the status switch in `mintTokenHTTP`
  (`token.go`). Replace the fall-through `case 400/403` arms with explicit handling: 401 →
  existing auth error; 403 → a new "forbidden / not authorized for this request" error that
  does NOT mention a wrong Aura environment (niche internal-only cause); default non-2xx → a
  clear error naming the status (reuse the existing response-error helpers where possible
  rather than the generic
  `please report` panic). After a successful HTTP exchange, also treat an **empty
  `grant.AccessToken`** as an error. Because this is the single mint chokepoint, both the
  env-var and stored-credential paths in `getToken` inherit the fix. Add tests via the
  `mintToken` seam covering 401, 403, another non-2xx (e.g. 422/500), and a 200-with-empty-token
  response. Confirm the redaction rules (no secret-word `:`/`=` prefixes) still hold for any new
  debug/error text per AGENTS.md.

- **Empty-token cache guard (REQ-F-020)**: in `writeTokenCache` (or the `getToken` envMode
  caller), skip the write when `token == ""`. Add a `token_cache_test.go` case asserting no file
  is created for an empty token. With REQ-F-019 returning early on a failed mint, the caller
  branch may not even reach the write — keep the guard regardless so the cache layer is correct
  in isolation.

### Third-round QA fixes (REQ-F-021..022)

- **Flag help env-var clauses (REQ-F-021)**: find every flag whose `Usage` string ends with a
  gated `[env: NEO4J_*]` clause and strip that clause. These live on the `query` flag set
  (`--uri`/`--username`/`--password`/`--database` and the `--embed-*` flags) and any shared
  helper that builds the same flags for the `admin` tree — grep for `[env: NEO4J_` to find
  them all; do NOT touch `--debug`'s `[env: NEO4J_DEBUG ...]`. Then add a single gate-aware
  "Environment variables" paragraph to the top-level `Long` of `query` and the `admin` root
  (covering connection vars; embed vars on `query`) stating the vars are read only when
  `accept-env-vars` is enabled and how to enable it. Because flag usage strings + `Long`s feed
  the generated skill bundle, regenerate (`go generate ./neo4j-cli/internal/skill/...`) and
  commit; keep the `query`/`credential`/leaf `Example:` gates and the input/output casing
  gates green. Keep the new `Long` prose flush-left where the skill renderer requires it
  (see AGENTS.md skill notes).

- **Source-aware partial message (REQ-F-022)**: in `dbconn.ResolveConn`, the on-mode
  `ValidateEnvCredentialSet(DBMSEnvSpec, …)` check (currently run over the RESOLVED
  uri/username/password) fires before the partial switch and so intercepts flag-only subsets
  with an env-named message, leaving the REQ-F-018 on-mode dual-form branch dead. Fix by
  scoping the env-completeness check to the **env-sourced** values only: build the validation
  map from `cfg.GatedGetenv(...)`/dotenv-derived values (the env half) rather than the
  post-flag resolved values, so a purely-flag subset does NOT trip it and instead falls
  through to the partial switch, which names the `--flags`. A subset that includes any
  `NEO4J_*` value still trips `ValidateEnvCredentialSet` and names the missing `NEO4J_*`
  var(s). Then DELETE the unreachable dual-form arm of the partial switch (so both gate states
  share the single `--flag`-named partial message) and fix the now-correct comment. Preserve
  the three-required-params/database-optional completeness (REQ-F-014) and the off-mode
  wording (REQ-F-018). Add `conn` tests: on-mode `--uri`-only (no env) → `--flag` message;
  on-mode `NEO4J_URI`-only (no flags) → `NEO4J_*` message; on-mode `--uri` flag +
  `NEO4J_USERNAME` env, missing password → names missing `NEO4J_PASSWORD`.

### Fourth-round QA fix (REQ-F-023)

- **Embed API-key off-mode message (REQ-F-023)**: the missing-key message currently lives in
  two places — the on-mode-gated empty-key check in `Resolve()` (`neo4j-cli/query/embed/embed.go`
  ~241-248) and the unconditional per-provider checks in `openai.go` (~62-67), `gemini.go`
  (~79-83), `huggingface.go` (~62-66), each guarding `p.cfg.APIKey == ""` in `Embed()`. The
  embed `Config` already carries `AcceptEnvVars` (added for REQ-F-017). Fix in two parts:
  - **(a)** In `Resolve()`, drop the `if cfg.Global.AcceptEnvVars()` guard around the empty-key
    check so it fires for a needs-key provider with no resolved key in BOTH gate states, and
    branch its wording on the gate: off → `.env`/stored-cred + optional "enable accept-env-vars
    to read <key env vars>"; on → may name the env vars. (`providerNeedsKey` already exists.)
  - **(b)** Branch the three provider `Embed()` messages (and their `WithSuggestion` text) on
    `p.cfg.AcceptEnvVars` so the backstop obeys the same rule when reached directly. Keep a
    small shared helper for the off/on message bodies if it reads cleanly across the three
    providers + the `Resolve()` site (DRY the wording), but do not over-abstract — distinct
    provider key-var lists (HF_TOKEN; GEMINI/GOOGLE; OPENAI) may stay per-provider.
  - The `.env` suggestion must appear in both modes (dotenv is never gated). The
    key/Authorization value must never appear in any message. The on-path is untouched.
  - Tests: for openai/gemini/huggingface, OFF + provider-via-`--embed-provider` + key only in
    OS env (gated) → message references `.env`/stored cred (+ accept-env-vars) and does NOT
    advertise the OS key var as a direct fix; ON + no key → message may name the env vars; a
    `.env`-supplied key off-mode resolves with no error.

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

### QA-driven additions (REQ-F-014..016, REQ-NF-008/009)

- [ ] With `accept-env-vars=true` and a stored default credential,
  `NEO4J_URI`+`NEO4J_USERNAME`+`NEO4J_PASSWORD` (no `NEO4J_DATABASE`) overrides the stored
  credential and resolves without an "all four required" error (REQ-F-014).
- [ ] The same three supplied as `--uri`/`--username`/`--password` flags (no `--database`)
  override a stored default credential without an "all four required" error (REQ-F-014).
- [ ] Supplying only one or two of uri/username/password (via env OR flags) with a stored
  default credential is still rejected as a partial override, naming the missing values
  (`NEO4J_*` in env mode, `--flags` in flag mode) (REQ-F-014).
- [ ] `--database`/`NEO4J_DATABASE`, when supplied, is applied to the connection but is never
  required to complete an override (REQ-F-014).
- [ ] With `accept-env-vars` off and a connection value missing, the error message does not
  tell the user to set a gated env var as if it would work; it references flags/`.env` and/or
  `accept-env-vars` (REQ-F-015).
- [ ] The discovery hint suggests `neo4j-cli config set accept-env-vars true --rw`, and that
  exact command succeeds when run non-interactively (REQ-F-016).
- [ ] `config get accept-env-vars` renders consistently with sibling boolean keys (or the
  display quirk is documented), and `NEO4J_CLI_ACCEPT_ENV_VARS=1` does not surface as `"1"`
  (REQ-NF-008).
- [ ] Sentinel-only hint detection is documented as intended behaviour (REQ-NF-009).
- [ ] README updated for the three-param override (database optional) across flags and env
  vars, the off-mode message behaviour, and the hint `--rw`.

### Second-round QA additions (REQ-F-017..020)

- [ ] With `accept-env-vars` off and no resolvable embed provider, the "missing embed provider"
  error does NOT advertise `NEO4J_EMBED_PROVIDER` as a fix; it references `--embed-provider` /
  `.env` / a stored embed credential (and may mention enabling `accept-env-vars`). With
  `accept-env-vars` on it may name `NEO4J_EMBED_PROVIDER` (REQ-F-017).
- [ ] With `accept-env-vars` off, a partial flag-only DBMS override (one or two of
  `--uri`/`--username`/`--password`) is rejected with a message naming only the `--flags`, not
  `NEO4J_URI`/`NEO4J_USERNAME`/`NEO4J_PASSWORD` (REQ-F-018). NOTE: REQ-F-022 supersedes the
  original "on-mode may use the dual form" wording — a flag-only subset now names only the
  `--flags` in BOTH gate states; only env-sourced subsets name `NEO4J_*`.
- [ ] An Aura OAuth mint returning 401 yields the invalid-credentials auth error; a 403 yields a
  distinct "forbidden / not authorized" error (NOT the 401 wording, and NOT mentioning a wrong
  Aura environment); any other non-2xx (e.g. 422/500) and a 2xx with an empty `access_token`
  yield a clear error naming the status (not the generic "please report") (REQ-F-019).
- [ ] The mint fix applies on both the env-var-synthesized and stored-credential paths (single
  `mintTokenHTTP` chokepoint); a forbidden/empty mint never produces an
  `Authorization: Bearer ` (empty) downstream request (REQ-F-019).
- [ ] No on-disk token-cache file is written when the minted token is empty;
  `token_cache_test.go` covers the empty-token case (REQ-F-020).
- [ ] `make test`, `make fmt-check`, `make lint` pass; `go generate ./neo4j-cli/internal/skill/...`
  produces no bundle drift after the message changes.

### Third-round QA additions (REQ-F-021..022)

- [ ] No gated-flag `--help` usage string advertises a `[env: NEO4J_*]` clause:
  `query --help` and the `admin` leaves no longer show `[env: NEO4J_URI/USERNAME/PASSWORD/
  DATABASE]` or `[env: NEO4J_EMBED_*]` on the individual flags (REQ-F-021).
- [ ] The gated env vars are still documented — the `query` and `admin` top-level `Long`
  descriptions carry a gate-aware "Environment variables" paragraph naming them and stating
  they are read only when `accept-env-vars` is enabled (REQ-F-021).
- [ ] `--debug` still shows its `[env: NEO4J_DEBUG ...]` clause unchanged (REQ-F-021).
- [ ] With `accept-env-vars` **on**, a flag-only partial override (e.g. only `--uri`, no
  `NEO4J_*` set) is rejected with a message naming only the `--flags` — NOT
  `NEO4J_URI`/`NEO4J_USERNAME`/`NEO4J_PASSWORD` (REQ-F-022).
- [ ] With `accept-env-vars` **on**, a partial override that includes any `NEO4J_*` value
  (env-only or mixed flag+env) names the missing `NEO4J_*` variable(s) (REQ-F-010/F-022).
- [ ] The unreachable on-mode dual-form partial branch in `dbconn.ResolveConn` is removed and
  its comment corrected; no dead code remains (REQ-F-022).
- [ ] `make test`, `make fmt-check`, `make lint` pass; `go generate
  ./neo4j-cli/internal/skill/...` produces no bundle drift after the help/message changes.

### Fourth-round QA addition (REQ-F-023)

- [ ] With `accept-env-vars` **off**, `query --embed-provider openai` (provider via flag) with
  `OPENAI_API_KEY`/`NEO4J_EMBED_API_KEY` present only in OS env vars (gated) fails with a
  message that does NOT advertise those OS env vars as a direct fix — it references a `.env`
  file / a stored embed credential and MAY mention enabling `accept-env-vars` (REQ-F-023).
- [ ] The same off-mode behaviour holds for `gemini` (`GEMINI_API_KEY`/`GOOGLE_API_KEY`) and
  `huggingface` (`HF_TOKEN`) providers (REQ-F-023).
- [ ] With `accept-env-vars` **on** and a needs-key provider with no resolved key, the message
  MAY name the provider's key env vars (REQ-F-023).
- [ ] Off-mode, a key supplied via a `.env` file resolves successfully (no missing-key error),
  confirming dotenv is never gated (REQ-F-023).
- [ ] The empty-key check is centralized in `Resolve()` for both gate states AND the three
  provider `Embed()` backstop messages are gate-aware; no API-key message advertises a gated
  env var off-mode (REQ-F-023).
- [ ] `make test`, `make fmt-check`, `make lint` pass; `go generate
  ./neo4j-cli/internal/skill/...` produces no bundle drift (REQ-F-023).

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

Resolved during post-implementation QA (2026-06-26):
- The default-path override-completeness contradiction (QA Finding 1) is fixed by making the
  three required params (uri/username/password) the "complete" set with database always
  optional, applied **uniformly to both flags and env vars** — the user chose uniform
  behaviour over an env-only relaxation, intentionally relaxing the older flag-only "all four"
  contract (REQ-F-014). CLI-212's `--credential` + database-override path (`applyDBOverride`,
  REQ-F-013) is separate and unchanged.
- Off-mode error messages are reworded to not advertise gated env vars; they reference flags/
  `.env` and `accept-env-vars` instead (QA Finding 2, REQ-F-015).
- The discovery hint includes `--rw` so it works non-interactively (QA Finding 3, REQ-F-016).
- The `config get`/`list` display quirk and the sentinel-only hint behaviour are addressed
  (normalise-or-document; document-as-intended respectively) (QA Findings 4/5, REQ-NF-008/009).

Resolved during second-round QA (2026-06-26, live local Enterprise + live Aura):
- The embed "missing embed provider" message was the one off-mode site REQ-F-015 missed; it is
  made gate-aware like the connection/password messages (REQ-F-017).
- The DBMS partial-params error no longer names gated `NEO4J_*` vars when the gate is off
  (REQ-F-018).
- `mintTokenHTTP` was silently swallowing 403/400 (and other unlisted non-2xx), returning an
  empty token that produced a downstream 422 "please report"; it now returns clear per-status
  errors — 401 (invalid credentials) vs 403 (a distinct "forbidden / not authorized" message
  that deliberately does NOT mention a wrong Aura environment, since that is a niche
  internal-only cause) vs other non-2xx — and never returns an empty token as success. The fix
  lives at the single mint chokepoint so stored AND env-var credentials benefit (REQ-F-019).
  Decision: fix all mint paths (not env-var-only) and distinguish 401 vs 403 vs other.
- The token cache no longer persists an empty token (REQ-F-020).
