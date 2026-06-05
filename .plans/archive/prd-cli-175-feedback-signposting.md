# PRD: Clearer CLI feedback & issue signposting (CLI-175)

## Overview

Make it easier for `neo4j-cli` users to find where to report bugs / give feedback, and
make sure the bug-report URL we already show is correct. Three small, independent
changes: (1) centralize and fix the bug-report URL used in error messages, (2) add a
feedback/issues signpost to the installer output, (3) a bounded sweep adding actionable
`.WithSuggestion()` hints to a few high-traffic error paths.

Source of truth: `/Users/oskarhane/.claude/plans/time-to-take-on-quizzical-fountain.md`.
Scope, decisions, and out-of-scope are already settled there and reflected below.

## Goals

- Users always see a correct, single canonical bug-report URL
  (`https://github.com/neo4j-labs/neo4j-cli/issues`) — never the bare Go module path
  `github.com/neo4j/cli`.
- A one-line feedback/issues signpost appears at the end of a successful install (sh + ps1).
- A few common, currently-unactionable error paths gain a one-line next-step suggestion,
  reusing the existing `clierr.WithSuggestion()` infrastructure.

## Non-Goals

- No `neo4j-cli feedback <text>` command (explicitly skipped by the user).
- No broad rewrite of every error site lacking a suggestion — only the 3 named targets.
- No changes to `gh-pages` install scripts (`neo4j.sh` curl-target) — tracked as a
  separate ticket; that branch is prompt-driven via `.github/prompts/website-update.md`.

## Requirements

### Functional Requirements

- REQ-F-001: Add an exported constant `IssuesURL = "https://github.com/neo4j-labs/neo4j-cli/issues"`
  to `common/clierr/error.go`, with a comment noting `github.com/neo4j/cli` is only the
  Go module path and the real repo is `neo4j-labs/neo4j-cli`.
- REQ-F-002: Replace every hardcoded "report an issue in https://github.com/neo4j…"
  literal with `clierr.IssuesURL`, passed as a `%s` format arg (not concatenated into
  the format string). Sites:
  - `neo4j-cli/aura/internal/api/response.go` lines 80, 87, 107, 118, 132, 145, 162, 172,
    181, 191, 473, 483.
  - `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/utils.go:43`.
  - `neo4j-cli/main.go:33`.
- REQ-F-003: After this change, no user-facing "report an issue" message references the
  bare `github.com/neo4j/cli` URL.
- REQ-F-004: Leave `repoSlug` in `neo4j-cli/internal/subcommands/update/release.go`
  unchanged — it is the GitHub API slug for release downloads (already correct), a
  separate concern from the issue tracker.
- REQ-F-005: Add a feedback/issues signpost line to the post-install output of
  `distribution/installation-scripts/install-neo4j-cli.sh`, after the PATH check, using
  the existing `info()` helper:
  `info "Questions or found a bug? https://github.com/neo4j-labs/neo4j-cli/issues"`
  (preceded by a blank `echo ""`).
- REQ-F-006: Add the equivalent signpost to
  `distribution/installation-scripts/install-neo4j-cli.ps1`, after the final
  `Write-Ok "Done!…"`, using `Write-Step` (preceded by `Write-Host ""`).
- REQ-F-007: Add a bats test in
  `distribution/installation-scripts/tests/install-neo4j-cli.bats` asserting the install
  output contains `neo4j-labs/neo4j-cli/issues` (reuse existing full-run stub setup).
- REQ-F-008: Bolt auth-failure suggestion — in `neo4j-cli/query/errors.go`
  `categorizeBoltError`, detect `Neo.ClientError.Security.Unauthorized` and return a
  validation error with a one-line suggestion pointing at `neo4j-cli credential dbms add`
  / checking `--credential`. Preserve the error chain via `%w`.
- REQ-F-009: Bolt connection-refused / `ServiceUnavailable` suggestion — extend the
  `upstreamFrom` path (or a dedicated branch) so transport/unavailable failures carry a
  one-line suggestion to verify the URI / that Neo4j is running (e.g.
  `neo4j-cli docker list` / `docker start`).
- REQ-F-010: Embed missing-API-key suggestion — the `NewAuthError` sites in
  `neo4j-cli/query/embed/{openai,gemini,huggingface,vertex}.go` gain a one-line
  suggestion pointing at `neo4j-cli credential embed add`.

### Non-Functional Requirements

- REQ-NF-001: All suggestions are a single imperative line naming the exact `neo4j-cli …`
  command, consistent with existing patterns in `query/errors.go` and
  `aura/internal/subcommands/utils/resolve.go`.
- REQ-NF-002: No `go generate` / skill-bundle regeneration required (no cobra help/Long
  or command-tree changes; suggestions are runtime strings). `make test`'s generate gate
  is the backstop.
- REQ-NF-003: Shell-script URL stays a literal (not Go), kept byte-identical to the Go
  constant value.
- REQ-NF-004: A user-facing changelog entry is added via changie, kind `Minor`.

## Technical Considerations

- The constant must live under `common/` (specifically `common/clierr`, already imported
  at every error site). `common/*` cannot import `neo4j-cli/internal/*`, and the wrong-URL
  sites live in `neo4j-cli/aura/internal/api`, so `clierr` is the correct shared home.
- Error messages are `fmt`-style format strings; append `%s` and pass the constant as an
  argument to satisfy `go vet` / lint (don't build a dynamic format string).
- `categorizeBoltError` already short-circuits on already-typed `*clierr.CLIError` and has
  a wrong-port branch — the new auth/unavailable branches slot in alongside, before the
  generic `validationFrom`/`upstreamFrom` fallbacks. Tests inject plain
  `errors.New("Neo.ClientError…")`, so detect via both `errors.As(*neo4j.Neo4jError)` and
  string-prefix match, matching the existing two-pass style.
- PowerShell `.ps1` files require CRLF line endings (see AGENTS.md) — preserve them when
  editing.
- bats tests stub all external commands via `STUBS_DIR`; the new assertion reuses the
  existing full-run harness rather than adding new stubs.

## Acceptance Criteria

- [ ] `clierr.IssuesURL` exists and is the single source for the bug-report URL.
- [ ] `grep -rn "github.com/neo4j/cli\"" --include=*.go neo4j-cli common` finds no
      user-facing "report an issue" literal (only legitimate module import paths remain).
- [ ] `install-neo4j-cli.sh` and `.ps1` print the issues signpost on successful install.
- [ ] New bats test passes and asserts the issues URL is printed.
- [ ] Bolt auth-failure, Bolt connection-refused, and embed missing-API-key errors carry
      a `.Suggestion`, verified by colocated table-driven tests.
- [ ] `make test`, `make fmt-check`, `make lint` all pass; bats suite passes.
- [ ] Changelog entry added (changie, kind `Minor`).

## Out of Scope

- `neo4j-cli feedback <text>` command.
- Broad error-suggestion rewrite beyond the 3 named targets (no config/credential-cmd sweep).
- `gh-pages` install scripts / `neo4j.sh` — separate ticket.

## Open Questions

None — all clarifying questions were resolved during planning:
1. Install link wording approved as-is.
2. gh-pages handled as a separate ticket.
3. Sweep limited to the 3 named targets.
