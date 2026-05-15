<!-- BEGIN GENERATED: AGENTS-MD -->

# AGENTS.md

Learnings and patterns for future agents working on this project.

## Feedback Instructions

TEST COMMANDS: [`make test`]
BUILD COMMANDS: [`make build`, `make run-neo4j`]
LINT COMMANDS: [`make lint`]
FORMAT COMMANDS: [`make fmt-check`] — runs `gofmt -l .` and fails on any output. `make fmt` rewrites silently and is NOT a gate; use `make fmt-check` to verify. CI's golangci-lint v2 includes `gofmt` as a formatter and will fail the build on unformatted code.
LICENSE CHECK: [`make license-check`]

**Always run `make test`, `make fmt-check`, AND `make lint` as final gates before marking any task or plan complete.** All tests must pass, no file may need gofmt, and lint must be clean — a build that compiles but has failing tests, unformatted code, or lint errors is not done. `make fmt-check` is the local equivalent of CI's gofmt linter, so drift fails before the push instead of after.

## Cobra Command Layout

The repo follows a strict one-file-per-leaf cobra layout. Every command tree under `neo4j-cli/aura/internal/subcommands/<resource>/` and `common/skill/` follows it. Mirror it for any new command tree:

- **Parent file `<resource>.go`** — defines `NewCmd(cfg, ...) *cobra.Command`, registers persistent flags, calls `cmd.AddCommand(newXxxCmd(cfg, ...))` for each leaf. Keep it small (≤80 lines).
- **One file per leaf action `<action>.go`** — defines a private constructor like `newInstallCmd(cfg, ...) *cobra.Command` containing the leaf's flags + `RunE`. No leaf bodies inlined into the parent.
- **Colocated tests `<action>_test.go`** — tests for each leaf live next to its source.
- **Shared test helpers** in `<resource>_helpers_test.go` (or similar) when needed.

Examples: `neo4j-cli/aura/internal/subcommands/instance/{instance.go, list.go, list_test.go, get.go, delete.go, ...}` and `common/skill/{skill.go, install.go, remove.go, list.go, check.go, helpers.go}`.

Don't inline multiple leaves in the parent. Don't name the parent `command.go` — name it after the resource so `grep <resource>.go` finds it. Adding a new leaf = new `<action>.go` + `<action>_test.go`, plus one `cmd.AddCommand(...)` line in the parent.

See [`.agents/cobra.md`](.agents/cobra.md) for Cobra flag access and flag precedence gotchas.

## Project Overview

PRIMARY LANGUAGES: [Go]

Neo4j CLI (`neo4j-cli`) is a command-line tool for interacting with Neo4j.

## Build System

BUILD SYSTEMS: [Go toolchain, Makefile, golangci-lint, GoReleaser, changie]

See [`.agents/build.md`](.agents/build.md) for full details.

- Local build: `make build` (produces `bin/neo4j-cli`)
- Local run (no build): `make run-neo4j`
- Release build (current platform, ldflags baked in): `make snapshot` (uses goreleaser, outputs to `bin/`)
- npm publish dry-run (template + ordering check): `make npm-publish-dry`. Works against empty `dist/` because `publish.sh --dry-run` stubs missing platform binaries with a 1-byte placeholder; run `make snapshot` first if you want real binaries packed. Real-binary path (CI) still hard-errors on missing binaries — the stub is dry-run-only.
- All `.go` files must start with the Neo4j copyright header (enforced in CI via `addlicense`)
- PRs require a changelog entry via `make changelog` **only for user-facing changes** (new features, bug fixes, behaviour changes visible to CLI users). Internal changes (CI/CD workflow fixes, build scripts, code refactors with no visible effect) do not need changelog entries. Use `changie new --projects neo4j-cli --kind <kind> --body <body>` for non-interactive use.

## Testing Framework

TESTING FRAMEWORKS: [Go testing, testify, afero (in-memory FS)]

See [`.agents/testing.md`](.agents/testing.md) for full details and output testing notes.

- Tests are colocated with source as `*_test.go` files
- Run with `go test ./...`; CI runs on ubuntu, windows, and macos
- Mock HTTP server and filesystem helpers live in `neo4j-cli/aura/internal/test/testutils/`
- `neo4j-cli/` (the super-CLI package) has no test files; this is a pre-existing gap
- **Prefer table-driven tests** (`for _, tc := range []struct{...}{...}`) when writing new tests — they reduce duplication and make it easy to add cases later
- **Name test files per command**, not per package — use `get_test.go`, `set_test.go`, `list_test.go` mirroring the source files; put shared helpers in `helpers_test.go`. Avoid aggregating all tests in a single `config_test.go`.
- **Never use `afero.NewOsFs()` in query package tests** — the dev machine has real credentials at `~/Library/Preferences/neo4j/cli/credentials.json`. Tests using a real FS will fail if any dbms credential is stored. Always use `testfs.GetTestFs(`{"format":"json"}`, "{}")` (empty credentials) even when testing dotenv walk-up; write the dotenv into the memFs at `filepath.Join(t.TempDir(), ".env")` and `t.Chdir(tmp)` so `os.Getwd()`+`cfg.Aura.Fs()` finds it.

## Architecture

ARCHITECTURE PATTERN: Cobra command tree — one file per leaf command, directory structure mirrors command hierarchy

See [`.agents/architecture.md`](.agents/architecture.md) for architecture details and toon-go notes.

One binary is produced:
- **`neo4j-cli`** — single CLI entrypoint (`neo4j-cli/main.go`); the Aura command tree lives under the `aura` subcommand.

```
neo4j-cli/
  app/app.go               # neo4j-cli cobra tree builder (NewCmd, Version) — importable
  main.go                  # thin entrypoint; mounts aura subcommand as "aura"
  internal/skill/          # per-binary skill template (bundle, description.txt, additions.md, gen/)
  aura/
    cmd/main.go            # historical standalone entrypoint (compiled but not built/shipped)
    aura.go                # Root command, registers subcommands
    internal/
      api/                 # HTTP client for Neo4j Aura REST API
      flags/               # Custom reusable flag types
      output/              # JSON, table, and toon rendering
      skill/               # per-binary skill template (mirrors neo4j-cli/internal/skill)
      subcommands/         # One directory per resource, one file per action
        instance/, tenant/, credential/, config/,
        deployment/, dataapi/graphql/, graphanalytics/,
        import/, customermanagedkey/
common/
  clicfg/                  # Config, credentials, project state (OS-specific paths)
  clierr/                  # Shared error types
  skill/                   # Shared agent-skill logic (catalog, render, installer, cobra wrapper)
```

Agent-skill subsystem: `common/skill/` holds the binary-agnostic logic (agent catalog, path expansion, bundle render, install/remove/list/check, cobra wrapper). The neo4j-cli binary has its own `neo4j-cli/internal/skill/` template (`embed.go` + `description.txt` + `additions.md` + `gen/main.go` + committed `bundle/`). Adding a new standalone CLI in the future = copy the template, edit `description.txt`/`additions.md`/`gen/main.go` import, mount `skill.NewCmd(cfg, binskill.Bundle, "<newcli>")`, run `go generate`. No edits to `common/skill/`. See `CONTRIBUTING.md` "Generated content" for the full workflow.

Key CLI conventions (see `CONTRIBUTING.md`):
- Singular nouns for commands (`instance`, not `instances`)
- `<resource> <action>` form (`instance list`, not `list-instance`)
- One positional argument max; extras become flags
- `--format json|table|toon` for all read commands
- `--wait` flag for async operations
- Follow CLI best practices from https://clig.dev/ — source at https://github.com/cli-guidelines/cli-guidelines/blob/main/content/_index.md (fetch the raw markdown for token-efficient reference)

## Deployment

DEPLOYMENT STRATEGY: GitHub Releases via GoReleaser, triggered by `CHANGELOG-neo4j.md` updates on `main`

See [`.agents/deployment.md`](.agents/deployment.md) for changie workflow, release workflow, and GoReleaser gotchas.

- `changie` batches changelog entries and opens a release PR automatically (single project: `neo4j-cli`)
- Merging a release PR triggers GoReleaser to publish binaries for linux/windows/darwin (amd64 + arm64)
- macOS binaries are code-signed and notarized
- The release version comes from `GORELEASER_CURRENT_TAG` (set by the GoReleaser action)
- `release-notes.md` is generated with a `## Changes` section (neo4j-cli changelog body) before GoReleaser runs
- GoReleaser `brews.post_install:` takes the method **body** only — GoReleaser wraps it in `def post_install ... end` automatically. Do NOT include the `def`/`end` yourself or you get a nested def in the formula.
- `make snapshot` (single-target) does **not** generate the Homebrew formula. To validate formula output locally run `GORELEASER_CURRENT_TAG=dev goreleaser release --snapshot --skip=publish --clean` (full multi-platform build) and inspect `dist/homebrew/Formula/neo4j-cli.rb`.

## Makefile Notes

- `make generate-check` is `git diff --exit-code` after `go generate`. It flags ANY tracked-file diff, including unrelated edits in the working tree (e.g. a `.plans/tasks-*.yml` status flip). When validating "did this task introduce bundle drift?", inspect the diff output — only `internal/skill/bundle/**` paths matter for the gate. The hone harness's mid-task in_progress flip will always show up here; ignore it.
- `license-check` target uses `$(GOPATH)/bin/addlicense` (not bare `addlicense`) — GOPATH/bin may not be on PATH
- `license-check` requires a Unix shell (`find` + `xargs`); won't work natively on Windows
- `make generate` runs `go generate ./...`; `make generate-check` runs generate then `git diff --exit-code` (CI gate). Wired in `.github/workflows/test.yml` between Build and Lint, runs on full OS matrix.
- Drift sim: editing a bundle file directly to test generate-check is futile — `go generate` overwrites it. Mutate a real cobra-tree input (e.g. a Short string in `app.go`) to simulate stale-bundle detection.
- Changing `ValidFormatValues` in `common/clicfg/clicfg.go` affects the `--format` flag help text, which is embedded in skill bundle reference docs. Run `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...` after any such change; `TestGenerator_RoundTrip` is the gate that catches stale bundles.
- Adding any new command to the neo4j-cli command tree (including sub-sub-packages like `credential/dbms/`) also requires `go generate ./neo4j-cli/internal/skill/...` — otherwise `TestGenerator_RoundTrip` fails with a "references/credential.md differs" message. Run this immediately after any command-tree change before the test gate.
- `aura-client` credential cmd lives in TWO places: `neo4j-cli/internal/subcommands/credential/credential.go` (user-facing `credential aura-client add`, feeds neo4j-cli skill bundle) AND `neo4j-cli/aura/internal/subcommands/credential/add.go` (standalone aura `credential add`, feeds aura standalone skill bundle). Any flag/Long change must hit BOTH for behavioural symmetry AND both `go generate ./neo4j-cli/internal/skill/...` and `go generate ./neo4j-cli/aura/internal/skill/...` must run, otherwise the OTHER tree's `TestGenerator_RoundTrip` fails.

## Changie Notes

- The repo uses changie's `projects:` mechanism even though only one project (`neo4j-cli`) is configured — version files live at `changesDir/<key>/v*.md` (e.g., `.changes/neo4j-cli/v1.7.0.md`) because changie appends the project key to `changesDir` automatically.
- All change files share the unreleased directory at `.changes/unreleased/` and are tagged with a `project:` field inside the YAML.
- `changie latest --project neo4j-cli` outputs `neo4j-cliv1.7.0` (project key prepended with no separator by default) — shell workflows must strip the `neo4j-cli` prefix (e.g., `sed 's/^neo4j-cli//'`).
- `ProjectsVersionSeparator` in `.changie.yaml` controls whether the prefix has a separator (`neo4j-cli-v1.7.0` if set to `-`); leave unset for the current `neo4j-cliv1.7.0` shape.
- This repo uses kind labels `Major`, `Minor`, `Patch` (not `added`/`feat`) — check `.changie.yaml` `kinds:` before using `--kind`.
- If changie isn't installed locally, hand-author YAML files under `.changes/unreleased/` named `neo4j-cli-<Kind>-<YYYYMMDD>-<HHMMSS>.yaml` with fields `project / kind / body / time` (single-quoted body, RFC3339 time).

## Repo Doc Notes

- `CLAUDE.md` is a symlink to `AGENTS.md` (`ls -la` confirms). Edit `AGENTS.md` once — both surfaces update. Don't write to `CLAUDE.md` directly.
- Contributor-facing workflows (e.g. `make generate` / add-new-CLI procedure) live in `CONTRIBUTING.md` "Development" subsections. AGENTS.md Architecture orients readers and links to CONTRIBUTING.md for the procedure rather than duplicating it.

## Website (neo4j.sh)

The public marketing/install site at https://neo4j.sh is served from the `gh-pages` branch, NOT from `main`. Four files are served:

- `gh-pages/index.html` — landing page (quickstart + examples toggles, CLI vs agentic mode).
- `gh-pages/llms.txt` — LLM-discoverable site summary.
- `gh-pages/install.sh` — POSIX install script (curl | sh target).
- `gh-pages/install.ps1` — Windows install script.

The site is **prompt-driven**, not generated from Go code in `main`. The source of truth for content updates is `.github/prompts/website-update.md`; running that prompt against an agent rewrites `gh-pages/index.html` (and adjacent files) in place.

Update workflow:

1. `git worktree add gh-pages gh-pages` from the repo root to get a working tree on the `gh-pages` branch.
2. Run `.github/prompts/website-update.md` against an agent inside that worktree.
3. Review the resulting `gh-pages` diff (especially `index.html`) before committing.
4. Commit and push on the `gh-pages` branch — GitHub Pages serves it automatically.

Rendering invariants (e.g. `> ` agent-prompt prefix, single `→ loading skill neo4j-cli` line per agent prompt, `:not(.cli-mode)` dim sweep over command content) are enforced inside the prompt. Do **not** hand-edit `gh-pages/index.html` in ways that violate them — re-run the prompt instead so the invariants stay encoded in one place.

## Repo Layout Notes

See [`.agents/repo-layout.md`](.agents/repo-layout.md) — gotchas around skill subsystem layout, embed roots, mount points, and bundle regen.

- Aura SKILL.md's Global Flags table lists only `--rw` (not `--format`); `--format` is registered per-subcommand via `RegisterOutputFlag` and surfaces in `references/<cmd>.md`. neo4j-cli SKILL.md DOES list `--format` at root because it's bound globally there. When a flag-description change needs to land in bundle/SKILL.md Global Flags, expect it on the neo4j-cli side only — aura side propagates through references/*.md.

## Hermetic Test Notes

See [`.agents/hermetic-tests.md`](.agents/hermetic-tests.md) — env/path expansion, TTY seams, `httptest` cancellation timeouts, cobra completion injection, gate-test repo walking.

## Windows CI Gotchas

See [`.agents/windows-ci.md`](.agents/windows-ci.md) — path-separator handling in `expandPath` helpers and LF-pinning of committed `.md` / golden / bundle files via `.gitattributes`.

## Installer Script Testing Notes

- Bats-core tests for `install-neo4j-cli.sh` live in `distribution/installation-scripts/tests/install-neo4j-cli.bats`. Run with `bats distribution/installation-scripts/tests/` (install bats-core via `brew install bats-core` or `apt-get install bats`).
- The installer is a monolith; tests stub ALL external commands (curl, tar, sha256sum, shasum, uname, sudo) via a `STUBS_DIR` prepended to `PATH`. The curl stub for the checksums file must emit a fake sha256 line matching the archive filename so `grep <archive> checksums.txt | sha256sum -c` succeeds.
- The tar stub creates a recording `neo4j-cli` binary (using `STUB_CALLS` env var) — this is critical because the installer `mv`s the tar-extracted binary to `INSTALL_DIR`, overwriting any pre-seeded stub. The recording logic must be embedded in what tar creates.
- Always run `shellcheck` on `.bats` files; use `local stub_path=...` to avoid SC2097/SC2098 when constructing a `PATH=...` prefix that references other variables in the same assignment.

## npm Distribution Notes

See [`distribution/npm/README.md`](distribution/npm/README.md).

## PyPI Distribution Notes

See [`distribution/pypi/README.md`](distribution/pypi/README.md) for maintainer-facing channel docs (install commands, version mapping, recovery, auth). See [`.agents/pypi.md`](.agents/pypi.md) for workflow-side gotchas (`workflow_run` shape, `go-to-wheel` invocation, PEP 440 normalisation, heredoc indentation).

## golangci-lint Notes

- Version installed: v2.11.4 (via Homebrew)
- golangci-lint v2 requires `version: "2"` at the top of `.golangci.yml`
- In v2, `gofmt` is a **formatter** (not a linter); put it under `formatters.enable`, not `linters.enable`
- Use `linters.default: none` to disable auto-enabled defaults (e.g. `ineffassign`) and run only explicitly listed linters
- Config lives at `.golangci.yml` in repo root
- In CI, `golangci/golangci-lint-action@v6` is used as the lint step — it installs, caches, and runs golangci-lint using `.golangci.yml`. This is equivalent to `make lint`. Renovate will pin the SHA.
- If `make lint` reports issues whose paths point to a non-existent worktree (e.g. `.claude/worktrees/agent-…`) the cache is stale; run `golangci-lint cache clean` once and re-run — issues evaporate when the source path no longer exists.

## Credentials Storage Notes

See [`.agents/credentials.md`](.agents/credentials.md) — `load()` re-wiring of `onUpdate` callbacks, sensitive-field omission, omitempty-vs-printable patterns, cross-type validation order, and test fixture seeding for the `Aura` / `Dbms` / `Embed` credential types.

## query Subsystem Notes

See [`.agents/query.md`](.agents/query.md) for Bolt driver, execution, credential integration, embedding-provider plumbing, and local verification gotchas.

## Cobra Help / Skill Bundle Rendering Notes

- `common/skill/render/render.go:235` strips a cobra command's `Example` field via `strings.TrimSpace` before wrapping it in a fenced code block. The leading 2-space "cobra convention" indent is therefore stripped from the FIRST line only and preserved on subsequent lines, producing a ragged block. Write multi-line Examples with NO leading indent so the rendered bundle stays flush-left and consistent.
- Adding/changing a `Long` field on any cobra command in `neo4j-cli/internal/subcommands/credential/...` or `neo4j-cli/query/...` requires `go generate ./neo4j-cli/internal/skill/...` to refresh the bundle, otherwise `TestGenerator_RoundTrip` (the gate in `make test`) fails with a "references/<sub>.md differs" diff.
- `make generate-check` is `git diff --exit-code` after `go generate ./...`; on a working tree with uncommitted source-side help-text edits it WILL fail (the gate is meaningful only against a clean tree, i.e. CI). Locally: commit the source AND the regenerated bundle in the same commit, then re-run.
- `--param` flag Usage on the `query` parent now mentions the `:embed` modifier (`key:embed=<text>`) — keeping the modifier discoverable in `--help` was cheaper than a separate flag. The full rule (JSON-array rejection, empty-text accepted) lives in README and the bundle additions.md, not the flag Usage.
- README's "Aura API Credentials" example uses `credential aura-client add` (canonical neo4j-cli path), NOT the standalone-aura `aura credential add` form. The standalone aura binary is no longer built/shipped, so README must lead with commands the shipped binary actually has.
- Skill bundle `description.txt` (frontmatter description) is single-paragraph, ≤1024 chars, third-person. When adding new top-level capability to it, list every credential subtree explicitly ("Aura, Neo4j connection (dbms), and embedding-provider credentials") rather than collapsing them — the agent-side trigger phrasing matches user wording better when each subtree is named.
- Every runnable cobra command reachable from `app.NewCmd(cfg)` must have a non-empty flush-left `Example:` field (≥3 invocations, each preceded by a `# comment` line, blank-line separators, `neo4j-cli` prefix, `--rw` on write invocations, at least one `--format json` on read invocations). Enforced by `TestAllLeafCommands_HaveExamples` in `neo4j-cli/internal/subcommands/agentcontext/agentcontext_test.go` — failure message names the full command path.

## Agent Context Notes

See [`.agents/agent-context.md`](.agents/agent-context.md) — `neo4j-cli agent-context` reflects the live cobra tree, with hand-coded `schemaVersion` / `exitCodes` / `errorCodes` / `asyncFlag` in `agentcontext/build.go`.

## PowerShell Installer Test Notes

- **Use `.cmd` stubs, not `.ps1`**: When testing PowerShell installer scripts that call `& neo4j-cli`, put the stub in a `.cmd` file (not `.ps1`). Windows resolves bare `& neo4j-cli` to `.cmd`/`.bat` before `.ps1` when scanning PATH — a `.ps1` stub is often silently skipped.
- **Write subprocess commands to a temp `.ps1` file**: Pass the wrapper script via `pwsh -File <path>` rather than `pwsh -Command <big-string>`. This avoids escaping backslashes, single-quotes, and `$` signs in nested here-strings.
- **Pass paths via env vars**: When stub scripts need to write to a file (e.g. a calls-recorder), pass the path via an environment variable (e.g. `$env:NEO4J_CALLS_FILE`) rather than embedding it as a literal string — avoids escaping backslashes on Windows paths.
- **CRLF for `.ps1` files**: Any `.ps1` file in `distribution/installation-scripts/` must have Windows CRLF line endings. After writing with any tool on macOS/Linux, convert with `python3 -c "..."` (unix2dos not available by default on macOS). Verify with: `python3 -c "import sys; ... count b'\\r\\n'"`.
- `pwsh` is not installed by default on macOS dev machines — Pester tests are gated on `windows-latest` CI. Validate syntax locally by reading the file; don't block task completion on local Pester execution.

---

_This AGENTS.md was generated using agent-based project discovery._

<!-- END GENERATED: AGENTS-MD -->
