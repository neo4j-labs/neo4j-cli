# PRD: Fix broken Aura auth link in default-credential error (CLI-80)

## Overview

When a user runs an `aura ...` subcommand without a stored default Aura Console API credential, `AuraCredentials.GetDefault()` returns a `UsageError` whose body points to a broken docs URL and references a subcommand path that no longer matches the shipped `neo4j-cli` binary. Replace the URL and subcommand wording in that single error string so the message is actionable: lead with the Aura Console page where credentials are minted, link the working docs page as a secondary reference, and use the canonical `credential aura-client add` subcommand.

Linear: https://linear.app/neo4j/issue/CLI-80/fix-broken-aura-auth-link-in-cli-error

Source line: `common/clicfg/credentials/aura.go:77` (the only emission site; no test asserts the verbatim text).

Reported failure mode (verbatim):

```
$ neo4j-cli aura instance list --format table
Error: default credential not set, please follow the instructions at https://neo4j.com/docs/aura/classic/platform/api/authentication/#_creating_credentials and use the `credential add` subcommand to add the created credentials
```

The URL 404s; `credential add` is the standalone-aura binary's path, which is no longer built/shipped (see `AGENTS.md` §"Cobra Help / Skill Bundle Rendering Notes" line 237). Shipped `neo4j-cli` uses `credential aura-client add`.

## Goals

- The "default credential not set" error names a working URL the user can click to mint Aura Console API credentials.
- The error references the canonical, shipped subcommand path (`credential aura-client add`).
- Docs URL replacement points at the documented Aura API auth page (`https://neo4j.com/docs/aura/api/authentication/`), not the removed classic/anchor page.
- Behavior in tests stays green without modification (no test asserts the body text).

## Non-Goals

- Changing the error type, exit code, or `clierr.UsageError` envelope.
- Adding a new code path that auto-suggests `credential aura-client add` via cobra hints.
- Localizing or templating the error message.
- Reworking the standalone-aura binary's wording (binary is no longer shipped; only `neo4j-cli` is in scope).
- Updating the Aura Console URL elsewhere (README, skill bundle, install scripts) — out of scope unless the same broken URL is found there during implementation. `grep` confirms only the single occurrence in `aura.go:77`.
- Bumping any dependency.

## Requirements

### Functional Requirements

- REQ-F-001: `AuraCredentials.GetDefault()` continues to return a non-nil `clierr.UsageError` when `c.DefaultCredential == ""`.
- REQ-F-002: The returned error message contains the substring `https://console.neo4j.io/account` (primary, minting location).
- REQ-F-003: The returned error message contains the substring `https://neo4j.com/docs/aura/api/authentication/` (docs reference).
- REQ-F-004: The returned error message references the subcommand `credential aura-client add` (the path under shipped `neo4j-cli`).
- REQ-F-005: The returned error message does NOT contain the old broken URL `https://neo4j.com/docs/aura/classic/platform/api/authentication/#_creating_credentials`.
- REQ-F-006: The returned error message does NOT reference the bare `credential add` subcommand (avoid wording that mismatches the shipped CLI).
- REQ-F-007: A user-facing `Patch` changelog entry is recorded via `changie new --projects neo4j-cli --kind Patch --body "..."` mentioning CLI-80 and the fix.

### Non-Functional Requirements

- REQ-NF-001: Single-file change scoped to `common/clicfg/credentials/aura.go`. No other source files modified.
- REQ-NF-002: No public-API change: `(c *AuraCredentials) GetDefault() (*AuraCredential, error)` signature is unchanged.
- REQ-NF-003: All three local gates pass: `make fmt-check`, `make lint`, `make test`.
- REQ-NF-004: `make generate-check` stays green — the error string is not embedded in any skill-bundle artifact, so no `go generate` step is required (verified: `grep "default credential not set" -r .` finds only the production line, no fixture or bundle hit).
- REQ-NF-005: No new external dependencies.
- REQ-NF-006: No new tests strictly required, but a small regression test is added asserting the four substring invariants (REQ-F-002 through REQ-F-005) so future drift fails locally rather than in user-facing copy.

## Technical Considerations

### Files touched

- `common/clicfg/credentials/aura.go` — replace the single `clierr.NewUsageError(...)` argument inside `(c *AuraCredentials) GetDefault()` at line 77.

### Proposed wording

```
default credential not set, create Aura API credentials at https://console.neo4j.io/account (see https://neo4j.com/docs/aura/api/authentication/) and run `credential aura-client add` to store them
```

Rationale:

- Console URL listed first — it's the actionable step (where the user clicks to mint a client-id/secret).
- Docs URL in parenthetical — preserves the docs-link discoverability the original message intended.
- `credential aura-client add` matches the shipped command (`neo4j-cli/internal/subcommands/credential/credential.go:33,63`).
- Backticks on the subcommand are kept (matches the prior message's typographic style and is rendered as code in most terminals' history search).

### Test

Add a unit test in `common/clicfg/credentials/aura_test.go` (file exists; co-located with `aura.go`) named `TestAuraCredentials_GetDefault_NoDefaultErrorBody` that:

1. Builds an `AuraCredentials` with `DefaultCredential: ""`.
2. Calls `GetDefault()`.
3. Asserts the returned error is non-nil.
4. Asserts `err.Error()` contains each of: `https://console.neo4j.io/account`, `https://neo4j.com/docs/aura/api/authentication/`, `credential aura-client add`.
5. Asserts `err.Error()` does NOT contain: the old broken URL, the bare token `credential add` (use `strings.Contains` with the precise sub-string ` credential add ` or assert by absence of the legacy URL — see Open Questions).

Test exists purely to lock the invariants in REQ-F-002 through REQ-F-006 so a future careless string edit fails locally.

### Why not parametrize the URL via a const

The original code inlines the URL/message. A const buys nothing here (single call site, no duplication, no test fixtures to share). Inlining keeps the change a one-liner and matches the surrounding pattern in `aura.go`. Skipping this avoids scope creep per AGENTS.md "Don't add features, refactor, or introduce abstractions beyond what the task requires."

### Why not also update the standalone-aura binary's wording

AGENTS.md (line 237) confirms the standalone aura binary is no longer built/shipped. Updating the message to match `neo4j-cli`'s subcommand path is the right call. If the standalone binary is ever resurrected, that's a future-day problem.

### Changelog

```
changie new --projects neo4j-cli --kind Patch --body "Fix broken Aura docs URL and update subcommand wording in the 'default credential not set' error (CLI-80)."
```

Per AGENTS.md: this IS user-visible (error text change), so a changelog entry is required.

### Branch / commit / PR

- Branch: `oskar/cli-80-fix-broken-aura-auth-link` (per personal CLAUDE.md `oskar/` prefix).
- Commit message: `fix(cli): update broken Aura auth link in default-credential error (CLI-80)`.
- PR title must contain `CLI-80` for Linear auto-link.

## Acceptance Criteria

- [ ] `common/clicfg/credentials/aura.go:77` returns a `UsageError` whose body matches the wording in §"Proposed wording" verbatim.
- [ ] `grep -rn "neo4j.com/docs/aura/classic/platform/api/authentication" .` returns zero hits across the repo.
- [ ] A new test in `common/clicfg/credentials/aura_test.go` asserts the four substring invariants (REQ-F-002 through REQ-F-005) and passes.
- [ ] `make fmt-check` is clean.
- [ ] `make lint` is clean.
- [ ] `make test` is green.
- [ ] `make generate-check` is green (no skill-bundle drift; this change does not feed the bundle).
- [ ] A `Patch`-kind changelog entry exists under `.changes/unreleased/` referencing CLI-80.
- [ ] Manual smoke: with a fresh credentials.json (no default), `./bin/neo4j-cli aura instance list --format table` prints the new message; both URLs resolve in a browser.
- [ ] PR opened against `main` with title containing `CLI-80`; Linear auto-linked.

## Out of Scope

- Searching for other places in the codebase that might link to outdated Aura docs URLs (none found in this scope; `grep` of `neo4j.com/docs/aura` returns only the single line in `aura.go:77`).
- Reworking `clierr.NewUsageError` formatting.
- Adding a `--help`-time tip about how to create credentials.
- Updating the website (`gh-pages/`) — separate prompt-driven workflow per AGENTS.md.
- Migrating any other "credential add" references to "credential aura-client add" (only the error string is updated; standalone-aura binary tests using `credential add` stay as-is because they target the historical compiled-but-not-shipped binary).

## Open Questions

- Negative-assertion phrasing in the regression test: do we assert the absence of the legacy URL only (cleaner, lower risk of false positives), or also the absence of bare ` credential add ` token (catches accidental rewording but requires careful boundary matching since `credential aura-client add` legitimately contains `add`)? Recommend: assert absence of the legacy URL only; the positive assertion on `credential aura-client add` already prevents reverting to `credential add` since the two strings can't coexist in a single sensible sentence.
