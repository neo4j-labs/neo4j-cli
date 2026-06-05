# PRD: Load Example Datasets Easily (CLI-204)

## Overview

Give `neo4j-cli` users a one-command way to load a known-good example Neo4j
dataset into a database they manage. Many GitHub repos — primarily the
`neo4j-graph-examples/*` org — ship a dataset as a `.dump` file plus a
`relate.project-install.json` manifest describing, per Neo4j version range, which
dump file to use and which plugins it needs (`apoc`, `graph-data-science`).

A dataset is addressed by its GitHub `<owner>/<repo>` (e.g.
`neo4j-graph-examples/movies`), so any repo containing the manifest works — not a
closed catalog. The load action is a **`load <owner/repo>` verb added to each
target's own command tree** (docker, desktop dbms, aura instance), adapted to that
tree's existing `create` flags. A discovery command `neo4j-cli dataset` provides
`dataset list` (curated suggestions) and a `--help` that signposts the three
loaders. There is **no** central `dataset load` command.

Linear: CLI-204. Plan basis: `/Users/oskarhane/.claude/plans/i-want-to-have-cached-cray.md`.

## Goals

- Load an example dataset into a Neo4j DBMS with a single command.
- Address datasets by `<owner>/<repo>`; any repo with the manifest works.
- Resolve the correct dump file + required plugins from the repo's
  `relate.project-install.json`, matching the target's Neo4j version (newest
  compatible when ambiguous).
- Support three targets via a per-tree `load` verb: `docker load`,
  `desktop dbms load`, `aura instance load`.
- docker & desktop: load into an **existing** DB **or** create a **new** one.
- aura: create a **new instance** only.
- `neo4j-cli dataset list` shows a curated suggestion set; `dataset --help`
  signposts the loaders.
- Reuse existing CLI machinery (docker client, desktop relate client, Aura
  create/push) rather than introducing parallel infrastructure.

## Non-Goals

- A central `dataset load --target ...` command (explicitly removed; the load verb
  lives on each target tree).
- Loading data into an **existing** Aura instance (aura is new-instance-only for v1).
- A general LOAD CSV / arbitrary-file import command (datasets are dump-based).
- Supporting datasets distributed by means other than a `.dump` +
  `relate.project-install.json` manifest (e.g. raw Cypher seed scripts, AX/"use
  cases" packages — possible later work).
- Installing the `graph-data-science` plugin onto Aura.
- Private/authenticated GitHub repos.

## Requirements

### Functional Requirements

**Discovery command (`neo4j-cli/internal/subcommands/dataset/`)**

- REQ-F-001: Add a `neo4j-cli dataset` parent command (registers `--format
  json|table|toon`) whose `Long`/help text signposts the three per-target `load`
  commands. One-file-per-leaf cobra layout.
- REQ-F-002: `neo4j-cli dataset list` prints a curated set of suggested datasets
  (slug, title, description, `owner/repo`) honoring `--format`. The list is a
  suggestion set, not a constraint on the `load` verbs.
- REQ-F-003: No `load` leaf under `dataset` — the `dataset` command is
  discovery-only.

**Per-target `load` verb**

- REQ-F-004: `neo4j-cli docker load <owner/repo> --name <new-or-existing> [--wait]
  [--version ...] [--max-size ...]` — registered in `docker/docker.go`. `--name`
  resolves new-vs-existing container.
- REQ-F-005: `neo4j-cli desktop dbms load <owner/repo>` — registered in
  `desktop/dbms/dbms.go`. Existing DBMS selected by `--dbms-id <uuid>` (consistent
  with `delete`/`start`/`stop`/`upgrade`); a new DBMS is created by `--name <name>`.
  `--dbms-id` and `--name` are mutually exclusive; exactly one is required.
- REQ-F-006: `neo4j-cli aura instance load <owner/repo> --name <new-instance>` plus
  the `aura instance create` flag set (`--cloud-provider`, `--region`, `--memory`,
  `--type`, org/project) — registered in `instance/instance.go`. New instance only.
- REQ-F-007: Every `load` verb takes the positional `<owner/repo>` arg and these
  shared flags: `--max-size` (default `2GiB`), `--database` (default `neo4j`; the
  target database the dump is loaded into), and `--force`. Reject malformed slugs
  with a usage error.
- REQ-F-007a: Loading into an **existing** docker/desktop database overwrites its
  contents (`--overwrite-destination=true`). This is **refused unless `--force` is
  given**; without `--force`, error with a message naming the data that would be
  destroyed. (New containers/DBMSs and the new Aura instance don't require `--force`.)

**Catalog & resolution (`neo4j-cli/internal/dataset/`)**

- REQ-F-008: Curated `list` suggestions are sourced from the README of
  `neo4j-graph-examples/demo.neo4jlabs.com` (movies, recommendations, northwind,
  fincen, twitter, companies, stackoverflow, gameofthrones, neoflix, wordnet, slack,
  twitch, offshoreleaks, network-management, openstreetmap, …); exact slugs verified
  against that README at implementation time. Embedded in the binary.
- REQ-F-009: `Resolve(ctx, owner/repo, neo4jVersion)` fetches
  `https://raw.githubusercontent.com/<owner>/<repo>/<branch>/relate.project-install.json`,
  trying branch `main` then `master`, parses `dbms[]`, and selects the entry whose
  `targetNeo4jVersion` semver range matches the target's Neo4j version; on ambiguity
  pick the **newest compatible**. Returns `{DumpPath, Plugins, MatchedVersionRange}`.
- REQ-F-010: Verify the selected `dumpFile` path exists in the repo (manifest can
  drift from `data/`); error clearly if not.

**Download**

- REQ-F-011: Download the dump from the Git-LFS media host
  (`https://media.githubusercontent.com/media/<owner>/<repo>/<branch>/<dumpFile>`),
  not `raw.githubusercontent.com` (which returns an LFS pointer).
- REQ-F-012: Detect an LFS pointer payload (`version https://git-lfs...`) and fail
  loudly rather than treating it as a dump.
- REQ-F-013: HTTPS-only, host-allowlist (`media.githubusercontent.com`,
  `raw.githubusercontent.com`, `github.com`, `codeload.github.com`), streamed to a
  temp file (no full in-memory read), capped at `--max-size`, written 0600, cleaned
  up after load. Model on `update/swap.go` (`assertAllowedHost`, `redactURL`).

**Docker loader**

- REQ-F-014: New container — run a one-shot loader container on a named volume
  (`neo4j-admin database load neo4j --from-path=/import --overwrite-destination=true`,
  dump bind-mounted), then create the server container reusing that volume with
  `NEO4J_PLUGINS` from the manifest. Image version must satisfy the dump's
  `targetNeo4jVersion`. Expose this as a reusable helper
  (`LoadDumpIntoNewContainer(...)`) for the aura loader to stage through.
- REQ-F-015: Existing container (`--name <existing>`, requires `--force`) — Bolt
  `STOP DATABASE <db>` → `docker exec neo4j-admin database load <db>
  --overwrite-destination=true` → `START DATABASE <db>` (mirrors `docker.PushToAura`).
  If the existing container lacks a manifest-required plugin, **refuse** with a clear
  error (plugins can't be added to a running container without recreating it).
- REQ-F-016: Honor `--wait` via `docker.WaitForBolt`.

**Desktop loader**

- REQ-F-017: Add `desktopclient.LoadDump(ctx, dbmsID, db, sourceFilePath, overwrite)`
  calling `POST /fastify/api/dbmss/:id/databases/:db/load-dump` (synchronous; DBMS
  must be stopped), mirroring the existing `UploadDatabase`.
- REQ-F-018: Existing DBMS (`--dbms-id`, requires `--force`) — stop if running, load
  dump into `--database`, install each manifest plugin via `InstallPlugin`, start.
- REQ-F-019: New DBMS (`--name`) — create + load via `POST /fastify/api/desktop/dbmss`
  with `dumpOrBackupPath`, then install plugins and start. Newest compatible Neo4j
  version when the manifest spans ranges.

**Aura loader (new instance only)**

- REQ-F-020: Create a new Aura instance (reuse `instance/create_core.go`), stage the
  dump into an ephemeral docker neo4j (the docker new-loader helper), then
  `docker.PushToAura(...)` over Bolt to the new instance; tear down the ephemeral
  container. **Error early (before creating the instance) if no local Docker daemon
  is available**, since staging requires it.
- REQ-F-021: If resolved manifest plugins include `graph-data-science`, **hard-error
  before doing any work**. `apoc` is allowed.
- REQ-F-022: Resolve org/project context via
  `aura/internal/subcommands/utils/resolve.go`.

### Non-Functional Requirements

- REQ-NF-001: All `.go` files carry the Neo4j copyright header (CI `addlicense`).
- REQ-NF-002: Every new runnable leaf (`dataset list`, `docker load`,
  `desktop dbms load`, `aura instance load`) has a flush-left `Example:` with ≥2
  invocations, each preceded by a `# comment`, writes using `--rw` where applicable,
  reads including at least one `--format json` (`TestAllLeafCommands_HaveExamples`).
- REQ-NF-003: After the command-tree change, regenerate the skill bundle
  (`go generate ./neo4j-cli/internal/skill/...`) so `TestGenerator_RoundTrip` passes.
- REQ-NF-004: A user-facing changelog entry via changie (kind `Minor`).
- REQ-NF-005: Tests hermetic — manifest + LFS fetches stubbed via `httptest`, docker
  client swapped via the `clientFactory` seam, desktop via its client seam; no
  `afero.NewOsFs()` in query-adjacent tests.
- REQ-NF-006: Security — download allowlist + HTTPS-only, LFS-pointer detection,
  temp files 0600, no secret/URL leakage (`redactURL`). Run the `golang-security`
  skill as a final gate.
- REQ-NF-007: `make test`, `make fmt-check`, and `make lint` all pass.

## Technical Considerations

**Package layout**
- `neo4j-cli/internal/subcommands/dataset/` — discovery command (`dataset.go`,
  `list.go`, `list_test.go`); no `load` leaf.
- `neo4j-cli/internal/dataset/` — non-cobra shared support: `catalog.go` (embedded
  suggestions), `resolve.go` (manifest fetch + version match), `download.go`
  (capped, allowlisted, LFS-aware streamer). Exports `Resolve(...)` and
  `Download(...)` used by all three target leaves.
- Target leaves: `docker/load.go`, `desktop/dbms/load.go`,
  `aura/.../instance/load.go`, each registered in its tree's parent.

**Reused machinery**
- Docker: `docker.NewDeployClient()`, `dockerClient` interface
  (`Run`/`Inspect`/`ExecWithEnv`), `docker.WaitForBolt`, `docker.PushToAura`,
  `create.go` port-conflict/name-collision helpers, `renderEnvFile`. The aura tree
  already imports docker (`instance/deploy.go`, CLI-164), so reuse is precedented.
- Desktop: `neo4j-cli/internal/desktopclient` (discovery, `GetDbms`, `StopDbms`,
  `StartDbms`, `InstallPlugin`) plus a new `LoadDump` method.
- Aura: `instance/create_core.go` (`buildCreateInstanceBody`,
  `createAndStoreInstance`), `utils/resolve.go`.
- Mount `dataset.NewCmd(cfg)` in `neo4j-cli/app/app.go`.

**Key risks / footguns**
- Git-LFS: the dump bytes live only on `media.githubusercontent.com`; `raw.` yields a
  pointer. The single biggest implementation footgun.
- neo4j-admin version compatibility: loader image / Desktop DBMS / Aura must be ≥ the
  dump's source version. Error clearly on mismatch.
- Aura has no dump-upload API — only a live `neo4j-admin upload` over Bolt, hence the
  ephemeral-docker staging step (silently requires a local Docker daemon).
- Large dumps (embedding datasets) — stream to disk, never `io.ReadAll`.
- Overwrite (`--overwrite-destination=true`) wipes the existing DB on an existing
  docker/desktop target — gated behind `--force` (REQ-F-007a).

## Acceptance Criteria

- [ ] `neo4j-cli dataset list` shows curated suggestions in all `--format`s; `neo4j-cli
      dataset --help` lists the three `load` commands.
- [ ] `neo4j-cli docker load neo4j-graph-examples/movies --name ex-movies --wait`
      creates a container and `query "MATCH (n) RETURN count(n)"` returns the expected
      count.
- [ ] `neo4j-cli docker load neo4j-graph-examples/recommendations --name r1 --wait`
      loads with plugins (`RETURN apoc.version()`, `RETURN gds.version()` work).
- [ ] `neo4j-cli docker load ... --name <existing> --force` updates an existing
      container's data and re-verifies node count; without `--force` it refuses; a
      plugin gap on the existing container refuses with a clear error.
- [ ] `neo4j-cli desktop dbms load ... --dbms-id <uuid> --force` (existing) and
      `--name demo` (new) load into Desktop (verified via `query --credential desktop`).
- [ ] `--database <name>` loads into a non-default database; default is `neo4j`.
- [ ] `aura instance load` errors early when no local Docker daemon is available.
- [ ] `neo4j-cli aura instance load neo4j-graph-examples/movies --name demo
      --cloud-provider ... --region ... --memory ... --type ...` creates a new instance
      with the data (verified over Bolt).
- [ ] A GDS-requiring dataset against `aura instance load` hard-errors before any work.
- [ ] `--max-size` rejects an over-cap download; an LFS pointer is detected and errors
      loudly.
- [ ] Skill bundle regenerated; `make test && make fmt-check && make lint` pass;
      changelog entry added; `golang-security` gate clean.

## Out of Scope

- Central `dataset load --target ...` command.
- Loading into an existing Aura instance.
- Non-dump dataset formats (raw Cypher seed scripts, AX/"use cases" packages).
- Installing GDS onto Aura.
- Private/authenticated GitHub repos.
- A generic file-import / LOAD CSV command.

## Open Questions

None outstanding. Resolved during planning:
- Overwrite of an existing docker/desktop DB is gated behind `--force` (REQ-F-007a).
- `aura instance load` errors early if no local Docker daemon is available (REQ-F-020).
- An existing docker container missing a manifest plugin is **refused** (REQ-F-015).
- Target database is `neo4j` by default, overridable via `--database` (REQ-F-007).
