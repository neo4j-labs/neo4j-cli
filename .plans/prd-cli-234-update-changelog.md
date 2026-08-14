# PRD: Print changelog after `neo4j-cli update`

Linear: [CLI-234](https://linear.app/neo4j/issue/CLI-234/neo4j-cli-update-should-print-the-changelog)
Branch: `cli-234-neo4j-cli-update-should-print-the-changelog`

## Overview

After a successful self-update, `neo4j-cli update` prints only `Successfully updated from vX to vY`. The user has no idea what changed. This is worst when versions are skipped — a v1.8.0 binary jumping to v1.11.0 silently gains the whole `admin` tree, `aura virtual-graph`, and a **breaking** `accept-env-vars` change, with nothing on screen to say so.

This feature prints the GitHub release notes for the releases between the old and new version, newest first, capped at the 3 newest. `--no-changelog` suppresses it. `--format json` and `--format toon` carry the same data as a `release_notes` field.

The changelog source question posed in the Linear issue is settled by a codebase finding: **the data is already fetched and thrown away.** `fetchReleases` (`neo4j-cli/internal/subcommands/update/release.go:139`) pulls the 30 latest releases in a single API call, but the `Release` struct (`release.go:72`) decodes only `tag_name`/`draft`/`prerelease`. Adding one field — `Body string \`json:"body"\`` — exposes every release's changie-generated notes at zero extra network cost. Verified present and well-formed on all 12 published tags (~100–3500 bytes each). No embedded `CHANGELOG.md` (which would only ever hold the *old* version's history), no separate repo-file fetch.

## Goals

- Show the user what changed, including across skipped versions.
- Reuse the release payload already in flight; add no new API endpoint.
- Keep scripted/agent consumers first-class via `release_notes` in structured output.
- Leave the existing plain-text narrative and exit-code contract untouched.
- Harden the package's test isolation so no test in it can reach real GitHub.

## Non-Goals

- Rendering markdown (no glamour/lipgloss dependency; notes print as raw markdown text).
- Paging long output.
- A changelog preview in `update check`.
- Embedding `CHANGELOG.md` in the binary.
- Any change to `Latest`/`GetByTag` signatures (`neo4j-cli/internal/versioncheck/versioncheck.go` binds `latestFn = update.Latest`).

## Requirements

### Functional Requirements

- **REQ-F-001**: `Release` (`release.go:72`) gains `Body string \`json:"body"\``. Decoded by the existing `fetchReleases` under its current 4 MiB `io.LimitReader` cap and `assertAllowedHostURL` host pin — no change to either guard.
- **REQ-F-002**: New `ListReleases(ctx context.Context, preReleases bool) ([]Release, error)` in `release.go`, beside `Latest`. Calls `fetchReleases` and applies the same three filters `Latest` uses: skip drafts, skip `!semver.IsValid(tag)`, skip prereleases unless `preReleases`. Returns API order (newest first).
- **REQ-F-003**: After a **successful swap only**, `update` prints the notes for releases in `(current, target]` — `semver.Compare(tag, current) > 0 && semver.Compare(tag, target) <= 0` — newest first.
- **REQ-F-004**: The range is capped at the **3 newest** entries. Entries with an empty body are skipped and do not consume a slot.
- **REQ-F-005**: When the cap elides releases, **or** when `current` predates the 30 fetched releases (stale binary), a trailing `Full changelog: https://github.com/neo4j-labs/neo4j-cli/releases` line is printed. The URL is a package const, not a literal at the print site.
- **REQ-F-006**: Each release body is trimmed at the first line that is exactly `---`, dropping the redundant `## Versions`/`## Changes` boilerplate. If trimming yields a blank remainder (pre-v1.3-era tags are boilerplate-only), the full body is used instead. A leading `## Release notes` heading is stripped — it would sit redundantly beneath our own `## <tag>` header.
- **REQ-F-007**: Plain-text layout: for each entry, `## <tag>`, blank line, then the trimmed body; blank line between entries. Printed to **stdout** via `cmd.Printf`, after the existing success line and any skill-refresh line. The progress narrative (`Current version:`, `Checking for updates...`) stays on **stderr** as today.
- **REQ-F-008**: New `--no-changelog` bool flag on `update` (kebab-case per CLI-127), description "Suppress the changelog printed after a successful update". Threaded through `runOpts` as `noChangelog`. When set, no notes are fetched, printed, or emitted in structured output.
- **REQ-F-009**: `--format json` and `--format toon` include `release_notes` (`omitempty`) as an array of `{version, notes}` objects — snake_case keys per CLI-127. `changelog_url` (also `omitempty`) accompanies it whenever REQ-F-005's elision condition holds.
- **REQ-F-010**: `--format table` **omits** `release_notes` and `changelog_url`. Multi-line 3 KB markdown in a grid cell is unreadable. This falls out of the existing architecture for free: `printToon` marshals through `MarshalJSON` (`common/output/output.go:330`) while `printTable` reads `AsArray` + `fields` (`output.go:114`). So `release_notes` goes in `printableUpdateResult.MarshalJSON` **only** — not in `AsArray`, not in `fieldOrder`. No format-sniffing branch is needed.
- **REQ-F-011**: A `listReleasesFn` failure is **non-fatal** — the swap already succeeded. Emit a brief stderr warning (matching the existing `refreshSkillBundles` convention at `update.go:569`), leave `release_notes` empty, and keep `updated: true` with exit code 0.
- **REQ-F-012**: A downgrade via `--version <older>` yields an empty range by construction; nothing is printed and no special-case branch exists.
- **REQ-F-013**: `update check` is unchanged — no changelog, no `--no-changelog` flag, byte-identical output to today.
- **REQ-F-014**: The `Example:` block on `update` gains a flush-left `--no-changelog` invocation with a `#` comment, satisfying `TestAllLeafCommands_HaveExamples`.

### Non-Functional Requirements

- **REQ-NF-001 (security)**: Every release body passes through `output.StripControl` before printing. Bodies are third-party content rendered straight into a terminal; a tampered release could otherwise inject ANSI escapes to move the cursor or spoof output. Note the toon path already gets this via `stripControlDeep` (`output.go:345`), and the JSON path via `encoding/json` escaping — the **plain-text path is the one that needs the explicit call**.
- **REQ-NF-002 (test isolation)**: `grep -c 'withLatest(t\|withGetByTag(t'` → **44 sites in `update_test.go` + 4 in `check_test.go`**. None would stub a new `listReleasesFn`. Because seams default to the production impl, a naive `listReleasesFn = ListReleases` makes all 48 issue a **real request to api.github.com** during `make test` — green locally, flaky and rate-limited in CI. Two mitigations, both required:
  1. Default `listReleasesFn` to an empty-list stub inside the shared helpers `runWithOptsFormat` (`update_test.go:107`) and `runWithOptsSplit` (`update_test.go:129`) — one edit each; all 48 sites inherit it. Tests wanting a changelog opt in via a new `withListReleases(t, fn)`.
  2. Add a `TestMain` to the package (it currently has none) pointing `httpDoFn` at a fake that returns an error and fails the test. Any future unstubbed network path then fails loudly instead of silently reaching GitHub.
- **REQ-NF-003 (output size)**: The 3-entry cap bounds plain-text output at roughly 10 KB worst case.
- **REQ-NF-004 (rate limit)**: The flow makes a second call to the same `releases?per_page=30` endpoint (once via `latestFn`, once via `listReleasesFn`). Accepted: identical URL, CDN-served, one interactive command, and unauthenticated GitHub allows 60/hour. Deliberately **not** memoizing `fetchReleases` — hidden cross-call package state is a worse bug source than one request, and reworking `resolveTarget` to derive the target from a single list-fetch would bypass `latestFn` and break all 44 stub sites.
- **REQ-NF-005**: `releaseNotesEntry` stays unexported, so `common/output/casing_gate_test.go`'s `outputStructAllowlist` needs no new entry; the rendered field names are pinned in `MarshalJSON` instead.

## Technical Considerations

**Files**

| File | Change |
|---|---|
| `neo4j-cli/internal/subcommands/update/release.go` | `Body` field on `Release`; new `ListReleases` |
| `neo4j-cli/internal/subcommands/update/changelog.go` | **new** — `releaseNotesEntry`, `changelogForRange`, `trimReleaseBody`, `printChangelog`, `changelogURL`, cap const |
| `neo4j-cli/internal/subcommands/update/update.go` | `listReleasesFn` seam; `noChangelog` in `runOpts` + flag + Example; `releaseNotes`/`releaseNotesElided` on `updateResult`; `MarshalJSON` fields; call site after swap |
| `neo4j-cli/internal/subcommands/update/changelog_test.go` | **new** |
| `neo4j-cli/internal/subcommands/update/update_test.go` | `withListReleases`, helper defaults, `TestMain`, golden + JSON updates |
| `neo4j-cli/internal/subcommands/update/release_test.go` | `TestListReleases_*` |
| `test/e2e/release_fixture/testdata/releases-{stable,pre-release}-head.json` | add `body` per entry (additive; nothing asserts its absence) |
| `neo4j-cli/internal/skill/bundle/references/update.md` | regenerated |

**Signatures**

```go
func ListReleases(ctx context.Context, preReleases bool) ([]Release, error)

type releaseNotesEntry struct{ tag, body string }

func changelogForRange(releases []Release, current, target string) (entries []releaseNotesEntry, elided bool)
func trimReleaseBody(body string) string
func printChangelog(cmd *cobra.Command, entries []releaseNotesEntry, elided bool)
```

**Call site**: `update.go` after the successful swap and after `refreshSkillBundles` (~line 490), gated on `!opts.noChangelog`. Populates `result.releaseNotes` / `result.releaseNotesElided` before `printResult`, and `printChangelog` runs inside the existing plain-text closure (~line 494) so structured formats stay silent until the single final document.

**Gates**
- `go generate ./neo4j-cli/internal/skill/...` — the new flag and edited `Example:` drift `bundle/references/update.md`; `TestGenerator_RoundTrip` enforces it. Unset any `NEO4J_CLI_FLAG_*` first. Commit source + bundle together.
- `policy.golden` needs **no** update — verified it indexes command paths and gates (`update  deny`, `update check  deny`), not flags.
- Changelog: `changie new --projects neo4j-cli --kind Minor --body "..."`. Kinds are `Major`/`Minor`/`Patch` only (verified in `.changie.yaml:9-15`); there is no `added` kind. Body states only the observable effect.

**Risks**
- `TestPlainTextOutput_GoldenSuccess` (`update_test.go:803`) asserts byte-for-byte merged stdout+stderr and **will break** — expected, and the update is part of the work.
- Piping `neo4j-cli update` to a file now captures markdown after the success line. Accepted during planning.

## Acceptance Criteria

- [ ] v1.8.0 → v1.11.0 prints `## v1.11.0`, `## v1.10.0`, `## v1.9.0` newest-first, with no `## Versions`/`## Changes` boilerplate and no `Full changelog:` line (exactly 3, nothing elided).
- [ ] v1.0.0 → v1.11.0 prints the same 3 newest entries **plus** the `Full changelog:` line.
- [ ] Single-version bump prints exactly one entry, no link line.
- [ ] `--no-changelog` reproduces today's plain-text output byte-for-byte.
- [ ] `--format json` includes `release_notes` as `[{version, notes}]`, plus `changelog_url` only when elided; both absent under `--no-changelog`.
- [ ] `--format toon` includes `release_notes`; `--format table` omits it and renders an intact grid.
- [ ] A body containing ANSI/C0 bytes is neutralised in plain-text output.
- [ ] A `listReleasesFn` error yields a stderr warning, `updated: true`, and exit code 0.
- [ ] `--version <older>` (downgrade) prints no changelog.
- [ ] `update check` output is byte-identical to today and has no `--no-changelog` flag.
- [ ] `Latest` and `GetByTag` signatures unchanged; `versioncheck` compiles untouched.
- [ ] `TestMain` guard trips if any test in the package reaches `httpDoFn` unstubbed.
- [ ] `make test`, `make fmt-check`, `make lint` all pass, including `TestGenerator_RoundTrip`.

## Out of Scope

- Markdown rendering, colorization, paging.
- `update check` changelog preview.
- `release_notes` in `--format table`.
- Caching release notes in `versioncheck`'s on-disk cache.
- Embedding or fetching `CHANGELOG.md`.
- Any user-configurable cap (fixed at 3).

## Open Questions

None — all design questions were resolved during planning (suppress-flag name, range size and cap, body trimming, output stream, structured-output shape per format, `update check` scope, fetch-failure behaviour).

## Verification

1. `make test` — full suite incl. updated goldens and `TestGenerator_RoundTrip`.
2. `make fmt-check && make lint`.
3. `go test -count=1 ./neo4j-cli/internal/subcommands/update/...` with networking blocked — confirms REQ-NF-002.
4. `go test -tags=e2e_seams -count=1 ./test/e2e/...` — exercises notes through the fixture server.
5. Manual smoke on a **throwaway copy** of the binary (never the dev tree's own `bin/neo4j-cli` mid-session), covering each acceptance row above across `--format json`, `toon`, `table`, default, and `--no-changelog`.
