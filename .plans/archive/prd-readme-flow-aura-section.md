# PRD: README Flow Update — Add Aura Section, Drop Usage

## Overview

The top of `README.md` is stale. The `## Usage` section duplicates content covered better in surrounding sections (binary extraction, PATH move, generic help discovery) and buries the one piece that *is* useful — Aura API credential creation — between unrelated install hygiene tips. Sections below it (Credentials, Querying Neo4j, Write operations, Agent skills) are current and well-structured.

This change replaces `## Usage` with a focused `## Aura` quickstart positioned between `## Credentials` and `## Querying Neo4j`. Aura is the headline use case for the CLI; users should land from Installation, through the Credentials reference, straight into a working "list / create instance" flow without reading stale PATH-move advice along the way.

README-only change. No code, no skill-bundle regeneration, no changelog entry.

## Goals

- Replace the stale `## Usage` block with a concise, current Aura quickstart.
- Position Aura above Querying Neo4j so the README reads top-to-bottom as install → credentials → Aura → Cypher.
- Keep the existing Credentials section as the comprehensive reference for `aura-client`, `dbms`, and `embed`; the new Aura section *points* to it rather than duplicating credential-add commands.
- Lift only the still-relevant content from the old Usage section (Aura API credentials creation, instance list example).

## Non-Goals

- No CLI runtime, flag, or help-text changes.
- No skill-bundle (`SKILL.md` / `references/*.md`) edits — README is not part of the bundle.
- No changelog entry — per `CLAUDE.md` the changelog is for user-facing CLI behaviour, not docs.
- No restructuring of Credentials, Querying Neo4j, Write operations, Agent skills, Building locally, or Developing/contributing sections.
- No preservation of the `#usage` anchor (no known external dependents).
- Not adding a full Aura cookbook — quickstart only (list + create). Pause/resume/delete/snapshot/overwrite stay discoverable via `--help` and the skill bundle reference.

## Requirements

### Functional Requirements

- REQ-F-001: Delete the `## Usage` block in `README.md` (currently lines 49–92), including the binary-rename advice for Windows/macOS and the generic `--help` discovery hints.
- REQ-F-002: Insert a new `## Aura` section between `## Credentials` (currently ending around line 125) and `## Querying Neo4j` (currently line 127). Section content per draft below.
- REQ-F-003: The new section MUST link back to the existing `[Credentials](#credentials)` section for the `aura-client` credential add/list/use/remove surface; it MUST NOT duplicate the `credential aura-client add ...` command.
- REQ-F-004: The new section MUST include working examples for:
  - `neo4j-cli aura instance list --format table`
  - `neo4j-cli aura instance create --type free-db ...` (free-db form)
  - `neo4j-cli aura instance create --type professional-db ... --await --rw` (paid form)
- REQ-F-005: Tenant-ID placeholder MUST be `<tenant-id>` (literal angle-bracket placeholder), not the all-zeros UUID used in `--help` output.
- REQ-F-006: All `aura instance create` examples MUST include `--rw` because the command is annotated `write: true` and would fail without it.
- REQ-F-007: The section MUST mention that initial DB credentials returned by `instance create` are auto-stored as a `dbms` credential named `<instance-id>-default`, and reference `--no-credential-storage` as the opt-out.
- REQ-F-008: The section MUST point readers to `aura tenant list` for discovering tenant IDs.
- REQ-F-009: The Account-Settings link target MUST be `https://console.neo4j.io/#account` (matches existing README usage).
- REQ-F-010: No other section in `README.md` is modified. Existing in-document anchors (e.g. `[Credentials](#credentials)` from the Querying section's surroundings) remain valid.

### Non-Functional Requirements

- REQ-NF-001: `make test`, `make fmt-check`, and `make lint` must pass — README-only change, but the gates are mandatory final checks per `CLAUDE.md`.
- REQ-NF-002: Markdown must render correctly on GitHub: section headings, fenced code blocks, and inline links resolve. Spot-check via `glow README.md` or GitHub preview.
- REQ-NF-003: All command examples in the new section must be copy-pasteable and match current flag names verified against `neo4j-cli credential aura-client add --help`, `neo4j-cli aura instance list --help`, and `neo4j-cli aura instance create --help`.
- REQ-NF-004: No CRLF artefacts. The repo's `.gitattributes` already pins committed bundle/golden `.md` files to LF; `README.md` is not pinned but the editor must save LF on darwin (default).

## Technical Considerations

### Drafted Aura section (verbatim insertion)

```markdown
## Aura

Manage Neo4j Aura instances from the terminal. Requires an `aura-client` credential — create one in your Aura [Account Settings](https://console.neo4j.io/#account) and add it via [Credentials](#credentials) above.

### List your instances

```bash
neo4j-cli aura instance list --format table
```

### Create an instance

```bash
# Free-db — no cloud provider, region, or memory required
neo4j-cli aura instance create --name my-free-db --type free-db --tenant-id <tenant-id> --rw

# Professional-db on AWS, awaiting readiness
neo4j-cli aura instance create --name my-pro-db --type professional-db --cloud-provider aws \
  --region us-east-1 --memory 4GB --tenant-id <tenant-id> --await --rw
```

`aura tenant list` shows tenant IDs. Initial DB credentials returned by `instance create` are auto-stored as a `dbms` credential (named `<instance-id>-default`), so `neo4j-cli query` can connect immediately. Use `--no-credential-storage` to skip that.
```

### Resulting top-to-bottom README structure

1. `# Neo4j CLI`
2. `## Installation` (unchanged)
3. ~~`## Usage`~~ — deleted
4. `## Credentials` (unchanged — full reference for `aura-client`, `dbms`, `embed`)
5. `## Aura` — **new**, sits above Querying
6. `## Querying Neo4j` (unchanged)
7. `## Write operations` (unchanged)
8. `## Agent skills` (unchanged)
9. `## Feedback / Issues` (unchanged)
10. `## Building locally` (unchanged)
11. `## Developing and contributing` (unchanged)

### Why the section sits below Credentials, not above

Original ask was "Aura above Querying Neo4j". Credentials already sits above Querying. Per follow-up clarification, the user wants Credentials to remain the first reference after Installation. With Credentials above, the new Aura section delegates the `credential aura-client add ...` example to Credentials and goes straight to instance examples — avoiding duplicated command-add prose.

### Why no skill-bundle regeneration

Per `CLAUDE.md` `Repo Doc Notes` and `Repo Layout Notes`, the skill bundle is generated from cobra command trees and `additions.md`/`description.txt` templates under `neo4j-cli/internal/skill/`. `README.md` is not embedded into the bundle. `make generate-check` will not flag this change.

### Why no changelog entry

Per `CLAUDE.md` `Build System` notes: changelog entries are required for user-facing CLI changes (new features, bug fixes, behaviour changes visible to CLI users). README documentation edits do not change CLI behaviour. The `make test` / `make fmt-check` / `make lint` gates are still mandatory.

### Verification commands

The example commands in the new section must match the current CLI surface. Verify against live help output before committing:

```bash
go run ./neo4j-cli credential aura-client add --help
go run ./neo4j-cli aura tenant list --help
go run ./neo4j-cli aura instance list --help
go run ./neo4j-cli aura instance create --help
```

Confirm `--name`, `--client-id`, `--client-secret`, `--type`, `--tenant-id`, `--memory`, `--region`, `--cloud-provider`, `--await`, `--no-credential-storage`, `--format`, and `--rw` all exist as written.

## Acceptance Criteria

- [ ] `## Usage` section is gone from `README.md`.
- [ ] `## Aura` section exists between `## Credentials` and `## Querying Neo4j`.
- [ ] Aura section links back to `#credentials` for the credential-add surface; does not contain a `credential aura-client add ...` command.
- [ ] Aura section contains `instance list`, `instance create --type free-db`, and `instance create --type professional-db ... --await` examples, all using `<tenant-id>` placeholder, all with `--rw` on the create commands.
- [ ] Aura section mentions `aura tenant list`, the auto-stored `<instance-id>-default` dbms credential, and `--no-credential-storage`.
- [ ] No other section in `README.md` is modified (diff scoped to lines ≈49–125 region).
- [ ] `make test`, `make fmt-check`, `make lint` all pass.
- [ ] Manual GitHub-preview render of `README.md` confirms headings, code fences, and links work; the in-document `[Credentials](#credentials)` link from the new section resolves.
- [ ] No changelog entry added under `.changes/unreleased/`.
- [ ] No `bundle/SKILL.md` or `references/*.md` files modified; `make generate-check` (if run) is clean.

## Out of Scope

- CLI behaviour, flag, or help-text changes.
- Skill-bundle regeneration or edits to `additions.md` / `description.txt`.
- Changelog entry under `.changes/unreleased/`.
- Restructuring or content edits to Credentials, Querying Neo4j, Write operations, Agent skills, or any section other than Usage / new Aura.
- Backwards-compatible `#usage` anchor preservation (no known external dependents).
- Adding pause/resume/delete/snapshot/overwrite examples to README — skill bundle and `--help` already cover them.
- Touching `CONTRIBUTING.md`, `AGENTS.md`, or per-distribution READMEs (`distribution/npm/cli/README.md`, `distribution/pypi/README.md`).

## Open Questions

None at PRD time — both originally-flagged decisions (drop binary-rename advice; use `<tenant-id>` placeholder; no changelog; no `#usage` anchor preservation) were resolved during planning.
