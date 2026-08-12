# PRD: Remove Data API Commands from Beta Flag Gate

## Overview

The `data-api` subcommand tree (including `graphql`, `graphql auth-provider`, and `graphql cors-policy allowed-origin`) is currently only registered when `flag.aura-beta` is enabled. This feature removes that gate so Data API commands are available to all users unconditionally.

## Goals

- Make all `aura data-api` commands available to users without needing to enable the beta flag.
- Update the beta flag registry to accurately reflect that `dataapi` is no longer gated.
- Keep `import` and `deployment` subcommands behind the beta flag (they are not part of this change).

## Non-Goals

- Removing the `flag.aura-beta` flag itself (that is CLI-154).
- Ungating `import` or `deployment` commands.
- Changing any Data API command logic, flags, or output.

## Requirements

### Functional Requirements

- REQ-F-001: `aura data-api` and all its subcommands (`graphql`, `graphql auth-provider`, `graphql cors-policy allowed-origin`) are registered unconditionally in `aura.go`, regardless of `flag.aura-beta`.
- REQ-F-002: `aura import` and `aura deployment` remain gated behind `flag.aura-beta` (no change to their registration logic).
- REQ-F-003: The `Gates` description in the `flag.aura-beta` registry entry in `common/clicfg/flags.go` is updated to remove `dataapi` from the list.
- REQ-F-004: Any test that sets `flag.aura-beta = true` solely to access `data-api` commands has that override removed.
- REQ-F-005: The skill bundle is regenerated (`go generate ./neo4j-cli/internal/skill/...`) so `TestGenerator_RoundTrip` passes.
- REQ-F-006: A `Patch` changelog entry is added for this user-facing change.

### Non-Functional Requirements

- REQ-NF-001: `make test`, `make fmt-check`, and `make lint` all pass after the change.
- REQ-NF-002: No existing Data API command behaviour changes; this is registration-only.

## Technical Considerations

### Key Change — `neo4j-cli/aura/aura.go` lines 41–45

Current:
```go
if cfg.Flags.Enabled("flag.aura-beta") {
    cmd.AddCommand(dataapi.NewCmd(cfg))
    cmd.AddCommand(_import.NewCmd(cfg))
    cmd.AddCommand(deployment.NewCmd(cfg))
}
```

After:
```go
cmd.AddCommand(dataapi.NewCmd(cfg))
if cfg.Flags.Enabled("flag.aura-beta") {
    cmd.AddCommand(_import.NewCmd(cfg))
    cmd.AddCommand(deployment.NewCmd(cfg))
}
```

### Flag Registry Update — `common/clicfg/flags.go` line 48

Change `Gates` from:
```
"aura {dataapi, import, deployment} subcommands; v1beta5 API path"
```
to:
```
"aura {import, deployment} subcommands; v1beta5 API path"
```

### Test Cleanup

Search all `*_test.go` files under `neo4j-cli/aura/internal/subcommands/dataapi/` and related test helpers for calls to `cfg.Flags.SetForTest("flag.aura-beta", true)`. Remove any that exist solely because the command was beta-gated (keep any that test beta-specific behaviour if present).

### Skill Bundle Regeneration

After the structural change to `aura.go`, run:
```
go generate ./neo4j-cli/internal/skill/...
```
This refreshes `neo4j-cli/internal/skill/bundle/` so `TestGenerator_RoundTrip` does not fail.

### Changelog

Add a `Patch` entry via:
```
changie new --projects neo4j-cli --kind Patch --body "Data API commands (data-api graphql, auth-provider, cors-policy) are now enabled for all users and no longer require the beta flag."
```

## Acceptance Criteria

- [ ] `aura data-api` and all sub-subcommands are accessible without setting `flag.aura-beta`.
- [ ] `aura import` and `aura deployment` still require `flag.aura-beta = true`.
- [ ] `common/clicfg/flags.go` `Gates` field no longer lists `dataapi`.
- [ ] No `data-api` test file sets `flag.aura-beta` as a prerequisite.
- [ ] `make test` passes (including `TestGenerator_RoundTrip`).
- [ ] `make fmt-check` passes.
- [ ] `make lint` passes.
- [ ] A changelog entry exists under `.changes/unreleased/` describing the change.

## Out of Scope

- Removal of the `flag.aura-beta` flag itself (tracked in CLI-154, which this PR unblocks).
- Any changes to Data API command logic, API paths, or output format.
- Ungating `import` or `deployment`.

## Open Questions

None — scope is fully defined by the Linear card and existing code.
