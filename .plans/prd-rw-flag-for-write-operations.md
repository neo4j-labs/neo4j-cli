# PRD: `--rw` flag for write operations

Linear: https://linear.app/neo4j/issue/CLI-29
Plan: `~/.claude/plans/i-want-to-implement-precious-rabbit.md`

## Overview

Add a global persistent `--rw` boolean flag to both shipped binaries
(`neo4j-cli` super-CLI and standalone `aura-cli`) that gates **every**
state-mutating command. Writes refuse to execute without `--rw`; reads
remain frictionless. Today every leaf can mutate state with no opt-in,
so an autocomplete / fat-finger / agent-driven `aura instance delete
<id>` runs immediately. `--rw` is a guard rail that costs one keystroke
and prevents an entire class of accidents.

For `query run`, where the cypher itself determines read-vs-write, the
CLI does a preflight `EXPLAIN <cypher>` round trip and inspects the
resulting query type. Only non-read-only queries require `--rw`.

Use cobra `Annotations["write"] = "true"` on every write leaf and a
single root `PersistentPreRunE` that walks the annotation and rejects
the call when the flag is unset. No per-leaf branching.

## Goals

- Block accidental mutations across the Aura API, local config,
  credentials, skill install/remove, and write cypher.
- Keep all reads frictionless — no flag changes for `list`, `get`,
  `query schema`, `query run` of read-only cypher.
- One uniform mechanism (cobra Annotations + composed root hook) so
  future write leaves only add a one-line annotation.
- Surface the requirement in help text, both `SKILL.md` bundles
  (auto-generated), the `additions.md` gotchas, the README, and the
  npm user-facing README.
- Ship as a dual-project `Minor` changie entry (aura-cli + neo4j-cli).

## Non-Goals

- No env-var alternative (`NEO4J_CLI_RW=1`). Flag-only by user
  decision — explicit per-invocation, harder to leave on by accident.
- No detection of write cypher by string parsing — the preflight
  EXPLAIN is the only classifier.
- No granular gating (e.g. separate flags for "API" vs "local config").
  All writes are gated uniformly.
- No `--force` / `--yes` to combine with `--rw`. `--rw` is the single
  opt-in; existing `--await` and other flags are unaffected.
- No interactive confirmation prompt. The flag is the confirmation.
- No allowlist exception for "harmless" local writes like
  `aura config set format json` — gated for consistency.

## Requirements

### Functional Requirements

- REQ-F-001: `common/flags/flags.go` MUST export
  `RegisterRwFlag(cmd *cobra.Command)` that registers a persistent
  boolean flag `--rw` (no shorthand, default `false`) on `cmd`. Help
  text: `"Allow write operations. Required for any command that
  mutates state (Aura API, local config, credentials, skills, write
  cypher)."`.
- REQ-F-002: The two existing root commands MUST mount the flag:
  - `neo4j-cli/app/app.go::NewCmd` after the existing
    `RegisterOutputFlag` call.
  - `neo4j-cli/aura/aura.go::NewStandaloneCmd` (the standalone
    aura-cli root). `NewCmd` (when aura is mounted as a subcommand
    under `neo4j-cli`) MUST NOT register the flag separately — it
    inherits the super-CLI's persistent flag.
- REQ-F-003: Every write leaf listed below MUST set
  `cmd.Annotations = map[string]string{"write": "true"}` inside its
  `newXxxCmd` constructor (33 leaves):
  - `neo4j-cli/aura/internal/subcommands/instance/`: create, delete,
    update, pause, resume, overwrite
  - `neo4j-cli/aura/internal/subcommands/instance/snapshot/`: create
  - `neo4j-cli/aura/internal/subcommands/customermanagedkey/`: create,
    delete
  - `neo4j-cli/aura/internal/subcommands/deployment/`: create, delete
  - `neo4j-cli/aura/internal/subcommands/deployment/token/`: create,
    delete, update
  - `neo4j-cli/aura/internal/subcommands/dataapi/graphql/`: create,
    delete, update, pause, resume
  - `neo4j-cli/aura/internal/subcommands/dataapi/graphql/authprovider/`:
    create, delete
  - `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/`:
    add, remove
  - `neo4j-cli/aura/internal/subcommands/graphanalytics/session/`:
    create, delete
  - `neo4j-cli/aura/internal/subcommands/import/job/`: create, cancel
  - `neo4j-cli/aura/internal/subcommands/credential/`: add, remove,
    use
  - `neo4j-cli/aura/internal/subcommands/config/`: set
  - `neo4j-cli/aura/internal/subcommands/config/project/`: add,
    remove, use
  - `neo4j-cli/internal/subcommands/config/`: set
  - `neo4j-cli/internal/subcommands/credential/database/`: add,
    remove, use
  - `common/skill/`: install.go, remove.go
- REQ-F-004: A single root `PersistentPreRunE` MUST compose the
  existing format-binding logic (currently inside `RegisterOutputFlag`)
  with a new write-gate check. Refactor `RegisterOutputFlag` to expose
  its body as a callable helper rather than directly setting
  `PersistentPreRunE`; the root then composes both helpers in one
  hook.
- REQ-F-005: The write-gate check MUST evaluate `cmd.Annotations["write"]
  == "true"` and the resolved value of `--rw`. If write and `--rw` is
  false, return
  `clierr.NewUsageError("this command writes; pass --rw to allow it")`.
- REQ-F-006: `cobra.EnableTraverseRunHooks = true` is already set in
  `neo4j-cli/app/app.go:45` and the `aura-cli` `cmd/main.go`. Tests
  MUST cover that the root hook still fires for deeply nested writes
  (e.g. `dataapi graphql corspolicy allowedorigin add`).
- REQ-F-007: `neo4j-cli/query/run.go::runQuery` MUST run a preflight
  EXPLAIN before the real `runStatement` call, but ONLY when `--rw` is
  not set. When `--rw` is already true, skip the preflight (saves a
  round trip; the user has already opted in).
- REQ-F-008: The preflight MUST send `"EXPLAIN " + cypher` with the
  user's params via the existing `runStatement` helper using the same
  resolved `conn` (so auth, TLS, database, user-agent are reused).
- REQ-F-009: `neo4j-cli/query/connect.go::queryResponse` MUST be
  extended to capture the query-type field returned by the Neo4j HTTP
  Query API v2 EXPLAIN response. The exact JSON path and enum values
  MUST be confirmed against a docker `neo4j:latest` (mirror the boot
  pattern in `neo4j-cli/query/query_https_smoke_test.go`); only after
  empirical confirmation is the struct field written.
- REQ-F-010: When the EXPLAIN response classifies the cypher as
  anything other than read-only, `runQuery` MUST return
  `clierr.NewUsageError("this command writes; pass --rw to allow it")`
  before the real `runStatement` runs.
- REQ-F-011: When EXPLAIN itself errors (cypher syntax error, network
  error, etc.), the error MUST surface as-is — the user gets the same
  error they would have seen without the preflight, just earlier.
- REQ-F-012: Both root commands MUST gain a `Long` description (or
  extend `Short`) referencing the `--rw` requirement. Locations:
  `neo4j-cli/app/app.go:28-32` and `neo4j-cli/aura/aura.go:23-27`.
- REQ-F-013: `neo4j-cli/internal/skill/additions.md` and
  `neo4j-cli/aura/internal/skill/additions.md` MUST gain a "Write
  operations" gotcha block: agents MUST pass `--rw` for any mutating
  command; for `query run`, the CLI runs EXPLAIN to detect write
  cypher and refuses without `--rw`.
- REQ-F-014: `make generate` MUST be re-run so the regenerated
  `bundle/SKILL.md` files in both
  `neo4j-cli/internal/skill/bundle/` and
  `neo4j-cli/aura/internal/skill/bundle/` reflect `--rw` in their
  "Global Flags" section (this happens automatically via
  `common/skill/render/render.go:130-135` since `--rw` is a persistent
  root flag).
- REQ-F-015: `README.md` (repo root) MUST gain a short "Write
  operations require `--rw`" section near the existing usage examples,
  with one example each for an aura write, a config write, and a
  query write.
- REQ-F-016: `distribution/npm/cli/README.md` (the user-facing tarball
  README) MUST mention `--rw` if it currently documents flags or
  shows examples of mutating commands.
- REQ-F-017: `CONTRIBUTING.md` MUST gain a one-liner under the
  cobra-tree convention section: every new write leaf must set
  `Annotations["write"] = "true"` in its constructor.
- REQ-F-018: Two changie YAMLs MUST land in `.changes/unreleased/`,
  both `kind: Minor`:
  - `project: aura-cli` — body: `Require --rw flag for any write
    operation (instance create/delete/update, config set, credential
    add/remove/use, etc.)`
  - `project: neo4j-cli` — body: `Require --rw flag for any write
    operation; query run uses EXPLAIN to detect write cypher`
- REQ-F-019: All new `.go` files MUST begin with the Neo4j copyright
  header (enforced by `make license-check`).

### Non-Functional Requirements

- REQ-NF-001: `make test` passes on linux, windows, macos.
- REQ-NF-002: `make fmt-check` passes (gofmt clean).
- REQ-NF-003: `make lint` passes (golangci-lint v2).
- REQ-NF-004: `make license-check` passes.
- REQ-NF-005: `make generate-check` passes (CI gate; the bundle diff
  catches stale generated output).
- REQ-NF-006: No new dependencies in `go.mod`.
- REQ-NF-007: For read-only `query run` invocations the preflight
  costs exactly one extra HTTP round trip. Acceptable; EXPLAIN is
  cheap and the cost vanishes as soon as the user passes `--rw`.

## Technical Considerations

**Read vs write inventory** (full classification — used for choosing
which leaves to annotate):

- **Aura API writes (24)** — instance×6 + snapshot.create + cmk×2 +
  deployment×2 + deployment.token×3 + graphql×5 + authprovider×2 +
  cors.allowedorigin×2 + session×2 + import.job×2.
- **Local writes (9)** — aura.config.set + aura.config.project×3 +
  aura.credential×3 + neo4j.config.set + neo4j.credential.database×3.
  (= 11; revise if exact list shifts.)
- **Skill writes (2)** — install, remove.
- **Query** — runtime classification via EXPLAIN.
- **Reads (everything else)** — list/get/check/list across all the
  above, plus `query schema` and read-only `query run`.

**Hook composition strategy.** `RegisterOutputFlag` today both
registers `--format` AND sets `cmd.PersistentPreRunE` for binding +
validation (`common/flags/flags.go:17-45`). Adding a second
registration that also sets `PersistentPreRunE` would clobber the
first. Refactor:

1. Extract the format-binding body of `RegisterOutputFlag` into a
   private helper `bindFormatFromFlag(cmd, cfg) error`.
2. Add `RegisterRwFlag(cmd)` that just registers the `--rw` flag (no
   hook).
3. Add `enforceWriteGate(cmd) error` that walks
   `cmd.Annotations["write"]` and the `--rw` value.
4. The root commands set `PersistentPreRunE` exactly once, calling
   both helpers in order: format binding first (so format errors
   surface even on writes without `--rw`), then write-gate check.

**Annotation lookup.** Inside `PersistentPreRunE`, `cmd` is the
*invoked leaf*, not the root. `cmd.Annotations` is a per-cmd map
populated at construction time, so reading `cmd.Annotations["write"]`
on the leaf is the correct check.

**Flag access from leaf.** `cmd.Flag("rw").Value.String()` (the
`Flag()` form, not `Flags().GetBool()`) reads the persistent flag
through the parent chain even when only the local flagset has been
merged. Convert with `strconv.ParseBool`. This pattern is documented
in `CLAUDE.md` under "Cobra Flag Access Notes".

**EXPLAIN response shape.** The Neo4j HTTP Query API v2 response
likely includes a `summary` or sibling field with a query-type marker
(`r`/`rw`/`w`/`s` or similar enum). The exact JSON path is unknown
and MUST be confirmed by booting docker `neo4j:latest` (mirror
`neo4j-cli/query/query_https_smoke_test.go:* :: TestHTTPS_Smoke` —
already does the cert+container dance) and dumping the response body
for `EXPLAIN MATCH (n) RETURN n` and `EXPLAIN CREATE (n)`. Add the
struct field after observation. Tests then mock the observed JSON.

**Preflight reuses connection.** The existing `conn` struct
(`neo4j-cli/query/connect.go:49-57`) holds uri, basic auth, database,
TLS, user-agent. Calling `runStatement(ctx, c, "EXPLAIN "+cypher,
params)` with the same `c` reuses everything. No new HTTP client.

**Persistent flag inheritance.** The super-CLI mounts aura via
`aura.NewCmd(cfg)`; aura's `NewCmd` does NOT register `--rw` because
it inherits the super-CLI's persistent flag automatically. Only
`NewStandaloneCmd` (used by the aura-cli binary) registers it. This
mirrors how `RegisterOutputFlag` is already invoked.

**Generated bundle drift.** Adding a persistent root flag changes
the "Global Flags" table in `bundle/SKILL.md` for both binaries. Any
edit to the cobra tree without re-running `go generate` fails the
CI gate (`TestGenerator_RoundTrip` and `make generate-check`). Run
`make generate` and commit the regenerated bundles in the same PR.

**Files touched (summary):**

- `common/flags/flags.go` *(refactor + add `RegisterRwFlag`)*
- `common/flags/flags_test.go` *(new)*
- `neo4j-cli/app/app.go` *(call `RegisterRwFlag`, extend `Long`,
  compose hook)*
- `neo4j-cli/aura/aura.go` *(call `RegisterRwFlag` in
  `NewStandaloneCmd`, extend `Long`)*
- 33 write leaves *(one-line annotation each)*
- `neo4j-cli/query/run.go` *(preflight EXPLAIN call when `!rw`)*
- `neo4j-cli/query/connect.go` *(extend `queryResponse` with
  query-type field after docker-confirmed shape)*
- `neo4j-cli/query/run_test.go` *(read/write/with-rw cases)*
- `neo4j-cli/internal/skill/additions.md`
- `neo4j-cli/aura/internal/skill/additions.md`
- `README.md`
- `distribution/npm/cli/README.md`
- `CONTRIBUTING.md`
- `.changes/unreleased/aura-cli-Minor-<ts>.yaml`
- `.changes/unreleased/neo4j-cli-Minor-<ts>.yaml`
- Regenerated `neo4j-cli/internal/skill/bundle/**` and
  `neo4j-cli/aura/internal/skill/bundle/**`

**Risks:**

- Forgetting to annotate a write leaf — that write would silently run
  without `--rw`. Mitigation: per-resource test that asserts the
  command rejects the call without `--rw` (one positive + one
  negative case per resource as a tripwire).
- EXPLAIN preflight masks real-execution errors — only if EXPLAIN
  succeeds and real execution then fails. The preflight runs
  EXPLAIN, not the real query, so this is rare; surface real errors
  unchanged.
- `make generate-check` failure if the developer forgets to
  regenerate bundles after adding `--rw`. Already covered by CI.

## Acceptance Criteria

- [ ] `common/flags/flags.go` exports `RegisterRwFlag` and a refactored
      composable format-binding helper; existing `RegisterOutputFlag`
      callers continue to work.
- [ ] `--rw` appears in `--help` output of both `neo4j-cli` and
      `aura-cli` roots, with the documented help text.
- [ ] Every write leaf listed in REQ-F-003 sets
      `Annotations["write"] = "true"`.
- [ ] `bin/neo4j-cli aura instance list` succeeds with no `--rw`.
- [ ] `bin/neo4j-cli aura instance delete <id>` exits non-zero with
      message `"this command writes; pass --rw to allow it"`.
- [ ] `bin/neo4j-cli aura instance delete <id> --rw` proceeds (against
      mock or test API).
- [ ] `bin/neo4j-cli config set foo bar` errors without `--rw`,
      succeeds with `--rw`.
- [ ] `bin/neo4j-cli skill install claude-code` errors without `--rw`,
      succeeds with `--rw`.
- [ ] `bin/neo4j-cli query run "MATCH (n) RETURN count(n)"` succeeds
      with no `--rw` (EXPLAIN classifies as read-only).
- [ ] `bin/neo4j-cli query run "CREATE (n:Test)"` errors with
      `"this command writes; pass --rw to allow it"` BEFORE any
      mutation reaches the database.
- [ ] `bin/neo4j-cli query run "CREATE (n:Test)" --rw` succeeds and
      skips the preflight (single HTTP call, not two).
- [ ] EXPLAIN failure (e.g. cypher syntax error) surfaces the original
      error, not a generic preflight wrapper.
- [ ] Standalone `bin/aura-cli` repeats all the above for its scope.
- [ ] Both `additions.md` files contain the new "Write operations"
      gotcha block.
- [ ] `bundle/SKILL.md` "Global Flags" section in both binaries lists
      `--rw` after `make generate`.
- [ ] `README.md` mentions `--rw` with at least three examples (aura
      write, config write, query write).
- [ ] `distribution/npm/cli/README.md` mentions `--rw` if it shows
      mutating examples.
- [ ] `CONTRIBUTING.md` documents the `Annotations["write"] = "true"`
      convention for new write leaves.
- [ ] Two changie YAMLs in `.changes/unreleased/`, project keys
      `aura-cli` and `neo4j-cli`, kind `Minor`, body per REQ-F-018.
- [ ] `make test`, `make fmt-check`, `make lint`, `make
      license-check`, `make generate-check` all pass.

## Out of Scope

- `NEO4J_CLI_RW=1` environment variable alternative.
- A separate `--force`/`--yes` flag.
- Granular gating (e.g. distinct flags for API vs. local).
- Detection of write cypher by string parsing (no regex on the cypher
  body — EXPLAIN is the only classifier).
- Interactive confirmation prompts.
- Allowlist exceptions for "harmless" local writes.
- Migrating any existing flag default behavior; the only change is the
  addition of `--rw`.
- Changes to `query schema`'s read-only behavior.

## Open Questions

None — the resolved decisions block in the source plan covers them
all (EXPLAIN response shape verified at implementation time via
docker, error wording fixed, preflight skipped when `--rw` already
set, local config writes gated for consistency).
