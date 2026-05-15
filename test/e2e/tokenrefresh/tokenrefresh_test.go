// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build e2e_tokenrefresh

// Package tokenrefresh is a smoke test for the parallel OAuth token-refresh
// race fix (CLI-xxx). It builds the real neo4j-cli binary once, seeds a
// credentials.json with an *expired* access token, then fires N subprocess
// invocations in parallel, each pointing at an in-process mock HTTP server.
//
// Before the fix, two or more processes racing to rename credentials.json.tmp
// over credentials.json would crash with:
//
//	rename .../credentials.json.tmp .../credentials.json: no such file or directory
//
// After the fix (gofslock sidecar lock + atomic rename inside the lock) all N
// processes must exit 0 and none may print "no such file or directory".
//
// Gated by build tag `e2e_tokenrefresh` so it stays out of the default
// `go test ./...` matrix.
package tokenrefresh_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// binPath holds the absolute path to the freshly built neo4j-cli binary.
// Populated once by TestMain; all subtests share it.
var binPath string

// repoRoot walks up from this file to the repo root (directory that contains
// go.mod). Mirrors the helper in test/e2e/exitcodes and test/e2e/writegate.
func repoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("could not determine caller file path")
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
	return "", errors.New("could not locate repo root (go.mod) walking up from test file")
}

// TestMain builds the neo4j-cli binary once and runs all tests.
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

	dir, err := os.MkdirTemp("", "neo4j-cli-tokenrefresh-bin-*")
	if err != nil {
		return 0, fmt.Errorf("mkdir bin dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	name := "neo4j-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(dir, name)

	cmd := exec.Command("go", "build", "-o", out, "./neo4j-cli")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if combined, buildErr := cmd.CombinedOutput(); buildErr != nil {
		return 0, fmt.Errorf("go build failed: %w\n%s", buildErr, combined)
	}
	binPath = out

	return m.Run(), nil
}

// configHomeFor returns env-var assignments that redirect the CLI's config
// prefix to `dir`. Mirrors the helpers in test/e2e/exitcodes and writegate.
//
// IMPORTANT: on darwin the binary resolves ConfigPrefix via os/user.Current()
// (passwd database) rather than $HOME, so $HOME has no effect. Tests that
// require write-isolation must skip on darwin; see canRedirectConfigDir.
func configHomeFor(dir string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"HOME=" + dir}
	case "linux":
		return []string{
			"HOME=" + dir,
			"XDG_CONFIG_HOME=" + filepath.Join(dir, ".config"),
		}
	case "windows":
		return []string{"LOCALAPPDATA=" + dir}
	default:
		return nil
	}
}

// configDirFor returns the directory inside the redirected home where
// neo4j-cli will write credentials.json.
func configDirFor(dir string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(dir, "Library", "Preferences", "neo4j", "cli")
	case "linux":
		return filepath.Join(dir, ".config", "neo4j", "cli")
	case "windows":
		return filepath.Join(dir, "neo4j", "cli")
	default:
		return ""
	}
}

// canRedirectConfigDir reports whether the platform honours the env-var
// redirect produced by configHomeFor. Returns false on darwin where
// os/user.Current() reads from the passwd database, ignoring $HOME.
func canRedirectConfigDir() bool {
	return runtime.GOOS != "darwin"
}

// seedExpiredCreds writes credentials.json with a token whose expiry is in the
// past, forcing every subprocess into the token-refresh code path.
func seedExpiredCreds(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir credentials dir: %v", err)
	}
	// Put the expiry 5 seconds in the past — well before time.Now().UnixMilli()
	// so HasValidAccessToken returns false regardless of clock skew.
	expiry := time.Now().Add(-5 * time.Second).UnixMilli()
	body := fmt.Sprintf(`{
  "aura": {
    "default-credential": "e2e",
    "credentials": [
      {
        "name": "e2e",
        "client-id": "cid",
        "client-secret": "csec",
        "access-token": "expired-token",
        "token-expiry": %d
      }
    ]
  }
}`, expiry)
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write credentials.json: %v", err)
	}
}

// childEnv builds the minimal env for a subprocess: PATH, optional
// SystemRoot (Windows DLL loading), telemetry suppression, the
// config-home redirect, and the mock-server URLs.
func childEnv(home, baseURL string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"DO_NOT_TRACK=1",
		"NEO4J_CLI_NO_UPDATE_NAG=1",
		"AURA_BASE_URL=" + baseURL,
		"AURA_AUTH_URL=" + baseURL + "/oauth/token",
		// Suppress agent detection so the write gate never fires.
		"CLAUDECODE=",
	}
	if sr := os.Getenv("SystemRoot"); sr != "" {
		env = append(env, "SystemRoot="+sr)
	}
	env = append(env, configHomeFor(home)...)
	return env
}

// mockServer starts an httptest.Server that serves:
//   - POST /oauth/token → fresh access token (expires_in: 3600)
//   - GET  /v1/instances → empty data list (200 OK)
//
// A counter tracks how many token-refresh requests are served so the test can
// assert that at least one refresh occurred (i.e. the expired-token seed
// actually triggered the code path under test).
func mockServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var refreshCount atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		refreshCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh-token","expires_in":3600,"token_type":"bearer"}`))
	})
	mux.HandleFunc("/v1/instances", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &refreshCount
}

// runCLI launches the binary and returns exit code, stdout, stderr.
func runCLI(bin string, args, env []string) (int, string, string) {
	cmd := exec.Command(bin, args...)
	cmd.Env = env

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return exitCode, outBuf.String(), errBuf.String()
}

// TestParallelTokenRefresh_NoRace fires N parallel neo4j-cli subprocesses,
// each seeing an expired token, and asserts that none crash with the
// "no such file or directory" rename panic that the race produced before the
// fix.
func TestParallelTokenRefresh_NoRace(t *testing.T) {
	if !canRedirectConfigDir() {
		t.Skipf("skipping on %s: clicfg.ConfigPrefix is resolved via os/user.Current() and cannot be redirected via env vars; running the test would race against the user's real credentials.json", runtime.GOOS)
	}

	const parallelism = 8

	home := t.TempDir()
	credsDir := configDirFor(home)
	seedExpiredCreds(t, credsDir)

	srv, refreshCount := mockServer(t)

	args := []string{"aura", "instance", "list", "--format", "json"}
	env := childEnv(home, srv.URL)

	type result struct {
		exitCode int
		stdout   string
		stderr   string
	}
	results := make([]result, parallelism)

	var wg sync.WaitGroup
	wg.Add(parallelism)
	for i := 0; i < parallelism; i++ {
		i := i
		go func() {
			defer wg.Done()
			code, out, errOut := runCLI(binPath, args, env)
			results[i] = result{code, out, errOut}
		}()
	}
	wg.Wait()

	for i, r := range results {
		if strings.Contains(r.stderr, "no such file or directory") {
			t.Errorf("process %d: stderr contains race-condition panic:\n%s", i, r.stderr)
		}
		if r.exitCode != 0 {
			t.Errorf("process %d: exit code = %d (want 0)\nstdout:\n%s\nstderr:\n%s",
				i, r.exitCode, r.stdout, r.stderr)
		}
	}

	// At least one subprocess must have hit /oauth/token — confirms the
	// expired-token seed actually triggered the code path under test.
	if refreshCount.Load() == 0 {
		t.Error("no OAuth token refresh requests received; the expired-token seed may not have triggered the refresh path")
	}

	// Final credentials.json must contain a valid (non-expired) access token —
	// proves at least one subprocess successfully wrote the refreshed token back.
	credsPath := filepath.Join(credsDir, "credentials.json")
	raw, err := os.ReadFile(credsPath)
	if err != nil {
		t.Fatalf("reading final credentials.json: %v", err)
	}
	var cf struct {
		Aura struct {
			Credentials []struct {
				AccessToken string `json:"access-token"`
				TokenExpiry int64  `json:"token-expiry"`
			} `json:"credentials"`
		} `json:"aura"`
	}
	if err := json.Unmarshal(raw, &cf); err != nil {
		t.Fatalf("parsing final credentials.json: %v", err)
	}
	if len(cf.Aura.Credentials) == 0 {
		t.Fatal("final credentials.json has no Aura credentials")
	}
	cred := cf.Aura.Credentials[0]
	if cred.AccessToken == "" || cred.AccessToken == "expired-token" {
		t.Errorf("final credentials.json still holds the expired token %q; refresh may not have been persisted", cred.AccessToken)
	}
	if time.Now().UnixMilli() >= cred.TokenExpiry {
		t.Errorf("final credentials.json token-expiry %d is in the past; expected a future expiry after refresh", cred.TokenExpiry)
	}
}
