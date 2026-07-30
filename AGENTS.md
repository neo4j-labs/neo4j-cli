<!-- BEGIN GENERATED: AGENTS-MD -->

# AGENTS.md

Learnings and patterns for future agents working on this project.

## Feedback Instructions

TEST: `make test` · BUILD: `make build`, `make run-neo4j` · LINT: `make lint` · FORMAT: `make fmt-check` · LICENSE: `make license-check`

**Final gates before done: `make test`, `make fmt-check`, `make lint` — all must pass.** `fmt-check` runs `gofmt -l .` (fails on output); CI's golangci-lint v2 also gofmt-gates. `make fmt` rewrites but is not a gate.

The aura root's `PersistentPreRunE` (resolves `--debug` onto `cfg.Aura`) only runs on the mounted `neo4j-cli aura ...` surface because the neo4j-cli root sets `cobra.EnableTraverseRunHooks = true` — several aura subcommands (instance/project/…) define their own `PersistentPreRunE` that would otherwise shadow it. Mirror that flag in tests that exercise the aura-root hook through a subcommand.

## Project Overview

PRIMARY LANGUAGES: [Go]

`neo4j-cli` — command-line tool for interacting with Neo4j (Cypher, schema, Aura, local Docker/Desktop, datasets, agent skills).

## Cobra Command Layout

Strict one-file-per-leaf layout under `neo4j-cli/aura/internal/subcommands/<resource>/` and `common/skill/`. Mirror it for new trees. See [`.agents/cobra.md`](.agents/cobra.md) for flag access/precedence gotchas.

- Parent `<resource>.go` — `NewCmd(cfg, ...)`, persistent flags, `cmd.AddCommand(newXxxCmd(...))` per leaf. ≤80 lines. Name it after the resource (not `command.go`).
- Leaf `<action>.go` — private constructor (`newInstallCmd(...)`) with the leaf's flags + `RunE`. No leaf bodies in the parent.
- Colocated `<action>_test.go`; shared helpers in `<resource>_helpers_test.go`.
- New leaf = `<action>.go` + `<action>_test.go` + one `AddCommand` line.
- Documented exception: `admin privilege` grant/deny/revoke generate their 7 category leaves (property/entity/graph/label/load/database/dbms) from a single `categoryMeta` table via `newCategoryCmd` in `privilege/helpers.go` — no per-leaf file.

## Build System

BUILD SYSTEMS: [Go toolchain, Makefile, golangci-lint, GoReleaser, changie]. See [`.agents/build.md`](.agents/build.md).

- `make build` → `bin/neo4j-cli`; `make run-neo4j` (no build); `make snapshot` (goreleaser, current platform, ldflags).
- `make npm-publish-dry` — template/ordering check; stubs missing binaries (dry-run only). Run `make snapshot` first for real binaries.
- All `.go` files need the Neo4j copyright header (CI `addlicense`).
- Changelog via `make changelog` **only for user-facing changes**. Non-interactive: `changie new --projects neo4j-cli --kind <kind> --body <body>`. The body describes **only the observable user-facing impact** (new/changed/removed output, flags, commands, behavior, errors) — NOT the internal mechanics that produced it. Skip pure refactors, endpoint/transport migrations, and any change invisible to the end user; if such a change has an incidental observable effect (e.g. a new output field), the entry states just that effect, not the refactor. Verify the claimed effect is real by diffing observable output (`AssertOutJson`/`AssertErr` golden strings), not by describing the implementation.

## Testing Framework

TESTING FRAMEWORKS: [Go testing, testify, afero in-memory FS]. See [`.agents/testing.md`](.agents/testing.md) + [`.agents/hermetic-tests.md`](.agents/hermetic-tests.md).

- Colocated `*_test.go`; CI on ubuntu/windows/macos. Mock HTTP + FS helpers in `neo4j-cli/aura/internal/test/testutils/`. `neo4j-cli/` super-CLI pkg has no tests (known gap).
- Prefer table-driven tests. Name test files per command (`get_test.go`, not one big `config_test.go`); shared helpers in `helpers_test.go`.
- **Never `afero.NewOsFs()` in query-package tests** — dev machine has real creds at `~/Library/Preferences/neo4j/cli/credentials.json`. Use `testfs.GetTestFs(...)` (empty creds); for dotenv walk-up write `.env` into memFs + `t.Chdir(tmp)`.
- `afero.MemMapFs` quirks: no symlink support; `OpenFile` auto-creates missing parent dirs (unlike `OsFs`). Symlink + missing-dir error paths must use `OsFs` + `t.TempDir()`.
- An EXTERNAL test pkg (e.g. `package api_test`) can import a parent that depends on its package-under-test (`api_test` importing `neo4j-cli/aura`, which imports `aura/internal/api`) — no cycle, external test pkgs compile separately. Lets you drive a full aura command end-to-end while still calling the api pkg's `*_test.go`-only seam (`SetDebugWriterForTest`). Mount the aura tree under a stub `neo4j-cli` root with `cobra.EnableTraverseRunHooks=true`, `--format`/`ComposeRootPersistentPreRunE` on the root (mirrors `app.go`).

## Architecture

ARCHITECTURE PATTERN: Cobra command tree — one file per leaf, dirs mirror command hierarchy. See [`.agents/architecture.md`](.agents/architecture.md), [`.agents/repo-layout.md`](.agents/repo-layout.md).

One binary: `neo4j-cli` (`neo4j-cli/main.go`); Aura tree lives under the `aura` subcommand.

```
neo4j-cli/
  app/app.go        # cobra tree builder (NewCmd, Version) — importable
  main.go           # entrypoint; mounts aura, renders clierr
  query/            # Bolt subsystem (Cypher, :schema, embed, desktop/dotenv connect)
  internal/
    skill/          # per-binary skill template (bundle, description.txt, additions.md, gen/)
    subcommands/    # native leaves (credential, query, docker, dataset, update, agentcontext, ...)
    dataset/        # example-dataset resolver + downloader
    desktopclient/  # Neo4j Desktop discovery (mDNS) + REST client
    skillrefresh/ versioncheck/ quip/
  aura/
    aura.go         # root, registers subcommands
    internal/
      api/ flags/ output/ skill/
      subcommands/  # instance, project, organization, credential, config,
                    # agent, dataapi/graphql, graphanalytics, customermanagedkey, workspace, utils
common/
  clicfg/           # config, credentials, project state (OS paths)
  clicmd/ clierr/ clievents/ agent/ analytics/ configmigrate/
  confirm/ flags/ output/ tee/ skill/
```

- **Internal-package rule**: `common/*` CANNOT import `neo4j-cli/internal/*`. Helpers reachable from `clicfg.NewConfig` must live under `common/`. Once clicfg depends on a helper, that helper's tests can't import clicfg/testfs (import cycle) — seed memFs with a hard-coded relative path.
- **Skill subsystem**: `common/skill/` = binary-agnostic logic; `neo4j-cli/internal/skill/` = per-binary template (`embed.go`, `description.txt`, `additions.md`, `gen/main.go`, committed `bundle/`). New CLI = copy template, edit 3 files, mount `skill.NewCmd(...)`, `go generate`. No `common/skill/` edits. See `CONTRIBUTING.md`.
- **CLI conventions**: singular nouns; `<resource> <action>`; ≤1 positional (extras → flags); `--format json|table|toon` on reads; `--wait` for async. Follow https://clig.dev/.

## Deployment

DEPLOYMENT STRATEGY: GitHub Releases via GoReleaser, triggered by `CHANGELOG.md` on `main`. See [`.agents/deployment.md`](.agents/deployment.md).

- `changie` batches changelog + opens release PR; merging triggers GoReleaser → linux/windows/darwin (amd64+arm64). macOS signed+notarized. Version from `GORELEASER_CURRENT_TAG`.
- `brews.post_install:` takes the method **body only** (GoReleaser wraps `def...end`).
- `make snapshot` does NOT build the Homebrew formula. Validate: `GORELEASER_CURRENT_TAG=dev goreleaser release --snapshot --skip=publish --clean`, inspect `dist/homebrew/Formula/neo4j-cli.rb`.

## `go generate` / Skill Bundle Gate

`TestGenerator_RoundTrip` (in `make test`) fails if the committed skill bundle drifts. Run `go generate ./neo4j-cli/internal/skill/...` after ANY of:

- Adding/changing a command in the tree (incl. sub-sub-pkgs like `credential/dbms/`) → `references/<cmd>.md` drifts.
- `Long`/`Example` changes on `credential/...` or `query/...` commands.
- Changing `ValidFormatValues` in `clicfg.go` (affects `--format` help → bundle).
- Mutating `common/skill/AGENTS` catalog (install/remove Long embeds `agentNames()`).

`make generate-check` = `go generate ./...` + `git diff --exit-code`; only meaningful on a clean tree (CI). Locally commit source + regenerated bundle together. Editing a bundle file directly is futile (generate overwrites). To simulate drift, mutate a cobra input (e.g. a `Short` in `app.go`).

- Skill `Example:` fields render flush-left (`render.go` TrimSpaces first line only) — write multi-line Examples with NO leading indent.
- Every runnable leaf needs a flush-left `Example:` (≥2 invocations, `# comment` per invocation, `neo4j-cli` prefix, `--rw` on writes, ≥1 `--format json` on reads). Gate: `TestAllLeafCommands_HaveExamples` (agentcontext).
- `description.txt` frontmatter: single paragraph, ≤1024 chars, third-person; name each credential subtree explicitly.

## Changie

- Single project `neo4j-cli` but uses `projects:` mechanism — version files at `.changes/<key>/v*.md`; unreleased files share `.changes/unreleased/` tagged with `project:`.
- `changie latest --project neo4j-cli` → `neo4j-cliv1.7.0` (no separator); strip prefix in shell (`sed 's/^neo4j-cli//'`). `ProjectsVersionSeparator` controls this — leave unset.
- Kinds are `Major`/`Minor`/`Patch` (check `.changie.yaml`). If changie absent, hand-author `.changes/unreleased/neo4j-cli-<Kind>-<YYYYMMDD>-<HHMMSS>.yaml` (`project`/`kind`/`body`/`time`, single-quoted body, RFC3339).

## golangci-lint

- v2.11.4 (Homebrew). `.golangci.yml` needs `version: "2"`. `gofmt` is a **formatter** (`formatters.enable`), not a linter. `linters.default: none` to run only listed. CI uses `golangci-lint-action@v6` (== `make lint`).
- Stale-cache symptom: issues at non-existent worktree paths → `golangci-lint cache clean`.

## Distribution

- npm: [`distribution/npm/README.md`](distribution/npm/README.md).
- PyPI: [`distribution/pypi/README.md`](distribution/pypi/README.md) (channel docs) + [`.agents/pypi.md`](.agents/pypi.md) (workflow gotchas).
- Installer tests: bats (`distribution/installation-scripts/tests/install-neo4j-cli.bats`, `bats <dir>`), stub ALL external cmds via `STUBS_DIR` on PATH; tar stub must create a recording binary (installer `mv`s over pre-seeded stubs); shellcheck `.bats`.
- PowerShell installer tests: use `.cmd` stubs (not `.ps1` — Windows resolves `.cmd`/`.bat` first); pass wrapper via `pwsh -File`; pass paths via env vars; `.ps1` files need CRLF (verify with python3); Pester gated to `windows-latest` (no pwsh on macOS).

## Secrets / Redaction

- `common/clievents/RedactArgs` is the SINGLE source of truth for secret scrubbing (telemetry + panic/error msgs + on-disk history). Scrubs `secretFlags` allow-list (incl. `-p`) + `--uri` userinfo. Add secret-bearing flags THERE.
- `clievents.RedactText` (tee redactor) is shape-based (key=value, JSON, URIs, auth headers) — NOT table cells. Runtime secret printed only in a table → `clievents.RegisterSecretValue(value)` BEFORE printing (see `instance/create_core.go`). Secret-word vocab single-sourced as `secretWords` in `redact.go`. `docker.redactString` delegates to `RedactText`.
- See [`.agents/credentials.md`](.agents/credentials.md) for Aura/Dbms/Embed credential types.
- **Credential env-var gate (`accept-env-vars`)**: reading credentials from well-known env vars (`NEO4J_URI`/`USERNAME`/`PASSWORD`/`DATABASE`, embed `NEO4J_EMBED_*`/provider keys, `NEO4J_AURA_CLIENT_*`) is opt-in behind `cfg.Global.AcceptEnvVars()` (config key `accept-env-vars`, env bootstrap `NEO4J_CLI_ACCEPT_ENV_VARS=1`). The single shared gate is the method `(*clicfg.Config).GatedGetenv(name)` (returns `""` when off; nil-safe on receiver+`Global`) — use it, don't reintroduce a local `gatedGetenv`. The env-var NAME constants are single-sourced in `common/clicfg/credentials/env_spec.go` (`credentials.EnvURI`, `EnvEmbedProvider`, …) — reference those, not new literals; `dbconn.Env*` are re-export aliases. DBMS reads are gated at the single chokepoint `dbconn.ResolveConn` so query+admin+desktop inherit it. The dotenv (`--env` walk-up) mechanism is SEPARATE and NEVER gated; explicit flags are never gated. Tests that need an env var honoured must `t.Setenv("NEO4J_CLI_ACCEPT_ENV_VARS", "1")` (viper `GetBool` treats `"1"` true) — otherwise the read returns `""` and resolution falls to dotenv/stored cred.
- When emitting free-text debug/log lines through `RedactText`, do NOT start a line with a `<secretword>:` prefix (`token:`, `auth:`, `secret:`, `password:`, `key:`) — its assignment regex treats `<secretword>[:=] <word>` as a secret assignment and scrubs the next word to `***`. Word prose so no secret word immediately precedes a `:`/`=` (see aura api `debugInfo`).
- Aura `--debug` diagnostics in `neo4j-cli/aura/internal/api/` route through the package-level `debugW` seam (default `os.Stderr`, overridable via `SetDebugWriterForTest`); guard all emit/redact work behind `cfg.Aura.Debug()` so the off-path is untouched. Every emitted line passes through `RedactText` then `output.StripControl` (the `scrub` helper) so secrets are redacted AND control/ANSI bytes neutralised. Use the `[aura-debug] > `/`[aura-debug] < ` prefixes (helpers in `debug.go`).
- Both aura AND docker (`neo4j-cli/internal/subcommands/docker/`, `[docker-debug] > `/`< ` prefixes) `--debug` traces write to a package-global `debugW` seam, NOT `cmd.ErrOrStderr()` — `runEnv` has no `*cobra.Command`. Tests must capture via `SetDebugWriterForTest(t, &buf)` and assert against `buf`; cobra-captured stderr stays empty (caused a smoke-test CI fail).
- `desktopclient` (`[desktop-debug] > `/`< `/` ` prefixes) also uses the package-global `debugW` + `debugEnabled` (toggled by `SetDebug`, resolved in the desktop-root `PersistentPreRunE`). Its `SetDebugWriterForTest`/`SetDebugForTest`/`DebugEnabled` live in PRODUCTION `debug.go` (not `export_test.go`) because the external `desktop_test` package drives them through the imported `desktopclient` and can't see `export_test.go` symbols. `debugEnabled` is a process-global: reset (`SetDebug(false)`) between resolution cases and don't `t.Parallel()` them.

## Tee-on-failure

- Failing commands tee redacted output to `common/tee` (`ConfigPrefix/neo4j/cli/tee/`); `tee_path` in error envelope. Root sets `SilenceErrors: true`, so `clierr.Render` runs AFTER capture is read in `main.go` — `teeContent` appends `err.Error()` to captured bytes before `tee.Save`. Preserve that or no-intermediate-output failures tee empty.

## clierr Rendering

- `clierr.Render` (`common/clierr/render.go`) renders `*clierr.CLIError` from `ce.Message`/`ce.Code` via `errors.As` — NOT from `Error()`. Wrapping with `fmt.Errorf("...: %w", ce)` DROPS appended text. To append: mutate `ce.Message` (via `errors.As`), return original error. Plain errors → `NewFatalError("%s", err.Error())` (so `%w` survives only for them).
- Aura test harness (`ExecuteCommand`/`E`) does NOT call `clierr.Render` (that's in `main.go`) — tests get cobra's default stderr. To assert envelope/exit-code, recover `*clierr.CLIError` via `errors.As`, inspect `ce.Code`/`ce.BuildEnvelope()`.

## Output / TTY / Casing

- Wrapping/replacing `os.Stdout`/`Stderr` (tee, capture, pager, color) MUST be re-checked vs TTY detection. `output.StdoutIsTerminal` / `flags.stdoutIsTerminal` read the global `os.Stdout` FD, NOT `cmd.OutOrStdout()`, so wrapping can't flip format/color (regressed CLI-109, fixed CLI-210).
- `output.ResolveOutput` precedence: explicit flag > agent harness (`IsAgent`→`toon`) > TTY (`table`) > `json`. `agent.Detect()` reads env (`CLAUDECODE`) — tests seed `output.IsAgent = func() bool { return false }` via TestMain.
- `toon.Marshal` rejects C0 control bytes (accepts `\t\n\r`). `printToonValue` `stripControlDeep`s before marshal and falls back to JSON on residual error — never panic on data-driven marshal failure.
- `printTable` (go-pretty `StyleLight`) UPPER-CASES header cells — table-output tests must assert `ID`, not `id`. `getNestedField` `StripControl`s string CELLS only, so any runtime-derived column name must be stripped by the caller.
- **Casing (CLI-127)**: OUTPUT field names = snake_case (JSON/TOON keys, table headers, `Print*` `fields` slices). INPUT identifiers = kebab-case (`Use`, aliases, flag long names; single-char shorthands exempt). Exemptions: wire/parse structs (`api/.../response.go`, `desktopclient/types.go`, OAuth, `update/release.go`), config keys, Docker label consts, enum VALUES. Gates: input → `agentcontext/casing_input_gate_test.go`; output → `common/output/casing_gate_test.go` (`Print*` literals + json-tag allowlist — add new output structs there). Neither gate covers `test/e2e/` build-tagged suites — update their decoders + run `go test -tags=e2e_desktop ./test/e2e/desktop/...` on output-name changes; leave `test/e2e/desktop_fixture/**` camelCase (mirrors Desktop wire).

## Invoker Classification

- `common/agent.Invoker()` is the single caller classifier (history + telemetry `invoker` prop): `"agent"` (harness env via `Detect()`), `"script"` (no harness, non-TTY stdin), `"human"` (no harness, TTY). Don't add a second. Tested via same-pkg `_test.go` setting unexported `getenv`/`stdinIsTerminal`. Consumers own a local `var invokerFn = agent.Invoker` seam (see `clievents.go`, `history/store.go`).

## Subsystem Notes

- **query** — Bolt driver, execution, creds, embedding providers: [`.agents/query.md`](.agents/query.md).
- **dataset** (`neo4j-cli/internal/dataset/`) — no semver-range lib; `version.go` hand-rolls npm-style comparator-set matcher (`rangeMatches`/`canonicalVersion`); `rawBaseURL`/`httpDoFn` are httptest seams. Dumps come as REGULAR Git blob (raw serves bytes) OR Git-LFS (raw serves pointer, bytes on media host) — `download.go` fetches raw, sniffs `version https://git-lfs`, falls back to media; don't revert to media-only. **Resolver**: calver (2025.x/2026.x) continues the 5.x line — `Resolve` treats calver target OR `"latest"` as matching dumps with lower bound `>=5.0.0`; concrete targets keep exact matching; `canonicalVersion` strips leading zeros (calver months zero-padded).
- **desktopclient mDNS** (`discovery_mdns.go`) — `mdns.QueryContext` honors caller's `params.Entries`; silence socket warnings via `params.Logger = log.New(io.Discard,...)`; keep all mDNS imports + macOS `dns-sd` exec isolated to this file.
- **Docker** (`neo4j-cli/internal/subcommands/docker/`) — shells to host `docker`; Docker is source-of-truth (managed containers carry `org.neo4j.cli.managed=true` + metadata labels; no state file). Details:
  - `client.go` `dockerClient` interface; `execClient` shells out; `clientFactory` var is the test seam (fake in `helpers_test.go`). Only exported constructor: `NewDeployClient()`.
  - `bolt_ready.go` `WaitForBolt(...)` (create/start `--wait`, vendored driver); `stop --wait` polls `Inspect` for `State.Running == false`.
  - `create` auto-suffixes name vs `PsAll` + `DbmsCredentials.List()`. `--ephemeral` adds `--rm`, skips cred persistence, emits `.env` blob to stdout or `--env-out-file` (0600); consumed by `query --env`.
  - Image tags: `enterprise + latest` → `neo4j:enterprise` (NOT `latest-enterprise`); explicit → `neo4j:<v>-enterprise`. Centralized in `enterpriseImage(version)`. `docker load` defaults `--version latest`; aura loader defaults `5`. Verify tags vs https://hub.docker.com/_/neo4j.
  - Volume flags `--data-dir`/`--logs-dir`/`--import-dir` bind to `/data`/`/logs`/`/import`; `~`/`$VAR` via `expandHostPath` (`homeDirFn` seam); dirs created 0o755; incompatible with `--ephemeral`.
  - Dataset loader wraps `dataset.Resolve`/`Download` via `resolveDatasetFn`/`downloadDatasetFn` seams. New-container path: `LoadDumpIntoNewContainer(...)` (one-shot `--rm` loader → server reusing volume w/ `NEO4J_PLUGINS`). Existing-container: `CopyTo` (`docker cp`) then STOP/load/deferred-START. `Container.Plugins` from `NEO4J_PLUGINS` env, fails CLOSED.
  - Loader MUST use the image's DEFAULT entrypoint (no `--entrypoint bash`) so files end up uid 7474 (matches server); stage dump world-readable (dir 0755, `.dump` 0644); pass `-e NEO4J_ACCEPT_LICENSE_AGREEMENT=eval` (license gate hit via default entrypoint).
  - Secrets via process env (`cmd.Env` + `docker exec -e NAME` passthrough), NEVER argv (world-readable via `/proc`). `runEnv`/`ExecWithEnv`. `PushToAura` passes creds as `NEO4J_USERNAME`/`NEO4J_PASSWORD` env.
  - Smoke test: `go test -tags=smoke ./.../docker/...` (NOT in `make test`; skips without docker). Missing-docker error suggests `alias docker=podman` if podman on PATH (`lookPathFn` seam).
- **agent-context** — `neo4j-cli agent-context` reflects live cobra tree; hand-coded `schemaVersion`/`exitCodes`/`errorCodes`/`asyncFlag` in `agentcontext/build.go`. See [`.agents/agent-context.md`](.agents/agent-context.md).
- **v2beta1 404 suggestions** — once a resource is migrated to the nested v2beta1 scoped path, its 404 suggestion should come natively from `api/response.go` `suggestionForResource(<singular>)` (parseResourceFromRequest tags the trailing plural/id), NOT a call-site `utils.WithNotFoundContext` rewrite — add a `case` there when migrating a resource.
- **feature flags** — naming `flag.<area>-<feature>`, default-false, config+env override. See [`.agents/feature-flags.md`](.agents/feature-flags.md).
- **config migrations** — `common/configmigrate/`, `_schema_version` marker, warn-and-continue, `runWith` seam, under `common/` due to internal-pkg rule. See [`.agents/config-migrations.md`](.agents/config-migrations.md).
- **`internal/quip/`** — intentional cosmetic terminal-message helper. Keep as-is; don't document triggers, simplify, or "tidy" the obfuscation.

## Misc

- `cfg.Aura.BaseUrl()`/`AuthUrl()` run `removePathParametersFromUrl` — any configured path is dropped, leaving `scheme://host`. A test base-url of `<server>/v1` therefore contributes NO path segment; don't "fix" a doubled prefix by overriding it.
- `cmd.Flags()` does NOT include a command's own persistent flags until cobra parses argv (`mergePersistentFlags` runs in `ParseFlags`). A test inspecting an unexecuted command's flag surface must use `cmd.LocalFlags()`.
- `aura-client` cred cmd lives in TWO places: `neo4j-cli/internal/subcommands/credential/credential.go` (shipped, user-facing `credential aura-client add`, feeds the bundle) AND `aura/internal/subcommands/credential/add.go` (in-process/test only). Flag/Long changes hit BOTH; only `go generate ./neo4j-cli/internal/skill/...` needed (aura tree has no generated bundle). README leads with `credential aura-client add` (standalone aura not shipped).
- An ephemeral Aura credential (e.g. env-var-synthesized via `cfg.Aura.SetActiveCredential`) is NOT in `cfg.Credentials.Aura`'s store, so `getToken` (`aura/internal/api/token.go`) MUST skip `UpdateAccessToken` for it — `UpdateAccessToken` does `c.Get(name)` which `panic`s on a not-found name. Guard with a non-panicking `cfg.Credentials.Aura.Get(name)` probe before persisting; such tokens are kept in-process only (never written to credentials.json/keyring).
- `fileutils.WriteFile` PANICS on error; `WriteFileErr` is the error-returning twin (atomic 0600). Use `WriteFileErr` in best-effort paths (e.g. history logging).
- `--param` Usage on `query` parent mentions `:embed` modifier (`key:embed=<text>`); full rule in README + bundle additions.md.
- **v2beta1 scoped-instance migration** — `utils.FetchScopedInstance(cfg, orgID, projectID, instanceID)` is the v2beta1 scoped GET (native scoping, no tenant_id preflight) used by `instance get`/`delete`; the older `utils.FetchAndVerifyInstanceInProject` (v1 flat path + tenant_id comparison) MUST stay untouched for the still-v1 commands (pause/resume/update/overwrite, snapshot, graphql). A nested v2beta1 404 auto-tags resourceType=instance via `parseResourceFromRequest`, so no `WithNotFoundContext` enrichment is needed.
- **macOS subprocess test isolation** — `clicfg/darwin.go` uses `$HOME` env (not `user.Current()`); subprocess tests pass `HOME=<tempdir>` + symlink `login.keychain-db`. go-keyring hardcodes `/usr/bin/security` (PATH stubs don't work) — use `gokeyring.MockInitWithError`.
- **Windows CI** — path-separator handling in `expandPath`; LF-pin committed `.md`/golden/bundle via `.gitattributes`. See [`.agents/windows-ci.md`](.agents/windows-ci.md).

## Website (neo4j.sh)

Served from `gh-pages` branch (NOT `main`): `index.html`, `llms.txt`, `install.sh`, `install.ps1`. **Prompt-driven** — source of truth is `.github/prompts/website-update.md`. Update: `git worktree add gh-pages gh-pages`, run the prompt in it, review diff, commit/push on `gh-pages`. Rendering invariants live in the prompt — don't hand-edit `index.html` in violation; re-run the prompt.

---

_This AGENTS.md was generated using agent-based project discovery._

<!-- END GENERATED: AGENTS-MD -->
