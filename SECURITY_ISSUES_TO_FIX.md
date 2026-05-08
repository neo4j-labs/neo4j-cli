# Security Issues To Fix

Findings from a whole-codebase security review. Ordered by severity. No CRITICAL findings.

---

## HIGH

### 1. Passwords + client secrets shipped to Mixpanel on every CLI run

**Where:** `common/clievents/clievents.go:41–106`, called from `neo4j-cli/main.go:30,39,41`.

**What:** `Emit(events, os.Args[1:], state)` fires on every invocation. Dispatch by `args[0]`:

- `query` → command name only (correct).
- `aura` → full args via `fmt.Sprint(args)` (line 77). Comment claims "Aura commands contain no PII" — **wrong**: `aura credential add` has `--client-secret` (`neo4j-cli/aura/internal/subcommands/credential/add.go:39`).
- `skill` → full args (no secret flags today).
- `default` → full args. Hits `neo4j-cli credential dbms add --password …` (`neo4j-cli/internal/subcommands/credential/dbms/add.go:60`) and `credential embed add --api-key …` (`embed/add.go:61`).

**Concrete leak commands:**
- `neo4j-cli aura credential add --client-secret SECRET` → `AURA` event with secret.
- `neo4j-cli credential dbms add --password SECRET` → `COMMAND` event with password.
- `neo4j-cli credential embed add --api-key SECRET` → `COMMAND` event with key.

**Why it's HIGH:** secrets persist at a third party (Mixpanel). Revocation = rotate every cred ever passed via these commands + Mixpanel data-deletion request. `Analytics.Disable()` exists at `common/analytics/analytics.go:164` but is never called in production — no opt-out exists today.

**Fix:** mirror the `query` branch — record only the command path (e.g. first 2-3 non-flag tokens) + `success` bool. Apply to `aura`, `skill`, default. Belt-and-braces: scrub known secret flags (`--password`, `--client-secret`, `--api-key`) at the top of `Emit()`.

**Cross-ref:** the redaction helper here closes #2 too — ship together.

---

### 2. Same secrets echoed to stdout via `os.Args[1:]` in panic/error templates

**Where:** eleven sites, all formatting `os.Args[1:]` into user-visible error text:
- `neo4j-cli/main.go:19` (panic recover; printed to stdout)
- `neo4j-cli/aura/cmd/main.go:22` (legacy entrypoint)
- `neo4j-cli/aura/internal/api/response.go:44, 51, 71, 83, 97, 111, 121, 131, 141, 335`
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/utils.go:42`

**What:** any unexpected HTTP status while running `aura credential add --client-secret …` (or the dbms/embed equivalents) prints the secret to stdout. response.go:141 also includes `resBody`. Bug reports / CI logs / shell history capture it.

**Fix:** drop `os.Args[1:]` from the templates; replace with a generic "report an issue" pointer. Or build a redaction helper that rewrites known secret-flag values to `***` before formatting — share with #1.

---

### 3. Cleartext Bolt URI accepted silently

**Where:** `neo4j-cli/query/uri.go:30–48`.

**What:** only `http://` / `https://` get rewritten. `bolt://` and `neo4j://` (cleartext) pass through with no warning. Combined with passwords in dotenv/flag, MITM reads the credential on hostile networks.

**Fix:** stderr warning when scheme is plaintext and host is not loopback; document `neo4j+s://` as recommended.

---

### 4. `claude.yml` runs third-party action with `id-token: write` + OAuth token, not SHA-pinned

**Where:** `.github/workflows/claude.yml:29, 35`.

**What:** `actions/checkout@v4` and `anthropics/claude-code-action@v1` are tag refs. Job carries `id-token: write` and `CLAUDE_CODE_OAUTH_TOKEN`. Compromise of either tag at upstream gains the secret.

**Fix:** pin both to commit SHAs (matches the rest of the repo's convention). Renovate's `pinDigests` should auto-maintain.

---

### 5. `cla-check.yml` — `pull_request_target` + PAT passed via argv

**Where:** `.github/workflows/cla-check.yml:4`.

**What:** triggered in privileged context (has secrets). Checks out `neo-technology/whitelist-check` (tag-pinned), runs `./bin/examine-pull-request` with `TEAM_GRAPHQL_PERSONAL_ACCESS_TOKEN` as a CLI argument (visible to `ps`). No top-level `permissions:` block — defaults are repo-wide.

**Fix:** add `permissions: { pull-requests: read, contents: read }`, SHA-pin the helper repo, pass PAT via env var (not argv).

---

### 6. `update-website.yml` workflow_run lacks branch/event guard

**Where:** `.github/workflows/update-website.yml:9–19`.

**What:** grants `contents: write` + `pull-requests: write`, gated only on `workflow_run.conclusion == 'success'`. Missing the `head_branch == default && event == 'push'` guard that `publish-pypi.yml:62-64` already applies.

**Fix:** add the same guard.

---

### 7. `credentials.json` write is non-atomic + `load()` panics on corrupt JSON

**Where:** `common/clicfg/fileutils/fileutils.go:44–49` (`WriteFile`), `common/clicfg/credentials/credentials.go:58` (`load()` panics on unmarshal).

**What:** `afero.WriteFile` truncates then writes — no `Sync()`, no temp+rename. Crash mid-write (power loss, OOM, `kill -9`) or a 1-byte tamper corrupts JSON. `load()` then `panic`s on every CLI invocation → user is bricked and loses every stored Aura/dbms/embed credential. `UpdateAccessToken` exercises this write path on every API call needing a fresh token, so the corruption window is hot during normal operation.

**Why HIGH:** data-loss grade for every user. One unlucky `kill` is enough.

**Fix:** atomic write — open `credentials.json.tmp` in the same dir (mode 0o600) → write → `Sync()` → `os.Rename` over target. Apply the same pattern to `clicfg.go:454` (config write). In `load()`, replace `panic` with a clean error; on unmarshal failure, save the bad bytes to `credentials.json.corrupt-<ts>` and reset to empty so the user keeps a forensic copy without being locked out.

**Cross-ref:** pairs with #19 (mutex) and #20 (umask race) — credentials.json hardening package.

---

### 8. Reachable stdlib vulns; Go toolchain unpinned

**Where:** `go.mod` (no `toolchain` directive, only `go 1.25.0`); `.github/workflows/test.yml`, `release.yml`, `publish-npm.yml`, `publish-pypi.yml` use `actions/setup-go@... go-version: stable`.

**What:** `govulncheck ./...` reports `GO-2026-4918` (HTTP/2 infinite loop on bad `SETTINGS_MAX_FRAME_SIZE`) reachable from `query/embed/{openai,ollama,huggingface}.go` and `common/analytics/analytics.go`. Aura API calls in `api.go`/`token.go` are also vulnerable but govulncheck didn't trace them because `panic` short-circuits its callgraph. Local toolchain is `1.26.2`; fix is in `1.26.3`. CI's `stable` may already pull the patched build but isn't guaranteed; local builds for sure aren't.

**Why HIGH:** a misconfigured proxy/CDN can hang the CLI indefinitely on every embed/analytics/Aura call.

**Fix:** add `toolchain go1.26.3` to `go.mod`. Pin CI matrix `go-version` to `'1.26.3'` (or higher patch). Add `govulncheck ./...` as a CI step so future stdlib regressions are caught.

---

### 9. Secrets accepted as plain CLI flags → leak via `ps` and shell history

**Where:**
- `neo4j-cli/internal/subcommands/credential/dbms/add.go:60` (`--password`)
- `neo4j-cli/internal/subcommands/credential/embed/add.go:61` (`--api-key`)
- `neo4j-cli/aura/internal/subcommands/credential/add.go` (`--client-secret`)
- `neo4j-cli/aura/internal/subcommands/instance/create.go` (`--instance-password`)
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/create.go:117` (`--instance-password`)

**What:** all required-as-flag, no stdin / TTY / env fallback at the leaf. Visible in `/proc/<pid>/cmdline` and `ps auxw` for the entire process lifetime — readable by any local user on shared hosts (CI runners, devcontainers, multi-user dev VMs). Quoted invocations land in `~/.zsh_history` / `~/.bash_history`. The `query --password` flow already does it correctly via `term.ReadPassword` (`neo4j-cli/query/run.go:139`) — that pattern needs to extend.

**Why HIGH:** `--client-secret` unlocks the entire Aura tenant; trivial exfil on multi-tenant boxes.

**Fix:** mirror `query promptPassword` — TTY prompt fallback when flag is empty. Add `--password-stdin` / `--api-key-stdin` / `--client-secret-stdin` for scripted use. Document env-var alternatives (`NEO4J_PASSWORD`, `OPENAI_API_KEY`, `HF_TOKEN`, `NEO4J_EMBED_API_KEY`) prominently in `--help`.

---

### 10. Credentials parent dir created world-traversable (0o755)

**Where:** `common/clicfg/fileutils/fileutils.go:62`, `common/clicfg/clicfg.go:72`.

**What:** `MkdirAll(filepath.Dir(path), 0o755)` for `~/.config/neo4j/cli/` (Linux) and `~/Library/Preferences/neo4j/cli/` (macOS). The file itself is 0o600 (good), but the directory is world-traversable. On Linux distros where `$HOME` defaults to 0o755, on shared CI runners, and in devcontainers, any local user can `ls` to enumerate stored credential filenames + stat metadata. macOS protects via `$HOME=0o700` — not portable.

**Fix:** `MkdirAll(..., 0o700)` for the credentials dir. Match SSH/GnuPG convention. Skill bundle install dirs (`common/skill/filesystem.go:52`, `installer.go:258`) can stay 0o755 — no secrets there.

---

### 11. Cross-host HTTP redirect can forward state-changing request bodies

**Where:** `neo4j-cli/aura/internal/api/api.go:40` (`http.Client{}`), `neo4j-cli/aura/internal/api/token.go:43`.

**What:** default Go redirect policy follows up to 10 hops with no host check. Stdlib correctly strips `Authorization` / `Cookie` cross-origin (mitigates bearer leak), but **request bodies are forwarded**. A 302 from a misconfigured / hostile `--base-url` (or a DNS hijack of `api.neo4j.io`) on a `PATCH /instances/<id>` request re-POSTs the body — which carries `password`, `customer_managed_key_id`, `cmk_arn`, etc. — to the redirect target.

**Why HIGH:** any attacker-positioned redirect on state-changing calls leaks fresh secrets. Risk gated on `--base-url` override or DNS hijack of the prod Aura host. Compounds with #18 (no SSRF check on `--base-url`).

**Fix:** custom `CheckRedirect` on both clients rejecting cross-host. Aura REST does not 302 in normal operation, so a hard-fail policy is fine:
```go
CheckRedirect: func(req *http.Request, via []*http.Request) error {
    if len(via) > 0 && req.URL.Host != via[0].URL.Host {
        return fmt.Errorf("aura: refusing cross-host redirect to %s", req.URL.Host)
    }
    if len(via) >= 3 { return http.ErrUseLastResponse }
    return nil
},
```

---

## MEDIUM

### 12. `findDotenv` walks parents to `/` — attacker-controlled `.env` overlay

**Where:** `neo4j-cli/query/connect.go:307–320`, `neo4j-cli/query/embed/embed.go:319–332`.

**What:** running `neo4j-cli query` from `/tmp/work` while `/tmp/.env` contains `NEO4J_PASSWORD=evil` (or `NEO4J_EMBED_API_KEY` / `NEO4J_EMBED_BASE_URL`) silently overlays the values. A malicious checkout can redirect API-key transmission.

**Fix:** stop walk at `$HOME` or git-repo root; print which `.env` was loaded; require explicit `--env` to read above CWD.

---

### 13. Aura HTTP / OAuth / embed clients have no `Timeout`

**Where:**
- `neo4j-cli/aura/internal/api/api.go:40` (`http.Client{}`)
- `neo4j-cli/aura/internal/api/token.go:43`
- `neo4j-cli/query/embed/openai.go:37`
- `neo4j-cli/query/embed/ollama.go:36`
- `neo4j-cli/query/embed/huggingface.go:43`

**What:** all four use `http.Client{}` with no `Timeout`. Slow-loris hangs the CLI indefinitely; CI jobs hang until the job-level timeout. Embed clients have a comment "cancellation owned by caller's ctx" but `cmd.Context()` has no deadline by default, so Ctrl-C is the only escape. The Aura clients additionally use `http.NewRequest` (no ctx), so SIGINT can't even cancel them. (Analytics client at `common/analytics/analytics.go:78` is the only one done right: `Timeout: 10 * time.Second`.)

**Fix:** `http.Client{Timeout: 60 * time.Second}` on all four. Plumb `cmd.Context()` into `MakeRequest` / `getToken` via `http.NewRequestWithContext` so SIGINT cancels in-flight calls.

---

### 14. Aura access tokens persisted plaintext to `credentials.json`

**Where:** `common/clicfg/credentials/aura.go:91–104, 160`.

**What:** `AccessToken` written next to `client-secret`. Expiry is honoured but tokens linger on disk past process exit; reading the file yields a usable bearer within `expires_in` (~1h).

**Fix:** keep token in-memory only, or zero `access-token`/`token-expiry` on graceful exit.

---

### 15. `publish-npm.yml` doesn't verify `dist/` against release checksums

**Where:** `.github/workflows/publish-npm.yml`.

**What:** consumes the `dist/` artifact from the release workflow_run without checksum verification. PyPI workflow does verify (`publish-pypi.yml:140–159`). Tampered artifact (workflow_run cache-poisoning class) would slip into npm packages.

**Fix:** mirror the PyPI `Verify dist archives against release checksums` step.

---

### 16. Cypher backtick escape gap in schema introspection

**Where:** `neo4j-cli/query/schema.go:212–214`.

**What:** `fmt.Sprintf("MATCH (n)-[r:`%s`]->(m)…", stripped)` — relType comes from the connected DB. `stripRelTypeWrap` only strips leading `:`/backtick/quote, NOT embedded backticks. A relType containing a backtick (e.g. `` Foo`]->()-[r:DROP `` ) breaks out. Trust boundary is server→client; needs server compromise to exploit, but the unsanitised interpolation is a code-smell.

**Fix:** reject `stripped` containing backtick/newline/null, or escape per Cypher rules (double the backtick).

---

### 17. ANSI / control-char passthrough in output

**Where:** `common/output/output.go:104, 128`, `neo4j-cli/query/schema.go:325–339`.

**What:** API responses + driver result strings rendered verbatim into terminal output. Attacker-controlled tenant/instance/relType names containing `\x1b[…]` can spoof terminal output / clickjack.

**Fix:** strip C0 control chars in `formatCell` before render.

---

### 18. SSRF — `--base-url` / `--auth-url` / `AURA_BASE_URL` accept any scheme/host

**Where:** `common/clicfg/clicfg.go:327–340` (`BaseUrl`), `clicfg.go:356–358` (`AuthUrl`), `neo4j-cli/aura/internal/api/api.go:54` (`url.ParseRequestURI` error swallowed via `_`). Embed `BaseURL` has the same gap (`query/embed/openai.go`, `huggingface.go`, `ollama.go`).

**What:** no scheme allowlist (`http://` accepted), no host blocklist (no rejection of RFC1918 / link-local / `169.254.169.254` metadata IP / loopback / `::1`). A malicious tutorial / shell history / LLM-generated config sets `AURA_BASE_URL=http://169.254.169.254/latest/meta-data/` and the CLI happily POSTs the OAuth token-fetch + bearer-auth GET to that endpoint. Compounds with #11 (cross-host redirect) and #13 (no timeout). The dropped `ParseRequestURI` error means a malformed base-url panics on `u.JoinPath` rather than returning a clean usage error.

**Fix:** in `BaseUrl()` / `AuthUrl()` and the embed base validators, require `https://` (allow `http://localhost` / `127.0.0.1` / `::1` for local dev only); reject private/link-local hosts. Surface the `ParseRequestURI` error at `api.go:54` instead of `_`.

---

### 19. `Credentials.onUpdate` callback has no concurrency control

**Where:** `common/clicfg/credentials/credentials.go` plus mutators in `aura.go`, `dbms.go`, `embed.go` (`Add` / `Remove` / `SetDefault` / `UpdateAccessToken` / `ClearAccessToken`).

**What:** every mutator updates the in-memory slice and calls `c.onUpdate()` (= `Credentials.save()` → marshal → file write) with no mutex. CLI is single-threaded today so this doesn't bite, but `UpdateAccessToken` runs from inside `getToken` on every refresh; once any future code adds a goroutine that issues two Aura calls concurrently (parallel snapshot list + delete with `--await`, batch operations), two `save()`s race on `credentials.json` → torn write or interleaved JSON. Combined with #7 (non-atomic write), that loses creds.

**Fix:** add a `sync.Mutex` to `Credentials`; lock around every mutation + `save()`. ~5-line change, cheap insurance.

---

### 20. `credentials.json` first-create umask race

**Where:** `common/clicfg/fileutils/fileutils.go:61–74` (`createFile`).

**What:** `fs.Create(path)` opens with mode 0o666 masked by process umask (typically 0o022 → file is briefly 0o644 on disk), then `fs.Chmod(path, 0o600)` tightens it. Window between Create and Chmod is microseconds but real — a colocated process can `open(O_RDONLY)` and hold the FD across Chmod, then read whatever is later written. Triggered only on first creation; subsequent `WriteFile` paths preserve the existing 0o600 mode, but those still rely on umask correctness for the create flag.

**Fix:** `fs.OpenFile(path, O_CREATE|O_WRONLY|O_TRUNC, 0o600)` directly. Never rely on umask. AGENTS.md "Hermetic Test Notes" already calls out this exact umask gotcha for tests — production code should match.
