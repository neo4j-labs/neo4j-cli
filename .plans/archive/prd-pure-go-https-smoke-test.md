# PRD: Pure-Go HTTPS smoke test for `neo4j-cli query`

## Overview

Replace `scripts/test-https.sh` (172 lines of bash) and its thin
`query_https_smoke_test.go` wrapper with a single self-contained pure-Go
integration test that boots a real `neo4j:5` Docker container with HTTPS
enabled and exercises the `--insecure` flag end-to-end.

The test stays env-gated (`NEO4J_HTTPS_TEST=1`) so `go test ./...` is
unaffected, but its body matches the rest of the suite: stdlib + testify,
in-process cobra invocation, no external shell tools beyond `docker`.

## Goals

- Remove the bash/openssl/curl/lsof dependency from the test path; only
  `docker` remains as an external requirement.
- Match the codebase's idiomatic Go test style (`NewCmd(cfg).Execute()`
  driven via `cmd.SetArgs(...)`, captured stdout/stderr buffers, testify
  assertions) — same harness shape as `neo4j-cli/query/run_test.go`.
- Drop the `make build` step from the smoke-test path (in-process
  invocation needs no binary).
- Keep both assertions: positive (`--insecure` succeeds, returns `1`) and
  negative (default verification rejects self-signed cert with TLS-related
  error).
- Eliminate hard-coded ports 7473/7474 so the test doesn't fail when those
  ports are already in use locally.

## Non-Goals

- Switching to `testcontainers-go` or any other new test dependency.
- Introducing a custom Dockerfile (a plain `docker run neo4j:5` with env
  vars + bind-mounted SSL dir is sufficient).
- Running the smoke test on Windows runners (Linux container model;
  build constraint stays `//go:build !windows`).
- Running the smoke test by default in `go test ./...` — it stays
  env-gated.
- Expanding coverage beyond the `--insecure` positive/negative pair (other
  HTTPS scenarios are out of scope here).
- Migrating any other shell-driven tests; only `scripts/test-https.sh` is
  in scope.

## Requirements

### Functional Requirements

- REQ-F-001: A single Go test function `TestHTTPS_Smoke` in
  `neo4j-cli/query/query_https_smoke_test.go` performs the full smoke
  flow: cert generation, container boot, readiness wait, positive +
  negative assertions, teardown.
- REQ-F-002: The test skips when `NEO4J_HTTPS_TEST` is not set to `1`,
  using `t.Skip` with a message that names the env var and the docker
  requirement.
- REQ-F-003: The test pre-flights `docker` via `exec.LookPath` and calls
  `t.Fatal` with a clear message when missing.
- REQ-F-004: The test generates a self-signed RSA-2048 cert (CN=localhost,
  30-day validity) using stdlib `crypto/rand`, `crypto/rsa`,
  `crypto/x509`, `encoding/pem`. Files written into `t.TempDir()` are
  `private.key` (0644), `public.crt` (0644), plus empty `trusted/` and
  `revoked/` subdirs (0755).
- REQ-F-005: The test selects two random free TCP ports on `127.0.0.1`
  via `net.Listen("tcp", "127.0.0.1:0")`, records each port, closes the
  listener, then passes them to `docker run` as `-p <https_port>:7473`
  and `-p <http_port>:7474`.
- REQ-F-006: The test boots `neo4j:5` via `exec.Command("docker", "run",
  "-d", "--rm", "--name", "neo4j-https-test", ...)` with the same env
  vars used today: `NEO4J_AUTH`, `NEO4J_server_https_enabled`,
  `NEO4J_dbms_ssl_policy_https_*`. The bind-mount target stays `/ssl:ro`.
- REQ-F-007: Before booting, the test best-effort removes any leftover
  container named `neo4j-https-test` so a crashed previous run doesn't
  block the next run. `t.Cleanup` registers a `docker rm -f` for
  guaranteed teardown.
- REQ-F-008: The test polls `https://127.0.0.1:<https_port>` with a 60s
  deadline using `http.Client` with `tls.Config{InsecureSkipVerify:
  true}`. On timeout, dumps `docker logs --tail 50 neo4j-https-test` to
  test output before calling `t.Fatalf`.
- REQ-F-009: The positive assertion drives the CLI in-process via
  `NewCmd(cfg)` + `cmd.SetArgs(...)` + `cmd.Execute()` with `--uri
  https://127.0.0.1:<https_port>`, `--insecure`, `-u neo4j`, `-p
  testtest`, `RETURN 1 AS n`. Asserts no error and stdout contains `1`.
- REQ-F-010: The negative assertion is the same invocation without
  `--insecure`. Asserts an error is returned and the error message
  matches `(?i)tls|x509|certificate`.
- REQ-F-011: `scripts/test-https.sh` is deleted.
- REQ-F-012: `.github/workflows/test.yml`'s "HTTPS smoke test" step
  changes from `run: bash scripts/test-https.sh` to `run:
  NEO4J_HTTPS_TEST=1 go test -run TestHTTPS_Smoke -v
  ./neo4j-cli/query/...`. The `if: matrix.os == 'ubuntu-latest'` gate is
  preserved.
- REQ-F-013: AGENTS.md "Local Verification Scripts" subsection (line
  ~208) is updated: drop the `scripts/test-https.sh` bullet, replace with
  a one-line note pointing at the env-gated Go test, drop
  `openssl`/`curl` from the requirements list, keep `docker` and the
  description of what the test exercises.

### Non-Functional Requirements

- REQ-NF-001: Build constraint stays `//go:build !windows`.
- REQ-NF-002: No new module dependencies. Only `os/exec` + stdlib `crypto/*`,
  `net`, `net/http`, `crypto/tls`, plus already-imported testify and
  internal packages (`clicfg`, `testfs`).
- REQ-NF-003: `make fmt-check` clean (no gofmt drift) — this is a CI gate.
- REQ-NF-004: `make lint` clean (golangci-lint v2 config in
  `.golangci.yml`).
- REQ-NF-005: `go test ./...` (without env var) is unaffected — the smoke
  test must reliably skip and add no measurable runtime to the default
  suite.
- REQ-NF-006: Container teardown happens even when assertions fail
  (`t.Cleanup` runs on failure paths). After the test exits, `docker ps`
  shows no `neo4j-https-test` container.

## Technical Considerations

### Drives the cobra command in-process

Matches the harness style in `neo4j-cli/query/run_test.go:77-87`:

```go
cmd := NewCmd(cfg)
cmd.SetOut(&stdout)
cmd.SetErr(&stderr)
cmd.SetContext(ctx)
cmd.SetArgs([]string{"--uri", uri, "--insecure", "-u", "neo4j",
    "-p", "testtest", "RETURN 1 AS n"})
err := cmd.Execute()
```

`cfg` comes from `clicfg.NewConfig(testfs.GetTestFs(...), "test")` — same
pattern as `run_test.go:52-56`. No need to reuse the full `runHarness`
struct (its stdin seams aren't relevant here); inline a minimal
stdout/stderr pair.

### Cert generation in stdlib

~30 lines: `rsa.GenerateKey(rand.Reader, 2048)`, `x509.Certificate{
SerialNumber, Subject: pkix.Name{CommonName: "localhost"}, NotBefore,
NotAfter, ... }`, `x509.CreateCertificate(...)`, `pem.Encode` for both
key and cert. No external `openssl` invocation. Cert files are
0644-readable (Neo4j container runs as uid 7474; bind mount is read-only;
host dir is inside `t.TempDir()` which is private).

### Random port selection

```go
func freePort(t *testing.T) int {
    l, err := net.Listen("tcp", "127.0.0.1:0")
    require.NoError(t, err)
    port := l.Addr().(*net.TCPAddr).Port
    require.NoError(t, l.Close())
    return port
}
```

Tiny TOCTOU race between `Close()` and `docker run -p` is acceptable for a
dev-only smoke test; far better than today's "fail hard if 7473/7474 are
in use".

### Container lifecycle

```go
_ = exec.Command("docker", "rm", "-f", "neo4j-https-test").Run() // pre-clean
out, err := exec.Command("docker", "run", "-d", "--rm",
    "--name", "neo4j-https-test",
    "-p", fmt.Sprintf("%d:7473", httpsPort),
    "-p", fmt.Sprintf("%d:7474", httpPort),
    "-e", "NEO4J_AUTH=neo4j/testtest",
    "-e", "NEO4J_server_https_enabled=true",
    "-e", "NEO4J_dbms_ssl_policy_https_enabled=true",
    "-e", "NEO4J_dbms_ssl_policy_https_base__directory=/ssl",
    "-e", "NEO4J_dbms_ssl_policy_https_private__key=private.key",
    "-e", "NEO4J_dbms_ssl_policy_https_public__certificate=public.crt",
    "-e", "NEO4J_dbms_ssl_policy_https_client__auth=NONE",
    "-v", certDir+":/ssl:ro",
    "neo4j:5",
).CombinedOutput()
require.NoError(t, err, "docker run: %s", out)
t.Cleanup(func() {
    _ = exec.Command("docker", "rm", "-f", "neo4j-https-test").Run()
})
```

### Readiness wait

60s deadline; sleep 1s between probes; on timeout dump `docker logs
--tail 50` before failing. Matches the script's behaviour.

### Why no Dockerfile

Baking the cert/config into a custom image would either commit keys
(bad) or require a multi-stage build that generates them at image-build
time (more moving parts than `docker run` with env vars). The plain
`docker run neo4j:5` approach with env vars + bind-mount is simpler and
already proven by the existing script.

### Why no testcontainers-go

`feedback_real_neo4j_for_integration.md` records a prior preference to
avoid it. The `os/exec` approach is ~40 lines and zero new deps;
testcontainers-go would pull in a `moby/*` dependency tree for marginal
ergonomic gain.

### Memory update

Once shipped, update `feedback_real_neo4j_for_integration.md` so the
recommendation flips from "follow `scripts/test-*.sh` + Go-wrapper
pattern" to "pure Go (`os/exec` + stdlib cert gen), no bash wrappers".
The "use real Neo4j docker, not httptest mocks" core stays.

## Acceptance Criteria

- [ ] `neo4j-cli/query/query_https_smoke_test.go` is the only file
      driving the smoke test; no bash, no separate wrapper.
- [ ] `scripts/test-https.sh` is removed; `grep -rn 'test-https' .`
      returns nothing in the working tree.
- [ ] `make test` and `make fmt-check` and `make lint` are clean.
- [ ] `go test ./...` (without env var) skips `TestHTTPS_Smoke` cleanly.
- [ ] `NEO4J_HTTPS_TEST=1 go test -run TestHTTPS_Smoke -v
      ./neo4j-cli/query/...` boots `neo4j:5`, both positive and negative
      assertions pass, container is torn down on exit.
- [ ] After a failed/aborted run (`Ctrl-C` mid-test), a subsequent run
      still succeeds — leftover `neo4j-https-test` container is cleaned
      up at startup.
- [ ] `.github/workflows/test.yml` invokes `go test` with the env var
      instead of `bash scripts/test-https.sh`; the ubuntu-only gate is
      preserved.
- [ ] AGENTS.md "Local Verification Scripts" subsection no longer
      mentions `scripts/test-https.sh` or `openssl`/`curl`; the new
      entry points at the Go test.
- [ ] `feedback_real_neo4j_for_integration.md` memory updated to
      recommend the pure-Go pattern.

## Out of Scope

- Migrating any other test or script to a different harness.
- Adding new HTTPS scenarios (mTLS, custom CA bundle, etc.) — only the
  current `--insecure` positive/negative pair is in scope.
- Windows support for the smoke test.
- Default-on coverage in `go test ./...` (stays env-gated).
- A new `make` target to invoke the smoke test (env-var prefix on `go
  test` is sufficient).

## Open Questions

None — all three previously open questions resolved during planning:

- Gate: `NEO4J_HTTPS_TEST=1` env var (kept).
- CLI driver: in-process `NewCmd(cfg).Execute()`.
- Ports: random free ports via `net.Listen("tcp", "127.0.0.1:0")`.
