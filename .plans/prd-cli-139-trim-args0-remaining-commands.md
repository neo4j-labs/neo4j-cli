# PRD: Apply args[0] TrimSpace fix to remaining aura subcommands (CLI-139)

Linear: https://linear.app/neo4j/issue/CLI-139
Followup to: CLI-98 / PR #128 (merged)
Coordination: PR #127 (oskar/cli-135-remove-the-aura-cli-build-config) — no file overlap; safe to land independently.

## Overview

PR #128 fixed a panic where shell command substitution (`$(some-cmd)`) leaves a trailing `\n` on a positional ID arg, which propagates into the URL path and crashes JSON unmarshalling. It applied `strings.TrimSpace(args[0])` to 9 aura subcommands. ~22 sibling commands still use raw `args[0]` with the same pattern. This feature closes that gap across all remaining commands that take a positional ID, name, or config key.

## Goals

- Eliminate the trailing-newline panic for every aura subcommand that consumes a positional identifier.
- Preserve the in-flight behaviour and error messages — no UX regressions for callers passing clean IDs.
- Match the implementation shape and test style of PR #128 so the change is mechanical and reviewable.

## Non-Goals

- Trimming positional **value** args (e.g. `args[1]` of `config set <key> <value>`) — values are user-chosen strings; trimming could mask legitimate input.
- Trimming flag values (`--instance-id=...` etc.) — flags don't exhibit the substitution-newline problem in the same way and weren't part of CLI-98.
- Extracting a `trimID(...)` helper — duplication is one-line; wrapping it adds an import for zero clarity gain.
- Auditing or fixing commands outside `neo4j-cli/aura/internal/subcommands/`.
- Any change to the standalone `aura` skill bundle being deleted by PR #127.

## Requirements

### Functional Requirements

- **REQ-F-001:** Each command in Group A (URL-path / API calls) trims `args[0]` with `strings.TrimSpace` and uses the trimmed value for every subsequent reference (URL path, log line, stderr message).

  Files (16):
  - `neo4j-cli/aura/internal/subcommands/instance/{delete,pause,resume,update}.go`
  - `neo4j-cli/aura/internal/subcommands/instance/snapshot/get.go` *(args[0] is snapshot ID — name the local `snapshotId`)*
  - `neo4j-cli/aura/internal/subcommands/customermanagedkey/{get,delete}.go`
  - `neo4j-cli/aura/internal/subcommands/dataapi/graphql/{get,delete,pause,resume,update}.go`
  - `neo4j-cli/aura/internal/subcommands/dataapi/graphql/authprovider/{get,delete}.go`
  - `neo4j-cli/aura/internal/subcommands/graphanalytics/session/{get,delete}.go`

- **REQ-F-002:** Each command in Group B (plain config / credential lookups, no `ValidArgs` pre-validation) trims `args[0]` with `strings.TrimSpace` inside `RunE` before passing to the config/credential store.

  Files (4):
  - `neo4j-cli/aura/internal/subcommands/config/project/{use,remove}.go`
  - `neo4j-cli/aura/internal/subcommands/credential/{use,remove}.go`

- **REQ-F-003:** Each command in Group C (validated config keys, `cobra.OnlyValidArgs` or manual validation) trims `args[0]` **inside the `Args` function before validation runs** by mutating the args slice in place. This ensures validation sees the trimmed value and `RunE` does too, preventing a "invalid argument" error when the user passes `default-tenant\n`.

  Files (2):
  - `neo4j-cli/aura/internal/subcommands/config/get.go` — replace `Args: cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs)` with a custom function that calls `cobra.ExactArgs(1)`, mutates `args[0] = strings.TrimSpace(args[0])`, then calls `cobra.OnlyValidArgs`.
  - `neo4j-cli/aura/internal/subcommands/config/set.go` — add `args[0] = strings.TrimSpace(args[0])` at the top of the existing Args func, immediately after the `ExactArgs(2)` check.

- **REQ-F-004:** `config set` MUST NOT trim `args[1]` (the value). Values are user-chosen strings (URLs, paths, format names, etc.) where leading/trailing whitespace may be intentional or — if accidental — should be a user-visible bug rather than silently swallowed.

- **REQ-F-005:** Each fixed command gains a unit test mirroring PR #128's `TestGetInstanceWithTrailingNewline` shape: invoke the command via `helper.ExecuteCommand(fmt.Sprintf("<cmd> %s\"\n\"", id))` (the quoted `\n` suffix simulates the shell-substitution newline), then assert the mock handler was called with the **trimmed** ID in the URL path (no `%0A`). Table-driven where it fits naturally; otherwise one focused test function per file mirroring the PR #128 style.

- **REQ-F-006:** Group B tests (no HTTP) follow the existing `use_test.go` / `remove_test.go` shape for those packages — set the default via the trimmed input, then assert the stored default matches the clean name.

- **REQ-F-007:** Group C tests add a second case asserting `config get default-tenant\n` (or equivalent for `set`) does NOT produce an `invalid argument` error — regression guard for the in-Args trim.

### Non-Functional Requirements

- **REQ-NF-001:** `make test`, `make fmt-check`, `make lint`, `make license-check`, and `make generate-check` must all pass.
- **REQ-NF-002:** The skill bundle (`neo4j-cli/internal/skill/bundle/**`) must be unchanged by this work — RunE-body and Args-func edits do not surface in bundle output. `make generate-check` confirms.
- **REQ-NF-003:** Changelog entry via changie: `changie new --projects neo4j-cli --kind Patch --body "Trim whitespace from positional ID args in remaining aura subcommands (CLI-139)"`.
- **REQ-NF-004:** Branch named `oskar/cli-139-trim-args0-remaining` (Oskar prefix per personal convention; descriptive slug).
- **REQ-NF-005:** Single PR with 3 commits, one per group (A → B → C). Allows reviewers to scan group-by-group; single merge keeps history coherent.

## Technical Considerations

### Pattern reference

PR #128 (`7e7f0fa`) sets the canonical shape. For Group A:

```go
import "strings"
...
RunE: func(cmd *cobra.Command, args []string) error {
    instanceId := strings.TrimSpace(args[0])
    path := fmt.Sprintf("/instances/%s", instanceId)
    ...
}
```

For files where `args[0]` is referenced 2-3× (e.g. `customermanagedkey/delete.go`, `dataapi/graphql/pause.go`), assign once to a local var and reuse — don't sprinkle `TrimSpace` at every callsite.

### Group C in-Args mutation rationale

`config/get.go` validates `args[0]` against `cfg.Aura.ValidConfigKeys` BEFORE `RunE` runs (cobra Args runs first). Trimming only in `RunE` means the user sees a confusing `invalid argument "default-tenant\n"` error when shell substitution adds a newline. Mutating the args slice inside the Args func is the cleanest path: cobra passes the slice by reference, so the trimmed value is visible to both `OnlyValidArgs` and `RunE`. The shape:

```go
Args: func(cmd *cobra.Command, args []string) error {
    if err := cobra.ExactArgs(1)(cmd, args); err != nil {
        return err
    }
    args[0] = strings.TrimSpace(args[0])
    return cobra.OnlyValidArgs(cmd, args)
},
```

`config/set.go` already has a custom Args func with manual validation — just add `args[0] = strings.TrimSpace(args[0])` immediately after the `ExactArgs(2)` check.

### PR #127 coordination

PR #127 deletes `neo4j-cli/aura/cmd/` and `neo4j-cli/aura/internal/skill/` and edits `.agents/*.md`, workflows, `AGENTS.md`, `CHANGELOG.md`, `RELEASING.md`, `common/skill/render/render.go`, `neo4j-cli/aura/aura.go`, and `neo4j-cli/internal/skill/{additions.md,bundle/SKILL.md}`. Zero overlap with CLI-139's target set (all under `neo4j-cli/aura/internal/subcommands/`). No merge conflict expected. CLI-139 can land before or after PR #127 in any order.

While both PRs are open, both skill generators still exist locally — run `go generate ./...` (or `make generate-check`) as a sanity gate; expect no diff.

### Cobra one-file-per-leaf layout

This repo follows the strict one-file-per-leaf cobra layout documented in AGENTS.md. Tests are colocated with source (`get.go` + `get_test.go`). No new files are created by this work — only edits to existing leaf files and additions to existing test files.

## Acceptance Criteria

- [ ] All 22 files in REQ-F-001 / REQ-F-002 / REQ-F-003 trim `args[0]` per the group-specific approach.
- [ ] `config/set.go` `args[1]` (value) is NOT trimmed.
- [ ] Each fixed file has a new unit test exercising the trailing-newline case and asserting the mock receives the trimmed ID.
- [ ] Group C tests add an `invalid argument` regression guard.
- [ ] `make test` passes on the branch (CI green on ubuntu / windows / macos).
- [ ] `make fmt-check`, `make lint`, `make license-check`, `make generate-check` all pass.
- [ ] Skill bundle (`neo4j-cli/internal/skill/bundle/**`) is unchanged in the diff.
- [ ] Changelog entry exists under `.changes/unreleased/` with `project: neo4j-cli`, `kind: Patch`.
- [ ] PR contains exactly 3 commits (Group A, B, C); single PR titled `fix: trim args[0] in remaining aura subcommands (CLI-139)` or similar conventional-commit form.
- [ ] Manual smoke: `neo4j-cli aura instance get $(printf "00000000-0000-0000-0000-000000000000\n") --format json` returns JSON (or 404), does NOT panic.

## Out of Scope

- Trimming `args[1]` in `config set` (the value).
- Trimming flag values.
- Auditing commands outside `neo4j-cli/aura/internal/subcommands/`.
- Refactoring to a shared `trimID(...)` helper.
- Any change to the aura standalone skill bundle being deleted by PR #127.
- Backporting to released versions.

## Open Questions

None — all major decisions resolved in plan mode:
1. Single PR, 3 commits by group.
2. Group C trims via in-place `args[0]` mutation inside the Args func.
3. `config set` value (args[1]) is not trimmed.
