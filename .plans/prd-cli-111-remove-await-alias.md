# PRD: CLI-111 — Remove deprecated `--await` alias

## Overview

CLI-87 renamed `--await` → `--wait` across all async commands and kept `--await` as a hidden, cobra-`MarkDeprecated` alias for one release. CLI-111 removes the alias entirely so the next release ships only `--wait`. After this change, `--await` returns the standard cobra `Error: unknown flag: --await`.

Source of truth: <https://linear.app/neo4j/issue/CLI-111/remove-deprecated-await-alias-follow-up-to-cli-87>. The Linear issue enumerates the canonical aura targets; an additional grep turned up docker subtree references and the actual common-package implementation that the issue did not name.

## Goals

- Remove every code path that registers, documents, or tests `--await`.
- Keep `--wait` working unchanged on every async leaf (aura + docker).
- Land a user-facing `Minor` changelog entry so anyone with `--await` in scripts knows why their upgrade broke.
- Regenerate the `neo4j-cli` skill bundle so agent-facing reference docs no longer demonstrate the removed alias.

## Non-Goals

- No changes to `--wait` semantics, polling behaviour, or async leaf logic.
- No new flags, no rename of any other flag.
- No backport to the previously released line — the deprecation cycle already occurred.
- No edits to the aura skill bundle (already clean per grep; the standalone aura binary no longer ships a generated bundle per AGENTS.md).

## Requirements

### Functional Requirements

- **REQ-F-001**: `common/flags/wait.go` no longer registers `--await`. Specifically: the `AwaitFlagAlias` const is removed, the second `BoolVar(wait, AwaitFlagAlias, …)` line is removed, the `MarkDeprecated` call and its `panic`-on-error block are removed. The package keeps `WaitFlag = "wait"` and a `RegisterWait(cmd, *bool, helpText)` body that only registers `--wait`.
- **REQ-F-002**: `neo4j-cli/aura/internal/flags/wait.go` no longer re-exports `AwaitFlagAlias`. The doc comment is updated to drop the alias narrative. The thin delegator to `commonflags.RegisterWait` stays.
- **REQ-F-003**: `neo4j-cli/aura/internal/flags/wait_test.go` no longer asserts alias behaviour. Specifically: the `--await` alone and `--wait --await` table rows are removed, along with the `deprecationMsg` constant, the `wantDeprecated` field, and the stderr-capture machinery used solely to assert the deprecation warning. `TestRegisterWait_HidesAliasFromHelp` retains the positive `--wait` assertion AND the negative `--await` assertion (the latter is cheap regression insurance — if someone reintroduces the alias, this fires).
- **REQ-F-004**: `neo4j-cli/internal/subcommands/docker/start.go` and `neo4j-cli/internal/subcommands/docker/stop.go` drop the trailing two Example lines that demonstrate the `--await` alias.
- **REQ-F-005**: `neo4j-cli/internal/subcommands/docker/create_test.go` deletes `TestCreate_Wait_AwaitAlias_StillWorks` (currently ~lines 826-860).
- **REQ-F-006**: Each docker leaf affected by REQ-F-004 keeps ≥3 `Example:` invocations to satisfy `TestAllLeafCommands_HaveExamples` (the gate in `agentcontext_test.go`). `docker start` currently has 4 invocations and drops to 3 cleanly. `docker stop` currently has 3 and would drop to 2 — add a third invocation (e.g. another `--wait --rw` form against a differently-named container) so the gate still passes.
- **REQ-F-007**: `CONTRIBUTING.md` §3.5 (the async-operations paragraph, currently line 211) drops the sentence "`--await` is accepted as a deprecated alias for one release and will be removed in the following release."
- **REQ-F-008**: Run `go generate ./neo4j-cli/internal/skill/...` and commit the bundle diff in the same commit as the source change. The expected diff is the deletion of the two `--await` example blocks from `neo4j-cli/internal/skill/bundle/references/docker.md`.
- **REQ-F-009**: Add a `neo4j-cli` changelog entry via `changie new --projects neo4j-cli --kind Minor --body "Removed deprecated --await alias; --wait is now the only spelling (CLI-111)."` (or hand-author the YAML file under `.changes/unreleased/` if changie is not installed locally — naming `neo4j-cli-Minor-<YYYYMMDD>-<HHMMSS>.yaml`).

### Non-Functional Requirements

- **REQ-NF-001**: `make fmt-check && make lint && make test && make generate-check && make license-check` all pass on a clean tree (the AGENTS.md-mandated gates).
- **REQ-NF-002**: No regression in the agent-context surface — `bin/neo4j-cli agent-context` still reports `async_flag: "--wait"` (already correct since CLI-87; no source change needed in `agentcontext/build.go`, but the value must remain correct after this branch).
- **REQ-NF-003**: No bundle drift beyond the expected docker.md deletions — `make generate-check` is the gate.

## Technical Considerations

- **Single implementation point**: `commonflags.RegisterWait` is the single source of truth for both the aura subtree and the docker subtree. Removing the alias binding at that one site automatically clears it from every leaf — no per-leaf edits to flag registration needed. The leaf-side changes are docs (Example strings) and tests only.
- **Cobra "unknown flag" behaviour**: After removal, `cobra` returns `Error: unknown flag: --await` on stderr and exits non-zero. This is the documented post-removal behaviour from the Linear issue; no custom handling required.
- **Bundle regeneration**: The `neo4j-cli` skill bundle's `references/docker.md` currently contains two `--await` example blocks (lines 167-168 and 194-195). Once the source-side Example strings are stripped, `go generate` deletes those lines. The `aura` bundle has no `--await` references per grep — no action needed there.
- **Example-count gate**: `TestAllLeafCommands_HaveExamples` is a hard gate. `docker stop` Example is the only one that dips below 3 after the trim; mitigation is to add a third invocation (REQ-F-006). Pick the simplest form that keeps the existing tone (a `--wait --rw` variant against a different container name is the lowest-friction choice).
- **Changie kind**: `Minor`. Confirmed by the user. Frame: alias removal is user-visible behaviour change (scripts still passing `--await` break), so it matches CLI-87's `Minor` rename precedent rather than a `Patch` cleanup.
- **Internal-package boundary**: `common/flags` is the only common-side file touched. The aura-side delegator is a re-exporter; both files need their const dropped because the aura const is a `= commonflags.AwaitFlagAlias` reference that will not compile once the source const is gone.

## Acceptance Criteria

- [ ] `common/flags/wait.go` and `neo4j-cli/aura/internal/flags/wait.go` no longer mention `AwaitFlagAlias` or `--await`; `grep -rn "AwaitFlagAlias\|--await" common/ neo4j-cli/` returns no production-source hits (test files may still reference the literal string `"--await"` only inside the negative regression assertion in `wait_test.go`).
- [ ] `neo4j-cli/aura/internal/flags/wait_test.go` retains the `--wait`, `neither flag`, and `hides from help` cases; the two alias rows and their support code are gone.
- [ ] `neo4j-cli/internal/subcommands/docker/start.go` and `stop.go` Example fields contain no `--await` text; both leaves still satisfy `TestAllLeafCommands_HaveExamples` (≥3 invocations each).
- [ ] `neo4j-cli/internal/subcommands/docker/create_test.go` no longer contains `TestCreate_Wait_AwaitAlias_StillWorks`.
- [ ] `CONTRIBUTING.md` §3.5 paragraph mentions `--wait` only.
- [ ] `neo4j-cli/internal/skill/bundle/references/docker.md` contains no `--await` text.
- [ ] A new `.changes/unreleased/neo4j-cli-Minor-*.yaml` file exists with the CLI-111 removal body.
- [ ] `make fmt-check && make lint && make test && make generate-check && make license-check` all green.
- [ ] `bin/neo4j-cli aura instance create --await ...` exits with cobra's standard `Error: unknown flag: --await`.
- [ ] `bin/neo4j-cli aura instance create --wait ...` reaches the API call (the eventual API failure for missing credentials in a dev env is fine — we're verifying flag acceptance, not the API).
- [ ] `bin/neo4j-cli docker start dev --await --rw` and `bin/neo4j-cli docker stop dev --await --rw` both exit with cobra's `Error: unknown flag: --await`.
- [ ] `bin/neo4j-cli aura instance create --help` shows `--wait` and does not show `--await`.

## Out of Scope

- Any `--wait` semantics change (timeouts, polling cadence, exit codes).
- Any aura skill bundle change (already clean).
- Any change to `neo4j-cli/internal/subcommands/agentcontext/build.go` — `asyncFlag = "--wait"` is already correct.
- Removal of historical `--await` mentions from `CHANGELOG.md`, `CHANGELOG-aura.md`, `.changes/` archives, or `.plans/archive/` (history is preserved deliberately).
- Backport / patch release on the previous line.

## Open Questions

None.
