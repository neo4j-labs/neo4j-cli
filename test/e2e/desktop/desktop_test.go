// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build e2e_desktop

// Package desktop_test drives the freshly built `neo4j-cli-desktop-e2e` binary
// (built with `-tags e2e_desktop_seams`) as a subprocess against the
// relate-shaped fixture in `test/e2e/desktop_fixture`. It covers every
// endpoint the production `desktop` / `desktop connection` / `query
// -c desktop[-connection:<uuid>]` flow touches, plus the transport and
// resource sad-paths that have user-visible canonical error messages.
//
// Gated by build tag `e2e_desktop` so it stays out of the default
// `go test ./...` matrix, mirroring the `e2e_exitcodes` pattern in
// `test/e2e/exitcodes/`.
package desktop_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Scenario admin endpoints + fixed values shared across cases. Listed here so a
// quick read enumerates the wire surface the suite drives.
const (
	scenarioReset    = "/_scenario/reset"
	scenarioDbms     = "/_scenario/dbms"
	scenarioConn     = "/_scenario/connection"
	scenarioCred     = "/_scenario/credential"
	scenarioAuth     = "/_scenario/auth_mode"
	scenarioVers     = "/_scenario/versions"
	scenarioLog      = "/_scenario/log"
	scenarioAutoProg = "/_scenario/auto_progress"
	scenarioPlug     = "/_scenario/plugin"
	fixtureSalt      = "testsalt"
	fixtureHomeKey   = "fixturehome"
)

// binPath holds the absolute path to the freshly-built CLI binary tagged with
// `e2e_desktop_seams`. Populated once by TestMain so every subtest shares one
// build (a multi-target build is a few seconds; per-subtest would be wasteful).
var binPath string

// fixtureBin holds the absolute path to the desktop_fixture binary so each
// subtest can spawn a fresh fixture process (a fresh process gives every test
// its own listen port and avoids cross-test scenario-state contamination).
var fixtureBin string

// TestMain builds both binaries once into a temp dir, runs the suite, then
// removes the dir. CGO_ENABLED=0 mirrors release builds and avoids cgo
// toolchain issues on minimal CI runners.
func TestMain(m *testing.M) {
	code, err := runMain(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

func runMain(m *testing.M) (int, error) {
	root, err := repoRoot()
	if err != nil {
		return 0, err
	}
	dir, err := os.MkdirTemp("", "neo4j-cli-desktop-e2e-*")
	if err != nil {
		return 0, fmt.Errorf("mkdir bin dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	cliName := "neo4j-cli-desktop-e2e"
	fixName := "desktop-fixture"
	if runtime.GOOS == "windows" {
		cliName += ".exe"
		fixName += ".exe"
	}
	binPath = filepath.Join(dir, cliName)
	fixtureBin = filepath.Join(dir, fixName)

	build := exec.Command("go", "build", "-tags", "e2e_desktop_seams", "-o", binPath, "./neo4j-cli")
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if combined, buildErr := build.CombinedOutput(); buildErr != nil {
		return 0, fmt.Errorf("go build (CLI) failed: %w\n%s", buildErr, combined)
	}

	buildFx := exec.Command("go", "build", "-o", fixtureBin, "./test/e2e/desktop_fixture")
	buildFx.Dir = root
	buildFx.Env = append(os.Environ(), "CGO_ENABLED=0")
	if combined, buildErr := buildFx.CombinedOutput(); buildErr != nil {
		return 0, fmt.Errorf("go build (fixture) failed: %w\n%s", buildErr, combined)
	}

	return m.Run(), nil
}

// repoRoot walks up from this test file to the repo root (directory containing
// go.mod). Mirrors test/e2e/exitcodes's helper.
func repoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot determine caller file path")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("could not locate repo root walking up from test file")
}

// fixture spawns the fixture binary on a fresh ephemeral port, captures its
// stdout to read back the `listening on http://...` line, and registers a
// t.Cleanup that SIGKILLs the process when the test ends. Returns the parsed
// listen URL so tests can drive the scenario admin endpoints directly.
type fixture struct {
	URL    string
	Cmd    *exec.Cmd
	Stderr *syncBuf
}

// syncBuf is a goroutine-safe bytes.Buffer wrapper — the fixture's stderr
// drain runs in a goroutine and tests may dump it on failure from the test
// goroutine. The whole thing is locked under one mutex; throughput doesn't
// matter for a debug log buffer.
type syncBuf struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// startFixture spawns the fixture, scrapes the listen URL off its first stdout
// line, and stashes stderr in a buffer the test can dump on failure.
func startFixture(t *testing.T) *fixture {
	t.Helper()
	cmd := exec.Command(fixtureBin, "--salt", fixtureSalt)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr := &syncBuf{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fixture: %v", err)
	}

	// First stdout line is `listening on http://127.0.0.1:<port>`; everything
	// after is dropped (we don't care about further fixture stdout — the
	// trace lives on the scenarioLog admin endpoint).
	reader := bufio.NewReader(stdout)
	type readResult struct {
		line string
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		line, err := reader.ReadString('\n')
		ch <- readResult{line: line, err: err}
	}()
	var line string
	select {
	case r := <-ch:
		if r.err != nil {
			_ = cmd.Process.Kill()
			t.Fatalf("read fixture stdout: %v (stderr: %s)", r.err, stderr.String())
		}
		line = r.line
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("timed out waiting for fixture listen line (stderr: %s)", stderr.String())
	}

	// Drain the rest of stdout in the background so the pipe never blocks.
	go func() {
		_, _ = io.Copy(io.Discard, reader)
	}()

	line = strings.TrimSpace(line)
	const prefix = "listening on "
	if !strings.HasPrefix(line, prefix) {
		_ = cmd.Process.Kill()
		t.Fatalf("unexpected fixture greeting %q (stderr: %s)", line, stderr.String())
	}
	url := strings.TrimPrefix(line, prefix)

	fx := &fixture{URL: url, Cmd: cmd, Stderr: stderr}
	t.Cleanup(func() {
		// Best-effort terminate; the fixture exits cleanly on SIGTERM but
		// SIGKILL is the cross-platform fallback when SIGTERM is racy.
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	// Wait until the fixture's HTTP listener answers — the listen-line print
	// runs BEFORE srv.Serve(ln) so there is a tiny window where the line is
	// printed but the listener hasn't accepted yet. Poll the probe path.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/fastify/api-docs")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				return fx
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("fixture %s did not answer /fastify/api-docs within 3s (stderr: %s)", url, stderr.String())
	return nil
}

// scenarioPOST is a thin helper that wraps an unauthenticated POST to one of
// the fixture's `_scenario/*` admin endpoints. Failures terminate the test —
// scenario setup errors are never recoverable.
func scenarioPOST(t *testing.T, fx *fixture, path string, body any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal scenario body: %v", err)
		}
		rdr = strings.NewReader(string(buf))
	}
	req, err := http.NewRequest(http.MethodPost, fx.URL+path, rdr)
	if err != nil {
		t.Fatalf("scenario request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("scenario do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("scenario %s: status %d body %s", path, resp.StatusCode, string(b))
	}
}

// scenarioPostID is like scenarioPOST but reads back the generated id (for the
// dbms / connection scenario endpoints which echo `{"id":"..."}` on success).
func scenarioPostID(t *testing.T, fx *fixture, path string, body any) string {
	t.Helper()
	var rdr io.Reader
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal scenario body: %v", err)
	}
	rdr = strings.NewReader(string(buf))
	req, err := http.NewRequest(http.MethodPost, fx.URL+path, rdr)
	if err != nil {
		t.Fatalf("scenario request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("scenario do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("scenario %s: status %d body %s", path, resp.StatusCode, string(b))
	}
	var out struct{ ID string }
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("scenario decode: %v", err)
	}
	return out.ID
}

// fixtureTrace returns the accumulated `/fastify/api/*` request log so test
// failures can dump it for debugging. Best-effort: a fixture that's already
// dead returns an empty string.
func fixtureTrace(fx *fixture) string {
	resp, err := http.Get(fx.URL + scenarioLog)
	if err != nil {
		return fmt.Sprintf("(trace fetch failed: %v)", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// runCLI invokes the seam-built binary as a subprocess against the supplied
// fixture. Pipes the fixture's URL into the four seam env vars so the binary's
// ProbePort / origin / salt / dataDir all redirect into it. stdin is closed
// (non-TTY) — tests that need a TTY-only branch should assert the canonical
// non-TTY error instead.
func runCLI(t *testing.T, fx *fixture, args ...string) (int, string, string) {
	return runCLIWithTimeout(t, fx, 30*time.Second, args...)
}

// runCLIWithTimeout is like runCLI but caps the subprocess at the supplied
// duration. Bolt-class connect-refused failures in the `query` sad-paths can
// stretch past 30s as the driver retries; the suite caps them at a few
// seconds so total wall-time stays within the CI budget. On timeout the
// process is SIGKILLed and the returned exit code is whatever the resulting
// ExitError carries (usually a SIGKILL signal, surfaced as a non-zero code).
func runCLIWithTimeout(t *testing.T, fx *fixture, timeout time.Duration, args ...string) (int, string, string) {
	t.Helper()
	u, err := url.Parse(fx.URL)
	if err != nil {
		t.Fatalf("parse fixture url: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, args...)
	home := t.TempDir()
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"NEO4J_CLI_DESKTOP_E2E_PORT=" + u.Port(),
		"NEO4J_CLI_DESKTOP_E2E_HTTP_ORIGIN=" + fx.URL,
		"NEO4J_CLI_DESKTOP_E2E_SALT=" + fixtureSalt,
		"NEO4J_CLI_DESKTOP_E2E_DATA_DIR=" + filepath.Join(home, fixtureHomeKey),
		"NEO4J_CLI_NO_UPDATE_NAG=1",
		"DO_NOT_TRACK=1",
	}
	if runtime.GOOS == "linux" {
		env = append(env, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"))
		// Prevent dbus-launch --autolaunch hang on headless runners: an explicit
		// nonexistent socket makes godbus return ENOENT immediately instead of
		// blocking ~30 s trying to auto-launch a D-Bus session.
		env = append(env, "DBUS_SESSION_BUS_ADDRESS=unix:path=/nonexistent-dbus-socket")
	}
	if runtime.GOOS == "windows" {
		env = append(env, "LOCALAPPDATA="+home)
		if sr := os.Getenv("SystemRoot"); sr != "" {
			env = append(env, "SystemRoot="+sr)
		}
	}
	cmd.Env = env

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("run cli: %v", runErr)
		}
	}
	return code, outBuf.String(), errBuf.String()
}

// dumpOnFail attaches the fixture's request trace to the test log on failure
// so CI runs surface the wire interaction without needing to re-reproduce
// locally.
func dumpOnFail(t *testing.T, fx *fixture, stdout, stderr string) {
	t.Helper()
	if !t.Failed() {
		return
	}
	t.Logf("=== fixture trace ===\n%s", fixtureTrace(fx))
	t.Logf("=== fixture stderr ===\n%s", fx.Stderr.String())
	t.Logf("=== cli stdout ===\n%s", stdout)
	t.Logf("=== cli stderr ===\n%s", stderr)
}

// -----------------------------------------------------------------------------
// HAPPY PATH — desktop list / create / start / stop / delete
// -----------------------------------------------------------------------------

func TestDesktopList_Empty(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)

	code, stdout, stderr := runCLI(t, fx, "desktop", "list", "--format", "json")
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got struct {
		Dbmss       []map[string]any `json:"dbmss"`
		Connections []map[string]any `json:"connections"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if len(got.Dbmss) != 0 || len(got.Connections) != 0 {
		t.Fatalf("expected both empty; got %+v", got)
	}
}

func TestDesktopList_DbmssOnly(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "started-dbms", "status": "started",
		"connectionUri": "bolt://127.0.0.1:7687", "version": "5.20.0",
	})
	scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "stopped-dbms", "status": "stopped",
		"connectionUri": "bolt://127.0.0.1:7688", "version": "5.20.0",
	})

	code, stdout, stderr := runCLI(t, fx, "desktop", "list", "--format", "json")
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got struct {
		Dbmss       []map[string]any `json:"dbmss"`
		Connections []map[string]any `json:"connections"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Dbmss) != 2 {
		t.Fatalf("want 2 dbmss, got %d", len(got.Dbmss))
	}
	if len(got.Connections) != 0 {
		t.Fatalf("want 0 connections, got %d", len(got.Connections))
	}
	statuses := map[string]bool{}
	for _, d := range got.Dbmss {
		statuses[fmt.Sprint(d["status"])] = true
	}
	if !statuses["started"] || !statuses["stopped"] {
		t.Fatalf("expected both started + stopped; got %v", statuses)
	}
}

func TestDesktopList_ConnectionsOnly(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	scenarioPostID(t, fx, scenarioConn, map[string]any{
		"name": "aura-prod", "connectionUri": "neo4j+s://abc.databases.neo4j.io",
	})

	code, stdout, stderr := runCLI(t, fx, "desktop", "list", "--format", "json")
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got struct {
		Dbmss       []map[string]any `json:"dbmss"`
		Connections []map[string]any `json:"connections"`
	}
	_ = json.Unmarshal([]byte(stdout), &got)
	if len(got.Dbmss) != 0 || len(got.Connections) != 1 {
		t.Fatalf("want 0 dbmss + 1 connection; got %d / %d", len(got.Dbmss), len(got.Connections))
	}
}

func TestDesktopList_Both(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "d1", "status": "started", "connectionUri": "bolt://127.0.0.1:7687",
	})
	scenarioPostID(t, fx, scenarioConn, map[string]any{
		"name": "c1", "connectionUri": "neo4j+s://abc.databases.neo4j.io",
	})

	code, stdout, stderr := runCLI(t, fx, "desktop", "list", "--format", "json")
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got struct {
		Dbmss       []map[string]any `json:"dbmss"`
		Connections []map[string]any `json:"connections"`
	}
	_ = json.Unmarshal([]byte(stdout), &got)
	if len(got.Dbmss) != 1 || len(got.Connections) != 1 {
		t.Fatalf("want 1/1; got %d/%d", len(got.Dbmss), len(got.Connections))
	}
}

func TestDesktopCreate_PinnedVersion(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)

	// CLI normally polls until status=started; auto-progress flips
	// `starting`→`started` on the next GET so the wait loop converges
	// without an arm-race against the preflight enrichment.
	scenarioPOST(t, fx, scenarioAutoProg, map[string]any{"enabled": true})

	code, stdout, stderr := runCLI(t, fx,
		"desktop", "dbms", "create",
		"--name", "ham", "--version", "5.21.0", "--password", "hunter2", "--wait", "--rw",
		"--format", "json",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Status  string `json:"status"`
	}
	_ = json.Unmarshal([]byte(stdout), &got)
	if got.Name != "ham" || got.Version != "5.21.0" || got.Status != "started" {
		t.Fatalf("unexpected create payload: %+v", got)
	}
}

func TestDesktopCreate_VersionAutoPick(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	scenarioPOST(t, fx, scenarioVers, []map[string]any{
		{"edition": "enterprise", "version": "5.20.0", "origin": "cached", "dist": "x"},
		{"edition": "enterprise", "version": "5.26.1", "origin": "cached", "dist": "x"},
		{"edition": "enterprise", "version": "5.30.0-rc1", "origin": "online", "dist": "x"},
	})
	scenarioPOST(t, fx, scenarioAutoProg, map[string]any{"enabled": true})

	code, stdout, stderr := runCLI(t, fx,
		"desktop", "dbms", "create",
		"--name", "auto", "--password", "p", "--rw",
		"--format", "json",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal([]byte(stdout), &got)
	if got.Version != "5.26.1" {
		t.Fatalf("expected auto-picked 5.26.1 (highest stable enterprise); got %q\nstderr: %s", got.Version, stderr)
	}
}

func TestDesktopStart_WithWait(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "to-start", "status": "stopped",
	})
	// auto-progress flips `starting`→`started` on the next GET after the
	// start call, converging the production poll loop without arming a
	// per-id transition (which the preflight enrichment GET would
	// prematurely consume).
	scenarioPOST(t, fx, scenarioAutoProg, map[string]any{"enabled": true})

	code, stdout, stderr := runCLI(t, fx, "desktop", "dbms", "start", id, "--wait", "--rw", "--format", "json")
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestDesktopStop(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "to-stop", "status": "started",
	})
	scenarioPOST(t, fx, scenarioAutoProg, map[string]any{"enabled": true})

	code, stdout, stderr := runCLI(t, fx, "desktop", "dbms", "stop", id, "--wait", "--rw", "--format", "json")
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestDesktopDelete(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "to-go", "status": "stopped",
	})

	code, stdout, stderr := runCLI(t, fx, "desktop", "dbms", "delete", id, "--yes", "--force", "--rw", "--format", "json")
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

// -----------------------------------------------------------------------------
// HAPPY PATH — desktop connection CRUD
// -----------------------------------------------------------------------------

func TestDesktopConnection_Create(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)

	code, stdout, stderr := runCLI(t, fx,
		"desktop", "connection", "create",
		"--name", "aura-dev",
		"--uri", "neo4j+s://xyz.databases.neo4j.io",
		"--username", "neo4j",
		"--password", "supersecret",
		"--rw", "--format", "json",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got struct {
		Name          string `json:"name"`
		ConnectionURI string `json:"connection_uri"`
	}
	_ = json.Unmarshal([]byte(stdout), &got)
	if got.Name != "aura-dev" || got.ConnectionURI != "neo4j+s://xyz.databases.neo4j.io" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestDesktopConnection_Create_NoPasswordNonTTY(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)

	code, _, stderr := runCLI(t, fx,
		"desktop", "connection", "create",
		"--name", "n", "--uri", "neo4j://h", "--username", "u",
		"--rw", "--format", "json",
	)
	if code != 2 {
		t.Fatalf("expected usage exit 2; got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "--password") {
		t.Fatalf("expected stderr to mention --password; got %s", stderr)
	}
}

func TestDesktopConnection_UpdatePartialPatch(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioConn, map[string]any{
		"name": "before", "connectionUri": "neo4j+s://before",
	})

	code, stdout, stderr := runCLI(t, fx,
		"desktop", "connection", "update", id,
		"--description", "tagged-dev",
		"--rw", "--format", "json",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	// Round-trip through list and assert name + URI unchanged and description
	// applied. This is also implicit proof the PATCH body carried ONLY the
	// description key — the fixture's updateConnection only mutates keys
	// present in the body.
	_, listOut, _ := runCLI(t, fx, "desktop", "list", "--format", "json")
	var listGot struct {
		Connections []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ConnectionURI string `json:"connection_uri"`
			Description   string `json:"description"`
		} `json:"connections"`
	}
	_ = json.Unmarshal([]byte(listOut), &listGot)
	if len(listGot.Connections) != 1 {
		t.Fatalf("expected 1 connection; got %d", len(listGot.Connections))
	}
	c := listGot.Connections[0]
	if c.Name != "before" || c.ConnectionURI != "neo4j+s://before" {
		t.Fatalf("name/uri should be unchanged; got %+v", c)
	}
	if c.Description != "tagged-dev" {
		t.Fatalf("expected description set; got %+v", c)
	}
}

func TestDesktopConnection_UpdateFull(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioConn, map[string]any{
		"name": "before", "connectionUri": "neo4j+s://before",
	})

	code, stdout, stderr := runCLI(t, fx,
		"desktop", "connection", "update", id,
		"--name", "after",
		"--uri", "neo4j+s://after",
		"--username", "alice",
		"--password", "newpw",
		"--rw", "--format", "json",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestDesktopConnection_Delete(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioConn, map[string]any{
		"name": "doomed", "connectionUri": "neo4j+s://x",
	})

	code, stdout, stderr := runCLI(t, fx,
		"desktop", "connection", "delete", id, "--yes", "--force", "--rw", "--format", "json",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

// -----------------------------------------------------------------------------
// HAPPY PATH — query --credential desktop / desktop-connection:<uuid>
// -----------------------------------------------------------------------------

// TestQuery_CredentialDesktop_RunningDbms asserts the resolver picks up the
// single status=started DBMS and dispatches to the Bolt layer. We don't run a
// real Neo4j on CI, so the only observable success is the absence of the
// resolver's REQ-F-101 "no running DBMS" message — the resolver must have
// dispatched onwards for the Bolt-class connect timeout to surface (or for
// the test to time-cap the subprocess before bolt finishes retrying).
func TestQuery_CredentialDesktop_RunningDbms(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "running", "status": "started",
		"connectionUri": "bolt://127.0.0.1:1",
		"creds":         map[string]string{"username": "neo4j", "password": "p"},
	})

	// Bolt connect-refused can stretch many seconds as the driver retries;
	// cap the subprocess at a few seconds. SIGKILL via context is fine — we
	// only need to assert the resolver dispatched before the time cap (any
	// stderr captured before the kill that mentions "no running DBMS" would
	// indicate a regression).
	code, stdout, stderr := runCLIWithTimeout(t, fx, 4*time.Second,
		"query", "-c", "desktop", "RETURN 1",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	if strings.Contains(stderr, "no running DBMS") || strings.Contains(stderr, "No running DBMS") {
		t.Fatalf("resolver still flagged no-running-DBMS despite a started DBMS being present\nstderr: %s", stderr)
	}
	// `code` is intentionally unchecked beyond a smoke "compiles" — the
	// subprocess may be SIGKILLed or may have exited on its own with a Bolt
	// upstream error (exit 8); either way the resolver's job is done.
	_ = code
}

func TestQuery_CredentialDesktopConnection_KnownUUID(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioConn, map[string]any{
		"name": "remote", "connectionUri": "bolt://127.0.0.1:1",
		"creds": map[string]string{"username": "neo4j", "password": "p"},
	})

	code, stdout, stderr := runCLIWithTimeout(t, fx, 4*time.Second,
		"query", "-c", "desktop-connection:"+id, "RETURN 1",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	// Must NOT have surfaced a "no connection with id" or "is not a UUID"
	// error — the resolver succeeded.
	if strings.Contains(stderr, "no connection with id") || strings.Contains(stderr, "requires a UUID") {
		t.Fatalf("resolver flagged the known UUID as missing/invalid:\n%s", stderr)
	}
	_ = code
}

// -----------------------------------------------------------------------------
// SAD PATH — DBMS status matrix for `query -c desktop`
// -----------------------------------------------------------------------------

func TestQuery_DesktopActive_StatusMatrix(t *testing.T) {
	// One fixture covers many subtests via /_scenario/reset.
	fx := startFixture(t)

	type tc struct {
		name        string
		seed        []map[string]any // each map is a /_scenario/dbms body
		wantExit    int
		wantStderr  string // substring; empty means no specific assertion
		wantNoMatch string // substring stderr MUST NOT contain
	}

	cases := []tc{
		{
			name:        "two_started_yields_fatal",
			seed:        []map[string]any{{"name": "a", "status": "started"}, {"name": "b", "status": "started"}},
			wantExit:    1,
			wantStderr:  "Multiple running",
			wantNoMatch: "no running DBMS",
		},
		{
			name:       "no_dbmss_at_all",
			seed:       nil,
			wantExit:   1,
			wantStderr: "No running DBMS",
		},
		{
			name:       "only_stopped",
			seed:       []map[string]any{{"name": "a", "status": "stopped"}},
			wantExit:   1,
			wantStderr: "No running DBMS",
		},
		{
			name:       "only_starting",
			seed:       []map[string]any{{"name": "a", "status": "starting"}},
			wantExit:   1,
			wantStderr: "No running DBMS",
		},
		{
			name:       "only_stopping",
			seed:       []map[string]any{{"name": "a", "status": "stopping"}},
			wantExit:   1,
			wantStderr: "No running DBMS",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			scenarioPOST(t, fx, scenarioReset, nil)
			for _, d := range c.seed {
				scenarioPostID(t, fx, scenarioDbms, d)
			}
			code, stdout, stderr := runCLI(t, fx, "query", "-c", "desktop", "RETURN 1")
			defer dumpOnFail(t, fx, stdout, stderr)
			if code != c.wantExit {
				t.Fatalf("exit %d (want %d)\nstderr: %s", code, c.wantExit, stderr)
			}
			if c.wantStderr != "" && !strings.Contains(stderr, c.wantStderr) {
				t.Fatalf("expected stderr to contain %q; got %s", c.wantStderr, stderr)
			}
			if c.wantNoMatch != "" && strings.Contains(stderr, c.wantNoMatch) {
				t.Fatalf("stderr should not contain %q; got %s", c.wantNoMatch, stderr)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// SAD PATH — connection prefix
// -----------------------------------------------------------------------------

func TestQuery_DesktopConnection_MalformedUUID(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)

	code, _, stderr := runCLI(t, fx, "query", "-c", "desktop-connection:not-a-uuid", "RETURN 1")
	if code != 2 {
		t.Fatalf("expected usage exit 2; got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "desktop list") {
		t.Fatalf("expected usage error to point at `desktop list`; got %s", stderr)
	}
}

func TestQuery_DesktopConnection_UnknownUUID(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)

	const validButUnknown = "11111111-2222-3333-4444-555555555555"
	code, _, stderr := runCLI(t, fx, "query", "-c", "desktop-connection:"+validButUnknown, "RETURN 1")
	if code != 1 {
		t.Fatalf("expected fatal exit 1; got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "no connection") {
		t.Fatalf("expected 'no connection' in stderr; got %s", stderr)
	}
}

func TestQuery_DesktopConnection_NullCredsNonTTY(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioConn, map[string]any{
		"name": "nullcreds", "connectionUri": "bolt://127.0.0.1:1",
		"creds": nil, // explicit nil: relate returns 200 null
	})
	// Belt-and-braces: post a null credential record for `connection:<id>`.
	scenarioPOST(t, fx, scenarioCred, map[string]any{
		"key": "connection:" + id, "value": nil,
	})

	code, _, stderr := runCLI(t, fx, "query", "-c", "desktop-connection:"+id, "RETURN 1")
	if code == 0 {
		t.Fatalf("expected non-zero exit; got 0\nstderr: %s", stderr)
	}
	if !strings.Contains(strings.ToLower(stderr), "password") && !strings.Contains(strings.ToLower(stderr), "credential") {
		t.Fatalf("expected stderr to surface a password / credential issue; got %s", stderr)
	}
}

// -----------------------------------------------------------------------------
// SAD PATH — transport
// -----------------------------------------------------------------------------

func TestDesktop_Unreachable(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	// Hard-kill the fixture so the seam's port redirect points to a dead
	// socket. The CLI's ProbePort short-circuits via NEO4J_CLI_DESKTOP_E2E_PORT
	// (it skips /api-docs validation), so the first real call surfaces the
	// REQ-F-008 unreachable hint.
	_ = fx.Cmd.Process.Kill()
	_, _ = fx.Cmd.Process.Wait()
	// Give the OS a beat to release the port (test stability on Linux).
	time.Sleep(50 * time.Millisecond)

	code, _, stderr := runCLI(t, fx, "desktop", "list", "--format", "json")
	if code != 1 {
		t.Fatalf("expected exit 1; got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "doesn't appear to be running") {
		t.Fatalf("expected REQ-F-008 hint; got %s", stderr)
	}
}

func TestDesktop_AuthReject(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	scenarioPOST(t, fx, scenarioAuth, map[string]any{"mode": "reject"})

	code, _, stderr := runCLI(t, fx, "desktop", "list", "--format", "json")
	if code != 4 {
		t.Fatalf("expected auth exit 4; got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(strings.ToLower(stderr), "auth") {
		t.Fatalf("expected stderr to surface auth failure; got %s", stderr)
	}
}

func TestDesktop_FiveHundred(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	scenarioPOST(t, fx, scenarioAuth, map[string]any{"mode": "500"})

	code, _, stderr := runCLI(t, fx, "desktop", "list", "--format", "json")
	if code != 1 {
		t.Fatalf("expected fatal exit 1; got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "500") {
		t.Fatalf("expected stderr to surface the upstream 5xx code; got %s", stderr)
	}
}

func TestDesktop_MidStreamClose(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	scenarioPOST(t, fx, scenarioAuth, map[string]any{"mode": "close"})

	code, _, stderr := runCLI(t, fx, "desktop", "list", "--format", "json")
	if code != 1 {
		t.Fatalf("expected fatal exit 1; got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "doesn't appear to be running") {
		t.Fatalf("expected REQ-F-008 hint on mid-stream EOF; got %s", stderr)
	}
}

// -----------------------------------------------------------------------------
// SAD PATH — resource errors
// -----------------------------------------------------------------------------

func TestDesktop_DeleteUnknown(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	const unknown = "11111111-2222-3333-4444-555555555555"

	code, _, stderr := runCLI(t, fx, "desktop", "dbms", "delete", unknown, "--yes", "--force", "--rw", "--format", "json")
	if code == 0 {
		t.Fatalf("expected non-zero exit; got 0\nstderr: %s", stderr)
	}
	if !strings.Contains(stderr, unknown) && !strings.Contains(strings.ToLower(stderr), "not found") {
		t.Fatalf("expected stderr to surface the unknown id; got %s", stderr)
	}
}

func TestDesktop_StartUnknown(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	const unknown = "11111111-2222-3333-4444-555555555555"

	code, _, stderr := runCLI(t, fx, "desktop", "dbms", "start", unknown, "--rw", "--format", "json")
	if code == 0 {
		t.Fatalf("expected non-zero exit; got 0\nstderr: %s", stderr)
	}
}

func TestDesktop_StopUnknown(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	const unknown = "11111111-2222-3333-4444-555555555555"

	code, _, stderr := runCLI(t, fx, "desktop", "dbms", "stop", unknown, "--rw", "--format", "json")
	if code == 0 {
		t.Fatalf("expected non-zero exit; got 0\nstderr: %s", stderr)
	}
}

func TestDesktopConnection_CreateDuplicate(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	scenarioPostID(t, fx, scenarioConn, map[string]any{
		"name": "dup", "connectionUri": "neo4j+s://x",
	})

	code, _, stderr := runCLI(t, fx,
		"desktop", "connection", "create",
		"--name", "dup", "--uri", "neo4j+s://other",
		"--username", "u", "--password", "p", "--rw",
	)
	if code == 0 {
		t.Fatalf("expected non-zero exit on duplicate name; got 0\nstderr: %s", stderr)
	}
}

func TestDesktopConnection_UpdateUnknown(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	const unknown = "11111111-2222-3333-4444-555555555555"

	code, _, stderr := runCLI(t, fx,
		"desktop", "connection", "update", unknown,
		"--description", "x", "--rw",
	)
	if code == 0 {
		t.Fatalf("expected non-zero exit; got 0\nstderr: %s", stderr)
	}
}

func TestDesktopConnection_DeleteUnknown(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	const unknown = "11111111-2222-3333-4444-555555555555"

	code, _, stderr := runCLI(t, fx,
		"desktop", "connection", "delete", unknown, "--yes", "--force", "--rw",
	)
	if code == 0 {
		t.Fatalf("expected non-zero exit; got 0\nstderr: %s", stderr)
	}
}

func TestDesktopCreate_Duplicate(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "dup-dbms", "status": "stopped",
	})

	code, _, stderr := runCLI(t, fx,
		"desktop", "dbms", "create",
		"--name", "dup-dbms", "--version", "5.21.0", "--password", "p", "--rw",
		"--format", "json",
	)
	if code == 0 {
		t.Fatalf("expected non-zero exit on duplicate name; got 0\nstderr: %s", stderr)
	}
}

// -----------------------------------------------------------------------------
// PLUGINS — list / available / install / uninstall (REQ-F-044)
// -----------------------------------------------------------------------------
//
// Every scenario in this block exercises one of the four `desktop dbms plugin`
// leaves end-to-end against the fixture's `/dbmss/<id>/plugins/*` routes.
// Setup uses the `_scenario/plugin` injector (task-028) to pre-seed
// `availablePlugins` + `installedPlugins` per DBMS so the leaves see realistic
// catalogs without going through install/uninstall ahead of time. Auto-restart
// scenarios rely on the fixture's auto-progress toggle so the Stop → poll →
// Start → poll loop converges without arming per-id transitions.

// seedPlugins fans out a `_scenario/plugin` POST to seed available + installed
// plugin lists for one DBMS. Empty slices clear; nil leaves the side untouched.
// Mirrors the fixture-side helper used by main_test.go but emits the e2e admin
// shape directly so each test reads top-to-bottom.
func seedPlugins(t *testing.T, fx *fixture, dbmsID string, available, installed []map[string]any) {
	t.Helper()
	body := map[string]any{"dbms_id": dbmsID}
	if available != nil {
		body["available"] = available
	}
	if installed != nil {
		body["installed"] = installed
	}
	scenarioPOST(t, fx, scenarioPlug, body)
}

// containsAllLines is a small helper that asserts the supplied substrings are
// all present in `s` (any order). Used by auto-restart scenarios to check both
// breadcrumb lines without pinning their relative order.
func containsAllLines(t *testing.T, s string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(s, w) {
			t.Errorf("expected stderr to contain %q; got:\n%s", w, s)
		}
	}
}

// --- plugin list ---

func TestPluginList_HappyPath(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "with-plugins", "status": "started",
	})
	seedPlugins(t, fx, id, nil, []map[string]any{
		{"name": "apoc", "version": "5.21.0", "filePath": "/plugins/apoc-5.21.0.jar"},
		{"name": "gds", "version": "2.6.0", "filePath": "/plugins/gds-2.6.0.jar"},
	})

	code, stdout, stderr := runCLI(t, fx, "desktop", "dbms", "plugin", "list", id, "--format", "json")
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 plugins, got %d: %s", len(got), stdout)
	}
	names := map[string]bool{}
	for _, p := range got {
		names[fmt.Sprint(p["name"])] = true
	}
	if !names["apoc"] || !names["gds"] {
		t.Fatalf("expected apoc + gds; got %v", names)
	}
}

func TestPluginList_Empty(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "no-plugins", "status": "stopped",
	})

	// JSON-shape assertion: relate returns `[]`, leaf surfaces `[]` (NEVER
	// `null`) so scripts can treat the value as an array unconditionally.
	code, stdout, stderr := runCLI(t, fx, "desktop", "dbms", "plugin", "list", id, "--format", "json")
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "[]" {
		t.Fatalf("expected empty JSON array; got %q", stdout)
	}

	// Default format under non-TTY is JSON (see common/output.ResolveOutput);
	// pass `--format table` explicitly to assert the empty-table render with
	// the column header + `(none)` placeholder. CI runs the e2e suite under
	// non-TTY, so we must opt INTO table mode to exercise the placeholder.
	code2, tableOut, tableErr := runCLI(t, fx, "desktop", "dbms", "plugin", "list", id, "--format", "table")
	if code2 != 0 {
		t.Fatalf("table exit %d: %s", code2, tableErr)
	}
	// go-pretty uppercases column headers (`NAME`, `VERSION`, …), so match
	// against the uppercased form. The `(none)` row stays lowercase because
	// the renderer treats it as a cell value, not a column name.
	if !strings.Contains(tableOut, "NAME") || !strings.Contains(tableOut, "(none)") {
		t.Fatalf("expected table header + (none) placeholder; got:\n%s", tableOut)
	}
}

func TestPluginList_DbmsNotFound(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)

	code, _, stderr := runCLI(t, fx, "desktop", "dbms", "plugin", "list", "ghost", "--format", "json")
	if code == 0 {
		t.Fatalf("expected non-zero exit; got 0\nstderr: %s", stderr)
	}
	// REQ-F-042 canonical hint points at `neo4j-cli desktop dbms list`.
	if !strings.Contains(stderr, "DBMS") || !strings.Contains(stderr, "desktop dbms list") {
		t.Fatalf("expected REQ-F-042 canonical text; got %s", stderr)
	}
}

// --- plugin available ---

func TestPluginAvailable_HappyPath(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "catalog", "status": "stopped",
	})
	seedPlugins(t, fx, id, []map[string]any{
		{"name": "apoc", "version": "5.21.0", "filePath": "/cat/apoc.jar"},
		{"name": "gds", "version": "2.6.0", "filePath": "/cat/gds.jar"},
		{"name": "neo-semantics", "version": "5.20.0", "filePath": "/cat/n10s.jar"},
	}, nil)

	code, stdout, stderr := runCLI(t, fx, "desktop", "dbms", "plugin", "available", id, "--format", "json")
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 plugins, got %d: %s", len(got), stdout)
	}
}

func TestPluginAvailable_Empty(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "no-catalog", "status": "stopped",
	})

	code, stdout, stderr := runCLI(t, fx, "desktop", "dbms", "plugin", "available", id, "--format", "json")
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "[]" {
		t.Fatalf("expected empty JSON array; got %q", stdout)
	}
}

// --- plugin install ---

// TestPluginInstall_HappyAutoRestart asserts the full install+restart
// sequence on a running DBMS. With auto-progress on, the fixture flips
// `starting`→`started` and `stopping`→`stopped` on every GET so the
// production poll loop converges. Two stderr breadcrumbs (`restarting…` +
// `restarted; plugin "apoc" is now active`) are pinned by substring. A
// follow-up `plugin list` call confirms `pendingRestart` was flipped to
// `false` by the fixture's `clearPendingRestart` (REQ-F-043 simulation).
func TestPluginInstall_HappyAutoRestart(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	scenarioPOST(t, fx, scenarioAutoProg, map[string]any{"enabled": true})
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "running-host", "status": "started",
	})
	seedPlugins(t, fx, id, []map[string]any{
		{"name": "apoc", "version": "5.21.0", "filePath": "/cat/apoc.jar"},
	}, nil)

	code, stdout, stderr := runCLI(t, fx,
		"desktop", "dbms", "plugin", "install", id, "--plugin", "apoc", "--rw", "--format", "json",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s\ntrace: %s", code, stderr, fixtureTrace(fx))
	}
	// stdout is the DbmsPlugin payload.
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if got["name"] != "apoc" {
		t.Fatalf("expected installed name=apoc; got %v", got)
	}
	// Both breadcrumbs visible.
	containsAllLines(t, stderr,
		"Plugin change pending",
		"restarting DBMS",
		"DBMS restarted",
		`plugin "apoc" is now active`,
	)
	// Fixture request trace shows the install + Stop + Start lifecycle. The
	// pre-op GetDbms also lands; we only pin the install/stop/start trio
	// because the relative ordering of the pre-op GET vs install is an
	// implementation detail.
	trace := fixtureTrace(fx)
	for _, w := range []string{
		"POST /fastify/api/dbmss/" + id + "/plugins/install",
		"POST /fastify/api/desktop/dbmss/" + id + "/stop",
		"POST /fastify/api/dbmss/" + id + "/start",
	} {
		if !strings.Contains(trace, w) {
			t.Fatalf("expected trace to include %q; got:\n%s", w, trace)
		}
	}
	// Follow-up: `pendingRestart` flipped to false after the restart cycle.
	code2, listOut, listErr := runCLI(t, fx, "desktop", "dbms", "plugin", "list", id, "--format", "json")
	if code2 != 0 {
		t.Fatalf("follow-up list exit %d: %s", code2, listErr)
	}
	var listGot []map[string]any
	_ = json.Unmarshal([]byte(listOut), &listGot)
	if len(listGot) != 1 {
		t.Fatalf("expected 1 installed plugin; got %d (%s)", len(listGot), listOut)
	}
	if pr, _ := listGot[0]["pending_restart"].(bool); pr {
		t.Fatalf("expected pendingRestart=false after auto-restart; got %v", listGot[0])
	}
}

// TestPluginInstall_StoppedDbms covers the next-start hint path. The DBMS is
// stopped, so no Stop/Start calls fire and the leaf emits the "will be active
// on next start" breadcrumb. Fixture trace assertion proves no lifecycle
// calls were made.
func TestPluginInstall_StoppedDbms(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "off-host", "status": "stopped",
	})
	seedPlugins(t, fx, id, []map[string]any{
		{"name": "apoc", "version": "5.21.0", "filePath": "/cat/apoc.jar"},
	}, nil)

	code, stdout, stderr := runCLI(t, fx,
		"desktop", "dbms", "plugin", "install", id, "--plugin", "apoc", "--rw", "--format", "json",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "will activate on next start") {
		t.Fatalf("expected next-start breadcrumb; got %s", stderr)
	}
	if strings.Contains(stderr, "Plugin change pending") {
		t.Fatalf("did not expect auto-restart breadcrumb on stopped DBMS; got %s", stderr)
	}
	trace := fixtureTrace(fx)
	for _, forbidden := range []string{
		"POST /fastify/api/desktop/dbmss/" + id + "/stop",
		"POST /fastify/api/dbmss/" + id + "/start",
	} {
		if strings.Contains(trace, forbidden) {
			t.Fatalf("did not expect %q in trace; got:\n%s", forbidden, trace)
		}
	}
}

// TestPluginInstall_NoRestartOnRunningDbms verifies `--no-restart` opts out of
// the auto-restart on a running DBMS — the manual-restart hint surfaces and no
// Stop/Start calls fire.
func TestPluginInstall_NoRestartOnRunningDbms(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "noresta-host", "status": "started",
	})
	seedPlugins(t, fx, id, []map[string]any{
		{"name": "apoc", "version": "5.21.0", "filePath": "/cat/apoc.jar"},
	}, nil)

	code, stdout, stderr := runCLI(t, fx,
		"desktop", "dbms", "plugin", "install", id, "--plugin", "apoc", "--no-restart", "--rw", "--format", "json",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "--no-restart") {
		t.Fatalf("expected manual-restart hint mentioning --no-restart; got %s", stderr)
	}
	if strings.Contains(stderr, "Plugin change pending") {
		t.Fatalf("did not expect auto-restart breadcrumb under --no-restart; got %s", stderr)
	}
	trace := fixtureTrace(fx)
	for _, forbidden := range []string{
		"POST /fastify/api/desktop/dbmss/" + id + "/stop",
		"POST /fastify/api/dbmss/" + id + "/start",
	} {
		if strings.Contains(trace, forbidden) {
			t.Fatalf("did not expect %q in trace; got:\n%s", forbidden, trace)
		}
	}
}

// TestPluginInstall_PluginNotFound covers REQ-F-041: the relate 404 with a
// `Could not find plugin` body maps to ErrPluginNotFound which surfaces the
// catalog hint.
func TestPluginInstall_PluginNotFound(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "real-dbms", "status": "stopped",
	})
	// No availablePlugins seeded — install of `not-a-plugin` 404s with the
	// `Could not find plugin` body.

	code, _, stderr := runCLI(t, fx,
		"desktop", "dbms", "plugin", "install", id, "--plugin", "not-a-plugin", "--rw", "--format", "json",
	)
	if code == 0 {
		t.Fatalf("expected non-zero exit; got 0\nstderr: %s", stderr)
	}
	if !strings.Contains(stderr, "not-a-plugin") || !strings.Contains(stderr, "plugin available") {
		t.Fatalf("expected REQ-F-041 canonical text mentioning the catalog hint; got %s", stderr)
	}
}

// TestPluginInstall_DbmsNotFound covers REQ-F-042 via the install endpoint.
func TestPluginInstall_DbmsNotFound(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)

	code, _, stderr := runCLI(t, fx,
		"desktop", "dbms", "plugin", "install", "ghost-id", "--plugin", "apoc", "--rw", "--format", "json",
	)
	if code == 0 {
		t.Fatalf("expected non-zero exit; got 0\nstderr: %s", stderr)
	}
	if !strings.Contains(stderr, "ghost-id") || !strings.Contains(stderr, "desktop dbms list") {
		t.Fatalf("expected REQ-F-042 canonical text mentioning `desktop dbms list`; got %s", stderr)
	}
}

// TestPluginInstall_DesktopUnreachable covers REQ-F-008 against the install
// leaf — kill the fixture before invoking, the canonical unreachable hint
// surfaces.
func TestPluginInstall_DesktopUnreachable(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	// Match the existing transport-sad-path pattern: kill the fixture, give
	// the OS a beat to release the port, then expect the canonical hint.
	_ = fx.Cmd.Process.Kill()
	_, _ = fx.Cmd.Process.Wait()
	time.Sleep(50 * time.Millisecond)

	code, _, stderr := runCLI(t, fx,
		"desktop", "dbms", "plugin", "install", "any-id", "--plugin", "apoc", "--rw", "--format", "json",
	)
	if code != 1 {
		t.Fatalf("expected exit 1; got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "doesn't appear to be running") {
		t.Fatalf("expected REQ-F-008 hint; got %s", stderr)
	}
}

// --- plugin uninstall ---

// TestPluginUninstall_HappyAutoRestart mirrors install's happy path: a running
// DBMS triggers the Stop → Start restart cycle, both breadcrumbs surface, and
// a follow-up `plugin list` confirms the plugin is gone.
func TestPluginUninstall_HappyAutoRestart(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	scenarioPOST(t, fx, scenarioAutoProg, map[string]any{"enabled": true})
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "running-host", "status": "started",
	})
	seedPlugins(t, fx, id, nil, []map[string]any{
		{"name": "apoc", "version": "5.21.0", "filePath": "/cat/apoc.jar"},
	})

	code, stdout, stderr := runCLI(t, fx,
		"desktop", "dbms", "plugin", "uninstall", id, "--plugin", "apoc", "--rw", "--format", "json",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s\ntrace: %s", code, stderr, fixtureTrace(fx))
	}
	// JSON shape: {name, uninstalled: true} — no `id`, no `status`.
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if got["name"] != "apoc" {
		t.Fatalf("expected name=apoc; got %v", got)
	}
	if ok, _ := got["uninstalled"].(bool); !ok {
		t.Fatalf("expected uninstalled=true; got %v", got)
	}
	containsAllLines(t, stderr,
		"Plugin change pending",
		"restarting DBMS",
		"DBMS restarted",
		`plugin "apoc" is now removed`,
	)
	trace := fixtureTrace(fx)
	for _, w := range []string{
		"POST /fastify/api/dbmss/" + id + "/plugins/uninstall",
		"POST /fastify/api/desktop/dbmss/" + id + "/stop",
		"POST /fastify/api/dbmss/" + id + "/start",
	} {
		if !strings.Contains(trace, w) {
			t.Fatalf("expected trace to include %q; got:\n%s", w, trace)
		}
	}
	// Follow-up: plugin is gone.
	code2, listOut, listErr := runCLI(t, fx, "desktop", "dbms", "plugin", "list", id, "--format", "json")
	if code2 != 0 {
		t.Fatalf("follow-up list exit %d: %s", code2, listErr)
	}
	if strings.TrimSpace(listOut) != "[]" {
		t.Fatalf("expected installed list empty after uninstall; got %q", listOut)
	}
}

// TestPluginUninstall_Idempotent covers REQ-F-038: uninstalling a not-installed
// plugin still exits 0 with the same `{name, uninstalled: true}` shape.
func TestPluginUninstall_Idempotent(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "off-host", "status": "stopped",
	})
	// No installed plugins; the relate uninstall route still returns
	// `{name}` and the leaf surfaces it as a normal success.

	code, stdout, stderr := runCLI(t, fx,
		"desktop", "dbms", "plugin", "uninstall", id, "--plugin", "apoc", "--rw", "--format", "json",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if got["name"] != "apoc" {
		t.Fatalf("expected name=apoc on idempotent uninstall; got %v", got)
	}
	if ok, _ := got["uninstalled"].(bool); !ok {
		t.Fatalf("expected uninstalled=true; got %v", got)
	}
}

// TestPluginUninstall_NoRestartOnRunningDbms mirrors the install --no-restart
// path. The manual-restart hint surfaces and no Stop/Start calls fire.
func TestPluginUninstall_NoRestartOnRunningDbms(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)
	id := scenarioPostID(t, fx, scenarioDbms, map[string]any{
		"name": "running-host", "status": "started",
	})
	seedPlugins(t, fx, id, nil, []map[string]any{
		{"name": "apoc", "version": "5.21.0", "filePath": "/cat/apoc.jar"},
	})

	code, stdout, stderr := runCLI(t, fx,
		"desktop", "dbms", "plugin", "uninstall", id, "--plugin", "apoc", "--no-restart", "--rw", "--format", "json",
	)
	defer dumpOnFail(t, fx, stdout, stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "--no-restart") {
		t.Fatalf("expected manual-restart hint mentioning --no-restart; got %s", stderr)
	}
	trace := fixtureTrace(fx)
	for _, forbidden := range []string{
		"POST /fastify/api/desktop/dbmss/" + id + "/stop",
		"POST /fastify/api/dbmss/" + id + "/start",
	} {
		if strings.Contains(trace, forbidden) {
			t.Fatalf("did not expect %q in trace; got:\n%s", forbidden, trace)
		}
	}
}

// TestPluginUninstall_DbmsNotFound covers REQ-F-042 via the uninstall endpoint.
func TestPluginUninstall_DbmsNotFound(t *testing.T) {
	t.Parallel()
	fx := startFixture(t)
	scenarioPOST(t, fx, scenarioReset, nil)

	code, _, stderr := runCLI(t, fx,
		"desktop", "dbms", "plugin", "uninstall", "ghost-id", "--plugin", "apoc", "--rw", "--format", "json",
	)
	if code == 0 {
		t.Fatalf("expected non-zero exit; got 0\nstderr: %s", stderr)
	}
	if !strings.Contains(stderr, "ghost-id") || !strings.Contains(stderr, "desktop dbms list") {
		t.Fatalf("expected REQ-F-042 canonical text; got %s", stderr)
	}
}
