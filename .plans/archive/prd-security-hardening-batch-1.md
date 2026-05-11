# PRD: Security Hardening Batch 1

## Overview

Internal hardening pass against 16 security findings from the whole-codebase audit recorded in `SECURITY_ISSUES_TO_FIX.md`. Closes secret-leak vectors (telemetry + panic stderr), tightens credentials.json on-disk handling, adds outbound HTTP timeouts and an SSRF allowlist, hardens GitHub workflows, sanitises terminal output, and scopes `.env` discovery so it can no longer be poisoned by an ancestor directory.

Source plan: `/Users/oskarhane/.claude/plans/let-s-plan-to-fix-abundant-narwhal.md`.

Skipped from this batch (intentional, tracked separately): #9 (plain-flag → stdin migration), #11 (cross-host redirect), #14 (in-memory-only access tokens), #19 (Credentials mutex).

## Goals

- Stop secrets (`--password`, `--client-secret`, `--api-key`, `--instance-password`) reaching Mixpanel, panic stdout, or any error template that interpolates `os.Args[1:]`.
- Make `credentials.json` durable (atomic write) and recoverable (corrupt-file backup instead of panic), and tighten file/dir modes.
- Bound the Aura HTTP clients (`api.go`, `token.go`) with a request timeout; reject SSRF-shaped base URLs at request time across Aura and embed providers. (Embed clients keep their ctx-owned cancellation contract — see REQ-F-C01.)
- Pin Go toolchain to a patched stdlib version (`1.26.3`) and harden five GitHub workflows (`claude.yml`, `cla-check.yml`, `update-website.yml`, `publish-npm.yml`, `test.yml`/`release.yml`).
- Strip control characters from rendered table output; warn on cleartext Bolt URIs to non-loopback hosts; escape backticks in introspected Cypher relType names.
- Scope `.env` discovery to stop at the first `.git` ancestor or `$HOME` boundary.

## Non-Goals

- No CLI flag additions, removals, or renames (covered by deferred #9).
- No changes to outbound redirect-following policy (covered by deferred #11).
- No change to access-token persistence semantics (covered by deferred #14).
- No introduction of `sync.Mutex` to `Credentials` (covered by deferred #19).
- No ctx plumbing into `MakeRequest` / `getToken` — timeout-only fix this round; ctx refactor is a separate follow-up.
- No new external dependencies; all helpers are stdlib (`net`, `os`, `crypto/sha256` already present).

## Requirements

### Functional Requirements

#### Group A — Secret redaction (closes #1, #2)

- **REQ-F-A01:** Add `common/clievents/redact.go` exposing `func RedactArgs(args []string) string`. Replace values of these flags with `***`: `password`, `client-secret`, `api-key`, `instance-password`. Handle `--flag value`, `--flag=value`, and (defensive) `-f value` forms.
- **REQ-F-A02:** In `common/clievents/clievents.go`, replace `strings.Trim(fmt.Sprint(args), "[]")` with `RedactArgs(args)` in the `aura`, `skill`, and `default` branches. Leave the already-redacted `query` branch unchanged.
- **REQ-F-A03:** Replace `os.Args[1:]` with `clievents.RedactArgs(os.Args[1:])` in every panic / error template:
  - `neo4j-cli/main.go:19`
  - `neo4j-cli/aura/cmd/main.go:22`
  - `neo4j-cli/aura/internal/api/response.go:44, 51, 71, 83, 97, 111, 121, 131, 141, 335`
  - `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/utils.go:42`

#### Group B — credentials.json hardening (closes #7, #10, #20)

- **REQ-F-B01:** `common/clicfg/fileutils/fileutils.go` `WriteFile` performs atomic temp-write+rename: open `path+".tmp"` with `O_CREATE|O_WRONLY|O_TRUNC` mode `0o600`, write, `Sync`, `Close`, then `Rename` over `path`. On rename failure remove the temp file.
- **REQ-F-B02:** `createFile` replaces the `Create`+`Chmod` chain with a single `OpenFile(O_CREATE|O_WRONLY|O_TRUNC, 0o600)` so the umask race window cannot occur.
- **REQ-F-B03:** Both `MkdirAll` sites that materialise the credentials/config dir use `0o700`: `fileutils.go:62` and `clicfg.go:72`.
- **REQ-F-B04:** `common/clicfg/credentials/credentials.go` `load()` no longer panics on `json.Unmarshal` failure. On parse error, copy the file to `credentials.json.corrupt-<unix_ts>`, reset to empty in-memory credentials, and return a `clierr.NewFatalError` whose message names the backup path.

#### Group C — HTTP client hardening (closes #13, #18)

- **REQ-F-C01:** Set `Timeout: 60 * time.Second` on the two Aura `http.Client{}` constructions:
  - `neo4j-cli/aura/internal/api/api.go:40`
  - `neo4j-cli/aura/internal/api/token.go:43`

  The three embed clients (`openai.go:37`, `ollama.go:36`, `huggingface.go:43`) intentionally have no client-side timeout — cancellation is owned by the caller's ctx (see in-file comments) — and are excluded so legitimate long-running local inference (e.g. Ollama on CPU with a large model) is not capped at 60s. `--await` is unaffected: `Poll()` issues a fresh `MakeRequest` per iteration, so the per-request 60s wall does not bound the polling loop. ctx plumbing into `MakeRequest` / `getToken` remains out of scope for this batch.
- **REQ-F-C02:** Add `common/clicfg/urlcheck/urlcheck.go` with `func ValidateRemoteURL(raw string) error`. Accept `https://` to any host. Accept `http://` only when host is loopback (`localhost`, `127.0.0.1`, `::1`). Reject IP-literal hosts that match `IsPrivate()`, `IsLinkLocalUnicast()`, `IsLinkLocalMulticast()`, or `IsMulticast()`. Reject empty/malformed URLs and the metadata IP `169.254.169.254`. Hostnames are passed through (DNS-rebinding is documented as out of scope).
- **REQ-F-C03:** Apply `ValidateRemoteURL` at request time:
  - `aura/internal/api/api.go:54` — propagate error rather than discarding via `_`.
  - `aura/internal/api/token.go` — same for the auth URL.
  - `query/embed/openai.go`, `ollama.go`, `huggingface.go` — validate `BaseURL` before each request.

#### Group D — Workflow hardening (closes #4, #5, #6, #8, #15)

- **REQ-F-D01:** Add `toolchain go1.26.3` directive to `go.mod`.
- **REQ-F-D02:** `.github/workflows/test.yml` and `release.yml` switch from `go-version: "stable"` to `go-version-file: go.mod`. Same for any other workflow that calls `setup-go`.
- **REQ-F-D03:** `.github/workflows/test.yml` adds a `govulncheck ./...` step (Linux runner is sufficient).
- **REQ-F-D04:** `.github/workflows/claude.yml` pins `actions/checkout` and `anthropics/claude-code-action` to commit SHAs (with `# v6` / `# v1` comments). Resolve the latter via `gh api repos/anthropics/claude-code-action/git/ref/tags/v1`.
- **REQ-F-D05:** `.github/workflows/cla-check.yml` adds `permissions: { pull-requests: read, contents: read }` and passes `TEAM_GRAPHQL_PERSONAL_ACCESS_TOKEN` via env (not argv) to `examine-pull-request`. If the helper script doesn't already accept env input, wrap the invocation so the PAT lands in env only.
- **REQ-F-D06:** `.github/workflows/update-website.yml` adds the `workflow_run` guard from `publish-pypi.yml:62-64` (head_branch == default + event == push).
- **REQ-F-D07:** `.github/workflows/publish-npm.yml` adds a checksum-verification step mirroring `publish-pypi.yml:130-156`. The step is gated `if: github.event.workflow_run.event == 'push'` so `make npm-publish-dry` still succeeds.

#### Group E — Output / query hardening (closes #3, #16, #17)

- **REQ-F-E01:** `neo4j-cli/query/uri.go` `normalizeURI` returns a stderr-warning string when the resolved scheme is `bolt` or `neo4j` (cleartext) AND the host is not loopback. Caller prints it via `cmd.PrintErrln`. The warning uses `(*url.URL).Redacted()` so userinfo passwords are masked.
- **REQ-F-E02:** `neo4j-cli/query/schema.go:212-214` escapes embedded backticks in the relType per Cypher rules: `strings.ReplaceAll(stripped, "`", "``")` before interpolation.
- **REQ-F-E03:** `common/output/output.go` `formatCell` runs string values through a new `stripControl` helper that replaces C0 runes (`< 0x20` except `\t`, `\n`, `\r`) and DEL (`0x7F`) with `?`. JSON marshal branch is already safe. Mirror the strip at the schema-render site `neo4j-cli/query/schema.go:325-339` if it bypasses `formatCell`.

#### Group F — findDotenv scope (closes #12)

- **REQ-F-F01:** Extract the duplicated `findDotenv` from `neo4j-cli/query/connect.go:307-320` and `neo4j-cli/query/embed/embed.go:319-332` into a shared helper (e.g. `common/clicfg/dotenv/dotenv.go`).
- **REQ-F-F02:** The walk stops at the first of: a `.git` directory in an ancestor, or the `$HOME` boundary (resolved via `os.UserHomeDir()`). If neither is encountered, the walk does not cross `$HOME`.
- **REQ-F-F03:** When a `.env` is loaded from a directory above CWD, emit `info: loading .env from <path>` to stderr.
- **REQ-F-F04:** Update the `query` / connection section of `README.md` with a 2–3 sentence note describing the new `.env` discovery rule and the stderr `info:` line.
- **REQ-F-F05:** Add a `changie` entry under `--kind Patch` covering the user-visible scope change. Use `changie new --projects neo4j-cli --kind Patch --body "..."`.

### Non-Functional Requirements

- **REQ-NF-001:** `make test`, `make lint`, `make fmt-check`, and `make build` pass on macOS and Linux. Windows CI matrix continues to pass (no behaviour regression).
- **REQ-NF-002:** No new direct dependencies in `go.mod` (indirect bumps from toolchain pin are acceptable).
- **REQ-NF-003:** No CLI flag added, removed, renamed, or repurposed.
- **REQ-NF-004:** Bundle round-trip test (`TestGenerator_RoundTrip`) stays green; if any cobra `Long`/`Example` change ripples through, regenerate via `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`.
- **REQ-NF-005:** No tracked file in `internal/skill/bundle/**` should drift outside an explicit regen commit (per `make generate-check`).
- **REQ-NF-006:** Each new test file follows the colocated `<action>_test.go` naming convention; afero MemMapFs is used for filesystem tests (per AGENTS.md hermetic-test rules).

## Technical Considerations

### Architecture / placement

- A single `RedactArgs` helper lives in `common/clievents/redact.go`. If a circular import surfaces (because `aura/internal/api` and the cobra-leaf packages need to import it), lift to `common/redact/redact.go`. Keep the secret-flag set in this one place — it is the source of truth for "what is sensitive".
- `ValidateRemoteURL` lives in a new `common/clicfg/urlcheck/` package so both Aura and embed clients share it. No dep on cobra/cfg — pure URL validation.
- `dotenv` helper lives in `common/clicfg/dotenv/`. Both the `query` package and the `query/embed` package import it; the existing duplication comment can be removed.

### Reused utilities (do not reimplement)

- `clierr.NewFatalError` for the corrupt-credentials recovery message.
- `(*url.URL).Redacted()` (already used at `query/uri.go:52`) for the cleartext-Bolt warning.
- `publish-pypi.yml`'s checksum-verify block + `workflow_run` guard — copy verbatim into the npm and update-website workflows.
- afero `OpenFile` / `Rename` (works on both `OsFs` and `MemMapFs`) for atomic writes.
- Stdlib `net.ParseIP`, `IP.IsLoopback`, `IP.IsPrivate`, `IP.IsLinkLocalUnicast`, `IP.IsLinkLocalMulticast`, `IP.IsMulticast` for the SSRF allowlist — no third-party SSRF library.
- Existing analytics test recorder in `common/clievents` / `common/analytics` test scaffolding for asserting Mixpanel payload shape.

### Risks / edge cases

- **Atomic rename on Windows:** `os.Rename` maps to `MoveFileEx` with `MOVEFILE_REPLACE_EXISTING`; afero's OsFs delegates correctly. CI Windows runner must be exercised in tests.
- **`io/fs` + afero:** `afero.File` supports `Sync()`. Verify on `MemMapFs` it is a no-op (true).
- **Hostname-based rebinding** is documented as out of scope for SSRF check; `ValidateRemoteURL` only resolves IP literals at parse time.
- **`govulncheck` step** must not fail the build for vulns that are not callable; use the default exit semantics (non-zero only on callable vulns).
- **`-race` in CI** is *not* part of this batch (out of scope per the SECURITY_ISSUES_TO_FIX.md scoping). Re-evaluate post-Group B.
- **Bundle drift gate:** any `Long` / `Example` change on a cobra command requires a paired `go generate` in the same commit (per AGENTS.md). Group E's URI-warning addition does NOT touch cobra help text, so this should not trigger.
- **Changie file format:** `kind: Patch`, `project: neo4j-cli`. Hand-author the YAML if changie isn't installed locally — see AGENTS.md "Changie Notes".

### Test data hygiene

- Per AGENTS.md, never use `afero.NewOsFs()` in `query` package tests — always `testfs.GetTestFs(...)` because the dev machine may have real credentials at the OS path. New tests in this batch follow the same rule.

## Acceptance Criteria

- [ ] `RedactArgs` masks every secret-bearing flag value across all three argv shapes (`--flag X`, `--flag=X`, `-f X`).
- [ ] Mixpanel test recorder shows `***` (not the secret) for `aura credential add --client-secret SECRET ...`, `credential dbms add --password SECRET ...`, `credential embed add --api-key SECRET ...`.
- [ ] Mocked 415/502/307/malformed-body responses on those same commands produce panic / fatal messages containing `***`, not `SECRET`.
- [ ] `neo4j-cli/main.go` and `aura/cmd/main.go` panic-recover paths print redacted args to stdout.
- [ ] Killing an in-flight `credential aura-client add` mid-write leaves `credentials.json` valid; next CLI invocation succeeds.
- [ ] `credentials.json` corruption (1-byte garbage append) produces a fatal error naming the `*.corrupt-<ts>` backup path; the next invocation starts with empty credentials and works.
- [ ] `~/.config/neo4j/cli/` is mode `0o700`; `credentials.json` is mode `0o600`.
- [ ] `AURA_BASE_URL=http://169.254.169.254 ./bin/neo4j-cli aura instance list` returns a clean error within 60 s; same for `http://10.0.0.1` and `http://api.example.com`.
- [ ] All four embed clients reject `http://` BaseURL to non-loopback hosts.
- [ ] `go.mod` declares `toolchain go1.26.3`. `actions/setup-go` reads `go-version-file: go.mod` in test/release workflows. `govulncheck ./...` is green in CI.
- [ ] `claude.yml` references SHAs (not tags) for both `actions/checkout` and `anthropics/claude-code-action`.
- [ ] `cla-check.yml` has explicit `permissions:` and the PAT is no longer a positional CLI argument.
- [ ] `update-website.yml` workflow_run is guarded on default branch + push event.
- [ ] `publish-npm.yml` verifies `dist/` checksums against the GitHub Release before publish; `make npm-publish-dry` still passes.
- [ ] `bolt://prod.example` emits a stderr warning; `bolt://localhost` does not.
- [ ] A relType named `` Foo`]->()-[r:DROP `` round-trips through schema introspection without breaking out of the backticks.
- [ ] A cell value of `"foo\x1b[31mbar"` renders as `foo?[31mbar` in table output (no live ANSI).
- [ ] `cd /tmp/x && echo NEO4J_PASSWORD=evil > /tmp/.env && neo4j-cli query …` does NOT load `/tmp/.env` (walk stops at `$HOME`); credentials store value is used.
- [ ] `.env` loaded from a directory above CWD prints `info: loading .env from <path>` to stderr.
- [ ] README documents the new `.env` discovery rule.
- [ ] `.changes/unreleased/neo4j-cli-Patch-*.yaml` exists for the `.env` scope change.
- [ ] `make test`, `make lint`, `make fmt-check` clean. `TestGenerator_RoundTrip` and `TestCommittedBundlesAndTestdataAreLF` pass.

## Out of Scope

- #9 — migration of `--password`/`--api-key`/`--client-secret`/`--instance-password` to `*-stdin` + TTY prompt. Tracked in `SECURITY_ISSUES_TO_FIX.md` for a later batch.
- #11 — custom `CheckRedirect` to reject cross-host redirects on Aura clients.
- #14 — removing on-disk caching of Aura access tokens; OS keychain integration.
- #19 — `sync.Mutex` on `Credentials` and parent back-references on each sub-credential type.
- ctx plumbing through `MakeRequest` / `getToken` (separate refactor).
- `make test -race` adoption (separate ticket).
- Any changes to skill bundle install / generation pipeline.

## Open Questions

- Should `cla-check.yml`'s helper `examine-pull-request` script be modified upstream (`neo-technology/whitelist-check`) to accept the PAT via env var, or do we wrap the invocation locally? Default in-plan is "wrap locally if upstream doesn't already support env", but this needs confirmation when the change lands.
- For `claude.yml` — pin `anthropics/claude-code-action@v1` to which SHA exactly? Resolved at fix time via `gh api repos/anthropics/claude-code-action/git/ref/tags/v1`. No decision needed up front; confirm during implementation.
