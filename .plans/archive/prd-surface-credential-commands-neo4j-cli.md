# PRD: Surface Credential Commands at neo4j-cli Top Level

## Overview

Expose credential management commands (`add`, `list`, `remove`, `use`) at the `neo4j-cli` top level under a `credential` subcommand, with an `aura-client` type qualifier. This allows future `neo4j-cli` subcommands that need Aura API access to share a common credential store, without each subtree duplicating credential management. The standalone `aura-cli credential ...` paths remain fully unchanged.

## Goals

- Users can manage Aura credentials via `neo4j-cli credential <action> aura-client ...` without installing or invoking `aura-cli` directly.
- The `aura-client` type qualifier leaves room for future credential types for other Neo4j services.
- Existing `aura-cli credential ...` commands continue to work identically — no regressions.
- `neo4j-cli aura credential ...` does **not** exist; credentials are only at the top-level `neo4j-cli credential` path.

## Non-Goals

- Adding credential types other than `aura-client`.
- Changing the underlying credential storage format or location.
- Modifying any existing `internal/subcommands/credential/` code.
- Exposing any new commands in `aura-cli`.

## Requirements

### Functional Requirements

- REQ-F-001: `neo4j-cli credential add aura-client --name NAME --client-id ID --client-secret SECRET` adds an Aura credential to the shared credential store.
- REQ-F-002: `neo4j-cli credential list aura-client` lists all stored Aura credentials.
- REQ-F-003: `neo4j-cli credential remove aura-client <name>` removes the named Aura credential.
- REQ-F-004: `neo4j-cli credential use aura-client <name>` sets the named credential as the default.
- REQ-F-005: `aura-cli credential add/list/remove/use` continue to work exactly as before (no regression).
- REQ-F-006: `neo4j-cli aura credential` must not be a registered command (returns "unknown command").
- REQ-F-007: `--name`, `--client-id`, `--client-secret` flags on `add aura-client` are all required; omitting any returns an error.

### Non-Functional Requirements

- REQ-NF-001: The implementation must not violate Go's `internal` package visibility rules — `neo4j-cli/main.go` must not import `neo4j-cli/aura/internal/...` directly.
- REQ-NF-002: All existing tests pass without modification.
- REQ-NF-003: New tests follow the table-driven convention and use `testutils` helpers consistent with existing credential tests.
- REQ-NF-004: All new `.go` files carry the Neo4j copyright header.

## Technical Considerations

**Internal package constraint.** `neo4j-cli/aura/internal/subcommands/credential/` is only importable by code rooted at `neo4j-cli/aura/`. The solution is to add a new public file `neo4j-cli/aura/credential.go` in the `aura` package that exposes `NewCredentialCmd` — the `aura` package can freely import its own `internal` packages, and `neo4j-cli/main.go` can import `aura`.

**`NewStandaloneCmd` split.** Currently `aura.NewCmd` registers `credential.NewCmd` directly. To prevent `neo4j-cli aura credential` from existing, `credential.NewCmd` must be removed from `NewCmd`. A new `NewStandaloneCmd` wraps `NewCmd` and adds the credential subcommand, and is called by `aura-cli/cmd/main.go` instead.

**`NewCredentialCmd` design.** The new top-level credential tree uses `aura-client` as a positional-style subcommand under each action (`add`, `list`, `remove`, `use`). The actual credential operations delegate to the same `cfg.Credentials.Aura.*` methods used by the internal commands — no logic is duplicated.

**Testing.** The new `neo4j-cli/aura/credential_test.go` lives in the `aura` package test boundary. Because `AuraTestHelper.ExecuteCommand` uses `aura.NewCmd`, tests for the new `NewCredentialCmd` tree will need to invoke it directly or use a parallel helper that wires up `NewCredentialCmd` against the same `clicfg.Config`. The simplest approach is a thin test helper in the same file.

## Acceptance Criteria

- [ ] `./bin/neo4j-cli credential add aura-client --name test --client-id abc --client-secret xyz` exits 0 and stores the credential.
- [ ] `./bin/neo4j-cli credential list aura-client` exits 0 and shows stored credentials.
- [ ] `./bin/neo4j-cli credential remove aura-client test` exits 0 and removes the credential.
- [ ] `./bin/neo4j-cli credential use aura-client test` exits 0 and sets the default.
- [ ] `./bin/neo4j-cli aura credential` returns a non-zero exit and prints "unknown command".
- [ ] `./bin/aura-cli credential add --name test2 --client-id abc2 --client-secret xyz2` exits 0.
- [ ] `./bin/aura-cli credential list` shows all credentials from the shared store.
- [ ] `make test` passes with all existing and new tests green.
- [ ] `make build` produces both binaries without errors.
- [ ] `make lint` passes on all new files.

## Files to Create/Modify

| File | Change |
|------|--------|
| `neo4j-cli/aura/credential.go` | **Create** — exports `NewCredentialCmd(cfg)` with `aura-client` subcommands |
| `neo4j-cli/aura/credential_test.go` | **Create** — tests for the new `neo4j-cli credential` command tree |
| `neo4j-cli/aura/aura.go` | Remove `credential.NewCmd` from `NewCmd`; add `NewStandaloneCmd` |
| `neo4j-cli/aura/cmd/main.go` | `aura.NewCmd` → `aura.NewStandaloneCmd` |
| `neo4j-cli/main.go` | `cmd.AddCommand(aura.NewCredentialCmd(cfg))` |

## Out of Scope

- Adding a `list_test.go` for the internal `credential` package (pre-existing gap).
- Credential types beyond `aura-client`.
- Any changes to `internal/subcommands/credential/` or its tests.

## Open Questions

None — the plan document fully specifies the required command shapes, file changes, and test expectations.
