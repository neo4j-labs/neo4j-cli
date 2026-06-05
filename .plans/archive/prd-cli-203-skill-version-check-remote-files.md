# PRD: Switch skill version check to remote skill files

## Overview

`neo4j-cli skill check` (and `skill list`) detect version drift by comparing the
`version:` frontmatter of each *installed* `SKILL.md` against an *available*
version. Today the available version for every curated catalog skill is a single
shared value: the top-level `version` field of the upstream `plugin.json`
(`catalog.Catalog.Version`). With ~28 curated skills sharing one number, the
check cannot report which individual skill drifted, and install stamps that same
shared number into every skill it writes.

Upstream (`github.com/neo4j-contrib/neo4j-skills`) now carries a per-skill
`version:` line in each skill's own `SKILL.md` frontmatter (verified: each
currently reads `version: 1.0.1`). This feature switches the version check to
read each skill's *own* `SKILL.md` version instead of the shared `plugin.json`
version, so drift is reported per skill, and makes `install` preserve the
upstream per-skill version verbatim.

## Goals

- Source the "available" version for a catalog skill from that skill's own
  remote `SKILL.md` `version:` frontmatter (as cached locally), not from
  `plugin.json`'s top-level `version`.
- `skill install` writes a catalog skill's upstream `version:` verbatim, so the
  installed version matches the remote per-skill version at install time and
  only diverges (→ drift) when that specific skill is bumped upstream.
- Keep the existing classification semantics (`ok` / `drift` / `unknown-version`
  / `not-installed` / `partial`) and JSON/table/toon output shapes unchanged.

## Non-Goals

- Changing the catalog cache/refresh mechanics. The
  `cachedVersion != pj.Version` tarball re-extract gate in
  `common/skill/catalog/refresh.go` stays as-is; `plugin.json`'s top-level
  `version` remains the cache key, prune key, and refresh trigger (upstream
  bumps it on every change). (Decision confirmed with stakeholder.)
- Adding per-skill versioning to `plugin.json` itself.
- Changing the self-skill version source (still the running binary version,
  `cfg.Version`).
- Changing the `skill refresh` command's reported catalog version (still the
  `plugin.json` version).
- Network/HTTP, security/extraction, or `--format` flag behavior changes.

## Requirements

### Functional Requirements

- REQ-F-001: For each catalog skill, `BuildInventory`
  (`common/skill/inventory.go`) MUST set `AvailableVersion` from the `version:`
  frontmatter of that skill's cached remote `SKILL.md`
  (`<cache>/content/<plugin-version>/<name>/SKILL.md`), not from `cat.Version`.
- REQ-F-002: The per-skill available version MUST be obtained by reusing the
  existing `catalog.Catalog.Lookup` accessor (per-skill `fs.FS`) and the
  existing `parseVersion` helper (`common/skill/installer.go`). No new
  version-parsing regex and no new field on `catalog.SkillEntry`.
- REQ-F-003: When a catalog skill's cached content is missing or its `SKILL.md`
  lacks a parseable `version:` line, the available version MUST resolve to `""`,
  yielding the existing `unknown-version` classification (no panic, no error).
- REQ-F-004: `skill install` of a catalog skill MUST preserve the upstream
  `SKILL.md` `version:` line verbatim — `resolveSkillSource`
  (`common/skill/catalog_load.go`) MUST pass `Source{Version: ""}` for catalog
  entries (the documented "leave frontmatter unchanged" mode). The self-skill
  path (binary version injection) MUST remain unchanged.
- REQ-F-005: `skill check` MUST continue to exit non-zero when any installed row
  is `drift` or `unknown-version`, and `skill list`/`check` JSON/table/toon
  column names and shapes MUST remain unchanged
  (`installed_version`, `current_version`/`available_version`, `status`, etc.).
- REQ-F-006: `check.go`'s `Long` help text MUST be updated to describe the
  catalog-skill source as the skill's own `SKILL.md` `version:` (replacing the
  "plugin.json version for catalog skills" wording). The generated skill bundle
  (`neo4j-cli/internal/skill/bundle/references/skill.md`) MUST be regenerated to
  match (`go generate ./neo4j-cli/internal/skill/...`).

### Non-Functional Requirements

- REQ-NF-001: Test fixtures `makeCatalogTarball` and `seedCatalogCache`
  (`common/skill/catalog_load_test.go`) MUST stamp a `version: <version>` line
  into each generated `SKILL.md`, defaulting to the catalog `version` argument,
  so existing assertions remain valid while sourcing the version from the file.
- REQ-NF-002: A new regression test MUST prove per-skill sourcing: seed a
  catalog skill whose `SKILL.md` `version:` differs from `plugin.json`'s
  top-level version and assert the check's `current_version` reflects the
  `SKILL.md` value, not `plugin.json`.
- REQ-NF-003: `make test`, `make fmt-check`, `make lint`, and
  `make generate-check` MUST all pass; only `internal/skill/bundle/**` diffs are
  expected from regeneration.
- REQ-NF-004: A single user-facing changelog entry MUST be added via changie
  (kind `Patch`): `changie new --projects neo4j-cli --kind Patch --body "..."`.

## Technical Considerations

- **Layering**: `common/skill` imports `common/skill/catalog`; the catalog
  package already "exposes per-skill `fs.FS` handles to the rest of
  `common/skill`" via `Lookup`. Reading the version in `inventory.go` through
  `Lookup` + `parseVersion` honors that direction with zero duplication and no
  catalog struct changes.
- **Consistency**: after REQ-F-004, the installed file and the available-version
  lookup read the *same* upstream `SKILL.md` bytes, so a freshly installed skill
  reports `ok`; drift appears only after upstream bumps that skill and the cache
  refreshes.
- **`Lookup` preconditions**: rejects reserved names (already skipped in
  `BuildInventory`) and requires a non-empty `cat.Version` + present content
  dir; the error path maps cleanly to REQ-F-003's `""`.
- **Touch points** (production): `common/skill/inventory.go` (line ~64 + new
  helper, add `io/fs` import) and `common/skill/catalog_load.go` (line ~171).
  Help text: `common/skill/check.go`. `refresh.go` unchanged.
- **Test surface**: `catalog_load_test.go` (shared fixtures), `check_test.go`
  (+ new regression test), and an audit of `install_test.go`,
  `installer_test.go`, `list_test.go`, `resolve_test.go` for any assertion that
  a catalog skill's installed/available version equals the `plugin.json`
  version. Verify `list.go` `Long` has no `plugin.json` mention (current grep:
  none).
- **Conventions**: Go, Makefile gates, testify + afero in-memory FS, cobra
  one-file-per-leaf, Neo4j copyright header on all `.go` files, per AGENTS.md.

## Acceptance Criteria

- [ ] `skill check`/`list` report each catalog skill's available version from
      its own cached `SKILL.md` `version:`, not from `plugin.json`.
- [ ] Distinct upstream per-skill versions produce distinct `current_version`/
      `available_version` values in one `check`/`list` run.
- [ ] `skill install <catalog-skill>` preserves the upstream `version:` verbatim;
      an immediate `check` reports `ok` for that skill.
- [ ] Editing an installed catalog `SKILL.md` `version:` to a stale value yields
      `status: drift` with `current_version` still equal to the upstream
      `SKILL.md` value.
- [ ] Self-skill version behavior (binary version) is unchanged.
- [ ] New regression test (REQ-NF-002) passes; existing skill-package tests pass.
- [ ] `make test`, `make fmt-check`, `make lint`, `make generate-check` clean.
- [ ] Single changie `Patch` entry added; updated `references/skill.md` bundle
      committed alongside the source change.

## Out of Scope

- Refresh/cache-key changes (`refresh.go` gate kept).
- Per-skill version fields in `plugin.json`.
- Self-skill version source changes.
- Any HTTP/security/extraction or output-format changes.

## Open Questions

- None. Refresh-trigger behavior confirmed: keep the `plugin.json`-version gate.
