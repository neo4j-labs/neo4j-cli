# CLI-32 — Implementation reference (Desktop local API)

Companion to [`prd-cli-32-local-desktop-management.md`](prd-cli-32-local-desktop-management.md). Captures the relate API surface, authentication scheme, data-dir resolution, DBMS / credentials endpoints, and source-file references in `neo4j-desktop-2` that the implementation depends on. Originally lived as a comment on Linear card [CLI-32](https://linear.app/neo4j/issue/CLI-32/local-neo4j-desktop-management-v10); moved here so it travels with the code.

Nothing in here is secret — it all derives from the open-source `neo4j-desktop-2` repo — but it's separated from the PRD to keep the PRD focused on user-facing behaviour.

## Relate API surface

Base URL: `http://localhost:<port>/fastify/api`

Port discovery: Desktop calls `detectPort(44222)` and walks up on conflict. CLI probes `44222..44232`; first port returning HTTP 200 on `/fastify/api-docs` wins. A raw TCP-open does NOT count — must see a 200 on the docs path.

## Auth

Every request carries:

- `X-Client-Id: <random UUID v4>` — fresh per CLI invocation
- `X-API-Token: <HS256 JWT>` — payload `{iat, exp}` (7d default expiry), key = `"<instanceSalt>-<httpOrigin>-<clientId>"`

Where:

- `instanceSalt` = contents of `<dataDir>/relate.secret.key` (a UUID v4 file Desktop writes on first auth)
- `httpOrigin` = `http://localhost:<port>` of the relate server (literal — see `packages/electron/src/api/relate-env-setup.ts:39-49`)

Source files in `neo4j-desktop-2`:

- Token signing: `packages/common/src/token.service.ts:16-36`
- Verification: `packages/common/src/entities/environments/environment.local.ts:60-66`
- Middleware: `packages/web/src/fastify/auth/api-token.middleware.ts:74`

## Data-dir resolution (where to find the salt)

In order:

1. `NEO4J_DESKTOP_DATA_PATH` env var → `<custom>/Application/Data`
2. Walk relate env JSONs at `<userConfig>/com.Neo4j.Relate/Config/environments/*.json`, pick `active: true` (or the one named by `NEO4J_DESKTOP_ENV`), use its `relateDataPath`.
   - macOS: `~/Library/Application Support/...`
   - Linux: `~/.config/...`
   - Windows: `%APPDATA%/...`
3. Per-OS Desktop-2 default:
   - macOS: `~/Library/Application Support/neo4j-desktop/Application/Data`
   - Linux: `~/.config/neo4j-desktop/Application/Data`
   - Windows: `~/.Neo4jDesktop2/Application/Data`

Each env JSON has `{name, id, active, type, relateDataPath?, httpOrigin?, serverConfig}`. During active-env discovery, the env's `httpOrigin` is matched against the probed live origin to identify the currently-running env.

## DBMS endpoints

- `GET /dbmss` → `[DbmsInfo]`
- `GET /dbmss/:id` → `DbmsInfo`
- `GET /dbmss/:id/config` → `{config: [{key, value}]}`
- `GET /dbmss/versions?limited=<bool>` → `[{edition, version, origin, dist}]` (version catalog; see "Versions endpoint" section below for the observed shape powering REQ-F-030)
- `POST /dbmss` body `{name, version, credentials, edition?, noCaching?, limited?}` → `DbmsInfo`. **Omit `edition` entirely** (Desktop 2 is enterprise-only; `dbmss.local.ts:125` defaults to `NEO4J_EDITION.ENTERPRISE`). `credentials` is the cleartext initial password.
- `DELETE /dbmss/:id` → `DbmsInfo`
- `POST /dbmss/:id/start` → string (stringified output)
- `POST /dbmss/:id/stop` → string
- `POST /dbmss/:id/upgrade` body `{version, options?}` → `DbmsInfo` (not surfaced in v1)
- `PATCH /dbmss/:id` body `{name?, description?, tags?, project?, metadata?}` (not surfaced in v1)

`DbmsInfo` shape (`packages/web/src/fastify/routes/dbms.routes.ts:7-28`):

```
{id, name, description, tags, project, metadata, connectionUri, rootPath,
 status, serverStatus, version?, edition?, prerelease?}
```

`DbmsInfo` does NOT echo back `credentials` — passwords are never returned by any DBMS endpoint.

## Credentials endpoints

`packages/electron/src/api/credentials.routes.ts`, mounted at `bootstrap-application.ts:56`. Same JWT auth as `/dbmss`.

- `GET    /credentials/:key` → `{username, password}` or `null`
- `POST   /credentials/:key` body `{username, password}` → `true`
- `DELETE /credentials/:key` → `true`

Key namespace:

- DBMS credentials: `dbms:<dbmsId>`
- Saved-connection credentials: `connection:<connectionId>`
- Proxy credentials: `proxy:proxy`

Storage backend: Electron `safeStorage` (OS keychain-keyed) at `~/Library/Application Support/<env-dir>/Application/Stores/credentials.store`. Source: `packages/electron/src/stores/credentials.store.ts`. Set on every DBMS install at `packages/electron/src/overrides/dbmss.override.ts:78`. Falls back to in-memory if `userSettings.storePasswords === false` or `safeStorage.isEncryptionAvailable() === false`.

`null` body is a real case for legacy DBMSes that pre-date `storePasswords` or for environments where `safeStorage` isn't available — the fallback handling in REQ-F-028 covers it.

## Live verification

Probed against the test machine (8 DBMSes on port 44222, env `Neo4j_Desktop_Workspace` active, salt at `~/Library/Application Support/neo4j-local/Application/relate-data-neo4j-local/relate.secret.key`):

- 6 DBMSes returned `{"username":"neo4j","password":"..."}` from `GET /credentials/dbms:<id>`
- 2 DBMSes returned `null` (legacy)
- All 8 returned `DbmsInfo` from `GET /dbmss`
- JWT auth verified working end-to-end

## `desktop install` — public URLs

electron-builder feed (publicly served, not sensitive):

- macOS: `https://dist.neo4j.org/neo4j-desktop-2/mac/latest-mac.yml`
- Linux: `https://dist.neo4j.org/neo4j-desktop-2/linux/latest-linux.yml`
- Windows: `https://dist.neo4j.org/neo4j-desktop-2/win/latest.yml`

Canary follow-up: `https://dist.neo4j.org/neo4j-desktop-2-canary/${os}/canary-*.yml`. Source: `packages/electron/src/electron-builder.config.ts:108-115` and `:123-130`.

## Files in `neo4j-desktop-2` referenced by the implementation

- `packages/electron/src/bootstrap-application.ts:56` — credentials routes mount
- `packages/electron/src/bootstrap-application.ts:113` — fastify.listen
- `packages/electron/src/api/credentials.routes.ts` — credentials CRUD
- `packages/electron/src/api/relate-env-setup.ts:16-77` — env wiring + httpOrigin
- `packages/electron/src/api/relate-env-setup-helpers/detect-existing-data-path.ts` — data-dir precedence
- `packages/electron/src/stores/credentials.store.ts` — safeStorage backend
- `packages/electron/src/overrides/dbmss.override.ts:78` — setCredentials on install
- `packages/electron/src/electron-builder.config.ts:108-130` — install feed URLs
- `packages/common/src/token.service.ts:16-71` — JWT sign + salt file
- `packages/common/src/entities/environments/environment.local.ts:60-66` — verify
- `packages/common/src/entities/dbmss/dbmss.local.ts:125` — enterprise default
- `packages/common/src/entities/dbmss/versions/dbms-versions.ts:101` — enterprise catalog
- `packages/web/src/fastify/routes/dbms.routes.ts` — DBMS routes + schemas
- `packages/web/src/fastify/auth/api-token.middleware.ts` — auth middleware

## Versions endpoint

`GET /fastify/api/dbmss/versions` returns the full version catalog Desktop knows about — both already-downloaded distributions and ones available from `dist.neo4j.org`. Probed against Desktop 2.1.4 with a local cache:

```json
[
  {"dist": "/Users/.../Cache/dbmss/neo4j-enterprise-2025.04.0", "edition": "enterprise", "origin": "cached", "version": "2025.04.0"},
  {"dist": "/Users/.../Cache/dbmss/neo4j-enterprise-2025.05.0", "edition": "enterprise", "origin": "cached", "version": "2025.05.0"},
  {"dist": "/Users/.../Cache/dbmss/neo4j-enterprise-2026.04.0", "edition": "enterprise", "origin": "cached", "version": "2026.04.0"},
  {"dist": "https://dist.neo4j.org/neo4j-enterprise-5.26.1-unix.tar.gz", "edition": "enterprise", "origin": "online", "version": "5.26.1"},
  {"dist": "https://dist.neo4j.org/neo4j-enterprise-5.26.0-unix.tar.gz", "edition": "enterprise", "origin": "online", "version": "5.26.0"}
]
```

Observed properties:

- `edition` is always `"enterprise"` against Desktop 2 (matches "enterprise-only" lock-in).
- `origin` is `"cached"` (already on disk, no download on create) or `"online"` (will pull from `dist.neo4j.org` during create).
- `version` mixes two release families:
  - Calendar-versioned `YYYY.MM.0` (the post-5.x scheme; `2026.04.0` was the newest cached entry observed)
  - Legacy `5.x.y` (still listed for backward compatibility; `5.26.1` was the newest 5.x entry)
- Server-side ordering is NOT consistent — cached entries appeared ascending, online entries descending. Treat the array as unsorted; semver-rank client-side.
- `?limited=true` / `?limited=false` query param controls whether Desktop filters the catalog (likely cached-only vs. include-online); both responses contain the same shape. The actual filter logic lives in `packages/common/src/entities/dbmss/versions/dbms-versions.ts`.

For REQ-F-030 "pick latest when `--version` is omitted": semver-compare all entries, prefer the highest stable version, prefer `origin: cached` on ties. Both 5.x and YYYY.MM compare correctly under standard semver (`2026.04.0 > 5.26.1`), so a single semver sort works.

Use `golang.org/x/mod/semver` for the compare — it understands prerelease suffixes (`-alpha`, `-rc1`, etc.) so the filter for "stable only" is one call to `semver.Prerelease()` returning empty string.
