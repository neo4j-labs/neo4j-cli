# PRD: `neo4j-cli update` UX fixes — friendly check output + sudo auto-elevation

## Overview

Two real-world UX issues with the recently-shipped `neo4j-cli update` command (PRD: `prd-self-update-command.md`, subcommand split: `prd-update-check-subcommand.md`), reported on the alpha build installed at the install-script default `/usr/local/bin/neo4j-cli`:

1. **`update check` output is hostile when a newer version exists.** RunE returns `clierr.NewUsageError(...)` to signal exit-1, which cobra renders as `Error: a newer version is available…` and dumps the full `--help` block. Finding a new version is the success case for `check` — the `Error:` prefix and the usage spew are both wrong.
2. **`update` (swap) on `/usr/local/bin` fails with `permission denied` and the same help dump.** `os.OpenFile(/usr/local/bin/neo4j-cli.new)` fails because the dir is root-owned. The cobra usage block dumping on swap errors is the second half of the problem.

Source plan: `/Users/oskarhane/.claude/plans/two-things-with-the-typed-fog.md`.

## Goals

- Make `update check` output friendly and actionable: one-line "new version available" message plus the exact install command the user should run; no `Error:` prefix; no help dump; exit 0 in both up-to-date and newer-available cases.
- Make `update` succeed on the install-script default install location (`/usr/local/bin`) without the user having to retype `sudo` or know about the permission issue ahead of time. Single sudo password prompt for the swap step; fall back gracefully on non-TTY / no-sudo / Windows.
- Stop the cobra usage block from being dumped on any swap-time error from `update`; the failure is not a misuse of the flag set.
- No regression to the writable-dir happy path on darwin / linux / WSL / windows.

## Non-Goals

- **No deprecation alias.** This is still alpha; no compat shim for the previous behavior.
- **No script-friendly non-zero exit on `update check` when a newer version exists.** Per user decision: `update check` exits 0 always; CI/scripts compare `current != latest` from the JSON output instead.
- **No `--no-elevate` opt-out flag.** Default to the auto-elevate path; users who want to avoid sudo can pre-elevate the whole command (`sudo neo4j-cli update`) or change `INSTALL_DIR` to a user-writable location.
- **No install-location migration.** Existing binaries at `/usr/local/bin` are not relocated; the auto-elevate path makes them updatable in place.
- **No change to `--force`, `--version`, `--pre-releases` semantics** or the install-method passthrough hint.
- **No new exit codes** for the elevated-swap branch — success is 0, any non-zero from sudo or install propagates as a `clierr.NewFatalError`.
- **No interactive UI beyond what sudo itself prints.** Default sudo prompt; no custom `-p` string.
- **No skill bundle / SKILL.md content changes** beyond regeneration after source edits — the user-facing CLI surface (flag set, subcommand tree) is unchanged.

## Requirements

### Functional Requirements

#### `update check` friendly output

- **REQ-F-001** — When `update check` discovers a newer version (`cmp < 0` branch in `runUpdate`, `neo4j-cli/internal/subcommands/update/update.go:373-383`), do NOT return an error. Print a two-line plain-text message to `cmd.OutOrStdout()` and return `nil`. Exit code is 0.
- **REQ-F-002** — Friendly-message shape (plain-text branch only):
  ```
  New version available: <current> -> <latest>
  Run `neo4j-cli update[ --pre-releases][ --version <tag>]` to install.
  ```
  ASCII `->` matches the existing error text style. The bracketed flags are included only when the user supplied them on the `check` invocation; flag reconstruction reads from `runOpts` (hermetic — `runOpts` covers `preReleases`, `version`, and `force`).
- **REQ-F-003** — JSON/table/toon output for `update check` is unchanged: `check: true`, `updated: false`, the existing `current` / `latest` / `channel` / `install_method` fields all remain. The change is plain-text only.
- **REQ-F-004** — `update check` when already on latest (`cmp == 0` branch, line 340) continues to print `Already on <version>. No update needed.` and exit 0 — unchanged.
- **REQ-F-005** — Up-to-date / friendly-hint / pkg-mgr-passthrough branches all stay non-erroring. The only behavior change is the newer-available branch flipping from error to nil.

#### Usage-block suppression on `update` swap errors

- **REQ-F-006** — `update` parent command sets `cmd.SilenceUsage = true` inside `runUpdate` after `--version` validation succeeds, matching the repo convention (`tenant/list.go:21`, etc.). Genuine flag misuse (`neo4j-cli update --bogus`) still triggers cobra's default usage path because validation runs after flag parsing.
- **REQ-F-007** — `check.go:50` `SilenceUsage: true` is preserved (defense in depth even though REQ-F-001 stops returning an error in that branch).

#### Sudo auto-elevation on non-writable target dir

- **REQ-F-008** — `Swap(ctx, urls, currentBinaryPath, stderr io.Writer)` gains an `io.Writer` parameter so the elevation-narrative line can land on `cmd.ErrOrStderr()`. The `swapFn` seam in `update.go:60` updates to match; all test fakes update accordingly.
- **REQ-F-009** — Insert a pre-flight `planSwap(abs string) (swapPlan, error)` between `filepath.Abs` (line 131) and the archive download (line 133). The plan struct: `{elevate bool; tmpDir string}`. Returning `elevate: false` keeps today's same-dir tmp file + `os.Rename` behavior.
- **REQ-F-010** — `planSwap` ordering:
  1. `dirWritableFn(filepath.Dir(abs))` — probe via `O_EXCL | O_CREATE` of a `.neo4j-cli-probe.<rand>` file in the target dir; delete on success. EACCES/EROFS → not writable. Any other error → propagate fatally.
  2. Writable → `{elevate: false, tmpDir: filepath.Dir(abs)}`. Return.
  3. Not writable + `swapGoosFn() == "windows"` → return `errPermissionWindows` sentinel (no sudo on Windows).
  4. Not writable + `geteuidFn() == 0` → already root; surface the raw permission error (immutable bit / SIP / read-only fs; sudo cannot help).
  5. Not writable + (`lookPathFn("sudo")` errors OR `lookPathFn("install")` errors OR `stdinIsTTYFn() == false`) → return `errSudoUnavailable` sentinel carrying the dir.
  6. Otherwise → `{elevate: true, tmpDir: tempDirFn()}`.
- **REQ-F-011** — Extract block (line 164-177) honors `plan.tmpDir`: `tmpNew := filepath.Join(plan.tmpDir, "neo4j-cli.new")`. Existing `os.Remove(tmpNew)` stale-cleanup and `extractBinary` + `chmod 0755` calls are unchanged otherwise.
- **REQ-F-012** — Rename block (line 184-196): if `plan.elevate`, write `Cannot write to <dir> (permission denied).\nElevating via sudo — you may be prompted for your password.\n` to `stderr`, then call `elevatedSwap(ctx, tmpNew, abs)`. On success or failure, `os.Remove(tmpNew)` runs from the original user's process. If `plan.elevate` is false, the existing `windowsSwap` / `renameFn` paths run unchanged.
- **REQ-F-013** — `elevatedSwap(ctx, src, dst string) error` builds a fixed-shape argv with NO shell:
  ```
  exec.CommandContext(ctx, sudoPath, installPath, "-m", "0755", src, dst)
  ```
  where `sudoPath` and `installPath` come from `lookPathFn("sudo")` and `lookPathFn("install")`. `src` and `dst` MUST pass `filepath.IsAbs`, not start with `-`, and not contain NUL — else reject before exec. Stdin / stdout / stderr inherit `os.Stdin` / `os.Stdout` / `os.Stderr` so the sudo prompt is interactive.
- **REQ-F-014** — `runUpdate` recognises the two new sentinel errors after `swapFn` returns:
  - `errSudoUnavailable(dir)`: build the friendly fallback hint using `cmd.CommandPath()` plus the active `runOpts` flags (`--pre-releases`, `--version <tag>`, `--force`), wrap as `clierr.NewFatalError`. Shape:
    ```
    cannot write to <dir> (permission denied).
    Re-run with sudo:

        sudo neo4j-cli update[ --pre-releases][ --version <tag>][ --force]
    ```
  - `errPermissionWindows(dir)`: friendly hint to re-run from an Administrator shell:
    ```
    cannot write to <dir> (permission denied).
    Re-run from an Administrator shell.
    ```
- **REQ-F-015** — Skill-bundle refresh (`refreshSkillBundles`, line 422-484) still runs as the original (non-root) user after the elevated swap. Verified by ordering: the elevated step runs only inside `Swap`; once `Swap` returns nil, runUpdate continues with `refreshSkillBundles` in the same process.

### Test Requirements

- **REQ-T-001** — Update existing `check_test.go` cases that assert `require.Error` on the newer-available branch (lines 120, 156, 184) to assert `require.NoError` and check the stdout contains both the version-pair line and the install command shape from REQ-F-002.
- **REQ-T-002** — Update `update_test.go` cases at lines 183, 213, 553, 995 (the four `--check + newer` cases) the same way: flip to `require.NoError`, assert stdout shape.
- **REQ-T-003** — New seam helpers in `swap_test.go`: `withGeteuid`, `withLookPath`, `withRunCommand`, `withDirWritable`, `withTempDir`, `withStdinIsTTY`. Pattern matches the existing `withSwapGoos` / `withRename` / `withRequireHTTPS` helpers.
- **REQ-T-004** — Elevation-path table-driven tests in `swap_test.go`:
  - Writable dir → `os.Rename` invoked (assert via `withRename` counter); `runCommandFn` NOT invoked; happy path unchanged.
  - Non-writable + linux + sudo available + TTY → `runCommandFn` invoked exactly once with argv `["/usr/bin/sudo", "/usr/bin/install", "-m", "0755", <abs tmpNew>, <abs dst>]`; `tmpNew` lives under `tempDirFn()`; `os.Remove(tmpNew)` runs after.
  - Non-writable + linux + already root (`geteuidFn → 0`) → no elevation attempted; raw permission error surfaces.
  - Non-writable + linux + no sudo (`lookPathFn("sudo") → exec.ErrNotFound`) → `errSudoUnavailable` returned; archive download did NOT occur (assert via `httpDoFn` call counter).
  - Non-writable + linux + non-TTY (`stdinIsTTYFn → false`) → `errSudoUnavailable` returned; no download.
  - Non-writable + windows (`swapGoosFn → "windows"`) → `errPermissionWindows` returned.
  - Elevation invoked but sudo returns non-zero (cancelled prompt) → error wraps with "sudo install:" prefix; `tmpNew` cleanup runs.
- **REQ-T-005** — `update_test.go` new tests covering the runUpdate sentinel-recognition path: the two sentinels produce the documented friendly hint via `clierr.NewFatalError`, and the hint reflects the supplied `opts` flags (test matrix: bare; `--pre-releases`; `--version v0.1.0-alpha.10`; `--force`).
- **REQ-T-006** — Argv safety guard tests: `elevatedSwap` rejects `src`/`dst` containing NUL, starting with `-`, or not absolute.
- **REQ-T-007** — Cross-OS regression: writable-dir happy path tests stay green on the existing ubuntu / macos / windows CI matrix.

### Documentation Requirements

- **REQ-D-001** — Add a changie entry covering both fixes (single Patch entry is fine, body example: `changie new --projects neo4j-cli --kind Patch --body "Fix 'update check' to print a friendly message (no Error: prefix, no help dump) and auto-elevate via sudo when updating from a non-writable install location."`).
- **REQ-D-002** — Regenerate `neo4j-cli/internal/skill/bundle/SKILL.md` and `neo4j-cli/internal/skill/bundle/references/update.md` if the cobra `Long` / `Short` strings change. The current plan does not change them, but `make generate-check` is the gate either way.
- **REQ-D-003** — Update `runUpdate` doc-comment block at `update.go:271-285` to reference the new elevation step in the REQ-F-012/013/014/015/016 paragraph.
- **REQ-D-004** — Update the `Swap` doc-comment in `swap.go:113-123` and the package doc at `swap.go:4-43` to describe the planSwap pre-flight and elevation branch.

### Non-Functional Requirements

- **REQ-NF-001 (Security — sudo argv shape)** — `elevatedSwap` MUST construct argv as separate `exec.Cmd` args, never `sh -c` or `system()`. Argv literal values are checked into the codebase; only `src` and `dst` come from runtime, and both are validated absolute, NUL-free, and non-flag-prefixed before exec.
- **REQ-NF-002 (Security — REQ-S-001 preserved)** — Host-pinning and HTTPS-required guards in `swap.go` are untouched. Elevation only runs AFTER checksum verify (line 152-156) succeeds; this ordering is preserved because elevation lives in the rename block, downstream of verify.
- **REQ-NF-003 (Security — REQ-F-013 preserved)** — No swap may occur (direct or elevated) without checksum verification. Branch placement enforces this — the elevation branch sits where the existing `os.Rename` does, downstream of the verify block at line 152-156.
- **REQ-NF-004 (No new dependencies)** — `golang.org/x/term` is already in `go.mod` via the `query` package (per CLAUDE.md "query Subsystem Notes"); the TTY probe uses it. `os/exec` and `os` are stdlib. No new modules.
- **REQ-NF-005 (Cross-OS)** — Behaves correctly on darwin / linux (incl. WSL) / windows. WSL semantics match native linux (Go reports `runtime.GOOS == "linux"`). The Windows native binary follows the windows branch. Already-root on linux containers does not deadlock or double-prompt; surfaces a clean error.
- **REQ-NF-006 (Test hermeticity)** — Production `dirWritableFn` probes the real filesystem. Tests using `t.TempDir()` get a naturally-writable result without seam override. Tests forcing the non-writable branch use `withDirWritable(t, fn)`.
- **REQ-NF-007 (License header)** — All new `.go` files start with the standard `// Copyright (c) "Neo4j" // Neo4j Sweden AB [http://neo4j.com]` header (gate: CI `addlicense`).
- **REQ-NF-008 (gofmt + lint)** — `make fmt-check && make lint` clean. `make test` green.

## Technical Considerations

- **Pre-flight before download.** The plan probes writability before the ~50 MB archive download so the unactionable-permission case (non-TTY + no sudo) fails fast.
- **Same-filesystem rename invariant.** Preserved on the writable path. On the elevated path, `sudo install` copies (not renames), so cross-fs is fine and the same-dir-as-binary placement is dropped intentionally — `tmpNew` lives in `os.TempDir()`.
- **Cleanup ordering.** `os.Remove(tmpNew)` runs from the original (non-root) user after elevation, regardless of success/failure. `tmpNew` was created by that user under `os.TempDir()` so no permission issue.
- **sudo timestamp warmth.** No `sudo -k`. A warm sudo timestamp in the shell session means zero password prompts for the user.
- **sudo `requiretty` edge case (WSL).** Some sudo configs require a TTY beyond what `stdinIsTTYFn` checks. If sudo refuses with "must be run from a terminal", `runCommandFn` returns non-nil and `runUpdate` surfaces it as a clean error. WSL default sudoers does NOT set `requiretty`; this is best-effort handling for hostile environments.
- **CommandPath for the re-run hint.** Use `cmd.CommandPath()` rather than `os.Args[0]` so the hint matches whatever name the user invoked (the binary may have been aliased / symlinked).
- **`opts`-based flag reconstruction.** The current `runOpts` contains `preReleases`, `version`, `force` — covers every flag the user can pass that the re-run needs to include. If a future flag is added, the reconstruction must be updated too; an inline comment near the helper reminds maintainers.
- **`runOpts.check` continues to live on the struct** even though Fix 1 no longer returns an error from the check branch — the JSON output still emits `check: true` and the doc-comment in `update.go:127` already explains the field.
- **Existing seam pattern.** Six new seams follow the established `var fooFn = realImpl` + `withFoo(t, fake)` convention from `swap_test.go`. No fundamental departures from how the package handles testability.

## Acceptance Criteria

- [ ] `neo4j-cli update check --pre-releases` on a binary one version behind the latest pre-release prints exactly:
  ```
  Current version: <current>
  Latest pre-release version: <latest>
  New version available: <current> -> <latest>
  Run `neo4j-cli update --pre-releases` to install.
  ```
  and exits 0. No `Error:` prefix, no usage block.
- [ ] `neo4j-cli update check` on a binary already on the latest stable prints `Already on <version>. No update needed.` and exits 0 (unchanged).
- [ ] `neo4j-cli update --pre-releases` against `/usr/local/bin/neo4j-cli` on darwin prints:
  ```
  Current version: <current>
  Checking for updates to latest version...
  Cannot write to /usr/local/bin (permission denied).
  Elevating via sudo — you may be prompted for your password.
  Password: <user types>
  Successfully updated from <current> to <latest>
  ```
  Exits 0. `neo4j-cli --version` reflects the new tag.
- [ ] Same command against a writable install location (`~/bin/neo4j-cli`, `INSTALL_DIR=$HOME/bin`) performs the in-place swap with NO sudo prompt, NO download change, NO behavioral diff vs the pre-PRD shipped behavior.
- [ ] Same command against `/usr/local/bin/neo4j-cli` from a non-TTY shell (`</dev/null`) prints the "Re-run with sudo: …" hint, exits non-zero, no archive download.
- [ ] Same command on linux (Ubuntu 22.04 + Alpine) — both elevation and direct paths exercised — behaves identically to darwin.
- [ ] Same command on WSL Ubuntu — `/usr/local/bin/neo4j-cli` triggers elevation through `wsl.exe`'s interactive shell; password prompt works; swap completes.
- [ ] Cobra usage block is NOT printed for any swap-time error from `update` or any newer-available branch from `update check`. Flag-parsing misuse (`neo4j-cli update --bogus`) still shows usage.
- [ ] `make fmt-check && make test && make lint` clean on the CI matrix (ubuntu / macos / windows).
- [ ] `make generate-check` clean — bundle regen committed alongside any cobra Long/Short string change (none planned, but the gate runs either way).
- [ ] Changie entry under `.changes/unreleased/` with kind `Patch` for the user-visible fix.

## Files Affected

- `neo4j-cli/internal/subcommands/update/update.go` — Fix 1 (check branch), Fix 2 (SilenceUsage), Fix 3 (sentinel error handling + re-run hint construction)
- `neo4j-cli/internal/subcommands/update/swap.go` — Fix 3 (planSwap, elevatedSwap, six new seams, Swap signature gains `stderr io.Writer`)
- `neo4j-cli/internal/subcommands/update/check_test.go` — flip three `require.Error` to `require.NoError`; add stdout-shape assertions
- `neo4j-cli/internal/subcommands/update/update_test.go` — flip four `require.Error` cases at lines 183/213/553/995; add sentinel-recognition coverage; update `swapFn` fake signatures to match the new `io.Writer` arg
- `neo4j-cli/internal/subcommands/update/swap_test.go` — six new seam helpers; elevation-path table-driven tests
- `.changes/unreleased/neo4j-cli-Patch-*.yaml` — changie entry
- `neo4j-cli/internal/skill/bundle/...` — regenerated only if Long/Short strings change (not planned)

## Out of Scope

- Non-zero exit code from `update check` when newer version is available (user decision: exit 0).
- A `--no-elevate` flag.
- Migrating existing `/usr/local/bin` installs to `~/.local/bin`.
- Changes to `--force`, `--version`, `--pre-releases`, or install-method passthrough behaviors.
- Custom sudo prompt text (`-p`).
- Detecting and special-casing sudo `requiretty` configurations.
- Any change to JSON / table / toon output shape — plain-text only.
- Refactoring the install script's sudo logic.
- Re-execing the entire command under sudo (only the install step is elevated).

## Open Questions

None — every UX choice resolved in the source plan's clarifying-question phase.
