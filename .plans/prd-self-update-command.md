# PRD: `neo4j-cli update` self-update command

## Overview

Add a top-level `neo4j-cli update` subcommand that compares the running binary's baked-in version against the latest GitHub release at `neo4j-labs/neo4j-cli` and, when newer, downloads + atomically swaps the binary in place. When the binary lives under a known package-manager prefix (Homebrew, npm-global, pipx, uv tool), the command refuses to overwrite and prints the channel-correct upgrade command instead.

The command surfaces in the embedded skill bundle so AI agents can run "update neo4j-cli" without the user having to remember which install channel they used.

Source plan: `/Users/oskarhane/.claude/plans/i-d-lie-ato-add-binary-dream.md`.

## Goals

- One command that updates the CLI regardless of how the user is staying current today.
- Default to **stable** semver tags; opt into prereleases with `--pre-releases`.
- Safe-by-default: SHA256-verified downloads, atomic file swap, refusal to clobber package-manager-managed binaries.
- Stdlib-only — no new third-party Go deps. (`golang.org/x/mod` already an indirect dep, promoted to direct.)
- Discoverable from the skill bundle so agents pick it up automatically after `go generate`.

## Non-Goals

- **No package-manager passthrough automation.** When detected, the command tells the user the right command and exits — it does not shell out to `brew`, `npm`, `pipx`, or `uv`.
- **No downgrade flow** beyond `--version <tag>`. Specifying an older tag works mechanically but no extra UX is built around it.
- **No GPG / Sigstore verification.** SHA256 checksum from the same release is the trust anchor, matching `distribution/installation-scripts/install-neo4j-cli.sh`.
- **No background / scheduled update checks.** The command runs only when invoked.
- **No update of the skill bundles installed in agent dirs.** That stays under `neo4j-cli skill install`.
- **No new release artifacts.** Reuses the existing GoReleaser archive layout (`neo4j-cli_<VER>_<TitleOS>_<unameArch>.tar.gz`/`.zip` + `_checksums.txt`).
- **No Windows MSI / installer integration.** In-place swap via rename-to-`.old` dance only.

## Requirements

### Functional Requirements

- **REQ-F-001** — Mount `update.NewCmd(cfg)` as a top-level subcommand on the `neo4j-cli` cobra tree, alongside `aura`, `credential`, `config`, `query`, `skill` (`neo4j-cli/app/app.go:42-46`).
- **REQ-F-002** — Resolve current version from `app.Version`. When `Version == "dev"`, print "running a dev build, nothing to update" and exit 0 without contacting GitHub.
- **REQ-F-003** — Fetch releases via `GET https://api.github.com/repos/neo4j-labs/neo4j-cli/releases?per_page=30`. Skip drafts. Order is API-default (created_at desc).
- **REQ-F-004** — Honor `GH_TOKEN` / `GITHUB_TOKEN` env when set: send `Authorization: Bearer <token>` for ratelimit relief. Never echo the token in error messages or logs (redact).
- **REQ-F-005** — Filter releases by `--pre-releases` using `golang.org/x/mod/semver.Prerelease(tag)`: empty string ⇒ stable, non-empty ⇒ prerelease. Default (flag absent) keeps stable only.
- **REQ-F-006** — When stable-only filtering yields zero releases (current state of the project), exit 0 with a friendly message: "no stable release published yet — pass `--pre-releases` to track alpha/beta/rc tags."
- **REQ-F-007** — `--version <tag>` skips discovery and uses that tag. Validate via `semver.IsValid`; reject tags containing `..`, `/`, `\`, NUL, or shell metacharacters before any URL interpolation.
- **REQ-F-008** — Compare current vs target with `semver.Compare`. `< 0` ⇒ update available; `== 0` ⇒ already up-to-date; `> 0` ⇒ refuse unless `--version <tag>` is explicitly set (treat as downgrade-on-request).
- **REQ-F-009** — Detect package-manager-managed binaries by resolving `os.Executable()` (incl. symlinks via `filepath.EvalSymlinks`) and matching the absolute path against:
  - Homebrew: `/opt/homebrew/`, `/usr/local/Cellar/`, `/home/linuxbrew/.linuxbrew/`. Print `brew upgrade neo4j-cli`.
  - npm: any path containing `/node_modules/@neo4j-labs/cli/`. Print `npm i -g @neo4j-labs/cli@latest` (also list pnpm/yarn equivalents).
  - pipx: `~/.local/pipx/venvs/neo4j-cli/`, `~/.local/share/pipx/`, or `~/.local/bin/neo4j-cli` symlinking into a pipx venv. Print `pipx upgrade neo4j-cli`.
  - uv tool: `~/.local/share/uv/tools/neo4j-cli/`. Print `uv tool upgrade neo4j-cli`.
- **REQ-F-010** — On package-manager match: exit 0 with the channel-correct hint and **do not download anything**. `--force` bypasses the check.
- **REQ-F-010a** — Alongside the package-manager upgrade command, the message also shows how to switch to a direct install (so future `neo4j-cli update` invocations work in place). Show:
  - The install script: `curl -sSfL https://neo4j.sh/install.sh | bash`.
  - A "before installing" line recommending the matching uninstall command for cleanliness, marked as optional: `brew uninstall neo4j-cli` / `npm uninstall -g @neo4j-labs/cli` / `pipx uninstall neo4j-cli` / `uv tool uninstall neo4j-cli`. The note clarifies it's only required when the package-manager binary is earlier on `PATH` than the install-script destination (`~/.local/bin/` by default) — otherwise PATH order picks up the new binary automatically.
  - Format: two short blocks under the same hint, e.g.:
    ```
    Installed via Homebrew. To upgrade in place, run:
      brew upgrade neo4j-cli

    To switch to a self-managed install (so 'neo4j-cli update' works directly):
      brew uninstall neo4j-cli   # optional — only needed if PATH still resolves the brew binary
      curl -sSfL https://neo4j.sh/install.sh | bash
    ```
- **REQ-F-011** — `--check` mode: print current + latest, set `updated: false` in JSON output. Exit code 0 if up-to-date, 1 if newer exists. Never download.
- **REQ-F-012** — Build asset URL from the GoReleaser pattern: `https://github.com/neo4j-labs/neo4j-cli/releases/download/<TAG>/neo4j-cli_<VER_NO_V>_<TitleOS>_<unameArch>.<ext>` with OS map (`linux→Linux`, `darwin→Darwin`, `windows→Windows`), arch map (`amd64→x86_64`, `386→i386`, `arm64→arm64`), and ext (`.tar.gz` for Linux/Darwin, `.zip` for Windows). Mirror `distribution/installation-scripts/install-neo4j-cli.sh:87-101`.
- **REQ-F-013** — Download the matching `neo4j-cli_<VER_NO_V>_checksums.txt` from the same release tag and compute SHA256 of the downloaded archive in-memory; abort if the computed hash doesn't match the `<archive>` row in `checksums.txt`. **No swap may occur if checksum verification has not succeeded.**
- **REQ-F-014** — Reject archive entries whose cleaned path escapes the extraction dir (zip-slip / tar-slip guard): `filepath.Clean` + check `strings.HasPrefix(rel, "../") == false`. Reject symlinks, hardlinks, devices — only regular files allowed.
- **REQ-F-015** — Atomic swap: write the extracted binary to `<current>.new` in the same directory as the running binary (so `os.Rename` stays on the same filesystem), `os.Chmod 0755`, then `os.Rename(<new>, <current>)`. On Windows, first `os.Rename(<current>, <current>.old)`, then `os.Rename(<new>, <current>)`. Best-effort remove any pre-existing `.old` at start of the swap.
- **REQ-F-016** — On any error during swap, attempt to restore the original binary and surface a clear error to the user (not just exit 1).
- **REQ-F-017** — Inherit `--format json|table` from the root persistent flag. Default plain-text output matches:
  ```
  Current version: v0.1.0-alpha.9
  Checking for updates to latest version...
  Successfully updated from v0.1.0-alpha.9 to v0.1.0-alpha.10
  ```
- **REQ-F-018** — JSON output shape: `{"current": "v...", "latest": "v...", "updated": bool, "check": bool, "channel": "stable"|"pre-release", "install_method": "binary"|"homebrew"|"npm"|"pipx"|"uv"}`. Use `output.PrintBodyMap` from `common/output/output.go` for consistency.
- **REQ-F-019** — Update `README.md:3-12` Installation section with a "Self-update" subsection covering `neo4j-cli update`, `--check`, `--pre-releases`, `--version`, `--force`, and the package-manager-passthrough behavior.
- **REQ-F-020** — Update `neo4j-cli/internal/skill/description.txt` (third-person, ≤1024 chars total) and `neo4j-cli/internal/skill/additions.md` (one-paragraph "Updating the CLI" section) to mention the new command. Regenerate via `go generate ./neo4j-cli/internal/skill/...` so `bundle/SKILL.md` and `bundle/references/update.md` pick up the new command — `TestGenerator_RoundTrip` is the gate.
- **REQ-F-021** — Add a `Minor` changelog entry via `changie new --projects neo4j-cli --kind Minor --body "..."`.

### Non-Functional Requirements

- **REQ-NF-001 (Cobra layout)** — One file per leaf per `CLAUDE.md` "Cobra Command Layout". Since `update` is a single command (no subcommands), it's `update.go` only; helpers split into `release.go`, `install_method.go`, `swap.go` for testability. Tests colocated as `*_test.go`.
- **REQ-NF-002 (Stdlib only)** — No new external dependencies. Use `net/http`, `archive/tar`, `archive/zip`, `compress/gzip`, `crypto/sha256`, `encoding/json`. Promote `golang.org/x/mod` from indirect to direct require (auto via `go mod tidy`).
- **REQ-NF-003 (Cross-platform)** — Builds and runs on Linux, macOS, Windows (amd64 + arm64). Windows-locked-file swap via `.old` rename. Path separators handled via `filepath.Join` / `filepath.FromSlash` per AGENTS.md "Windows CI Gotchas".
- **REQ-NF-004 (Hermetic tests)** — Use `testfs.GetTestFs(...)` (NOT `afero.NewOsFs()`) per AGENTS.md. Use `httptest.NewServer` with safety timeouts on handlers and 5s test-side fallbacks. Table-driven tests per AGENTS.md preference.
- **REQ-NF-005 (Test seams)** — Package-level `var fooFn = func() ...` seams for: HTTP API base URL, download base URL, `os.Executable()`, `runtime.GOOS`, `runtime.GOARCH`. Production fills with real impls; tests swap via `withX(t, val)` helpers with `t.Cleanup`.
- **REQ-NF-006 (Security)** — Pass the `golang-security` skill review with no HIGH / MEDIUM findings. See "Security Considerations" below.
- **REQ-NF-007 (CI gates)** — Pass `make test`, `make fmt-check`, `make lint`, `make generate-check`, `make license-check`.
- **REQ-NF-008 (License header)** — All new `.go` files start with the Neo4j copyright header (CI enforces via `addlicense`).
- **REQ-NF-009 (LF line endings)** — Any new committed `.md` / golden / bundle files pinned to LF via `.gitattributes` per AGENTS.md "Windows CI Gotchas". Existing rules already cover `**/internal/skill/bundle/**` and `**/internal/skill/additions.md`.

## Technical Considerations

### Architecture & Integration

- **New package**: `neo4j-cli/internal/subcommands/update/` — sibling of `credential/`, `config/`. Exports `NewCmd(cfg *clicfg.Config) *cobra.Command`.
- **Mount point**: `neo4j-cli/app/app.go` line ~45, single `cmd.AddCommand(update.NewCmd(cfg))` line.
- **Output**: reuse `common/output/output.go` `ResolveOutput` + `PrintBodyMap`. Plain-text default branch implemented inline (the example output isn't a body-map shape).
- **Version source**: `github.com/neo4j/cli/neo4j-cli/app.Version` (already imported pattern).
- **Skill bundle**: auto-regenerated. No hand-editing of `bundle/`.

### Why these choices (vs. alternatives)

| Decision | Chosen | Alternative | Why |
| --- | --- | --- | --- |
| Pkg-mgr handling | Detect & instruct | Always in-place | Avoids breaking brew/pipx/uv bookkeeping. `--force` is the escape hatch. |
| Swap mechanism | Stdlib download + atomic rename | `github.com/minio/selfupdate` | No new deps; behaviour fully visible/auditable in this repo. |
| Verification | SHA256 from `_checksums.txt` | GPG / cosign | Matches existing install script trust model; no new key infra. |
| Prerelease detection | Semver `-suffix` (via `golang.org/x/mod/semver`) | GitHub `prerelease` boolean | All current releases set `prerelease: true` in goreleaser config, so the GH boolean is uninformative. |
| Asset URL building | Mirror `install-neo4j-cli.sh` mapping | Iterate over GH release `assets[]` JSON | Deterministic and avoids extra parsing; falls back fine because the URL is stable. |

### Security Considerations (full audit list)

The `golang-security` skill is invoked as a final gate. Surfaces to validate:

1. **TLS** — Default `http.Client`. Never set `InsecureSkipVerify`.
2. **Checksum enforcement** — No code path may rename-into-place if SHA256 verification has not returned success.
3. **Zip-slip / tar-slip** — `filepath.Clean` + prefix check on every archive entry. Reject symlinks/hardlinks/devices.
4. **File permissions** — Binary written `0755`. Temp file `0600` until rename. Same directory as target (no `/tmp` cross-device risk).
5. **Symlink resolution** — `filepath.EvalSymlinks(os.Executable())` before swap so we replace the real binary, not a user's launcher symlink.
6. **HTTP redirects** — Cap at Go's default 10. Pin redirect host allowlist to `github.com`, `objects.githubusercontent.com`, `api.github.com`.
7. **Input validation on `--version <X>`** — `semver.IsValid` + reject `..`, `/`, `\`, NUL, shell metacharacters before URL interpolation.
8. **Token redaction** — `GH_TOKEN` / `GITHUB_TOKEN` never appears in errors or logs.

### Edge Cases & Failure Modes

| Case | Behavior |
| --- | --- |
| `app.Version == "dev"` | Friendly message, exit 0. No network call. |
| Stable-only filter, no stable releases yet | Exit 0 with "no stable release yet — pass `--pre-releases`" hint. |
| GitHub API rate-limit (HTTP 403) | Suggest setting `GH_TOKEN`; exit 1. |
| Network failure mid-download | Abort, no swap, exit 1 with cause. |
| Checksum mismatch | Abort, no swap, exit 1, cite expected vs got. |
| Archive contains traversal entry | Abort, no swap, exit 1 with offending path. |
| Pkg-mgr detected, no `--force` | Channel hint, exit 0. |
| Already up-to-date | "Already on vX.Y.Z." Exit 0. |
| `--check` and newer available | Print availability. Exit 1 (CI-friendly). |
| Windows: `<current>.old` already present | Best-effort `os.Remove` at start of swap. |
| Cross-device temp file (shouldn't happen given placement) | Wrap swap in restore-on-error defer. |

## Acceptance Criteria

- [ ] `bin/neo4j-cli update --help` shows `--pre-releases`, `--check`, `--version`, `--force`, plus inherited `--format`.
- [ ] `bin/neo4j-cli update --check` on current build (no stable yet) prints "no stable release yet — pass `--pre-releases`" and exits 0.
- [ ] `bin/neo4j-cli update --check --pre-releases` reports the actual latest prerelease tag and exits 1 (newer available).
- [ ] `bin/neo4j-cli update --check --pre-releases -f json` emits valid JSON matching the documented shape.
- [ ] `bin/neo4j-cli update --pre-releases` against a sandbox copy successfully replaces the binary; `<copy> --version` shows the new tag.
- [ ] A binary placed under `/opt/homebrew/bin/` (or symlinked from a `Cellar/`) prints the `brew upgrade neo4j-cli` hint and does NOT download anything; same with npm / pipx / uv prefixes.
- [ ] The same hint also prints the install-script command (`curl -sSfL https://neo4j.sh/install.sh | bash`) and the matching package-manager uninstall command (marked optional, with the PATH-order rationale).
- [ ] `--force` overrides the package-manager check and proceeds with the in-place swap.
- [ ] Tampered checksum file in a test fixture causes the command to abort before swap, with a clear "checksum mismatch" error.
- [ ] Archive containing a `../escape` entry is rejected with a clear error; no file written outside the target dir.
- [ ] Plain-text output on success matches the reference exactly:
  ```
  Current version: v0.1.0-alpha.9
  Checking for updates to latest version...
  Successfully updated from v0.1.0-alpha.9 to v0.1.0-alpha.10
  ```
- [ ] `README.md` Installation section includes the "Self-update" subsection.
- [ ] `neo4j-cli/internal/skill/description.txt` mentions the update capability (and stays ≤1024 chars).
- [ ] `neo4j-cli/internal/skill/additions.md` includes an "Updating the CLI" paragraph.
- [ ] `neo4j-cli/internal/skill/bundle/references/update.md` exists and is regenerated; `TestGenerator_RoundTrip` passes.
- [ ] `make test`, `make fmt-check`, `make lint`, `make generate-check`, `make license-check` all pass.
- [ ] `golang-security` skill review reports no HIGH / MEDIUM findings on the new package.
- [ ] A `.changes/unreleased/neo4j-cli-Minor-*.yaml` entry exists with a body describing the new command.
- [ ] No new third-party deps in `go.mod` (only `golang.org/x/mod` promoted from indirect to direct).

## Out of Scope

- Auto-update prompts on regular CLI invocation (no "a new version is available" nag at command start).
- Rollback to a previously installed binary (we don't keep prior versions beyond the Windows `.old` artifact).
- GPG / Sigstore / SBOM verification.
- Updating the skill bundles previously installed into agent dirs (covered by `neo4j-cli skill install`, which the user can run after upgrading).
- Update notifications via email / Slack / etc.
- Windows MSI installer integration.
- Mirror / CDN configuration; we always go straight to GitHub release assets.
- Support for forks / custom GitHub orgs (the repo identifier is hard-coded to `neo4j-labs/neo4j-cli`).

## Open Questions

None — clarifying round in plan mode resolved:

1. Pkg-mgr handling = detect & instruct (with `--force` override).
2. Swap mechanism = stdlib download + atomic rename.
3. Output flags = honor `--format`, add `--check` and `--version`.
4. Default `--pre-releases=false` even though no stable releases exist today (correct semver behavior; hint message points users to the flag).
