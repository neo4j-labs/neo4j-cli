# PRD: `neo4j-cli update check` subcommand

## Overview

Replace the `neo4j-cli update --check` boolean flag with a `neo4j-cli update check` subcommand so the shape matches `neo4j-cli skill check`. Both commands cover the same semantic — read-only "is the installed thing in sync with the running binary?" inspection — and should use the same form. The split currently leaks into the agent skill bundle reference docs, so this is more than a CLI cosmetic.

We're on `v0.1.0-alpha.10` with no public users yet. Clean break: drop the `--check` flag entirely, no hidden/deprecated alias.

Source plan: `/Users/oskarhane/.claude/plans/i-think-there-are-jiggly-abelson.md`.

## Goals

- Mirror `skill check` and `update check` as sibling-shaped read-only inspection subcommands.
- Preserve the existing `runUpdate` orchestration and JSON output shape (REQ-F-018 from the original self-update PRD) so external scripts continue to work.
- Keep `--pre-releases` and `--version <tag>` available on `update check` (you might want to check availability of a specific tag, or include pre-releases when checking).
- Pick up the change in the agent skill bundle (`SKILL.md` and `references/update.md`) so AI agents see the new shape after `go generate`.

## Non-Goals

- **No deprecation alias.** `--check` on `update` is removed outright; passing it errors as "unknown flag". Alpha-stage, no compat needed.
- **No `--force` on `update check`.** `--force` only matters for the swap path (it bypasses the pkg-mgr passthrough); `update check` never swaps, so there is nothing to bypass.
- **No restructuring of `update` into a fully-subcommand-driven tree.** Bare `neo4j-cli update` keeps performing the swap (rustup-style), saving typing for the common path. We are not adding `update apply`/`update run`/etc.
- **No verb-split clean-up across the rest of the tree.** `create`/`delete` for cloud-API-lifecycle resources and `add`/`remove` for local-storage entries and list-elements (e.g. CORS allowed-origins) follow a deliberate convention and stay as-is.
- **No rewriting of historical changelog/PRD/task files** that documented the original `--check` flag. The new changie entry covers the change.
- **No change to the `Aura` skill template** (`neo4j-cli/aura/internal/skill/`). The `update` command lives under the neo4j-cli super-CLI tree, not the aura subtree, and the aura template has no `--check` references.

## Requirements

### Functional Requirements

- **REQ-F-001** — Mount a new `check` subcommand on the existing `update` parent in `neo4j-cli/internal/subcommands/update/update.go`. Bare `neo4j-cli update` continues to run the swap via the parent's `RunE`; `neo4j-cli update check` runs the read-only path.
- **REQ-F-002** — Add `neo4j-cli/internal/subcommands/update/check.go` with `newCheckCmd(cfg *clicfg.Config, bundle fs.FS, skillName string) *cobra.Command`. `Use: "check"`. Short: "Report whether a newer version is available without downloading or swapping". `RunE` calls the existing `runUpdate(ctx, cmd, cfg, runOpts{check: true, preReleases: …, version: …, bundle: bundle, skillName: skillName})`.
- **REQ-F-003** — `update check` accepts `--pre-releases` (bool) and `--version <tag>` (string) and reuses their semantics from the parent (`--pre-releases` includes alpha/beta/rc tags in the latest-discovery flow; `--version` skips discovery and validates the named tag via `ValidateVersionTag`).
- **REQ-F-004** — `update check` does NOT accept `--force`. Unknown-flag rejection is enforced by cobra's default flag-set behavior; no extra wiring required.
- **REQ-F-005** — Remove `--check` flag from the `update` parent in `update.go`: drop the `check` var, `checkFlag` const, `cmd.Flags().BoolVar(&check, checkFlag, …)` line, and the `check: check` entry in the `runOpts{}` literal. `runOpts.check` field stays — `runUpdate` still branches on it and the JSON output preserves the `check: bool` field per the original REQ-F-018.
- **REQ-F-006** — Behavioral parity: `update check` delivers exactly the same exit codes, narrative text, and JSON shape as the previous `update --check` for every branch — dev-build short-circuit, `ErrNoStableRelease` friendly hint, `ErrTagNotFound` usage error, invalid-current-semver fatal, up-to-date (`cmp == 0`) exit 0, newer-available (`cmp < 0`) exit 1 via `clierr.NewUsageError`, downgrade-without-explicit-version refusal, and the pkg-mgr passthrough hint when the running binary is under a known package-manager prefix.
- **REQ-F-007** — JSON output shape (`-f json`) for `update check` is unchanged from `update --check`: `{"current": "v…", "latest": "v…", "updated": false, "check": true, "channel": "stable"|"pre-release", "install_method": "binary"|"homebrew"|"npm"|"pipx"|"uv"}`. Field order pinned in `printableUpdateResult.MarshalJSON` continues to apply.
- **REQ-F-008** — Update doc comments in `neo4j-cli/internal/subcommands/update/update.go` (`runUpdate` flow block ~lines 273–285 and `refreshSkillBundles` block ~line 453), `neo4j-cli/internal/subcommands/update/install_method.go:72`, and `neo4j-cli/internal/versioncheck/cache.go:10` to reference `update check` instead of `--check`.

### Documentation Requirements

- **REQ-D-001** — `README.md:24` "Self-update" example changes from `neo4j-cli update --check` to `neo4j-cli update check`.
- **REQ-D-002** — `neo4j-cli/internal/skill/additions.md:15` "Updating the CLI" paragraph: replace the sentence describing `--check` with one describing `neo4j-cli update check`. The `--pre-releases`, `--force`, and post-swap skill-refresh sentences stay intact.
- **REQ-D-003** — Regenerate `neo4j-cli/internal/skill/bundle/SKILL.md` and `neo4j-cli/internal/skill/bundle/references/update.md` via `go generate ./neo4j-cli/internal/skill/...`. `make generate-check` and `TestGenerator_RoundTrip` are the CI gates; both fail until the regenerated bundle is committed alongside the source change.
- **REQ-D-004** — `.github/workflows/test.yml`: e2e step at line ~109 uses `bin/neo4j-cli-e2e update check --pre-releases -f json` (not `update --check --pre-releases`); update the comment at line ~69 to match.
- **REQ-D-005** — `test/e2e/check_json/main.go`: comment text references to "`--check` mode" / "in --check mode" become "the `check` subcommand" / "from the `check` subcommand" (lines 21, 97, 102, 105, 107, 110). The JSON field `check` (bool) is unchanged, so no struct/assertion edits are needed.
- **REQ-D-006** — Add a changie entry: `changie new --projects neo4j-cli --kind Patch --body "Replace 'neo4j-cli update --check' flag with 'neo4j-cli update check' subcommand for consistency with 'skill check'."`. If changie isn't installed locally, hand-author `.changes/unreleased/neo4j-cli-Patch-<YYYYMMDD>-<HHMMSS>.yaml` with the matching schema.

### Test Requirements

- **REQ-T-001** — Existing `neo4j-cli/internal/subcommands/update/update_test.go` cases that drive `runUpdate` directly via `runWithOpts(…, runOpts{check: true})` stay verbatim — the internal flow is unchanged.
- **REQ-T-002** — The one cobra-args-level test in `update_test.go` (around line 756, `cmd.SetArgs([]string{"--check"})`) migrates to invoke the subcommand: either `cmd.SetArgs([]string{"check"})` against the parent or move into the new `check_test.go`.
- **REQ-T-003** — The flag-registration assertion loop in `update_test.go` line ~405 (`for _, name := range []string{"pre-releases", "check", "version", "force"}`) drops `"check"` for the parent. A parallel assertion covers the new subcommand's expected flag set (`pre-releases`, `version`).
- **REQ-T-004** — New `neo4j-cli/internal/subcommands/update/check_test.go` colocated with `check.go`. Style mirrors `common/skill/check_test.go`. Cover: (a) subcommand is mounted on the `update` parent (`cmd.Find([]string{"check"})` returns non-nil); (b) flags `pre-releases` and `version` are registered, `force` and `check` are NOT; (c) RunE plumbs through to `runUpdate` with `check: true` — stub `latestFn`, `detectFn`, `swapFn` seams and assert swap is never invoked, JSON shape sets `check: true` / `updated: false`, exit-code-bearing error returned when newer is available; (d) `--pre-releases` and `--version v0.1.0-alpha.10` reach `resolveTarget` correctly.

### Non-Functional Requirements

- **NF-001 (Cobra layout)** — Follow the project's one-file-per-leaf cobra layout (per AGENTS.md "Cobra Command Layout"). The new leaf gets its own `check.go` + `check_test.go`; no body inlined in `update.go`.
- **NF-002 (No new dependencies)** — Stdlib + existing project deps only. No third-party packages introduced.
- **NF-003 (Cross-platform)** — No platform-specific changes; the existing `update` flow already handles Linux/macOS/Windows and arm64/amd64.
- **NF-004 (Lint and format clean)** — `make fmt-check` and `make lint` clean (golangci-lint v2 config). License header on the new file matches the rest of the package (`// Copyright (c) "Neo4j" / // Neo4j Sweden AB [http://neo4j.com]`).

## Technical Considerations

- **Parent-with-RunE plus subcommands.** Cobra supports a command with both `RunE` and child subcommands — bare `update` runs the parent's `RunE` (swap), `update <subcmd>` runs the child. No special wiring needed beyond `cmd.AddCommand(newCheckCmd(...))`.
- **`runOpts.check` stays.** The field continues to gate behavior inside `runUpdate` (skill-refresh skipped, plain-text narrative shortened, JSON sets `check: true`). The new subcommand sets it to `true`; the parent (post-removal of the flag) always sets it to `false`. The shared orchestration path is preserved, minimizing test churn.
- **Flag duplication kept local, not persistent.** Re-declare `--pre-releases` and `--version` on the new subcommand rather than promoting them to `PersistentFlags()` on the parent. Persistent flags would surface them on the parent's `--help` for both contexts and complicate the closure-bound `var` model already used in `NewCmd`. Local re-declaration is simpler and matches how the `aura` subtree handles `--await`.
- **Test-seam reuse.** The existing `latestFn` / `getByTagFn` / `detectFn` / `swapFn` / `listSkillsFn` / `installSkillFn` package-level seams (defined in `update.go`) work unchanged for the new subcommand because both code paths funnel through `runUpdate`. The new `check_test.go` swaps the same seams that `update_test.go` does.
- **Bundle drift gate.** Source changes alone fail `TestGenerator_RoundTrip` and `make generate-check` until `go generate ./neo4j-cli/internal/skill/...` is run and the regenerated `bundle/SKILL.md` + `bundle/references/update.md` are committed. Run regen in the same commit as the source edit.
- **`additions.md` is hand-written — generation does NOT fix it.** Per AGENTS.md "Repo Layout Notes", `additions.md` is composed into `bundle/SKILL.md` at generate time but is itself committed as authored content. The edit at line 15 is manual.

## Acceptance Criteria

- [ ] `bin/neo4j-cli update --help` lists `check` as a subcommand; no `--check` flag appears.
- [ ] `bin/neo4j-cli update check --help` lists `--pre-releases` and `--version` flags; no `--force` or `--check` flag.
- [ ] `bin/neo4j-cli update check` against the live `neo4j-labs/neo4j-cli` repo prints current vs latest and exits 1 when a newer release exists (uses `clierr.NewUsageError`, exit code 1 via the existing handler).
- [ ] `bin/neo4j-cli update check -f json` emits valid JSON matching the documented shape with `check: true`, `updated: false`. The existing `test/e2e/check_json` harness passes against the new entry point.
- [ ] `bin/neo4j-cli update check --pre-releases` includes prerelease tags in latest discovery.
- [ ] `bin/neo4j-cli update check --version v0.1.0-alpha.10` resolves the named tag and reports comparison.
- [ ] `bin/neo4j-cli update check --force` errors with cobra's standard "unknown flag" message.
- [ ] On a homebrew-installed binary, `bin/neo4j-cli update check` prints the channel-correct passthrough hint (parity with previous `update --check` behavior).
- [ ] `make build` clean.
- [ ] `make test` clean — includes new `check_test.go` and updated `update_test.go`.
- [ ] `make fmt-check` clean.
- [ ] `make lint` clean.
- [ ] `make license-check` clean (header on `check.go` and `check_test.go`).
- [ ] `make generate-check` clean (regenerated `bundle/SKILL.md` and `bundle/references/update.md` committed).
- [ ] README, `additions.md`, CI workflow, and e2e comments all updated.
- [ ] Changie entry filed under `.changes/unreleased/`.

## Out of Scope

- Adding `update apply` / `update run` / similar sibling subcommands to make `update` a fully-symmetric subcommand tree. Bare `update` keeps doing the swap.
- Verb-split clean-up across the rest of the command tree (`create`/`delete` vs `add`/`remove`).
- Promoting `--pre-releases` / `--version` to persistent flags on the `update` parent.
- Changing `--await`, `--force`, or other modifier flags into subcommands.
- Rewriting historical changelog entries, PRD documents, or task YAML that documented the original `--check` flag.
- Touching the Aura skill template (`neo4j-cli/aura/internal/skill/`).

## Open Questions

- None — the three decisions (drop flag without alias / carry `--pre-releases` + `--version` / leave verb splits alone) were locked before PRD authoring. Behavioral parity for every branch (dev-build, up-to-date, newer-available, downgrade refusal, pkg-mgr passthrough) is captured in REQ-F-006 as "same as before".
