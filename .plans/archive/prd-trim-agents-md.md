# PRD: Trim AGENTS.md

## Overview

AGENTS.md (symlinked as CLAUDE.md) has grown to 322 lines / 43 KB. Much of its content is either stale historical context, granular per-subsystem implementation gotchas that belong in `.agents/` sub-files, or current-state codebase documentation that the code itself already encodes. The goal is to cut the file to a tight, durable set of agent-facing rules and pointers — reducing noise and token cost for every agent invocation.

## Goals

- Reduce AGENTS.md to durable, high-signal guidance that every agent needs on every task.
- Move granular subsystem gotchas to appropriate `.agents/` sub-files so they are available on demand rather than always loaded.
- Delete content that documents historical decisions or retired features with no ongoing relevance.
- Remove sections that document current codebase state that an agent can derive by reading the code.

## Non-Goals

- Rewriting or restructuring sections that are already concise and accurate (e.g., Feedback Instructions, Cobra Command Layout, Build System, Testing Framework overview).
- Changing any code.
- Removing content from `.agents/` sub-files that are already there.

## Requirements

### Functional Requirements

- REQ-F-001: Remove all "historical context" notes about retired or never-shipped features:
  - References to the retired `aura-cli` standalone binary (beyond the one factual note in Architecture that it still compiles but isn't shipped).
  - The commented-out `aura.output` / `output` → `format` migration notes.
  - Notes about the removed `--insecure` flag and `NEO4J_INSECURE` env var.
  - The `aura-cli` changie project removal context.

- REQ-F-002: Move the following granular per-subsystem sections from AGENTS.md into a new `.agents/query.md` file:
  - "query Bolt Driver Notes"
  - "query Bolt Execution Notes"
  - "query/connect.go Credential Integration Notes"
  - "Local Verification Scripts" (the `TestBolt_Smoke` entry)
  Add a short pointer line in AGENTS.md: `See [.agents/query.md](.agents/query.md) for Bolt driver, execution, and credential integration gotchas.`

- REQ-F-003: Move the following sections from AGENTS.md into a new `.agents/cobra.md` file:
  - "Cobra Flag Access Notes"
  Add a short pointer in AGENTS.md or merge the pointer into the existing Cobra Command Layout section.

- REQ-F-004: Move "Changie Workflow Notes", "Release Workflow Notes", and "GoReleaser Notes" from AGENTS.md into the existing `.agents/deployment.md`, appending them to that file.
  Replace the three sections in AGENTS.md with a one-line pointer: `See [.agents/deployment.md](.agents/deployment.md) for changie workflow, release workflow, and GoReleaser gotchas.`

- REQ-F-005: Move "common/output Testing Notes" and "toon-go Notes" from AGENTS.md into the existing `.agents/testing.md` and `.agents/architecture.md` respectively (or a new `.agents/output.md` if neither is a natural fit).
  Replace with pointer lines.

- REQ-F-006: Remove the following sections entirely, as they document the current codebase state that agents can derive by reading the code:
  - "Config Architecture Notes"
  - "Output Rendering Notes"
  - "Command Tree Restructuring Notes"

- REQ-F-007: Keep the following sections unchanged in AGENTS.md:
  - "Feedback Instructions"
  - "Cobra Command Layout"
  - "Project Overview"
  - "Build System" (summary + pointer to `.agents/build.md`)
  - "Testing Framework" (summary + pointer to `.agents/testing.md`)
  - "Architecture" (summary + pointer to `.agents/architecture.md`)
  - "Deployment" (summary + pointer to `.agents/deployment.md`)
  - "Makefile Notes"
  - "Changie Notes" (the concise bullet list about changie mechanics — keep, distinct from Changie Workflow Notes)
  - "Repo Doc Notes"
  - "Repo Layout Notes"
  - "Hermetic Test Notes"
  - "Windows CI Gotchas"
  - "npm Distribution Notes"
  - "PyPI Distribution Notes"
  - "golangci-lint Notes"
  - "Credentials Storage Notes"

### Non-Functional Requirements

- REQ-NF-001: AGENTS.md should target ≤ 150 lines after the trim.
- REQ-NF-002: No information that was moved should be lost — it must appear in the destination `.agents/` file.
- REQ-NF-003: All pointer lines added to AGENTS.md must use relative Markdown links that resolve from the repo root.

## Technical Considerations

- AGENTS.md is the source file; CLAUDE.md is a symlink to it. Edit AGENTS.md only — both surfaces update automatically.
- The file has a `<!-- BEGIN GENERATED: AGENTS-MD -->` / `<!-- END GENERATED: AGENTS-MD -->` wrapper. Keep these markers; they may be used by tooling.
- `.agents/deployment.md` already exists — append to it rather than creating a new file.
- `.agents/testing.md` already exists — append "common/output Testing Notes" to it.
- `.agents/architecture.md` already exists — append "toon-go Notes" to it.
- Create `.agents/query.md` (new) and `.agents/cobra.md` (new).
- No changelog entry needed — this is a documentation-only internal change.
- No `make test`, `make lint`, or `make fmt-check` required — no Go source files are modified.

## Acceptance Criteria

- [ ] AGENTS.md is ≤ 150 lines after editing.
- [ ] All sections listed in REQ-F-001 are deleted with no replacement.
- [ ] All sections listed in REQ-F-002 through REQ-F-005 appear verbatim in their destination `.agents/` file and are replaced by pointer lines in AGENTS.md.
- [ ] All sections listed in REQ-F-006 are deleted with no replacement.
- [ ] All sections listed in REQ-F-007 are present and unchanged in AGENTS.md.
- [ ] `.agents/query.md` and `.agents/cobra.md` are created and contain the moved content.
- [ ] `.agents/deployment.md`, `.agents/testing.md`, and `.agents/architecture.md` are updated with the moved content appended.
- [ ] The `<!-- BEGIN GENERATED: AGENTS-MD -->` and `<!-- END GENERATED: AGENTS-MD -->` markers are preserved.
- [ ] CLAUDE.md symlink continues to resolve correctly (no changes needed — symlink points to AGENTS.md).

## Out of Scope

- Rewriting the content of sections that are being kept.
- Restructuring `.agents/` sub-files beyond appending to them.
- Any code changes.
- Adding new documentation.

## Open Questions

None.
