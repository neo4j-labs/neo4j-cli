# PRD: `--yes` / `--force` Gate on Destructive Operations

## Overview

Add a uniform confirmation gate (`--yes` and `--force`) to every destructive
cobra leaf in `neo4j-cli`. Today the only gate on destructive operations is
`--rw` (a read/write permission toggle), so any agent that has been granted
`--rw` can delete arbitrary resources without further challenge. This PRD
introduces a shared `common/confirm` package and applies it to 15 leaves
across the Aura, Desktop, Docker, and credential subtrees.

Tracking: [CLI-90](https://linear.app/neo4j/issue/CLI-90). Spec: `agent-cli-auditor.md`
§6.1 (non-interactive by default) and §6.2 (confirmation triad).

## Goals

- Make destructive operations require explicit, intentional opt-in when invoked
  non-interactively.
- Preserve a TTY-friendly `y/N` prompt for humans across all destructive leaves
  (Aura subtree currently has no prompt at all).
- Standardise on a single helper so every destructive leaf has identical
  flag-binding, prompt wording, and error wording.
- Replace the three existing inconsistent single-flag implementations
  (`docker delete --force`, `desktop/dbms delete --yes`,
  `desktop/connection delete --yes`) with the shared helper.
- Refresh the generated skill bundle so agents discover `--yes` / `--force`
  via `SKILL.md` and `references/*.md`.

## Non-Goals

- `--confirm <resource-name>` reconfirmation for production-tier resources
  (audit §6.3 / P4) — tracked separately.
- Changing `--rw` semantics or coverage.
- Adding `--dry-run` to destructive commands (audit §6.4) — separate work.
- Adding `--no-input` flag (audit §6.2 triad) — separate work; `--yes --force`
  is sufficient for CLI-90.
- Touching non-destructive read or mutation leaves (`update`, `create`,
  `replace`).
- Migrating to a deprecated-alias period for the existing single-flag form on
  leaves 10–12 — the break ships in this PRD per the decision below.

## Requirements

### Functional Requirements

#### Shared helper package (`common/confirm/`)

- **REQ-F-001**: A new package at `common/confirm/` exports a `Flags` struct
  with `Yes bool` and `Force bool` fields.
- **REQ-F-002**: `confirm.Register(cmd *cobra.Command) *confirm.Flags`
  registers `--yes` and `--force` boolean flags on `cmd` and returns the
  bound `Flags`. Both flags default to `false`.
- **REQ-F-003**: `(*Flags).Require(cmd *cobra.Command, resourceID string) error`
  implements the confirmation gate:
  - When `Yes && Force` → return `nil` (proceed, no prompt).
  - When stdin is not a TTY and either flag is missing → return a
    `*clierr.CLIError` from `clierr.NewUsageError(...)` (exit code 2) whose
    message names the resource type and ID and instructs the caller to pass
    both `--yes` and `--force`.
  - When stdin IS a TTY and either flag is missing → write a `y/N` prompt to
    `cmd.ErrOrStderr()` (default N), read one line from `cmd.InOrStdin()`,
    accept `y`/`Y`/`yes` as confirmation. A cancellation (default, empty, or
    any other answer) returns the package-exported sentinel
    `confirm.ErrCancelled`.
- **REQ-F-004**: The resource type passed into prompt/error copy is derived
  from `cmd.Parent().Name()` (e.g. "instance", "deployment", "agent").
- **REQ-F-005**: When `resourceID` is the empty string, prompt/error copy
  degrades gracefully (e.g. "destroying this <type> is irreversible…")
  instead of rendering empty quotes.
- **REQ-F-006**: `confirm.ErrCancelled` is a package-exported sentinel error
  that can be compared via `errors.Is(err, confirm.ErrCancelled)`.
- **REQ-F-007**: TTY detection uses
  `term.IsTerminal(int(os.Stdin.Fd()))` (matches the existing pattern in
  `neo4j-cli/internal/subcommands/docker/delete.go`) and is exposed via a
  package-local `var stdinIsTerminal = func() bool { … }` for tests.

#### Leaf integration (15 commands)

- **REQ-F-010**: Every leaf below registers the confirm flags via
  `confirm.Register(cmd)` in its constructor and calls
  `confirmFlags.Require(cmd, args[0])` (or `""` when no positional exists) at
  the top of `RunE`, after pre-flight argument validation but before any
  destructive API call or filesystem mutation:
  1. `neo4j-cli/aura/internal/subcommands/instance/delete.go`
  2. `neo4j-cli/aura/internal/subcommands/deployment/delete.go` (no positional → pass `""` or the resolved deployment ID)
  3. `neo4j-cli/aura/internal/subcommands/agent/delete.go`
  4. `neo4j-cli/aura/internal/subcommands/customermanagedkey/delete.go`
  5. `neo4j-cli/aura/internal/subcommands/dataapi/graphql/delete.go`
  6. `neo4j-cli/aura/internal/subcommands/graphanalytics/session/delete.go`
  7. `neo4j-cli/aura/internal/subcommands/dataapi/graphql/authprovider/delete.go`
  8. `neo4j-cli/aura/internal/subcommands/deployment/token/delete.go` (no positional)
  9. `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/remove.go`
  10. `neo4j-cli/internal/subcommands/desktop/dbms/delete.go`
  11. `neo4j-cli/internal/subcommands/desktop/connection/delete.go`
  12. `neo4j-cli/internal/subcommands/docker/delete.go`
  13. `neo4j-cli/internal/subcommands/credential/dbms/remove.go`
  14. `neo4j-cli/internal/subcommands/credential/embed/remove.go`
  15. `neo4j-cli/aura/internal/subcommands/credential/remove.go`
- **REQ-F-011**: When `confirmFlags.Require` returns `confirm.ErrCancelled`,
  the leaf writes `"cancelled."` (newline-terminated) to
  `cmd.ErrOrStderr()` and returns `nil` so the process exits 0 with no API
  call made.
- **REQ-F-012**: Any non-cancelled error from `Require` is returned verbatim
  (cobra surfaces it via the configured error path; exit code 2 propagates
  from `clierr.CLIError`).

#### Removal of legacy single-flag plumbing

- **REQ-F-020**: Leaves 10–12 lose their inline `--yes` or `--force` flag
  declaration, inline prompt function, and inline non-TTY gate. All three
  delegate to `confirm.Register` + `confirmFlags.Require`. The flag names
  `--yes` and `--force` continue to exist (now BOTH bound on each of the
  three leaves), so previously-valid single-flag invocations on TTY callers
  start emitting the `y/N` prompt instead of bypassing it, and previously-
  valid single-flag non-TTY invocations start failing with exit 2. This is
  the intended break (see Acceptance Criteria).

#### Help text and examples

- **REQ-F-030**: Each leaf's `Long:` field is extended with a single sentence
  documenting the new gate, e.g. "Destructive: requires `--yes --force` (or
  a `y` answer at the TTY prompt) when invoked non-interactively."
- **REQ-F-031**: Each leaf's `Example:` block is extended with one
  invocation that includes both `--yes --force` so the
  `TestAllLeafCommands_HaveExamples` gate continues to pass and the
  ≥3-examples invariant from `.agents/...` is preserved. Existing examples
  are not removed.

#### Skill bundle

- **REQ-F-040**: After all leaf edits, the committed bundle at
  `neo4j-cli/internal/skill/bundle/` is regenerated via
  `go generate ./neo4j-cli/internal/skill/...` so `SKILL.md` and
  `references/<resource>.md` reflect the new flags. The diff is committed in
  the same change.

#### Changelog

- **REQ-F-050**: A single `Minor` changelog entry is added via
  `changie new --projects neo4j-cli --kind Minor` (or by hand-authoring
  `.changes/unreleased/neo4j-cli-Minor-<YYYYMMDD>-<HHMMSS>.yaml` if changie
  is unavailable). The body documents the new gate AND calls out the
  back-compat break for `docker delete`, `desktop dbms delete`, and
  `desktop connection delete` non-TTY callers.

### Non-Functional Requirements

- **REQ-NF-001**: All edits must pass `make fmt-check`, `make lint`,
  `make test`, and `make license-check` before merge — matches the standing
  AGENTS.md gate.
- **REQ-NF-002**: The shared package and every leaf change carry the Neo4j
  copyright header per the repo's `addlicense` policy.
- **REQ-NF-003**: `common/confirm/` MUST NOT import any
  `neo4j-cli/internal/*` package (Go internal-package rule — same constraint
  that put `configmigrate` under `common/`).
- **REQ-NF-004**: The non-TTY usage error must be deterministic and stable
  enough to be matched by `require.Contains(t, err.Error(), "...")` in leaf
  tests without coupling to a long copy paragraph.
- **REQ-NF-005**: TTY detection must remain testable — leaf tests override
  `confirm.stdinIsTerminal` (via an exported test seam such as
  `confirm.SetStdinIsTerminalForTest(t, fn)` mirroring the existing
  `SetDeleteStdinIsTTYFnForTest` pattern in `desktop/dbms/delete.go`).
- **REQ-NF-006**: Cross-platform behaviour: the helper must not regress on
  Windows CI — `term.IsTerminal` is already in use elsewhere in the repo and
  works on all three matrix OSes.

## Technical Considerations

**Package placement.** `common/confirm/` mirrors `common/clicfg`,
`common/clierr`, `common/configmigrate`, and `common/skill`. Putting the
helper under `common/` is mandatory: leaves 1–9 and 15 sit under
`neo4j-cli/aura/internal/subcommands/...` and leaves 10–14 sit under
`neo4j-cli/internal/subcommands/...`; without `common/` placement, the
aura subtree cannot import the helper from the `internal/` tree.

**Resource-type derivation.** Using `cmd.Parent().Name()` keeps copy in sync
with the cobra tree automatically. Two leaves (`deployment delete` and
`deployment token delete`) have no positional argument — they pass either
`""` or a flag-resolved deployment ID into `Require`. REQ-F-005 covers the
empty-string path.

**Cobra flag-binding ordering.** `confirm.Register(cmd)` returns a `*Flags`
that must be closed over by `RunE`. The constructor pattern in the leaf
example below resolves the chicken-and-egg by declaring an empty `*Flags`
first, then overwriting it from `Register` before returning the command —
identical pattern to existing constructor flag bindings in the repo.

**Test seam for TTY detection.** Existing `desktop/dbms/delete.go` uses a
package-local `var deleteStdinIsTTYFn` and exports
`SetDeleteStdinIsTTYFnForTest(t, fn)` to swap it. The shared `confirm`
package will follow the same shape: package-local `stdinIsTerminal` var,
exported `SetStdinIsTerminalForTest(t, fn)` helper that does
`t.Cleanup(...)` restoration so tests cannot leak the override.

**Stdin reader.** Reuse the bufio-line-read pattern from
`docker/delete.go:promptForDelete` for the y/N reader. EOF or read error
with no bytes is treated as cancellation (matches existing docker
behaviour, prevents a `stdin closed mid-prompt` looking like a delete
failure).

**Skill bundle regeneration.** Flag additions on commands under
`neo4j-cli/internal/subcommands/credential/...` and the Aura subtree mutate
generated `references/*.md`. The bundle generator is at
`neo4j-cli/internal/skill/gen/main.go` and is wired into
`go generate ./neo4j-cli/internal/skill/...`. The `TestGenerator_RoundTrip`
test in `make test` will fail if regeneration is skipped — that is the
intended safety net.

**Aura prompt is new behaviour.** Leaves 1–9 currently delete silently when
`--rw` is set. Adding a TTY prompt is a deliberate UX improvement and is
covered by the changelog wording in REQ-F-050.

**Breaking change scope.** Three leaves change non-TTY behaviour:
- `docker delete <name> --force` (no `--yes`) — now exits 2 in non-TTY.
- `desktop dbms delete <id> --yes` (no `--force`) — now exits 2 in non-TTY.
- `desktop connection delete <id> --yes` (no `--force`) — now exits 2 in non-TTY.

TTY callers of these three commands are unaffected when they pass either
flag (they get the prompt instead of an immediate delete) — slightly
extra friction for a human, but acceptable and consistent.

**Async commands.** Leaves 1, 5, 6, 7, 9 are async (HTTP 202). The
confirm gate runs BEFORE the request is dispatched, so async ledger / poll
behaviour is unaffected. Existing `--wait` flag handling is preserved.

## Acceptance Criteria

- [ ] `common/confirm/` package exists with `Flags`, `Register`,
  `(*Flags).Require`, `ErrCancelled`, `stdinIsTerminal`,
  `SetStdinIsTerminalForTest`.
- [ ] `go test ./common/confirm/...` covers all four (TTY × flag-state)
  combinations: TTY+both, TTY+missing (y answer), TTY+missing (N answer),
  non-TTY+both, non-TTY+missing.
- [ ] All 15 leaves listed in REQ-F-010 register confirm flags and call
  `Require` at the top of `RunE` after pre-flight validation but before any
  destructive operation.
- [ ] Each of the 15 leaves has a new test case covering: non-TTY without
  flags → exit 2 with usage error containing "pass both --yes and --force";
  non-TTY with both flags → proceeds to mocked API/FS layer; TTY with `y\n`
  → proceeds; TTY with empty `\n` → no API call, exit 0, `cancelled.` on
  stderr.
- [ ] Leaves 10–12 no longer have inline flag/prompt code (`force`,
  `yes`, `promptForDelete`, `stdinIsTerminal` vars) — all delegate to
  `common/confirm`.
- [ ] Each leaf's `Long:` and `Example:` fields are updated per REQ-F-030
  and REQ-F-031.
- [ ] `go generate ./neo4j-cli/internal/skill/...` produces no diff after
  the leaf edits (i.e. the regenerated bundle has been committed); CI's
  `TestGenerator_RoundTrip` passes.
- [ ] A new `.changes/unreleased/neo4j-cli-Minor-*.yaml` entry exists and
  documents both the new gate and the back-compat break on the three
  desktop/docker leaves.
- [ ] `make fmt-check`, `make lint`, `make test`, and `make license-check`
  all pass.
- [ ] Manual smoke per audit §12 #6 and #7:
  - `timeout 10 ./bin/neo4j-cli aura instance delete x --rw < /dev/null`
    → exit 2 (NOT 124, NOT 130).
  - `timeout 10 ./bin/neo4j-cli aura instance delete x --rw --yes < /dev/null`
    → exit 2 with a message that demands `--force`.
  - `./bin/neo4j-cli docker delete dev --yes --force --rw < /dev/null`
    → proceeds without prompting.

## Out of Scope

- `--confirm <resource-name>` reconfirmation for production-tier resources
  (audit §6.3).
- `--no-input` strict flag (third leg of the audit §6.2 triad).
- `--dry-run` for any destructive command.
- Restructuring or renaming destructive verbs (`remove` → `delete`, etc.).
- Touching non-destructive commands or read-only leaves.
- Migrating `docker delete --force` / `desktop * delete --yes` callers via a
  deprecation-warning period (the break ships in this PRD).
- MCP-server-side gate (not part of this repo's surface today).

## Open Questions

None.
