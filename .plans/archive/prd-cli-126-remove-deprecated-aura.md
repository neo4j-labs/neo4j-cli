# PRD: Hard-remove deprecated `aura tenant` commands and `--tenant-id` flag

## Overview

Hard-remove two surfaces that were soft-deprecated in v1.4.0 and have now
shipped through two further releases (v1.5.0, v1.6.0):

1. The `aura tenant list` and `aura tenant get` commands (hidden + stderr
   deprecation warning since CLI-99).
2. The `--tenant-id` flag on `aura instance`, `aura customermanagedkey`, and
   `aura graphanalytics session` (hidden + cobra `MarkDeprecated` since
   CLI-120).

Both replacements (`aura project list`, `aura organization get`,
`--project-id`) have been the canonical surface for two release cycles. After
this change, the deprecated names will return cobra's standard "unknown
command" / "unknown flag" error (exit code 2) instead of running with a
warning.

Tracking: [CLI-126](https://linear.app/neo4j/issue/CLI-126/hard-remove-all-deprecated-aura-commands-and-flags).
Branch: `oskar/cli-126-remove-deprecated-aura`.

## Goals

- Delete the deprecated `tenant` command tree and `--tenant-id` flag entirely
  from the cobra registration so the surface area matches the long-standing
  documented behaviour (project/organization-based).
- Keep the existing replacements (`project`, `organization`, `--project-id`)
  unchanged and continue to be the only way to address Aura projects.
- Refresh the generated skill bundle so agents no longer see any
  `tenant`/`--tenant-id` mentions in `references/*.md` or `SKILL.md`.
- Ship a single Minor changelog entry that names both removals and points at
  the replacement commands/flags.

## Non-Goals

- Removing or changing the **legacy `default-tenant` config key** migration
  message in `resolveIDs` (deprecation still in window — separate effort).
- Touching `tenant_id` JSON-field references in API response handling
  (`FetchAndVerify*InProject`, `customermanagedkey/*`, `instance/*`,
  `graphanalytics/session/*`) — those are Aura API keys, not the CLI flag.
- Reorganising the org/project filter logic beyond removing the two
  deprecated branches.
- Adding any new CLI surface or behaviour change beyond the removal.
- Bumping the major version. Removal ships as Minor to match the
  pattern used for the v1.4.0 deprecations.

## Requirements

### Functional Requirements

#### Command-tree removal

- **REQ-F-001**: The directory
  `neo4j-cli/aura/internal/subcommands/tenant/` (parent `tenant.go`, leaves
  `get.go` and `list.go`, plus colocated `get_test.go` and `list_test.go`)
  is deleted in full.
- **REQ-F-002**: `neo4j-cli/aura/aura.go` no longer imports the `tenant`
  package and no longer calls `cmd.AddCommand(tenant.NewCmd(cfg))`.
- **REQ-F-003**: Invoking `neo4j-cli aura tenant list` or
  `neo4j-cli aura tenant get` exits with code 2 and a cobra
  "unknown command" error on stderr. No deprecation warning is printed.

#### Flag removal

- **REQ-F-004**: `neo4j-cli/aura/internal/flags/orgproject.go` no longer
  exports `TenantIDFlag`, no longer registers the `--tenant-id` flag, and no
  longer calls `MarkDeprecated`. The `RegisterOrgProjectFlags` doc comment
  is updated to drop the "plus the deprecated --tenant-id alias" sentence.
- **REQ-F-005**: `neo4j-cli/aura/internal/subcommands/utils/resolve.go`
  `resolveIDs` no longer falls back to `flags.TenantIDFlag` after
  `flags.ProjectIDFlag` is empty; the doc comment on
  `ResolveAndValidateOrgProject` is updated to drop the
  "(2) deprecated --tenant-id flag" step and renumber the project-ID
  resolution list.
- **REQ-F-006**: Invoking any `aura instance|customermanagedkey|graphanalytics session`
  command with `--tenant-id foo` exits with code 2 and a cobra
  "unknown flag: --tenant-id" error on stderr. No deprecation warning is
  printed.

#### Test cleanup

- **REQ-F-007**: `neo4j-cli/aura/internal/flags/orgproject_test.go` no
  longer contains `TestRegisterOrgProjectFlags_DeprecatedTenantID` or
  `TestRegisterOrgProjectFlags_HidesTenantIDFromHelp`. The
  `TestRegisterOrgProjectFlags_OrgID` / `_ProjectID` tests remain
  unchanged.
- **REQ-F-008**: `neo4j-cli/aura/internal/subcommands/utils/resolve_test.go`
  no longer contains
  `TestResolveAndValidateOrgProject_ProjectFromDeprecatedTenantIDFlag`.
  `TestResolveAndValidateOrgProject_MigrationErrorWhenDefaultTenantSet`
  (which tests the `default-tenant` config-key path, not the flag) is kept.

#### Regression coverage

- **REQ-F-009**: A regression assertion locks the removal:
  - In `neo4j-cli/aura/internal/flags/orgproject_test.go`, add a test that
    constructs a cobra command, calls `RegisterOrgProjectFlags`, and
    asserts `cmd.PersistentFlags().Lookup(flags.TenantIDFlag)` returns
    `nil` (i.e., the flag is not registered at all).
  - Either in the same file or in a new `aura_test.go` under
    `neo4j-cli/aura/`, add a parse-time test that builds the standalone
    Aura command tree, invokes `--args ["tenant", "list"]`, and asserts
    `cmd.Execute()` returns an error whose message contains
    `unknown command`. Use the existing testutils pattern for command-tree
    construction if available; otherwise build minimally with
    `aura.NewStandaloneCmd(cfg)`.

#### Documentation

- **REQ-F-010**: `CONTRIBUTING.md` is updated so the "No arguments between
  commands" example (currently lines 167–168) no longer references the
  deprecated `aura tenant` / `--tenant-id` surface. Replace with:
  - ❌ `neo4j-cli aura project <project-id> instance get <id>`
  - ✅ `neo4j-cli aura instance get <id> --project-id <project-id>`
  The rule being illustrated (no positional arguments between commands)
  must remain identical.

#### Skill bundle regeneration

- **REQ-F-011**: After source edits, run
  `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`
  and commit the resulting bundle diff. Expected churn:
  - `neo4j-cli/internal/skill/bundle/references/aura.md` (and possibly
    `SKILL.md`) lose any `tenant get` / `tenant list` / `--tenant-id`
    references.
  - No new bundle files appear; only stale text is pruned.
- **REQ-F-012**: `make generate-check` passes (no `go generate` drift in the
  committed tree).

#### Changelog

- **REQ-F-013**: A single changelog entry is added via
  `changie new --projects neo4j-cli --kind Minor --body "Remove deprecated 'aura tenant list/get' commands and '--tenant-id' flag (deprecated in v1.4.0). Use 'aura project list', 'aura organization get', and '--project-id' instead."`.
  The resulting YAML lands under `.changes/unreleased/`.

### Non-Functional Requirements

- **REQ-NF-001**: All existing gates pass on the resulting tree:
  `make test`, `make fmt-check`, `make lint`, `make license-check`,
  `make generate-check`.
- **REQ-NF-002**: No new code is introduced beyond the regression test in
  REQ-F-009. This task is net-negative LOC.
- **REQ-NF-003**: No public exported symbol other than `TenantIDFlag`
  (in `neo4j-cli/aura/internal/flags`) is removed or renamed. The
  `tenant` package being internal-only and unused outside `aura.go` means
  the import graph contracts cleanly with no callers elsewhere.

## Technical Considerations

- **Deprecation-window evidence**: `.changes/neo4j-cli/v1.4.0.md` records
  both deprecations; v1.5.0 and v1.6.0 are tagged after. Two full release
  cycles satisfies the "at least one release" condition from the issue.
- **`tenant_id` API field is unrelated**: `utils/resolve.go` and several
  callers read `tenant_id` from Aura API responses. The Aura REST API has
  not renamed this field, so CLI-side code keeps reading it. Don't grep
  `tenant_id` and start renaming.
- **`default-tenant` config-key migration**: `resolveIDs` lines 54–58
  detect the legacy `default-tenant` config key and emit a migration
  message pointing at `aura workspace use`. That deprecation is separate
  (it's a config key, not a flag/command) and stays untouched. The
  matching test (`...MigrationErrorWhenDefaultTenantSet`) stays.
- **Skill bundle source-of-truth**: `references/aura.md` and `SKILL.md`
  are generated from the live cobra tree by
  `neo4j-cli/internal/skill/gen/main.go`. Removing the command/flag
  registrations is sufficient; the generator will prune the references
  during `go generate`. Manual edits to `bundle/**` are wrong.
- **No `aura tenant` skill `tenant.md` exists** — hidden commands skip
  bundle generation, so there's no `references/tenant.md` to delete.
- **Cobra error contract**: After removal, cobra emits `Error: unknown
  command "tenant" for "aura"` (or similar) on stderr and exits 2; this
  matches the existing usage-error contract `clierr.NewUsageError` enforces
  for other typos (see `aura.NewStandaloneCmd.SetFlagErrorFunc`).
- **Tests live in two clusters**:
  - The directly-affected tests are in
    `flags/orgproject_test.go`, `subcommands/utils/resolve_test.go`,
    and the deleted `subcommands/tenant/*_test.go`.
  - Many other `_test.go` files reference the string `tenant_id` in API
    response bodies; those are unaffected. (Spot-grep confirmed.)
- **No npm/PyPI distribution change** — this is a code surface removal,
  not a packaging change.

## Acceptance Criteria

- [ ] `neo4j-cli/aura/internal/subcommands/tenant/` directory no longer
      exists on the branch.
- [ ] `git grep -n "tenant.NewCmd"` returns no matches.
- [ ] `git grep -n "TenantIDFlag"` returns no matches.
- [ ] `git grep -n "tenant-id"` returns no matches in `.go` source
      files (matches in `.plans/archive/**` and
      `.changes/neo4j-cli/v1.4.0.md` are allowed and expected).
- [ ] `bin/neo4j-cli aura tenant list` exits with code 2 and stderr
      contains "unknown command".
- [ ] `bin/neo4j-cli aura instance list --tenant-id foo` exits with code
      2 and stderr contains `unknown flag: --tenant-id`.
- [ ] `bin/neo4j-cli aura instance list --help` output does not contain
      `--tenant-id`.
- [ ] `bin/neo4j-cli aura --help` output does not list `tenant` as a
      subcommand.
- [ ] Regression test from REQ-F-009 is present and passing.
- [ ] `CONTRIBUTING.md` no-arguments-between-commands example uses
      `project`/`--project-id`.
- [ ] `neo4j-cli/internal/skill/bundle/references/aura.md` post-regen
      contains no `tenant get`, `tenant list`, or `--tenant-id` strings.
- [ ] `.changes/unreleased/` contains a new
      `neo4j-cli-Minor-*.yaml` entry naming both removals.
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check`,
      `make generate-check` all pass on a clean tree.

## Out of Scope

- `default-tenant` legacy config-key migration message (separate
  deprecation, still in window).
- Renaming `tenant_id` fields in Aura API response handling — Aura API
  has not renamed them.
- `.plans/archive/**` historical PRDs — frozen.
- `.changes/neo4j-cli/v1.4.0.md` — frozen release notes.
- Any change to `--rw`, `--yes`, `--force`, `--organization-id`, or
  `--project-id` semantics.
- Any non-aura subtree (`query`, `credential`, `docker`, `desktop`,
  `skill`, `config`).

## Open Questions

None — branch name, changelog kind, and regression coverage are decided.
