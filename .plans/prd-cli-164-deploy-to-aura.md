# PRD: Deploy a local Neo4j database to a new Aura instance (`aura instance deploy`)

## Overview

Add `neo4j-cli aura instance deploy`: a single command that **creates a new Neo4j Aura instance and clones a local database into it**. This mirrors Neo4j Desktop's "Deploy to Aura" UI flow.

Investigation of Neo4j Desktop 2 (`../neo4j-desktop-2`) confirmed there is **no special Aura data-import API**. Desktop shells out to the bundled `neo4j-admin`:

1. `neo4j-admin database dump <db> --to-path=<dir>` → produces `<db>.dump`
2. `neo4j-admin database upload <db> --from-path=<dir> --to-uri=neo4j+s://… --to-user=neo4j --to-password=… --overwrite-destination` → pushes the dump into a **running** Aura instance over Bolt.

We replicate this forward direction from **two local sources**:

- a **neo4j-cli-managed Docker container** (`neo4j-cli docker`), where we run `neo4j-admin` inside the container via `docker exec`; and
- a **Neo4j Desktop 2-managed DBMS**, where Desktop's own `POST /fastify/api/dbmss/:id/databases/upload` route does the dump+upload internally and we poll its task.

Both paths push into a freshly created Aura instance whose generated credentials become the upload target.

## Goals

- One command provisions a new Aura instance **and** loads a local database into it.
- Support both Docker-managed containers and Desktop-managed DBMSs as the source.
- Reuse the existing `aura instance create` logic (provisioning, credential storage, polling) without duplication.
- Wait for the data load to complete and report a clear success/failure result (a half-loaded clone is useless).
- Never leak the Aura target password into logs, stderr, telemetry, or command history.

## Non-Goals

- **Reverse direction (Aura → local).** Desktop has no reverse flow and there is no confirmed public Aura snapshot-export/download API. Tracked separately.
- **Deploying into an existing Aura instance** (overwrite-existing). This release creates a new instance only.
- **Arbitrary host-installed Neo4j** as a source (a Neo4j not managed by `neo4j-cli docker` or Desktop). Source is limited to the two managed surfaces.
- **Multi-database deploy.** One `--database` per invocation; no `--all-databases`.
- **Source/target version-compatibility pre-validation.** We surface `neo4j-admin`/Desktop's own error verbatim instead of pre-checking version matrices.

## Requirements

### Functional Requirements

**Command surface**

- REQ-F-001: Add a new leaf `neo4j-cli aura instance deploy` defined in `neo4j-cli/aura/internal/subcommands/instance/deploy.go` as `NewDeployCmd(cfg *clicfg.Config) *cobra.Command`, registered in `instance.go` via `cmd.AddCommand(NewDeployCmd(cfg))`. Follows the one-file-per-leaf cobra layout with a colocated `deploy_test.go`.
- REQ-F-002: The command carries the `Annotations: map[string]string{"write": "true"}` annotation (it provisions an instance and writes data), gating it behind `--rw` and exempting its `Example` from the `--format json` requirement.
- REQ-F-003: Provide a flush-left `Example:` field with ≥2 invocations, each preceded by a `# comment`, blank-line separated, `neo4j-cli`-prefixed, and `--rw` present on every invocation (satisfies `TestAllLeafCommands_HaveExamples`). At least one example uses `--from-docker` and one uses `--from-desktop`.

**Source selection**

- REQ-F-004: Define mutually-exclusive source flags `--from-docker <container-name>` and `--from-desktop <dbms-id>`; exactly one is required (use cobra `MarkFlagsMutuallyExclusive` + `MarkFlagsOneRequired`).
- REQ-F-005: Provide `--database <name>` (default `neo4j`) naming the single source database to clone. Reject `system` with a usage error.
- REQ-F-006: Provide an optional `--desktop-port <int>` flag (mirroring the existing `desktop` subtree) used only with `--from-desktop` to pin Desktop API discovery.

**Aura target (create-new), mirroring `instance create`**

- REQ-F-007: Accept the same target-provisioning flags as `instance create`: `--type` (required; `free-db` is allowed as a target), `--memory`, `--region`, `--cloud-provider`, `--version`, `--name`, with identical validation (free-db rejects memory/region/cloud-provider; non-free requires them; version must be `4` or `5`).
- REQ-F-008: Accept the same credential-storage flags: `--credential-name`, `--no-credential-storage`, `--no-credential-print`, with identical semantics to `instance create`.
- REQ-F-009: Resolve org/project via `utils.ResolveAndValidateOrgProject(cmd, cfg)`, exactly as the other instance leaves do. Auto-generate an instance name when `--name` is omitted, reusing the existing default-name logic.
- REQ-F-010: Extract the create body-building and credential-storage from `create.go` into a shared package-internal helper file `instance/create_core.go` (e.g. `buildCreateInstanceBody(...)` and `createAndStoreInstance(cfg, body, credOpts) (map[string]any, error)`). Refactor `NewCreateCmd` to call these helpers; `NewDeployCmd` calls the same helpers. No behavioural change to `instance create`.
- REQ-F-011: After provisioning, poll the new instance to `running` via `api.PollInstance(cfg, id, api.InstanceStatusCreating)` **before** attempting any data load (`neo4j-admin upload` requires a running target).

**Docker source path**

- REQ-F-012: Add `Exec(ctx context.Context, name string, args []string) (string, error)` to the `dockerClient` interface and `execClient` (`docker exec <name> <args…>`), and add a matching method to the `fakeDockerClient` in `helpers_test.go`.
- REQ-F-013: Add an exported orchestrator in a new file `neo4j-cli/internal/subcommands/docker/deploy.go`, e.g. `PushToAura(ctx, cfg, containerName, database string, target AuraTarget) error`, with `type AuraTarget struct { URI, Username, Password string }`.
- REQ-F-014: `PushToAura` resolves the source database password from the stored dbms credential keyed by `containerName` (container name == credential name convention from `docker create`). If no credential is found, return a usage error instructing the user how to supply it.
- REQ-F-015: `PushToAura` performs, in order: (1) `STOP DATABASE <db>` over Bolt for a consistent offline dump; (2) `docker exec <name> neo4j-admin database dump <db> --to-path=/tmp/neo4j-cli-deploy`; (3) `docker exec <name> neo4j-admin database upload <db> --from-path=/tmp/neo4j-cli-deploy --to-uri=<target.URI> --to-user=<target.Username> --to-password=<target.Password> --overwrite-destination`; (4) `START DATABASE <db>`; (5) best-effort cleanup `rm -rf /tmp/neo4j-cli-deploy`. The STOP/START is performed behind an injectable `stopStartFn` seam for hermetic testing.
- REQ-F-016: The `START DATABASE <db>` restore step runs even if the dump/upload fails (defer/cleanup), so a deploy error never leaves the source database stopped.

**Desktop source path**

- REQ-F-017: Add to `neo4j-cli/internal/desktopclient/`: `UploadDatabase(ctx, dbmsID string, source UploadSource, target UploadTarget) error` → `POST /fastify/api/dbmss/{id}/databases/upload` with body `{source:{databaseName}, target:{uri,username,password,overwrite:true}}`, reusing the existing authed `do()` path (X-Client-Id / X-API-Token headers).
- REQ-F-018: Add `ListTasks(ctx) ([]Task, error)` → `GET /fastify/api/tasks`, with a new `Task` type capturing `id`, `tags []string`, and `status{isLoading, isSuccess, isError}`.
- REQ-F-019: Add a `WaitForUploadTask(ctx, client, dbmsID)` helper that polls `ListTasks`, filters for a task whose `tags` contain both `db:upload` and the dbmsID, and returns on `isSuccess` (nil) or `isError` (error). Polling cadence/timeout should mirror existing desktopclient/poll conventions.
- REQ-F-020: The deploy leaf manages the Desktop DBMS state on our side: before uploading, ensure the DBMS is in the state Desktop's dump requires (stop it if running, via `StopDbms` + poll-to-stopped); after the upload task completes, restore the DBMS to its prior state (restart it if it was running before). Resolve the client via the existing `newDesktopClient(...)` factory pattern.
- REQ-F-021: The Desktop path passes only the Aura **target** credentials in the request — Desktop owns the source DB credentials.

**Orchestration, output, and failure policy**

- REQ-F-022: Deploy flow: validate source flags → resolve org/project → create + poll Aura instance to running → dispatch to the docker or desktop push path → wait for load completion → print result.
- REQ-F-023: Deploy always waits for the data load to finish (no separate `--wait`); progress is narrated to stderr (e.g. "Creating instance…", "Waiting for instance to be ready…", "Uploading <db>…").
- REQ-F-024: Structured output (`--format json|table|toon`, via the existing `output.PrintBodyMap`) includes the new instance fields (`id`, `name`, `project_id`, `connection_url`, `username`, `password` unless `--no-credential-print`, `credential_name` unless `--no-credential-storage`, `cloud_provider`, `region`, `type`) **plus a discrete `deploy_status` field** (`succeeded` / `failed`).
- REQ-F-025: If the data-load push fails **after** the Aura instance has been created, leave the instance in place (no auto-delete), set `deploy_status: failed`, and print the instance id with a hint to retry or delete it manually. Surface `neo4j-admin`/Desktop's own error verbatim (no version-compat pre-validation).

**Documentation / generated content**

- REQ-F-026: Run `go generate ./neo4j-cli/internal/skill/...` after adding the command so the skill bundle regenerates (`references/instance.md` etc.); `TestGenerator_RoundTrip` must pass.
- REQ-F-027: Add a changelog entry: `changie new --projects neo4j-cli --kind Minor --body "<desc>"`.
- REQ-F-028: Extend the e2e Desktop fixture (`test/e2e/desktop_fixture/handlers.go`) with the `databases/upload` and `tasks` routes so the desktop path has e2e coverage.

### Non-Functional Requirements

- REQ-NF-001: The Aura target password and the source DB password must never appear in logs, stderr, telemetry, or on-disk command history. The docker upload argv is redacted via the docker package's existing `redactArgs`/`redactString`; any new secret-bearing flag/arg must route through `common/clievents.RedactArgs` where applicable.
- REQ-NF-002: All new code carries the Neo4j copyright header (enforced by `addlicense` / `make license-check`).
- REQ-NF-003: Tests are hermetic and colocated: unit tests use the mock Aura HTTP server (`test/testutils`), a `fakeDockerClient`, a stubbed `stopStartFn`, and the desktopclient `newAuthedServer` pattern — no real docker daemon, Desktop instance, or Aura account required in `make test`. (Per repo convention, query-package tests must not use `afero.NewOsFs()`.)
- REQ-NF-004: Cross-platform: the implementation must pass CI on ubuntu, windows, and macos. Committed `.md`/golden/bundle files keep LF endings per `.gitattributes`.
- REQ-NF-005: No new classifier or duplicated provisioning logic — `instance create` and `instance deploy` share one code path for instance creation and credential storage.

## Technical Considerations

- **Import direction is legal:** `neo4j-cli/aura/internal/subcommands/instance` may import `neo4j-cli/internal/subcommands/docker` and `neo4j-cli/internal/desktopclient` because both targets' `internal` ancestor is `neo4j-cli/`. The deploy leaf therefore calls the exported `docker.PushToAura(...)` and desktopclient methods directly.
- **Reuse points:** `instance/create.go` (provisioning + credential storage, ~lines 141-202), `api.PollInstance` / `api.InstanceStatusCreating`, `utils.ResolveAndValidateOrgProject`, `utils.RenameResponseField`, `output.PrintBodyMap`, the docker `dockerClient`/`execClient` and redaction helpers, the `query` connect/run machinery for STOP/START DATABASE, and the desktopclient authed `do()` + `newDesktopClient` factory.
- **`neo4j-admin upload` semantics (verified):** `--from-path` must point at a directory already containing a `<db>.dump`/`.backup`, so a `dump` step must precede `upload`; the target Aura instance must be running and reachable. Source DB should be stopped for a consistent offline dump (hence the STOP/START in both paths).
- **Desktop upload is asynchronous:** the route returns immediately; completion is observed by polling `GET /fastify/api/tasks` for the `db:upload` + dbmsID task. Desktop binds to `127.0.0.1`; auth is via X-Client-Id / X-API-Token (already handled by desktopclient).
- **Failure surface:** version/edition incompatibility between the local dump and Aura's Neo4j version is surfaced as the underlying tool's error (no pre-validation), consistent with how the docker subtree surfaces docker's verbatim stderr.
- **Potential challenge:** confirming the exact DBMS state Desktop's upload route requires (stopped vs running). The plan assumes "stopped for dump, restore prior state after"; verify against `../neo4j-desktop-2` `dbs.local.ts` during implementation and adjust REQ-F-020 if Desktop already stops internally.

## Acceptance Criteria

- [ ] `neo4j-cli aura instance deploy --from-docker <name> --type free-db --rw` provisions a new free-db instance and loads the named container's `neo4j` database into it; output includes `connection_url`, `username`, `password`, `credential_name`, and `deploy_status: succeeded`.
- [ ] `neo4j-cli aura instance deploy --from-desktop <dbms-id> --type professional-db --memory 1GB --cloud-provider gcp --region europe-west1 --rw` provisions the instance, drives Desktop's upload route, polls the task to success, and restores the DBMS to its prior running state.
- [ ] `--from-docker` and `--from-desktop` are mutually exclusive and exactly one is required (usage error otherwise); `--database system` is rejected.
- [ ] Non-free target enforces `--memory/--region/--cloud-provider`; `free-db` rejects them — identical to `instance create`.
- [ ] The Aura target password is absent from stderr narration, the upload argv echo, telemetry, and command history.
- [ ] On data-load failure after instance creation, the instance is left running, `deploy_status: failed`, and the output names the instance id with a retry/delete hint.
- [ ] On the docker path, the source database is `START`-ed again even when the dump/upload step fails.
- [ ] `instance create` behaviour is unchanged after the `create_core.go` extraction (existing create tests pass).
- [ ] `make test`, `make fmt-check`, `make lint`, and `make license-check` all pass; `go generate ./neo4j-cli/internal/skill/...` leaves no diff (`TestGenerator_RoundTrip` green); `TestAllLeafCommands_HaveExamples` green.
- [ ] A `Minor` changelog entry exists for the new command.

## Out of Scope

- Aura → local clone (reverse direction).
- Deploying into a pre-existing Aura instance / overwrite path.
- Sources other than `neo4j-cli docker`-managed containers and Desktop 2-managed DBMSs.
- Multi-database / `--all-databases` deploy.
- Version/edition compatibility pre-validation.

## Open Questions

1. **Exact Desktop DBMS state required by the upload route** — confirm in `../neo4j-desktop-2` whether Desktop stops the DBMS internally during dump; if so, REQ-F-020's explicit stop/restore may simplify to a no-op or a started-state check.
2. **Polling timeout for the Desktop upload task** — adopt the existing desktopclient polling budget, or introduce a deploy-specific (longer) timeout since dumps of large DBs can take minutes?
3. **Docker container edition/version assumptions** — `neo4j-admin database dump` of a stopped database works on both editions; confirm the official `neo4j` image exposes `neo4j-admin` on PATH inside the container for all supported tags (validated for 5.x).
