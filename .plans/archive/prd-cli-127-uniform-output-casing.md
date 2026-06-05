# PRD: CLI-127 Uniform Casing (output snake_case, input kebab-case)

## Overview

`neo4j-cli` output field names (the keys in `--format json`/`toon` and the column
headers in `--format table`) currently mix three casing styles: snake_case, kebab-case,
and camelCase. This is inconsistent across commands and surprising for users piping
output to `jq` or other tools. This feature standardizes **all rendered output field
names to `snake_case`** and **all input identifiers (commands, aliases, flags) to
`kebab-case`**, then adds gate tests so the conventions cannot silently drift.

Investigation (independent of the Linear comments) confirmed snake_case is already the
dominant output style — most surfaces are compliant. The actual work is converting a
small set of kebab/camel **output** outliers to snake_case, decoupling desktop output
from its camelCase wire format, and adding enforcement. Flags are already all kebab, so
no flag renames are required. Breaking changes to output field names are accepted (no
deprecation/transition layer).

## Goals

- One uniform casing for every rendered output field name: `snake_case`.
- One uniform casing for every input identifier (command names, aliases, flag names):
  `kebab-case`.
- Automated gate tests that fail CI when a new output field or input identifier violates
  the convention.
- Documented conventions in `AGENTS.md` and `CONTRIBUTING.md` for future contributors.

## Non-Goals

- Renaming any CLI flags or commands (already kebab-case and compliant).
- Changing wire/parse data shapes (Aura REST API request/response structs, Neo4j Desktop
  API structs, OAuth token fields, GitHub release fields).
- Renaming persisted Docker label keys (`org.neo4j.cli.bolt-port`, etc.).
- Adding a `--raw-envelope`/raw-API passthrough (CLI-92; separate work).
- Any deprecation, dual-emit, or backward-compatibility transition for the renamed output
  fields.

## Requirements

### Functional Requirements

- REQ-F-001: All rendered output field names (JSON keys, TOON keys, table column headers)
  across every command MUST be `snake_case` (`^[a-z0-9]+(_[a-z0-9]+)*$`). Single-word
  lowercase names already satisfy this.
- REQ-F-002: Docker output fields `bolt-port`/`http-port` MUST render as
  `bolt_port`/`http_port` in `docker list`/`get`/`create` (JSON, TOON, and table). The
  persisted Docker label constants in `neo4j-cli/internal/subcommands/docker/labels.go`
  (`org.neo4j.cli.bolt-port`, `org.neo4j.cli.http-port`) MUST remain unchanged.
- REQ-F-003: dbms credential output columns MUST render as `database_name` and
  `embed_credential` (was `database-name`/`embed-credential`) in `credential dbms list`.
- REQ-F-004: embed credential output column MUST render as `base_url` (was `base-url`) in
  `credential embed list`.
- REQ-F-005: workspace list output fields MUST render as `organization_id`, `project_id`,
  `project_name` (was `organizationId`/`projectId`/`projectName`).
- REQ-F-006: Desktop output fields MUST render as `connection_uri`, `pending_restart`,
  and `file_path` (was `connectionUri`/`pendingRestart`/`filePath`) in the desktop
  `connection`/`dbms`/`dbms plugin` list/create/delete commands. This is achieved by
  decoupling the rendered output from the camelCase `desktopclient` wire structs (a snake
  output projection), NOT by changing the wire structs.
- REQ-F-007: The `--database-name`, `--embed-credential`, `--base-url`,
  `--default-workspace` (and all other) CLI flags MUST remain kebab-case, even where the
  matching output column is renamed to snake_case. Request bodies sent to Neo4j Desktop
  MUST continue to send `connectionUri` (camelCase) unchanged.
- REQ-F-008: All input identifiers — command `Use` names, command aliases, and flag long
  names — MUST be `kebab-case` (`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`). Single-character flag
  shorthands are exempt from the multi-segment shape.
- REQ-F-009: An output-casing gate test MUST fail when a `fields []string` literal passed
  to `PrintBodyMap`/`PrintBodyMaps`/`PrintBody`/`PrintRawBody`, or a `json:"…"` tag on an
  enumerated output struct, is not snake_case.
- REQ-F-010: An input-casing gate test MUST walk the live cobra tree (`app.NewCmd(cfg)`)
  and fail when any command name, alias, or flag long name is not kebab-case.
- REQ-F-011: The `AGENTS.md` and `CONTRIBUTING.md` docs MUST state the two conventions
  and the exemptions (wire/parse structs, config keys, Docker label constants, enum
  values), so future additions follow them.

### Non-Functional Requirements

- REQ-NF-001: No change to surfaces already compliant with snake_case output — Aura
  commands, `update`, `agentcontext`, `query` JSON, the `clierr` error envelope, and
  desktop `doctor` — beyond what the gate may require.
- REQ-NF-002: The gate tests MUST be hermetic and run as part of `make test` (no network,
  no real FS credentials), and MUST produce a failure message naming the offending
  file/identifier.
- REQ-NF-003: The skill bundle MUST be regenerated (`go generate ./...`) so
  `TestGenerator_RoundTrip` stays green after output field-name changes surface in
  `references/*.md`.
- REQ-NF-004: A single changelog entry (changie, `--kind Minor`, `--projects neo4j-cli`)
  MUST document the breaking output field-name rename.

## Technical Considerations

- **Rendering pipeline** (`common/output/output.go`): `PrintBodyMap` renders JSON via
  `json.MarshalIndent`, table via explicit `fields []string` headers + map lookups, and
  TOON via a JSON intermediate. Output field names therefore originate from either (a)
  `map[string]any` keys (Aura responses, docker/credential output maps), or (b) `json:"…"`
  struct tags (typed responses), plus (c) the `fields` slice for table headers. The gate
  must cover (a)/(b) via the `fields` literals and the enumerated output structs.
- **Aura is already snake** because its output maps are the snake_case keys parsed
  straight from the Aura REST API; no Aura code change is needed (the existing
  `utils.RenameResponseField` `tenant_id`→`project_id` already yields snake).
- **Docker label decoupling**: output keys (`bolt_port`) are independent from the
  persisted label keys (`org.neo4j.cli.bolt-port`); only the rendered map key / json tag /
  `fields` entry changes, the `labels[LabelBoltPort]` lookup is untouched.
- **Credential output vs. flags**: `database-name`/`embed-credential`/`base-url` appear as
  both a kebab CLI flag and a `Printable()` output column key. Only the `Printable()` map
  key (`common/clicfg/credentials/{dbms,embed}.go`) and the `…Fields` slice change.
- **Desktop wire decoupling**: `desktopclient/types.go` camelCase tags parse Neo4j
  Desktop's API and serialize request bodies; the list renderers currently
  `json.Marshal` those structs directly. Introduce snake output projections (maps or
  output structs) so output is snake while parsing/requests stay camelCase.
- **Gate test patterns**: the input gate models the existing cobra tree-walk
  `TestAllLeafCommands_HaveExamples` in
  `neo4j-cli/internal/subcommands/agentcontext/agentcontext_test.go`. The output gate is a
  `go/parser`+`go/ast` repo-walk (see gate-test repo-walking notes in
  `.agents/hermetic-tests.md`) with an allowlist for wire/parse files
  (`aura/internal/api/response.go`, `internal/desktopclient/types.go`, OAuth,
  `update/release.go`).
- **One-file-per-leaf cobra layout** and colocated `*_test.go` conventions
  (AGENTS.md) apply to all edits; tests use `testfs`/afero memFS, not the real OS FS.

## Acceptance Criteria

- [ ] `docker list`/`get`/`create` output uses `bolt_port`/`http_port`; Docker label
      constants unchanged; container discovery still works.
- [ ] `credential dbms list` output uses `database_name`/`embed_credential`;
      `credential embed list` uses `base_url`; the `--database-name`/`--embed-credential`/
      `--base-url` flags are unchanged.
- [ ] `aura workspace list` output uses `organization_id`/`project_id`/`project_name`.
- [ ] Desktop `connection`/`dbms`/`dbms plugin` output uses
      `connection_uri`/`pending_restart`/`file_path`; requests to Desktop still send
      `connectionUri`.
- [ ] Output snake_case gate test passes on the converted tree and fails on a
      reintroduced kebab/camel output field.
- [ ] Input kebab-case gate test passes and fails on a reintroduced `--bad_flag` or
      snake/camel command name/alias.
- [ ] `make test`, `make fmt-check`, `make lint`, and `make generate-check` all pass.
- [ ] `AGENTS.md` and `CONTRIBUTING.md` document both conventions and exemptions.
- [ ] A single `--kind Minor` changie entry exists describing the breaking output rename.

## Out of Scope

- Flag/command renames (already kebab).
- Wire/parse struct changes, OAuth/GitHub fields, Docker label keys, enum values.
- `--raw-envelope` / raw API passthrough.
- Any deprecation/transition layer for renamed output fields.

## Open Questions

- None. Resolved during planning: changie kind = **Minor**; input gate covers **all**
  input identifiers (commands + aliases + flags, shorthands exempt); output gate uses the
  **recommended scope** (`fields` slice literals + enumerated output-struct json tags with
  the wire/parse allowlist).
