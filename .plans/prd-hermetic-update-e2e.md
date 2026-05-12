# PRD: Hermetic Tier-1 e2e for `update check` (+ soft schema-only live smoke)

## Overview

`.github/workflows/test.yml` runs a Tier-1 e2e at lines ~78–119 that builds `bin/neo4j-cli-e2e` against real `api.github.com` and asserts `--expect-channel pre-release` plus a `--cross-check` of `latest` against `gh release list --limit 1`. The assertion silently assumed the head of the GitHub release feed would always be a prerelease. v1.0.0 shipped 2026-05-11 — the head is now stable, the assertion is wrong, and every PR after the v1 launch fails this step on all three OSes (ubuntu/macos/windows). PR #89 (CLI-77 skill enrichment) is the first casualty.

Fix: pivot Tier 1 to a hermetic flow (same `e2e_seams` build-tag pattern Tier 2 already uses) that exercises both `channel: stable` and `channel: pre-release` deterministically, and add a small **schema-only** live smoke against real GitHub so we still notice contract drift without being coupled to the release calendar.

## Goals

- Stop tying CI green to GitHub's current release-feed state.
- Cover both Cypher-of-this-test code paths every run (stable head AND prerelease head) instead of "whichever the API happens to be in."
- Keep a soft live-API canary so a GitHub schema change still surfaces in CI.
- Reuse the existing `e2e_seams` build tag + `NEO4J_CLI_UPDATE_E2E_API_URL` seam (`neo4j-cli/internal/subcommands/update/seams_e2e.go`); zero changes to production binary code.

## Non-Goals

- Tier 2 (`bin/neo4j-cli-e2e2`, download+swap) — already hermetic; untouched.
- The `update` command's source logic in `neo4j-cli/internal/subcommands/update/` — no behavior change to the binary.
- Adding new build tags, seams, or runtime config knobs.
- Backporting to v0.x release branches — none exist.
- Replacing the GitHub release feed as an update channel.

## Requirements

### Functional Requirements

- **REQ-F-001:** Build the Tier-1 binary with `-tags e2e_seams` so it honors `NEO4J_CLI_UPDATE_E2E_API_URL`. Keep the existing `-ldflags='-X .Version=v0.0.0-e2e'` so the semver-compare path is unchanged.
- **REQ-F-002:** Add a new fixture binary `test/e2e/release_fixture/main.go` (~80 LoC) that serves `/repos/neo4j-labs/neo4j-cli/releases` from a committed JSON file and supports a `--scenario {stable-head | pre-release-head}` flag selecting which canned JSON to serve. Pure stdlib (mirroring `update_fixture/main.go`'s "no new module deps" rule). Listens on `127.0.0.1:0`, prints `listening on http://127.0.0.1:<port>` as the FIRST line of stdout (same handshake the existing fixture uses for the harness to scrape the port).
- **REQ-F-003:** Add two committed canned fixtures under `test/e2e/release_fixture/testdata/`:
  - `releases-stable-head.json` — array with stable v9.9.0 at index 0, prerelease v9.9.0-alpha.1 at index 1. Latest stable wins → binary reports `channel: stable`.
  - `releases-pre-release-head.json` — array with prerelease v9.9.1-alpha.1 at index 0, stable v9.9.0 at index 1. With `--pre-releases`, binary reports `channel: pre-release`.
  Both files use the slim GitHub release shape `release.go` consumes (`tag_name`, `draft`, `prerelease`); extra fields trimmed.
- **REQ-F-004:** Extend `test/e2e/check_json/main.go` with a `--schema-only` flag. When set, the helper still validates structure (six documented keys present, type checks, `latest` parses as semver, post-swap-only fields absent for `check`) AND enum membership (`channel ∈ {stable, pre-release}`, `install_method ∈ {binary, homebrew, npm, pipx, uv}`), but does NOT assert specific values. The existing `--expect-channel` / `--cross-check` flags are mutually exclusive with `--schema-only` — passing both should error before the schema check runs.
- **REQ-F-005:** Rewrite the workflow Tier-1 step to a hermetic block running both scenarios sequentially:
  1. Build `bin/neo4j-cli-e2e` with `-tags e2e_seams -ldflags='-X ....Version=v0.0.0-e2e'`.
  2. Launch `release_fixture --scenario stable-head` in background; scrape port from first stdout line; export `NEO4J_CLI_UPDATE_E2E_API_URL=http://127.0.0.1:<port>`.
  3. Run `bin/neo4j-cli-e2e update check --pre-releases -f json` → pipe to `check_json --expect-channel stable --cross-check v9.9.0` (note: with `--pre-releases` flag the binary considers prereleases but latest is still the stable v9.9.0 in this scenario).
  4. Kill the fixture; relaunch with `--scenario pre-release-head`; export new port.
  5. Run `bin/neo4j-cli-e2e update check --pre-releases -f json` → pipe to `check_json --expect-channel pre-release --cross-check v9.9.1-alpha.1`.
  6. Tear down fixture.
- **REQ-F-006:** Add a new workflow step "Update e2e (schema-only live smoke)" that runs the same Tier-1 binary (no seam env var set → talks to real api.github.com) and pipes `update check --pre-releases -f json` to `check_json --schema-only`. No value assertions, no cross-checks. The step is calendar-immune.
- **REQ-F-007:** Schema-only mode MUST verify, at minimum:
  - Valid JSON.
  - All six keys present: `current`, `latest`, `updated`, `check`, `channel`, `install_method`.
  - Types: `current`/`latest`/`channel`/`install_method` are strings; `updated`/`check` are bools.
  - `latest` parses as semver via `golang.org/x/mod/semver.IsValid`.
  - Enum: `channel ∈ {stable, pre-release}`.
  - Enum: `install_method ∈ {binary, homebrew, npm, pipx, uv}` (confirmed against `neo4j-cli/internal/subcommands/update/install_method.go:45-49`).
  - For `check` subcommand: `updated == false`, `check == true`, `updated_skills` and `skill_install_suggested` absent.

### Non-Functional Requirements

- **REQ-NF-001:** Tier-1 step runtime must stay within the same order-of-magnitude as today (current step is ~1s; hermetic version should be <5s including two fixture restarts).
- **REQ-NF-002:** Both fixture JSON files MUST be checked in (no runtime generation) so failures are reproducible from a clean checkout.
- **REQ-NF-003:** New `.go` files carry the `Neo4j Copyright` header (enforced by `addlicense` and `make license-check`).
- **REQ-NF-004:** Step works on all three CI OSes (ubuntu/macos/windows). The fixture must use only stdlib, no shell helpers beyond what already runs on Windows runners.
- **REQ-NF-005:** No production-binary impact. `seams_e2e_off_test.go` (the gate asserting `apiBaseURL` stays at production default without the tag) keeps passing unchanged.

## Technical Considerations

### Fixture binary placement — open question answered

The existing `test/e2e/update_fixture/main.go` already serves `/repos/neo4j-labs/neo4j-cli/releases` (line 97), but it's bundled with 338 LoC of archive-build machinery (`buildArchives` → `go build` of `fakebin` on startup → tar.gz/zip packaging) that Tier 1 doesn't need. Two options were considered:

- (a) Extend `update_fixture` with a `--scenario` flag and skip `buildArchives` when only release-list endpoints will be hit.
- (b) Add a sibling lightweight fixture `test/e2e/release_fixture/main.go`.

**Decision: (b) — sibling binary.** Single-responsibility, ~80 LoC vs touching the 338-LoC fixture, no `go build` of fakebin on Tier-1 invocations (faster startup), and Tier 2 stays exactly as-is. Tier 1's serial restart between scenarios benefits from sub-second startup.

### Workflow step shape

Pattern to mirror — the existing Tier-2 step (`.github/workflows/test.yml` ~line 142 "Update e2e (tier-2 download + swap)"): build with `-tags e2e_seams`, launch fixture in background, scrape port from stdout, export `NEO4J_CLI_UPDATE_E2E_API_URL`, run binary, tear down. Tier 1 adds the second-scenario-restart twist; everything else is copy-paste.

### `check_json` flag wiring

Current flags: `--expect-channel`, `--cross-check` — both optional value assertions today. Adding `--schema-only` (bool) is a small change to the existing `flag.Parse()` block (`test/e2e/check_json/main.go:60–62`). Reuse the existing struct + decoder; only the value-assertion block (lines 113–119) gets conditionally skipped, and the enum/post-swap-only checks become unconditional (most of them already are).

### Production-binary guard

`seams_e2e_off_test.go` asserts `apiBaseURL == "https://api.github.com"` when built without `e2e_seams`. The Tier-1 binary now uses `e2e_seams`, so this guard is NOT a regression — the production binary in releases is built without the tag, and the assertion still holds for shipped artifacts. Tier 1 was always a test binary; the build flag swap doesn't change that.

### Documenting the change

Add a one-line note to `AGENTS.md` under "Hermetic Test Notes" (the section already covers test seams). Pattern: "Tier-1 e2e for `update check` uses the `e2e_seams` build tag + `release_fixture` to cover both `channel: stable` and `channel: pre-release` deterministically; schema-only live smoke catches GitHub API contract drift."

## Acceptance Criteria

- [ ] Tier-1 step in `.github/workflows/test.yml` is hermetic — no `gh release list` calls, no value assertions tied to live API.
- [ ] Both scenarios (`stable-head`, `pre-release-head`) run in sequence and value-assert their respective channel via `check_json --expect-channel`.
- [ ] A new "schema-only live smoke" step calls real GitHub and runs `check_json --schema-only` — no channel/tag value assertions.
- [ ] `test/e2e/release_fixture/main.go` exists, uses pure stdlib, prints `listening on http://127.0.0.1:<port>` first on stdout, supports `--scenario stable-head | pre-release-head`, shuts down cleanly on SIGINT/SIGTERM.
- [ ] `test/e2e/release_fixture/testdata/releases-{stable-head,pre-release-head}.json` are committed and use the slim GitHub release shape (`tag_name`, `draft`, `prerelease`).
- [ ] `check_json --schema-only` rejects malformed JSON, missing keys, wrong types, non-semver `latest`, out-of-enum `channel` or `install_method`, AND fails when both `--schema-only` and `--expect-channel`/`--cross-check` are passed.
- [ ] All four project gates clean: `make test`, `make fmt-check`, `make lint`, `make generate-check`.
- [ ] `make license-check` clean (new `.go` files carry the Neo4j header).
- [ ] CI on the resulting PR turns the Tier-1 step green on a tree where stable v1.0.0 sits at the API head.
- [ ] Tier 2 step output is byte-equal pre- and post-change (no collateral edits).

## Out of Scope

- Tier 2 download+swap path — already hermetic, untouched.
- Modifying the production `update check` JSON output schema or any consumer-visible behavior of the binary.
- Removing or restructuring the `e2e_seams` build tag.
- Replacing the GitHub release feed with another channel.
- Adding unit tests for `release_fixture/main.go` (the fixture is exercised end-to-end by the workflow; a unit test layer is overkill for an HTTP server reading JSON off disk).

## Open Questions

- Add unit tests for `check_json --schema-only` mode? `check_json` currently has no `_test.go` file. The new mode is small enough that the workflow's pass/fail acts as integration coverage. Recommend: skip unit tests, document the mode in the package comment + `--help`.
- Snapshot the canned release JSON from real GitHub once (and refresh annually), or hand-author the minimum fields? Recommend: hand-author the minimum (`tag_name`, `draft`, `prerelease`) — matches what `release.go` decodes and avoids stale fields drifting.
- Should the Tier-1 binary be renamed (e.g. `bin/neo4j-cli-e2e1` for parity with `-e2e2`)? Cosmetic only. Recommend: leave as `bin/neo4j-cli-e2e` to keep the workflow diff minimal.
- `release.go`'s `Latest()` filter semantics under `--pre-releases`: confirm that with stable-head fixture, the binary returns the stable (latest by semver) and reports `channel: stable` even though `--pre-releases` was passed. Quick test against the existing `update_fixture` defaults (which serve `[prerelease v9.9.1-alpha.1, stable v9.9.0]`) suggests `--pre-releases` returns the prerelease; need to confirm by inverting the order in `release_fixture` so stable v9.9.0 > prerelease v9.9.0-alpha.1 by semver. PRD step 5 acceptance criteria assumes the stable scenario's stable tag has higher semver than its prerelease tag — encode this when authoring the fixtures.
