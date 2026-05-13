# PRD: Rename `--file` to `--env` on credential add commands

Linear: [CLI-101](https://linear.app/neo4j/issue/CLI-101/rename-file-to-env-flag-for-neo4j-cli-credential-dbmsaura-client-add)
Source plan: `/Users/oskarhane/.claude/plans/look-into-https-linear-app-neo4j-issue-c-dynamic-wozniak.md`
Mirrors / unblocks: aligns naming with `neo4j-cli query --env` (`neo4j-cli/query/query.go:54`). Touches the same three sources updated by CLI-75 (`credential dbms add --file`) and CLI-100 (`credential aura-client add --file`).

## Overview

`neo4j-cli credential dbms add`, `neo4j-cli credential aura-client add`, and the standalone-aura mirror `neo4j-cli aura credential add` each expose a `--file <path>` flag to import an Aura-console–exported `.env`-style credentials file. `neo4j-cli query` uses `--env <path>` for the analogous purpose. CLI-101 renames the credential-side flag from `--file` to `--env` so the CLI surface speaks the same word for the same thing.

This is a hard rename: the old `--file` flag is removed, with no hidden alias and no deprecation warning. The CLI is in `v0.1.0-alpha.*` and the aura-client `--file` flag shipped in commit `e153ca8` (CLI-100, PR #105) two days before this change, so adoption surface is negligible. Behaviour is identical to today's `--file`: explicit path required, no auto-discovery walk-up. Only the flag name (and the user-visible wording around it: Long, Example, flag description, error messages) changes.

## Goals

- Rename `--file` → `--env` on `credential dbms add`, `credential aura-client add`, and `aura credential add`.
- Keep the change strictly cosmetic at the call-graph level: the same `envfile.Parse` codepath, the same merge/validation logic, the same exit codes.
- Update both skill bundles (`neo4j-cli/internal/skill/bundle/references/credential.md` and `neo4j-cli/aura/internal/skill/bundle/references/credential.md`) via `go generate` so agent-facing docs match the new flag.
- Land a single Minor changie entry under the `neo4j-cli` project key.

## Non-Goals

- **No `--file` alias.** Hard rename — passing `--file` after this change must yield cobra's `unknown flag: --file` error.
- **No deprecation warning / hidden alias.** Same reason as above; alpha + 2 days of adoption.
- **No auto-discovery / `.env` walk-up.** `query --env` walks up from cwd when unset; credential-add `--env` does NOT. Credential add is a one-off import (typically of a download from the Aura console), not a per-invocation workflow flag, and silently reading a stray `.env` during a write op would be surprising. Path remains explicit; if `--env` is omitted, every required field must be supplied via its own flag, exactly as today.
- **No shorthand** (`-e`, `-f`). `query --env` has no shorthand; matching that.
- **No behaviour change to `envfile.Parse`, the merge ordering, the empty-value error, or the required-field error semantics.** The error message strings change (`--file` → `--env`) but the structure and exit code stay identical.
- **No README / `additions.md` / `description.txt` edits.** A grep across these surfaces confirmed none of them currently mention `--file`; nothing to update.
- **No edits to other credential subtrees.** `credential embed add` does not have a `--file` flag and is out of scope.
- **No edits to `neo4j-cli query --env`.** It already uses `--env`; that's the target convention.

## Requirements

### Functional Requirements

- **REQ-F-001:** In `neo4j-cli/internal/subcommands/credential/dbms/add.go`:
  - Rename the const `fileFlag = "file"` → `envFlag = "env"` (current line `add.go:31`).
  - Rename the local var `filePath` → `envPath` (declaration at `add.go:21`; usages at lines `64`, `125`).
  - Update `cmd.Long` to refer to `` `--env <path>` `` (current line `add.go:46`).
  - Update `cmd.Example` to use `--env` (current line `add.go:48`).
  - Update the inline comment at `add.go:57` to refer to `--env`.
  - Update the empty-value error at `add.go:125`: `clierr.NewUsageError("--env %q: %s has an empty value", envPath, c.envKey)`.
  - Update the required-field error template at `add.go:141`: `clierr.NewUsageError("--%s is required (provide via --env as %s, or pass --%s)", req.flag, req.envKey, req.flag)`.
  - Update the flag registration at `add.go:166`: `cmd.Flags().StringVar(&envPath, envFlag, "", "Path to a Neo4j Aura–exported credentials file. Recognised keys: NEO4J_URI, NEO4J_USERNAME, NEO4J_PASSWORD, NEO4J_DATABASE, AURA_INSTANCENAME. Explicit flags override file values.")`.

- **REQ-F-002:** In `neo4j-cli/internal/subcommands/credential/credential.go` (inline `aura-client add` leaf, `newCredentialAddCmd`, lines 50–154):
  - Rename const `fileFlag = "file"` → `envFlag = "env"` (current line `credential.go:62`).
  - Rename local var `filePath` → `envPath` (declaration `credential.go:55`; usages `94`, `126`).
  - Update Long (line `75`), Example (line `81`), inline comment (line `87`).
  - Update empty-value error (line `126`) and required-field error (line `140`) to the `--env` wording per REQ-F-001.
  - Update flag registration (line `151`).

- **REQ-F-003:** In `neo4j-cli/aura/internal/subcommands/credential/add.go` (standalone `aura credential add`, mirror per AGENTS.md):
  - Same shape of edits as REQ-F-002 at lines `25`, `37`, `43`, `48`, `90`, `105`, `116`.

- **REQ-F-004:** Behaviour parity — running with the new flag must yield byte-identical results (modulo the `--env` substring in error strings) to running with the old flag today. Specifically:
  - Empty `--env` value (i.e. flag absent) ⇒ no file is read; fields come from explicit flags only.
  - `--env <missing-path>` ⇒ wrapped open error (existing `envfile.Parse` behaviour).
  - File-supplied keys merge under existing precedence (explicit flag > file > default).
  - Empty value in a recognised file key with no flag override ⇒ `clierr.NewUsageError("--env %q: <KEY> has an empty value", envPath, key)`.

- **REQ-F-005:** Cobra must reject `--file` after the change. Passing `--file <path>` to any of the three commands must produce cobra's standard `unknown flag: --file` error. (This is automatic once the flag is removed; called out so reviewers and tests verify it.)

- **REQ-F-006:** Both skill bundles refresh under `go generate`:
  - `neo4j-cli/internal/skill/bundle/references/credential.md` — `--file` mentions at current lines `40`, `50`, `60`, `141`, `151`, `161` replaced with `--env`.
  - `neo4j-cli/aura/internal/skill/bundle/references/credential.md` — `--file` mentions at current lines `11`, `21`, `31` replaced with `--env`.
  - These are regenerated, not hand-edited; the change is whatever `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...` produces from the source edits above.

- **REQ-F-007:** Tests updated to expect `--env`:
  - `neo4j-cli/internal/subcommands/credential/dbms/add_test.go` — `--file` literals at lines `32`, `169`, `175`, `181`, `187`, `269`, `272`, `281`, `290`, `299`, `317`, `326`, `327`, `334`, `342`, `351`, `356`, `359`, `360`, `364`, `370`, `376`, `380` rewritten to `--env`. The `wantErr` strings updated to the `provide via --env as …` form.
  - `neo4j-cli/aura/internal/subcommands/credential/add_test.go` — same treatment at lines `50`, `60`, `65`, `70`, `117`, `119`, `126`, `133`, `147`, `154`, `155`, `161`, `162`, `168`, `175`, `180`.
  - Test case `name:` strings that read "happy path: --file alone populates…" and "missing --file path returns a wrapped open error" updated to `--env`.

- **REQ-F-008:** Changie entry — one new file under `.changes/unreleased/` named `neo4j-cli-Minor-<YYYYMMDD>-<HHMMSS>.yaml`, fields:
  ```yaml
  project: neo4j-cli
  kind: Minor
  body: 'Renamed --file to --env on ''credential dbms add'' and ''credential aura-client add'' for consistency with ''query --env'' (CLI-101).'
  time: <RFC3339 timestamp>
  ```
  Single entry covers all three command surfaces (one binary ships).

### Non-Functional Requirements

- **REQ-NF-001:** `make fmt-check` passes (no gofmt diff).
- **REQ-NF-002:** `make lint` passes (golangci-lint v2 clean).
- **REQ-NF-003:** `make test` passes on the full matrix locally; `TestGenerator_RoundTrip` in both skill packages must NOT report bundle drift (i.e. the regen committed in the same change matches the source).
- **REQ-NF-004:** `TestAllLeafCommands_HaveExamples` in `neo4j-cli/internal/subcommands/agentcontext/agentcontext_test.go` continues to pass — the rewritten `Example:` strings must keep flush-left indent, ≥3 invocations, `# comment` per invocation, blank-line separators, `neo4j-cli` prefix, `--rw` on write invocations.
- **REQ-NF-005:** `make license-check` is unaffected (no new files added beyond the changie entry, which is YAML and exempt).

## Technical Considerations

- **Layout.** Source change touches three files, two of which contain the same logic (aura-client add) per the AGENTS.md "lives in TWO places" note. The third is the dbms variant in its own leaf file (correct cobra layout per AGENTS.md "one-file-per-leaf"). All three import `common/clicfg/envfile` (introduced in CLI-100); no edits to that package — only the flag name binding changes.
- **Wording asymmetry.** The flag's `Usage:` string keeps the noun "credentials file" (it describes what the path points at). Renaming to `--env` shifts the flag name to match `query --env` semantics ("path to a dotenv-style file"), without changing the description verbiage. This keeps the help text informative — the agent or human still sees "Aura-exported credentials file" — while the flag name is consistent.
- **Error string semantics.** The error templates currently embed `--file` literally; after this change they embed `--env`. Tests assert these strings (table-driven `wantErr` rows), so they must be updated together. Downstream callers don't parse these strings — they're `clierr.UsageError`s consumed by humans.
- **Skill-bundle round-trip.** Both bundles regenerate; `TestGenerator_RoundTrip` is the gate. AGENTS.md flags this explicitly: any flag-text change on commands reachable from both surfaces requires running `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`.
- **No `--file` alias.** Cobra supports `cmd.Flags().SetNormalizeFunc` for alias normalization, but the user explicitly chose hard rename. Don't add a deprecated alias.
- **Changie kind = Minor.** Flag rename on alpha is visible behaviour, not internal. Major is reserved for the post-stable era; this codebase has shipped multiple flag-shape Minor entries in `v0.1.0-alpha.*` (see `--output` → `--format` rename in `v0.1.0-alpha.2`).
- **Linear branch name.** `cli-101-rename-file-to-env-flag-for-neo4j-cli-credential-dbmsaura`. Prefer the user's `oskar/` prefix per global instructions: e.g. `oskar/cli-101-rename-file-to-env`.

## Acceptance Criteria

- [ ] `--env` works identically to today's `--file` on all three commands; `--file` produces cobra's `unknown flag: --file`.
- [ ] All three source files updated symmetrically (const, var, Long, Example, comments, error messages, flag registration).
- [ ] `go generate ./...` produces a clean tree; both `references/credential.md` files reflect `--env` in the Long body, the flags table, and the Example block.
- [ ] `make fmt-check && make lint && make test` all pass.
- [ ] `TestGenerator_RoundTrip` (both skill packages) and `TestAllLeafCommands_HaveExamples` pass.
- [ ] Both `add_test.go` files have their `--file` literals and `wantErr` strings updated; test cases for `--env`-mode happy path, `--env` + flag override, `--env` + missing path wrapped error, etc. all green.
- [ ] One `neo4j-cli-Minor-*.yaml` added under `.changes/unreleased/` with the body from REQ-F-008.
- [ ] Manual smoke against built binary: `./bin/neo4j-cli credential dbms add --help` and `./bin/neo4j-cli credential aura-client add --help` show `--env` (not `--file`); a sample Aura-exported file imports correctly via `--env <path>`.
- [ ] PR title prefixed `feat(cli):` (or `refactor(cli):` — repository commit style uses both) and references `CLI-101` for Linear auto-link.

## Out of Scope

See **Non-Goals** above. In short: no alias, no deprecation, no auto-discovery, no shorthand, no embed-credential flag changes, no behavioural changes beyond the flag name + error wording.

## Open Questions

- None. Both design decisions (hard rename, rename-only — no auto-discovery) were confirmed in plan-mode Q&A before this PRD was written.
