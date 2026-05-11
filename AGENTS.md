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
- `--format json|table|toon` (shorthand `-f`) for all read commands
- `--await` flag for async operations
- Follow CLI best practices from https://clig.dev/ — source at https://github.com/cli-guidelines/cli-guidelines/blob/main/content/_index.md (fetch the raw markdown for token-efficient reference)

## Deployment

DEPLOYMENT STRATEGY: GitHub Releases via GoReleaser, triggered by `CHANGELOG-neo4j.md` updates on `main`

See [`.agents/deployment.md`](.agents/deployment.md) for changie workflow, release workflow, and GoReleaser gotchas.

- `changie` batches changelog entries and opens a release PR automatically (single project: `neo4j-cli`)
- Merging a release PR triggers GoReleaser to publish binaries for linux/windows/darwin (amd64 + arm64)
- macOS binaries are code-signed and notarized
- The release version comes from `GORELEASER_CURRENT_TAG` (set by the GoReleaser action)
- `release-notes.md` is generated with a `## Changes` section (neo4j-cli changelog body) before GoReleaser runs

## Makefile Notes

- `make generate-check` is `git diff --exit-code` after `go generate`. It flags ANY tracked-file diff, including unrelated edits in the working tree (e.g. a `.plans/tasks-*.yml` status flip). When validating "did this task introduce bundle drift?", inspect the diff output — only `internal/skill/bundle/**` paths matter for the gate. The hone harness's mid-task in_progress flip will always show up here; ignore it.
- `license-check` target uses `$(GOPATH)/bin/addlicense` (not bare `addlicense`) — GOPATH/bin may not be on PATH
- `license-check` requires a Unix shell (`find` + `xargs`); won't work natively on Windows
- `make generate` runs `go generate ./...`; `make generate-check` runs generate then `git diff --exit-code` (CI gate). Wired in `.github/workflows/test.yml` between Build and Lint, runs on full OS matrix.
- Drift sim: editing a bundle file directly to test generate-check is futile — `go generate` overwrites it. Mutate a real cobra-tree input (e.g. a Short string in `app.go`) to simulate stale-bundle detection.
- Changing `ValidFormatValues` in `common/clicfg/clicfg.go` affects the `--format` flag help text, which is embedded in skill bundle reference docs. Run `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...` after any such change; `TestGenerator_RoundTrip` is the gate that catches stale bundles.
- Adding any new command to the neo4j-cli command tree (including sub-sub-packages like `credential/dbms/`) also requires `go generate ./neo4j-cli/internal/skill/...` — otherwise `TestGenerator_RoundTrip` fails with a "references/credential.md differs" message. Run this immediately after any command-tree change before the test gate.

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

- For path-expansion tests using `~` / `$XDG_CONFIG_HOME`, use `t.Setenv("HOME", "...")` and `t.Setenv("XDG_CONFIG_HOME", "")` — Go's `os.Getenv` returns "" for both unset and set-to-empty, and `t.Setenv` auto-restores after the test.
- Use `afero.DirExists` (not `Exists`) for "is the agent installed?" checks — files at the marker path shouldn't count as detected.
- `go-pretty/v6/table` upper-cases header text by default — assertions on table output should compare against `strings.ToLower(...)` for header columns, exact case for body cells.
- Lightweight cobra command tests can wire `clicfg.NewConfig(testfs.GetTestFs(...), version)` directly without the heavier `testutils.NewAuraTestHelper` — the latter pulls in API mocking and credential setup that `skill` doesn't need.
- For repo-wide gate tests that must auto-discover content (e.g. `common/skill/bundles_test.go` walking every `<bin>/internal/skill/bundle/SKILL.md`), resolve repo root via `runtime.Caller(0)` then `filepath.Walk` from there. Suffix-match paths after `filepath.ToSlash` so Windows runs match. Prune `.git`, `node_modules`, `bin`, `.changes` to keep the walk fast.
- `os.OpenFile(..., 0o644)` mode bits are masked by umask on create — if downstream readers (e.g. a docker container running as a different uid) need a specific perm, follow up with `os.Chmod` rather than relying on the OpenFile mode. Same applies to `t.TempDir()` which creates with 0700; a read-only bind mount into a container needs `os.Chmod(dir, 0o755)` for the in-container user to traverse.
- Package-level test seams (e.g. `stdinIsTTY` at `neo4j-cli/query/run.go`, `stdoutIsTerminal` at `neo4j-cli/query/output.go`) are `var <name> = func(...) ...` declarations that production fills with the real impl. For TTY-related seams in the `query` package, `TestMain` in `testseam_test.go` seeds the seam to the most-common-existing-assertion value (e.g. TTY=true) so legacy tests stay green without per-test edits; tests that need the other branch use a small `withX(t, val)` helper that swaps the seam and registers `t.Cleanup` to restore it.
- `httptest.NewServer` server-side `r.Context().Done()` propagation from a closed client connection is best-effort and timing-dependent — when testing client ctx-cancellation paths, ALWAYS guard the handler with a short safety timeout (`select { case <-r.Context().Done(): case <-time.After(2*time.Second): }`) AND wrap the test-side wait on `errCh` in a `select` with a 5s fallback `t.Fatal`. Without these, a propagation miss hangs the handler until `-test.timeout` (default 10m), looking exactly like an infinite go-test loop. Symptom: `pkill -QUIT <pid>` shows the handler's goroutine blocked on `chanrecv` at `<-r.Context().Done()` while the client side has long returned.

## Windows CI Gotchas

- Path-separator bugs in `expandPath`-style helpers are Windows-only. Catalog entries keep forward slashes (portable convention); helpers MUST wrap any post-substitution path through `filepath.FromSlash` (or build via `filepath.Join`) so the whole path is OS-native. A `ReplaceAll(path, "$XDG_CONFIG_HOME", xdg)` where `xdg` came from `os.Getenv` produces mixed separators on Windows (`C:\…\.config/opencode`) — fix at the helper, not the catalog.
- Test expected values that hard-code separators bake in OS assumptions. Build expected values with `filepath.Join` / `filepath.FromSlash` rather than literals when asserting cross-OS path output. MemMapFs marker paths in detection tests must also be built OS-natively so they match what the (post-fix) helper looks up.
- Committed `.md` / golden / bundle files MUST be pinned to LF via `.gitattributes` — Windows runners have `core.autocrlf=true` by default and will rewrite to CRLF on checkout. The renderer (`common/skill/render`) and `make generate-check` both assume LF; a CRLF checkout breaks byte-equal golden tests AND `git diff --exit-code`. The repo-root `.gitattributes` covers `common/skill/render/testdata/**`, `**/internal/skill/bundle/**`, `**/internal/skill/additions.md`, `**/internal/skill/description.txt`. `common/skill/bundles_test.go::TestCommittedBundlesAndTestdataAreLF` is the assertion that catches a weakened/removed attribute.

## npm Distribution Notes

See [`distribution/npm/README.md`](distribution/npm/README.md).

## PyPI Distribution Notes

See [`distribution/pypi/README.md`](distribution/pypi/README.md) for the maintainer-facing channel docs (install commands, version mapping, recovery, auth).

- `publish-pypi.yml` mirrors `publish-npm.yml`'s `workflow_run + workflow_dispatch` shape — see that file's comments and the `## Release Workflow Notes` section in `.agents/deployment.md` for the cross-workflow gotchas (`workflows: ["release"]` matches the lowercase `name:`, cross-run `download-artifact` needs both `github-token` and `run-id`, `workflow_run` events have no `inputs.*`).
- The `go-to-wheel` CLI ALWAYS cross-compiles from Go source — its argparse accepts only `go_dir`, no `--binary-path`. To wrap pre-built GoReleaser binaries into wheels (REQ-F-009 binary-parity invariant), bypass the CLI and call `go_to_wheel.build_wheel(binary_path=..., ...)` directly via an inline `python3 - <<'PY'` heredoc. The library function takes a binary path and a platform_tag; iterate over the six platforms and read each binary from `dist/neo4j-cli_<VERSION>_<TitleOS>_<arch>/<binary>`.
- PEP 440 version normalisation lives in a single shell helper at `.github/scripts/version-to-pep440.sh` (pure bash, no python/jq deps). Both auto and manual paths invoke it once. The Go ldflags `Version` stays the original GoReleaser tag (so `neo4j-cli --version` and the smoke-test grep keep matching); only the wheel filename + PyPI metadata use the PEP 440 form.
- `.github/workflows/requirements-build.txt` pins `go-to-wheel` with `--require-hashes` for supply-chain safety. Update the hash via `pip3 download --no-deps -d /tmp/x go-to-wheel` then `pip3 hash /tmp/x/<wheel>`.
- YAML `run: |` block + bash heredoc gotcha: the heredoc terminator (`PY`) must be at the same indentation as the surrounding YAML block (which strips a fixed prefix). After strip, both the heredoc body and `PY` end up at column 0 — that IS valid for `<<'PY'`. Verify by round-tripping with PyYAML and `compile()`-ing the extracted python.
- `permissions: {}` works for jobs that download SAME-run artifacts via `actions/download-artifact@v4`. Cross-run downloads (workflow_run path consuming release.yml's artifacts) need workflow-level `actions: read`.

## golangci-lint Notes

- Version installed: v2.11.4 (via Homebrew)
- golangci-lint v2 requires `version: "2"` at the top of `.golangci.yml`
- In v2, `gofmt` is a **formatter** (not a linter); put it under `formatters.enable`, not `linters.enable`
- Use `linters.default: none` to disable auto-enabled defaults (e.g. `ineffassign`) and run only explicitly listed linters
- Config lives at `.golangci.yml` in repo root
- In CI, `golangci/golangci-lint-action@v6` is used as the lint step — it installs, caches, and runs golangci-lint using `.golangci.yml`. This is equivalent to `make lint`. Renovate will pin the SHA.
- If `make lint` reports issues whose paths point to a non-existent worktree (e.g. `.claude/worktrees/agent-…`) the cache is stale; run `golangci-lint cache clean` once and re-run — issues evaporate when the source path no longer exists.

## Credentials Storage Notes

- `Credentials.load()` re-wires `onUpdate` on Aura, Dbms, AND Embed after JSON unmarshal — JSON decode creates a new struct pointer that loses the callback. This is the correct pattern for any future credential type added to `CredentialsFile`.
- `DbmsCredentials.GetDefault()` returns `(nil, nil)` when no default is set (not a usage error). Use `nil` check at the call site to decide whether to fall back to other connection resolution strategies. `EmbedCredentials.GetDefault()` follows the same convention.
- `PrintableDbmsCredentials.AsArray()` / `MarshalJSON()` omit `password`; `PrintableEmbedCredentials.AsArray()` / `MarshalJSON()` omit `api-key`. Any future credential type with sensitive fields must follow this pattern.
- When adding a new credential type to `CredentialsFile` with `omitempty`, `load()` must defensively re-init the field if older credentials.json files lack the key — otherwise `c.Embed.onUpdate = c.save` panics on nil. Mirror the `if c.Embed == nil { ... }` guard in credentials.go.
- `PrintBodyMap` `fields` slice only affects TABLE rendering — it is ignored for JSON/toon formats. To suppress a sensitive field (e.g. `password`) from ALL output formats you must `delete(data, "field")` from the map before passing to `NewSingleValueResponseData`. The `fields` slice alone is insufficient.
- `DbmsCredentials.Add` can only fail with "already exists". Since `resolveCredentialName` guarantees the resolved name is free, Add cannot fail in a single-threaded context after a successful `resolveCredentialName` call. Storage failure warning paths are effectively dead code in normal operation.
- To verify DBMS credential storage in integration tests, use `helper.AssertCredentialsValue("dbms.credentials.0.name", "expected-name")`. To pre-populate DBMS credentials for collision tests, use `helper.SetCredentialsValue("dbms.credentials", []map[string]string{{...}})` + `helper.SetCredentialsValue("dbms.default-credential", "name")`.
- The default test helper credentials JSON only has `"aura": {...}` (no `"dbms"` key). During `Credentials.load()`, the absence of `"dbms"` in JSON leaves the field at its initial value `&DbmsCredentials{Credentials: []*DbmsCredential{}}`, so `cfg.Credentials.Dbms` is non-nil in all default test helpers. Passing `"dbms": null` in the JSON explicitly sets it to nil.
- `DbmsCredential.EmbedCredential` uses `json:"embed-credential,omitempty"` so older creds without a link don't gain the key on disk. `PrintableDbmsCredentials.AsArray` / `MarshalJSON` always emit the key (empty string when unset) so the column is stable across rows for table rendering and external JSON consumers. Pattern: omitempty on the persisted struct, unconditional emit on the printable wrapper.
- Cross-credential-type validation (e.g. `credential dbms add --embed-credential x`): validate the target with `Embed.Get(name)` BEFORE calling `Dbms.Add(...)` so a bad name never half-creates a cred. Then call `Dbms.SetEmbed(name, embedName)` AFTER `Dbms.Add` succeeds — keeps the storage `Add` signature stable rather than threading new optional fields through.
- Test fixture seeding: when a dbms test exercises code that touches `cfg.Credentials.Embed`, seed the `embed` block in `newDbmsTestHelper` initial JSON; sjson.Set on `embed.credentials` from a partial fixture works but seeding upfront is more honest about the test surface.

## query Subsystem Notes

See [`.agents/query.md`](.agents/query.md) for Bolt driver, execution, credential integration, embedding-provider plumbing, and local verification gotchas.

## Cobra Help / Skill Bundle Rendering Notes

- `common/skill/render/render.go:235` strips a cobra command's `Example` field via `strings.TrimSpace` before wrapping it in a fenced code block. The leading 2-space "cobra convention" indent is therefore stripped from the FIRST line only and preserved on subsequent lines, producing a ragged block. Write multi-line Examples with NO leading indent so the rendered bundle stays flush-left and consistent.
- Adding/changing a `Long` field on any cobra command in `neo4j-cli/internal/subcommands/credential/...` or `neo4j-cli/query/...` requires `go generate ./neo4j-cli/internal/skill/...` to refresh the bundle, otherwise `TestGenerator_RoundTrip` (the gate in `make test`) fails with a "references/<sub>.md differs" diff.
- `make generate-check` is `git diff --exit-code` after `go generate ./...`; on a working tree with uncommitted source-side help-text edits it WILL fail (the gate is meaningful only against a clean tree, i.e. CI). Locally: commit the source AND the regenerated bundle in the same commit, then re-run.
- `--param` flag Usage on the `query` parent now mentions the `:embed` modifier (`key:embed=<text>`) — keeping the modifier discoverable in `--help` was cheaper than a separate flag. The full rule (JSON-array rejection, empty-text accepted) lives in README and the bundle additions.md, not the flag Usage.
- README's "Aura API Credentials" example uses `credential aura-client add` (canonical neo4j-cli path), NOT the standalone-aura `aura credential add` form. The standalone aura binary is no longer built/shipped, so README must lead with commands the shipped binary actually has.
- Skill bundle `description.txt` (frontmatter description) is single-paragraph, ≤1024 chars, third-person. When adding new top-level capability to it, list every credential subtree explicitly ("Aura, Neo4j connection (dbms), and embedding-provider credentials") rather than collapsing them — the agent-side trigger phrasing matches user wording better when each subtree is named.

---

_This AGENTS.md was generated using agent-based project discovery._

<!-- END GENERATED: AGENTS-MD -->
