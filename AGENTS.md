<!-- BEGIN GENERATED: AGENTS-MD -->

# AGENTS.md

Learnings and patterns for future agents working on this project.

## Feedback Instructions

TEST COMMANDS: [`make test`]
BUILD COMMANDS: [`make build`, `make run-aura`, `make run-neo4j`]
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

## Project Overview

PRIMARY LANGUAGES: [Go]

Neo4j CLI (`neo4j-cli`) is a command-line tool for interacting with Neo4j.

## Build System

BUILD SYSTEMS: [Go toolchain, Makefile, golangci-lint, GoReleaser, changie]

See [`.agents/build.md`](.agents/build.md) for full details.

- Local build: `make build` (produces `bin/aura-cli` and `bin/neo4j-cli`)
- Local run (no build): `make run-aura` / `make run-neo4j`
- Release build (current platform, ldflags baked in): `make snapshot` (uses goreleaser, outputs to `bin/`)
- npm publish dry-run (template + ordering check): `make npm-publish-dry`. Works against empty `dist/` because `publish.sh --dry-run` stubs missing platform binaries with a 1-byte placeholder; run `make snapshot` first if you want real binaries packed. Real-binary path (CI) still hard-errors on missing binaries — the stub is dry-run-only.
- All `.go` files must start with the Neo4j copyright header (enforced in CI via `addlicense`)
- PRs require a changelog entry via `make changelog` **only for user-facing changes** (new features, bug fixes, behaviour changes visible to CLI users). Internal changes (CI/CD workflow fixes, build scripts, code refactors with no visible effect) do not need changelog entries. Because `neo4j-cli` bundles all child CLIs, user-facing changes to a child require entries for both — use `changie new --projects <child> --projects neo4j-cli --kind <kind> --body <body>` for non-interactive use

## Testing Framework

TESTING FRAMEWORKS: [Go testing, testify, afero (in-memory FS)]

See [`.agents/testing.md`](.agents/testing.md) for full details.

- Tests are colocated with source as `*_test.go` files
- Run with `go test ./...`; CI runs on ubuntu, windows, and macos
- Mock HTTP server and filesystem helpers live in `neo4j-cli/aura/internal/test/testutils/`
- `neo4j-cli/` (the super-CLI package) has no test files; this is a pre-existing gap
- **Prefer table-driven tests** (`for _, tc := range []struct{...}{...}`) when writing new tests — they reduce duplication and make it easy to add cases later
- **Name test files per command**, not per package — use `get_test.go`, `set_test.go`, `list_test.go` mirroring the source files; put shared helpers in `helpers_test.go`. Avoid aggregating all tests in a single `config_test.go`.
- **Never use `afero.NewOsFs()` in query package tests** — the dev machine has real credentials at `~/Library/Preferences/neo4j/cli/credentials.json`. Tests using a real FS will fail if any dbms credential is stored. Always use `testfs.GetTestFs(`{"format":"json"}`, "{}")` (empty credentials) even when testing dotenv walk-up; write the dotenv into the memFs at `filepath.Join(t.TempDir(), ".env")` and `t.Chdir(tmp)` so `os.Getwd()`+`cfg.Aura.Fs()` finds it.

## Architecture

ARCHITECTURE PATTERN: Cobra command tree — one file per leaf command, directory structure mirrors command hierarchy

See [`.agents/architecture.md`](.agents/architecture.md) for full details.

Two binaries are produced:
- **`neo4j-cli`** — super-CLI entrypoint (`neo4j-cli/main.go`); wraps `aura-cli` under the `aura` subcommand
- **`aura-cli`** — standalone Aura CLI (`neo4j-cli/aura/cmd/main.go`)

```
neo4j-cli/
  app/app.go               # neo4j-cli cobra tree builder (NewCmd, Version) — importable
  main.go                  # thin entrypoint; mounts aura subcommand as "aura"
  internal/skill/          # per-binary skill template (bundle, description.txt, additions.md, gen/)
  aura/
    cmd/main.go            # aura-cli standalone entrypoint
    aura.go                # Root command, registers subcommands
    internal/
      api/                 # HTTP client for Neo4j Aura REST API
      flags/               # Custom reusable flag types
      output/              # JSON + table rendering
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

Agent-skill subsystem: `common/skill/` holds the binary-agnostic logic (agent catalog, path expansion, bundle render, install/remove/list/check, cobra wrapper). Each binary has its own `<bin>/internal/skill/` template (`embed.go` + `description.txt` + `additions.md` + `gen/main.go` + committed `bundle/`). Adding a new standalone CLI = copy the template, edit `description.txt`/`additions.md`/`gen/main.go` import, mount `skill.NewCmd(cfg, binskill.Bundle, "<newcli>")`, run `go generate`. No edits to `common/skill/`. See `CONTRIBUTING.md` "Generated content" for the full workflow.

Key CLI conventions (see `CONTRIBUTING.md`):
- Singular nouns for commands (`instance`, not `instances`)
- `<resource> <action>` form (`instance list`, not `list-instance`)
- One positional argument max; extras become flags
- `--format json|table` (shorthand `-f`) for all read commands
- `--await` flag for async operations
- Follow CLI best practices from https://clig.dev/ — source at https://github.com/cli-guidelines/cli-guidelines/blob/main/content/_index.md (fetch the raw markdown for token-efficient reference)

## Deployment

DEPLOYMENT STRATEGY: GitHub Releases via GoReleaser, triggered by `CHANGELOG-neo4j.md` updates on `main`

See [`.agents/deployment.md`](.agents/deployment.md) for full details.

- `changie` batches changelog entries and opens release PRs automatically (dual-project: `aura-cli` + `neo4j-cli`)
- Merging a release PR triggers GoReleaser to publish binaries for linux/windows/darwin (amd64 + arm64)
- macOS binaries are code-signed and notarized
- Each binary gets its own version: `AURA_CLI_VERSION` for `aura-cli`, `GORELEASER_CURRENT_TAG` for `neo4j-cli`
- Combined `release-notes.md` is generated with `## Versions` and `## Changes` sections before GoReleaser runs

## Makefile Notes

- `license-check` target uses `$(GOPATH)/bin/addlicense` (not bare `addlicense`) — GOPATH/bin may not be on PATH
- `license-check` requires a Unix shell (`find` + `xargs`); won't work natively on Windows
- `make generate` runs `go generate ./...`; `make generate-check` runs generate then `git diff --exit-code` (CI gate). Wired in `.github/workflows/test.yml` between Build and Lint, runs on full OS matrix.
- Drift sim: editing a bundle file directly to test generate-check is futile — `go generate` overwrites it. Mutate a real cobra-tree input (e.g. a Short string in `app.go`) to simulate stale-bundle detection.
- Changing `ValidFormatValues` in `common/clicfg/clicfg.go` affects the `--format` flag help text, which is embedded in skill bundle reference docs. Run `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...` after any such change; `TestGenerator_RoundTrip` is the gate that catches stale bundles.
- Adding any new command to the neo4j-cli command tree (including sub-sub-packages like `credential/dbms/`) also requires `go generate ./neo4j-cli/internal/skill/...` — otherwise `TestGenerator_RoundTrip` fails with a "references/credential.md differs" message. Run this immediately after any command-tree change before the test gate.

## Changie Multi-Project Notes

- `ProjectConfig` in changie does NOT support `changesDir` or `changelogPath` fields — only `label`, `key`, `changelog`, and `replacements`
- Version files live at `changesDir/<key>/v*.md` (e.g., `.changes/aura-cli/v1.7.0.md`) — changie appends the project key to `changesDir` automatically
- All projects share a single unreleased directory at `changesDir/unreleasedDir/` (e.g., `.changes/unreleased/`) — change files are tagged with `project:` field inside the YAML, not by directory
- `changie latest --project aura-cli` outputs `aura-cliv1.7.0` (project key prepended with no separator by default) — use `--remove-prefix` to strip `v` but key is always prepended; shell workflows must strip `aura-cli` prefix (e.g., `sed 's/^aura-cli//'`)
- `ProjectsVersionSeparator` in `.changie.yaml` can be set to `-` to get `aura-cli-v1.7.0` instead of `aura-cliv1.7.0`; leave unset (empty) for `aura-cliv1.7.0`
- `changie merge` (no flags) automatically iterates all `projects:` in config and writes each to its own `changelog:` path — confirmed from source (`cmd/merge.go`). Calling `changie merge --project` is not supported by changie's CLI.
- `changie new --projects <a> --projects <b>` creates entries for multiple projects in one call; the interactive prompt (`make changelog`) also supports multi-select
- This repo uses kind labels `Major`, `Minor`, `Patch` (not `added`/`feat`) — check `.changie.yaml` `kinds:` before using `--kind`
- If changie isn't installed locally, hand-author YAML files under `.changes/unreleased/` named `<project>-<Kind>-<YYYYMMDD>-<HHMMSS>.yaml` with fields `project / kind / body / time` (single-quoted body, RFC3339 time). Write one file per project for dual-project entries.

## Changie Workflow Notes

- To detect whether `.changes/unreleased/` contains entries for a given project, use `grep -rl 'project: <key>' .changes/unreleased/ 2>/dev/null | grep -q .` — the `2>/dev/null` handles an absent/empty directory and `grep -q .` converts the file list to a boolean exit code
- Write boolean outputs to `GITHUB_OUTPUT` with `echo "has_<project>=true" >> $GITHUB_OUTPUT` / `false` in an if/else so downstream steps can use `if: steps.<id>.outputs.has_<project> == 'true'`
- Always gate terminal steps (e.g. `create-pull-request`) on the same detection outputs — skipped upstream steps produce empty outputs, not skipped downstream steps; without a guard the terminal step runs with blank inputs and creates a malformed artifact
- For multiline GitHub Actions outputs, use the heredoc form: `{ echo "key<<EOF"; echo "${VALUE}"; echo "EOF"; } >> $GITHUB_OUTPUT` — this avoids issues with inline `|` syntax
- When building multiline strings in `run: |` shell blocks, use `printf '...\n...'` instead of multi-line string assignment — the YAML indentation level (e.g. 10 spaces) carries into continuation lines as literal whitespace

## Release Workflow Notes

- Release workflow triggers on `CHANGELOG-neo4j.md` changes (not `CHANGELOG.md`)
- `AURA_CLI_VERSION` env var set in an earlier step must be re-referenced as `${{ env.AURA_CLI_VERSION }}` in the GoReleaser action's `env:` block — GitHub Actions does not auto-forward env vars set by previous steps into action env blocks
- The neo4j-cli changelog body for a version lives at `.changes/neo4j-cli/<version>.md`; `tail -n +2` strips the `## vX.Y.Z - DATE` header line
- Avoid heredoc indentation issues in `run: |` blocks: use `{ printf ...; } > file` instead of `cat > file << EOF ... EOF` when shell lines are indented under YAML
- Job-level `outputs:` block surfaces step outputs to downstream `workflow_run` consumers. To expose a step output, the step needs an `id:` and must `echo "key=val" >> $GITHUB_OUTPUT` — then reference as `${{ steps.<id>.outputs.<key> }}` in the job `outputs:` block. Output is always populated (downstream gates on the value, not whether it was set).
- All actions in `.github/workflows/` are SHA-pinned with a `# v<major>` trailing comment (e.g. `actions/upload-artifact@ea165f8d... # v4`) — match this convention for any new action; Renovate handles bumps.
- `release.yml`'s `include_neo4j` / `include_aura` are computed by diffing `git diff HEAD~1 --name-only` against changelog filenames — split into its own step (id: `changed`) so the job's `outputs:` block can wire it cleanly to downstream `workflow_run` workflows like `publish-npm.yml`.
- `workflow_run.workflows: ["<name>"]` matches by the upstream workflow's `name:` field, NOT the filename. `release.yml` declares `name: release` (lowercase) so the watcher uses `["release"]`. Mismatching never errors — it just silently never triggers.
- Cross-workflow artifact download with `actions/download-artifact@v4` requires both `github-token: ${{ secrets.GITHUB_TOKEN }}` AND `run-id: ${{ github.event.workflow_run.id }}`. Without `run-id` it looks at the current run only and 404s.
- `workflow_run` events do NOT have `inputs.*` populated; `workflow_dispatch` events do not have `github.event.workflow_run.*`. To pick a value cleanly across both triggers use the ternary pattern `${{ github.event_name == 'workflow_dispatch' && inputs.x || steps.<auto>.outputs.x }}` — short-circuit makes the unset side empty and `||` falls back.
- A `workflow_run`-triggered workflow's job-level `if:` cannot read artifact contents (artifacts haven't been downloaded yet). Gate the JOB on `github.event.workflow_run.conclusion == 'success'`, then download the meta artifact, parse with `jq`, and apply per-step `if:` gates on the parsed value.
- npm Trusted Publishers (OIDC) auth in `publish-npm.yml`: requires `permissions: id-token: write`, `actions/setup-node` with `registry-url`, and Node ≥ 20 (npm ≥ 11.5.1). setup-node writes a placeholder `~/.npmrc`; npm swaps in a short-lived OIDC token at publish time — no NPM_TOKEN secret needed. publish.sh stays auth-agnostic. Re-enabling the disabled `workflow_run` trigger requires re-adding `actions: read` to `permissions:` for cross-workflow `actions/download-artifact`.
- Cross-repo write from `release.yml` (e.g. push Homebrew formula to `neo4j-labs/homebrew-tap`) uses the GitHub App pattern via `actions/create-github-app-token` (SHA-pinned, `# v<major>`). Inputs: `app-id` + `private-key` from secrets, `owner` + `repositories` to scope the installation token. Output `steps.<id>.outputs.token` is plumbed into the consumer step's `env:` block — no long-lived PAT, token is per-run.

## GoReleaser Notes

- GoReleaser v2 deprecates `archives.format` (string) — use `archives.formats` (list)
- GoReleaser v2 deprecates `format_overrides.format` — use `format_overrides.formats`
- Each `archives` entry must have a unique `id`; omitting it defaults to `"default"` and causes errors when there are multiple archive blocks
- Use `{{ .Binary }}` in `name_template` (not `{{ .ProjectName }}`) when building multiple binaries so archives are named per binary
- `-X "<importpath>.Version=..."` ldflag must match the actual package path of the Version var. If you move Version from `package main` to e.g. `neo4j-cli/app`, update the ldflag to `-X "github.com/neo4j/cli/neo4j-cli/app.Version=..."` — a stale path silently no-ops and ships `dev`.
- `make snapshot` runs `goreleaser build --snapshot --single-target` which does NOT exercise the `brews:` step. To verify brew formula generation locally use `goreleaser release --snapshot --clean --skip=publish,sign,notarize` with `HOMEBREW_TAP_GITHUB_TOKEN=<anything>` set (the env var is referenced via template; value can be a stub for snapshot since `--skip=publish` short-circuits the push). Output appears at `dist/homebrew/Formula/neo4j-cli.rb`.
- GoReleaser v2.x emits a deprecation notice "brews is being phased out in favor of homebrew_casks" — informational only; `brews:` still works and is the documented path for non-cask formulas. Migration to `homebrew_casks:` is a separate decision.

## Repo Doc Notes

- `CLAUDE.md` is a symlink to `AGENTS.md` (`ls -la` confirms). Edit `AGENTS.md` once — both surfaces update. Don't write to `CLAUDE.md` directly.
- Contributor-facing workflows (e.g. `make generate` / add-new-CLI procedure) live in `CONTRIBUTING.md` "Development" subsections. AGENTS.md Architecture orients readers and links to CONTRIBUTING.md for the procedure rather than duplicating it.

## Repo Layout Notes

See [`.agents/repo-layout.md`](.agents/repo-layout.md) — gotchas around skill subsystem layout, embed roots, mount points, and bundle regen.

## Hermetic Test Notes

- For path-expansion tests using `~` / `$XDG_CONFIG_HOME`, use `t.Setenv("HOME", "...")` and `t.Setenv("XDG_CONFIG_HOME", "")` — Go's `os.Getenv` returns "" for both unset and set-to-empty, and `t.Setenv` auto-restores after the test.
- Use `afero.DirExists` (not `Exists`) for "is the agent installed?" checks — files at the marker path shouldn't count as detected.
- `go-pretty/v6/table` upper-cases header text by default — assertions on table output should compare against `strings.ToLower(...)` for header columns, exact case for body cells.
- Lightweight cobra command tests can wire `clicfg.NewConfig(testfs.GetTestFs(...), version)` directly without the heavier `testutils.NewAuraTestHelper` — the latter pulls in API mocking and credential setup that `skill` doesn't need.
- For repo-wide gate tests that must auto-discover content (e.g. `common/skill/bundles_test.go` walking every `<bin>/internal/skill/bundle/SKILL.md`), resolve repo root via `runtime.Caller(0)` then `filepath.Walk` from there. Suffix-match paths after `filepath.ToSlash` so Windows runs match. Prune `.git`, `node_modules`, `bin`, `.changes` to keep the walk fast.
- `os.OpenFile(..., 0o644)` mode bits are masked by umask on create — if downstream readers (e.g. a docker container running as a different uid) need a specific perm, follow up with `os.Chmod` rather than relying on the OpenFile mode. Same applies to `t.TempDir()` which creates with 0700; a read-only bind mount into a container needs `os.Chmod(dir, 0o755)` for the in-container user to traverse.
- Package-level test seams (e.g. `stdinIsTTY` at `neo4j-cli/query/run.go`, `stdoutIsTerminal` at `neo4j-cli/query/output.go`) are `var <name> = func(...) ...` declarations that production fills with the real impl. For TTY-related seams in the `query` package, `TestMain` in `testseam_test.go` seeds the seam to the most-common-existing-assertion value (e.g. TTY=true) so legacy tests stay green without per-test edits; tests that need the other branch use a small `withX(t, val)` helper that swaps the seam and registers `t.Cleanup` to restore it.

## Windows CI Gotchas

- Path-separator bugs in `expandPath`-style helpers are Windows-only. Catalog entries keep forward slashes (portable convention); helpers MUST wrap any post-substitution path through `filepath.FromSlash` (or build via `filepath.Join`) so the whole path is OS-native. A `ReplaceAll(path, "$XDG_CONFIG_HOME", xdg)` where `xdg` came from `os.Getenv` produces mixed separators on Windows (`C:\…\.config/opencode`) — fix at the helper, not the catalog.
- Test expected values that hard-code separators bake in OS assumptions. Build expected values with `filepath.Join` / `filepath.FromSlash` rather than literals when asserting cross-OS path output. MemMapFs marker paths in detection tests must also be built OS-natively so they match what the (post-fix) helper looks up.
- Committed `.md` / golden / bundle files MUST be pinned to LF via `.gitattributes` — Windows runners have `core.autocrlf=true` by default and will rewrite to CRLF on checkout. The renderer (`common/skill/render`) and `make generate-check` both assume LF; a CRLF checkout breaks byte-equal golden tests AND `git diff --exit-code`. The repo-root `.gitattributes` covers `common/skill/render/testdata/**`, `**/internal/skill/bundle/**`, `**/internal/skill/additions.md`, `**/internal/skill/description.txt`. `common/skill/bundles_test.go::TestCommittedBundlesAndTestdataAreLF` is the assertion that catches a weakened/removed attribute.

## npm Distribution Notes

See [`distribution/npm/README.md`](distribution/npm/README.md).

## golangci-lint Notes

- Version installed: v2.11.4 (via Homebrew)
- golangci-lint v2 requires `version: "2"` at the top of `.golangci.yml`
- In v2, `gofmt` is a **formatter** (not a linter); put it under `formatters.enable`, not `linters.enable`
- Use `linters.default: none` to disable auto-enabled defaults (e.g. `ineffassign`) and run only explicitly listed linters
- Config lives at `.golangci.yml` in repo root
- In CI, `golangci/golangci-lint-action@v6` is used as the lint step — it installs, caches, and runs golangci-lint using `.golangci.yml`. This is equivalent to `make lint`. Renovate will pin the SHA.

## Config Architecture Notes

- `Config.Global` (`*GlobalConfig`) holds top-level (non-namespaced) viper keys; `Config.Aura` holds `aura.*`-prefixed keys
- The `format` setting lives at the top-level viper key `"format"`, not `"aura.format"` — always read/write via `cfg.Global.Format()` and `cfg.Global.BindFormat()`
- Migration from old `aura.output`/`output` to `format` is **commented out** in `NewConfig` — this experimental release never shipped so the migration has never run; re-enable it for the first stable-release upgrade path (two source clauses: `"aura.output"` and `"output"`)
- When migration code is commented out, remove any imports it uniquely used (e.g. `gjson` was removed from `clicfg.go` when the migration block was commented out) to avoid unused-import compilation errors
- `Viper.IsSet()` returns `true` for keys set via `SetDefault` — use `gjson.GetBytes(data, key).Exists()` to distinguish file-backed values from viper defaults when writing migration conditions
- `cfg.Global.BindFormat(flag)` binds viper's `"format"` key to the pflag value; passing `--format json` overrides both the rendering format AND the config value returned by `cfg.Global.Get("format")` — they are the same viper key
- go-pretty renders table header rows in **uppercase** with `table.StyleLight` — assert for `"KEY"` and `"VALUE"` (not lowercase) in table output tests
- Test helpers default to `"format": "json"` at the JSON root; set format overrides with `helper.SetConfigValue("format", "table")` (not `"aura.format"`)
- Removing methods from `AuraConfig` that have call sites across the codebase requires updating all callers in the same task for `make test` to pass
- `cobra.EnableTraverseRunHooks = true` is a package-level global — set it in `main()` before `Execute()`, not in `NewCmd()`, since it affects all cobra executions in the process; in test helpers set it once in the constructor (`NewAuraTestHelper`), not on each `ExecuteCommand` call
- `pflag.AddFlagSet` silently ignores duplicate-named flags (the flag already present in the target set wins); child `PersistentFlags` shadow a parent's `PersistentFlags` with the same name — the root's `PersistentPreRunE` still finds the resolved flag via `cmd.Flags().Lookup()`
- When renaming a flag that is used in many test files, complete clicfg core changes AND all callers AND all test fixtures in the same iteration — otherwise the build or test suite is broken between tasks and feedback loops cannot gate completion

## Command Tree Restructuring Notes

- Go's `internal` package rules prevent `neo4j-cli/internal/subcommands/config` from directly importing `neo4j-cli/aura/internal/subcommands/config` — bridge via a thin wrapper function in the non-internal `neo4j-cli/aura/` package (e.g. `NewAuraConfigCmd` in `aura/config.go`)
- When moving a subcommand from one path to another (e.g. `neo4j aura config` → `neo4j config aura`), the `cmd.Use` field must be renamed to match the new path segment — set it on the returned command before mounting
- If `NewStandaloneCmd` calls `NewCmd` and then adds extras, removing a subcommand from `NewCmd` also removes it from standalone; add it back directly in `NewStandaloneCmd` as a temporary hold until the standalone-specific version is implemented
- The standalone aura-cli flat config command (`NewStandaloneConfigCmd`) routes key operations by checking `cfg.Global.IsValidConfigKey(key)` first, then `cfg.Aura.IsValidConfigKey(key)` — global keys take precedence; use `allStandaloneConfigKeys(cfg)` to combine both for `ValidArgs` on get subcommands
- Cobra's `legacyArgs` behavior: child commands (those with a parent) accept arbitrary positional args by default — only root commands with subcommands produce "unknown command" errors via `legacyArgs`. This means `neo4j aura config list` (where `config` doesn't exist under `aura`) shows the `aura` help and exits 0 rather than erroring. Test for help display and absence of "config" in the help output instead of asserting an error string.

## Output Rendering Notes

- `ResponseData` interface lives in `common/output` (not `aura/internal/api`) — `api.ResponseData` is a type alias (`= output.ResponseData`) so all existing callers continue to compile without import changes
- `PrintBodyMap`, `printTable`, and `getNestedField` live in `common/output/output.go`; `aura/internal/output/output.go` contains only `PrintBody` (parse + delegate) and a thin `PrintBodyMap` shim
- `api.ParseBody` stays in `aura/internal/api/response.go` since it is tightly coupled to the Aura HTTP response format
- Adding a type alias (`type X = pkg.X`) in an existing package is the zero-change way to move an interface while keeping all callers compiling — prefer this over updating all call sites
- `ConfigEntry` and `ConfigData` in `common/output` let config commands use `PrintBodyMap` without `ParseBody` — import `common/output` directly (not `aura/internal/output`) in config packages
- `ConfigData.MarshalJSON()` returns a flat `{key: value}` map so JSON output is `{"format": "json"}` rather than `[{"key": "format", "value": "json"}]`; the `AsArray()` method returns the `[{"key":k, "value":v}]` form used only for table rendering
- `PrintBodyMap` routes both `"table"` and `"default"` to table rendering — config commands in default mode now render as tables, matching all other commands. Tests must assert for `"KEY"` / `"VALUE"` column headers for default-mode output cases
- `cobra.NoArgs` on a leaf command with no subcommands produces a "unknown command" error (not "accepts 0 arg(s)") when a positional arg is passed — Cobra treats the arg as an unknown subcommand. The error format is `Error: unknown command "<arg>" for "<full-cmd-path>"`.
- `neo4j-cli/main.go` and `neo4j-cli/aura/cmd/main.go` do NOT call `os.Exit(1)` when `cmd.Execute()` returns an error — they only emit a failure telemetry event. Process exit code is always 0 even on cobra errors. Tests that assert "non-zero exit" must use `cmd.Execute()`'s return value (as colocated `*_test.go` already does), not shell `$?`.
- `PrintableAuraCredentials.AsArray()` in `common/clicfg/credentials/aura.go` is the single source of truth for credential output shape; changing field names there requires updating all callers (`neo4j-cli/aura/credential.go`, `neo4j-cli/aura/internal/subcommands/credential/list.go`) to use the new keys in their `fields []string` slice.
- `common/output.printTable` uses `getNestedField` which returns `""` for nil and `json.MarshalIndent` (indented) for slices — different from a custom `formatCell`. When implementing `AsArray()` for a type with diverse value types (booleans, arrays, nested maps), pre-format cell values in `AsArray()` via `formatCell` so that `getNestedField` just passes strings through unchanged. `MarshalJSON()` should use the raw typed values (not pre-formatted) so JSON output preserves types.
- When a name conflict prevents using the intended struct name (e.g. `queryResult` already exists in `connect.go`), use a semantically clear alternative (`renderResult`) — the package-internal name doesn't affect the public interface.
## Cobra Flag Access Notes

- `cmd.Flags().GetString("foo")` only sees LOCAL flags until `mergePersistentFlags()` runs (during `Execute` or `ParseFlags`). Calling it from a unit test that drives a function directly (without Execute) will fail with `flag accessed but not defined`. Use `cmd.Flag("foo").Value.String()` instead — `Flag()` falls through to persistent flags + parents' persistent flags via `persistentFlag()`/`updateParentsPflags()`. Same applies to GetBool — for bool defaults that overlap with "unset" (e.g. `--insecure` defaults false), gate on `cmd.Flag("name").Changed` to disambiguate.
- For first-non-empty-wins precedence pass values to a helper that picks the FIRST non-empty. For lowest→highest precedence (`.env` < env < flag) use a `last-non-empty-wins` helper instead — easy to swap accidentally; query/connect.go calls this `overlay()`.

## toon-go Notes

- Module: `github.com/toon-format/toon-go` — imported as `toon "github.com/toon-format/toon-go"` in Go source
- Key API: `toon.Marshal(v any, opts ...toon.EncoderOption) ([]byte, error)` and `toon.WithLengthMarkers(bool) toon.EncoderOption`
- `printToon` in `common/output/output.go` uses a JSON round-trip (marshal → unmarshal to `any` → toon.Marshal) to honour custom MarshalJSON implementations on concrete ResponseData types before encoding to TOON
- `go mod tidy` promotes toon-go from `// indirect` to a direct dependency automatically once the import is added

## Local Verification Scripts

- `TestHTTPS_Smoke` (`neo4j-cli/query/query_https_smoke_test.go`) — real-Neo4j HTTPS smoke for `query --insecure` (asserts positive + negative TLS paths). Pure Go: stdlib cert gen, boots `neo4j:latest` via `os/exec`, binds two random free TCP ports on 127.0.0.1. Gated by `NEO4J_HTTPS_TEST=1`; run via `NEO4J_HTTPS_TEST=1 go test -run TestHTTPS_Smoke -v ./neo4j-cli/query/...`. Requires `docker`. Skipped by default in `go test ./...`.
- Neo4j docker HTTPS env-vars use single-underscore for `.` and double for `_`: `NEO4J_server_https_enabled`, `NEO4J_dbms_ssl_policy_https_{enabled,base__directory,private__key,public__certificate,client__auth}`. Bind-mount cert dir at `<base_directory>` (e.g. `/ssl`) containing `private.key` + `public.crt` plus empty `trusted/` and `revoked/` subdirs (Neo4j requires both even when `client__auth=NONE`). Cert files must be world-readable (0644) — container user is uid 7474.

## Credentials Storage Notes

- `Credentials.load()` re-wires `onUpdate` on both `Aura` and `Dbms` after JSON unmarshal — JSON decode creates a new struct pointer that loses the callback. This is the correct pattern for any future credential type added to `CredentialsFile`.
- `DbmsCredentials.GetDefault()` returns `(nil, nil)` when no default is set (not a usage error). Use `nil` check at the call site to decide whether to fall back to other connection resolution strategies.
- `PrintableDbmsCredentials.AsArray()` and `MarshalJSON()` intentionally omit `password` — any future credential type that has sensitive fields must also exclude them from both methods.

## common/output Testing Notes

- `common/output` has no pre-existing test files — new tests live in `common/output/output_test.go` using `package output` (not `output_test`) so they can access the `StdoutIsTerminal` seam directly.
- Use `testfs.GetTestFs(`{"format":"toon"}`, "{}")` + `clicfg.NewConfig(fs, "test", clicfg.GlobalScope)` to build a config that returns "toon" from `cfg.Global.Format()`.
- To assert toon encoding: check that `json.Unmarshal([]byte(out), &v)` returns an error — toon uses `key: value` syntax, not JSON, so valid toon is invalid JSON.
- The json→any→toon round-trip in `printToon` honours custom `MarshalJSON` on the concrete `ResponseData` type. Test helpers that implement `MarshalJSON` (e.g. wrapping rows in `{"data": ...}`) let you verify the top-level envelope key appears in both toon and json output.

## query/connect.go Credential Integration Notes

- `resolveConn` integrates stored dbms credentials: when no params are set via flags/env/dotenv, the stored default credential is used; when a stored credential exists and only 1–3 of the 4 params are explicitly set, an all-or-nothing error is returned; when all 4 are set explicitly, the stored credential is bypassed entirely. When no stored credential exists, the original behavior (partial params + built-in defaults) applies unchanged.
- Use `cmd.Flag("name").Changed` (not `flagString(cmd, "name") != ""`) to detect explicit flag-setting — `Changed` is the only reliable indicator that the user set the flag, versus just reading the default value.
- `insecureExplicit` pattern: read `cmd.Flag("insecure").Changed` AFTER applying the insecure value, then gate credential's insecure on `!insecureExplicit` — ensures the explicit `--insecure=false` overrides the stored credential's `insecure:true`.

---

_This AGENTS.md was generated using agent-based project discovery._

<!-- END GENERATED: AGENTS-MD -->
