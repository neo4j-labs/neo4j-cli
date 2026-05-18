// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build e2e_exitcodes

// Package exitcodes drives the freshly-built `neo4j-cli` binary as a subprocess
// against an in-test fixture HTTP server and asserts the process exit code for
// every category in the audit §4.1 closed set:
//
//	0 success
//	1 fatal (untyped fallback; not exercised here)
//	2 usage (--bad-flag, cobra FlagErrorFunc)
//	3 not_found        (HTTP 404)
//	4 auth_error       (HTTP 401)
//	5 conflict         (HTTP 409)
//	6 validation_error (HTTP 400)
//	7 rate_limited     (HTTP 429 with Retry-After: 30 → exit 7 AND message
//	                    body contains "30")
//	8 upstream_error   (HTTP 503)
//
// Plus two zero-exit paths: happy GET → 0; bare `neo4j-cli` with no
// subcommand prints help → 0.
//
// Gated by build tag `e2e_exitcodes` so it stays out of the default
// `go test ./...` matrix, mirroring `test/e2e/release_fixture/`'s
// e2e_seams pattern.
package exitcodes_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// repoRoot walks up from this test file to the repo root (directory containing go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine caller file path")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not locate repo root (go.mod) walking up from test file")
	return ""
}

// buildBinary compiles neo4j-cli into a temp dir once per test process and
// returns the absolute path. The build flags mirror `make build` minimums:
// no debug, no stripping (kept simple — we only need a working binary).
func buildBinary(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)

	dir := t.TempDir()
	name := "neo4j-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(dir, name)

	cmd := exec.Command("go", "build", "-o", out, "./neo4j-cli")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, combined)
	}
	return out
}

// configHomeFor returns the env-var name and value that point the CLI at
// `dir` for its config/credentials. The CLI's clicfg.ConfigPrefix is set
// at init() from an OS-specific source: darwin = $HOME/Library/Preferences;
// linux = $XDG_CONFIG_HOME (or $HOME/.config); windows = %LOCALAPPDATA%.
//
// We return the smallest env override per OS so a clean subprocess env can
// be assembled without leaking host-side credentials into the test binary.
func configHomeFor(t *testing.T, dir string) []string {
	t.Helper()
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
		t.Skipf("unsupported GOOS %q for exitcodes e2e", runtime.GOOS)
		return nil
	}
}

// configDirFor mirrors configHomeFor and returns the absolute on-disk
// directory under which `neo4j-cli` will look for `config.json` and
// `credentials.json`.
func configDirFor(t *testing.T, dir string) string {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(dir, "Library", "Preferences", "neo4j", "cli")
	case "linux":
		return filepath.Join(dir, ".config", "neo4j", "cli")
	case "windows":
		return filepath.Join(dir, "neo4j", "cli")
	default:
		t.Skipf("unsupported GOOS %q for exitcodes e2e", runtime.GOOS)
		return ""
	}
}

// seedCreds writes a credentials.json that contains a long-lived access token
// so the CLI's getToken() short-circuits via HasValidAccessToken() and the
// request flows straight to the fixture (no /oauth/token hop). For the 401
// branch the fixture returns 401 directly; formatAuthorizationError clears
// the cached token (writing the file back) — the tempdir is writable so this
// is safe.
func seedCreds(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir credentials dir: %v", err)
	}
	// expiry: now + 1h in ms, matching UpdateAccessToken's units.
	expiry := time.Now().Add(1 * time.Hour).UnixMilli()
	body := fmt.Sprintf(`{
  "aura": {
    "default-credential": "e2e",
    "credentials": [
      {
        "name": "e2e",
        "client-id": "cid",
        "client-secret": "csec",
        "access-token": "stub-token",
        "token-expiry": %d
      }
    ]
  }
}`, expiry)
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write credentials.json: %v", err)
	}
}

// fixtureServer returns an httptest.Server that responds to /v1/instances
// (and any /v1/instances/<id>[/...] subpath) with the supplied status, body
// and optional Retry-After header. /oauth/token is stubbed defensively in
// case any path bypasses the cached access token.
func fixtureServer(t *testing.T, status int, retryAfter, body string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"stub-token","expires_in":3600,"token_type":"bearer"}`))
	})
	handler := func(w http.ResponseWriter, r *http.Request) {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
	mux.HandleFunc("/v1/instances", handler)
	// Trailing slash registers the subtree handler for /v1/instances/<id>[/...].
	mux.HandleFunc("/v1/instances/", handler)
	srv := httptest.NewServer(mux)
	// Force loopback bind so urlcheck.ValidateRemoteURL accepts http://.
	if !strings.HasPrefix(srv.URL, "http://127.0.0.1:") && !strings.HasPrefix(srv.URL, "http://localhost:") {
		srv.Close()
		t.Fatalf("expected loopback httptest URL, got %q", srv.URL)
	}
	// Sanity: ensure the server is up.
	conn, err := net.DialTimeout("tcp", srv.Listener.Addr().String(), 2*time.Second)
	if err != nil {
		srv.Close()
		t.Fatalf("fixture server not reachable: %v", err)
	}
	_ = conn.Close()
	return srv
}

// runCLI launches the freshly-built binary with the supplied args and a clean
// env that points it at `home` for config + `base` for the Aura API. Returns
// the exit code (from ProcessState), stdout, stderr.
func runCLI(t *testing.T, bin string, args []string, home string, baseURL string) (int, string, string) {
	t.Helper()

	cmd := exec.Command(bin, args...)

	// Build a minimal env: PATH (so child processes the binary spawns can
	// find tools), the OS-specific config-home redirect, AURA_BASE_URL /
	// AURA_AUTH_URL, and an explicit NEO4J_CLI_NO_UPDATE_NAG to keep the
	// background version-check from interfering with stderr expectations.
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"AURA_BASE_URL=" + baseURL,
		"AURA_AUTH_URL=" + baseURL + "/oauth/token",
		"NEO4J_CLI_NO_UPDATE_NAG=1",
		"DO_NOT_TRACK=1",
	}
	env = append(env, configHomeFor(t, home)...)
	// Windows needs SystemRoot for DLL loading; pass it through if set.
	if sr := os.Getenv("SystemRoot"); sr != "" {
		env = append(env, "SystemRoot="+sr)
	}
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
			t.Fatalf("running %s failed without ExitError: %v", bin, runErr)
		}
	}
	return exitCode, outBuf.String(), errBuf.String()
}

// scenarios cover every HTTP-status → exit-code mapping plus the cobra
// flag-parse and happy-path branches. The 1-fatal slot is deliberately
// not exercised here — it is the untyped fallback in exitCodeFor and is
// covered by main_test.go (unit-level).
type scenario struct {
	name       string
	args       []string
	status     int
	retryAfter string
	body       string
	wantExit   int
	// optional substring assertion on stderr (covers REQ-F-004 7→message
	// body contains the Retry-After value).
	wantStderrContains string
	skipServer         bool // for --bad-flag / no-arg help / --version
	// JSON envelope assertions: when wantJSONCode is non-empty the test
	// parses the JSON envelope out of stdout and asserts these fields.
	wantJSONCode         string
	wantJSONExitCode     int
	wantJSONResourceType string
	wantJSONResourceID   string
}

// validationBody is a well-formed Aura error envelope.
func errBody(msg string) string {
	return fmt.Sprintf(`{"errors":[{"message":%q}]}`, msg)
}

// happyInstancesBody is a minimal valid /v1/instances response so the happy
// scenario exits 0 cleanly.
const happyInstancesBody = `{"data":[]}`

func TestExitCodes(t *testing.T) {
	bin := buildBinary(t)

	scenarios := []scenario{
		{
			name:       "happy_path_200",
			args:       []string{"aura", "instance", "list", "--format", "json"},
			status:     http.StatusOK,
			body:       happyInstancesBody,
			wantExit:   0,
			skipServer: false,
		},
		{
			name:       "bad_flag_usage_2",
			args:       []string{"aura", "instance", "list", "--bad-flag"},
			wantExit:   2,
			skipServer: true,
		},
		{
			name:     "not_found_404_exit_3",
			args:     []string{"aura", "instance", "list"},
			status:   http.StatusNotFound,
			body:     errBody("instance not found"),
			wantExit: 3,
		},
		{
			name:     "auth_401_exit_4",
			args:     []string{"aura", "instance", "list"},
			status:   http.StatusUnauthorized,
			body:     errBody("invalid token"),
			wantExit: 4,
		},
		{
			name:     "conflict_409_exit_5",
			args:     []string{"aura", "instance", "list"},
			status:   http.StatusConflict,
			body:     errBody("duplicate instance"),
			wantExit: 5,
		},
		{
			name:     "validation_400_exit_6",
			args:     []string{"aura", "instance", "list"},
			status:   http.StatusBadRequest,
			body:     errBody("bad request"),
			wantExit: 6,
		},
		{
			name:               "rate_limited_429_exit_7",
			args:               []string{"aura", "instance", "list"},
			status:             http.StatusTooManyRequests,
			retryAfter:         "30",
			body:               errBody("rate limit"),
			wantExit:           7,
			wantStderrContains: "30",
		},
		{
			name:     "upstream_503_exit_8",
			args:     []string{"aura", "instance", "list"},
			status:   http.StatusServiceUnavailable,
			body:     errBody("service unavailable"),
			wantExit: 8,
		},
		{
			name:       "no_subcommand_help_exit_0",
			args:       []string{},
			wantExit:   0,
			skipServer: true,
		},
		// --format=json exits the structured envelope on stdout; cobra still
		// dumps usage info ahead of the envelope on a flag-parse error so the
		// JSON is extracted from the tail of stdout rather than the whole
		// buffer. Stderr keeps the one-line `Error: ... (exit N)` summary.
		{
			name:               "bad_flag_usage_2_json",
			args:               []string{"aura", "instance", "list", "--bad-flag", "--format=json"},
			wantExit:           2,
			skipServer:         true,
			wantStderrContains: "Error: unknown flag: --bad-flag (exit 2)",
			wantJSONCode:       "usage_error",
			wantJSONExitCode:   2,
		},
		// 404 against /v1/instances/<id> exercises the typed-error path with
		// .WithResource(...) chained on so resource_type and resource_id land
		// in the JSON envelope. --format binds successfully so stdout holds a
		// clean JSON envelope (no cobra usage preamble).
		{
			name:                 "not_found_404_get_json",
			args:                 []string{"aura", "instance", "get", "does-not-exist", "--format=json"},
			status:               http.StatusNotFound,
			body:                 errBody("instance not found"),
			wantExit:             3,
			wantStderrContains:   "Error: ",
			wantJSONCode:         "not_found",
			wantJSONExitCode:     3,
			wantJSONResourceType: "instance",
			wantJSONResourceID:   "does-not-exist",
		},
	}

	for _, tc := range scenarios {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			seedCreds(t, configDirFor(t, home))

			baseURL := "http://127.0.0.1:1"
			if !tc.skipServer {
				srv := fixtureServer(t, tc.status, tc.retryAfter, tc.body)
				defer srv.Close()
				baseURL = srv.URL
			}

			code, stdout, stderr := runCLI(t, bin, tc.args, home, baseURL)
			if code != tc.wantExit {
				t.Fatalf("%s: exit code = %d (want %d)\nargs=%v\nstdout:\n%s\nstderr:\n%s",
					tc.name, code, tc.wantExit, tc.args, stdout, stderr)
			}
			if tc.wantStderrContains != "" {
				combined := stdout + stderr
				if !strings.Contains(combined, tc.wantStderrContains) {
					t.Fatalf("%s: combined output does not contain %q\nstdout:\n%s\nstderr:\n%s",
						tc.name, tc.wantStderrContains, stdout, stderr)
				}
			}
			if tc.wantJSONCode != "" {
				assertJSONEnvelope(t, tc, stdout)
			}
		})
	}
}

// jsonEnvelope mirrors the shape clierr.Envelope marshals on stdout. Declared
// locally so the e2e package stays decoupled from clierr's internal layout.
type jsonEnvelope struct {
	Error struct {
		Code         string `json:"code"`
		ExitCode     int    `json:"exit_code"`
		Message      string `json:"message"`
		ResourceType string `json:"resource_type,omitempty"`
		ResourceID   string `json:"resource_id,omitempty"`
		Retryable    bool   `json:"retryable"`
	} `json:"error"`
}

// extractEnvelope finds the JSON envelope inside stdout. The cobra flag-parse
// path dumps usage info ahead of the envelope when a flag is rejected, so we
// scan for the last line beginning with `{` and treat it as the envelope.
func extractEnvelope(stdout string) (string, bool) {
	stdout = strings.TrimRight(stdout, "\n")
	lines := strings.Split(stdout, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			return line, true
		}
	}
	return "", false
}

func assertJSONEnvelope(t *testing.T, tc scenario, stdout string) {
	t.Helper()
	payload, ok := extractEnvelope(stdout)
	if !ok {
		t.Fatalf("%s: no JSON envelope found on stdout\nstdout:\n%s", tc.name, stdout)
	}
	var env jsonEnvelope
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		t.Fatalf("%s: stdout envelope is not valid JSON: %v\npayload:\n%s", tc.name, err, payload)
	}
	if env.Error.Code != tc.wantJSONCode {
		t.Fatalf("%s: envelope code = %q (want %q)\npayload:\n%s",
			tc.name, env.Error.Code, tc.wantJSONCode, payload)
	}
	if env.Error.ExitCode != tc.wantJSONExitCode {
		t.Fatalf("%s: envelope exit_code = %d (want %d)\npayload:\n%s",
			tc.name, env.Error.ExitCode, tc.wantJSONExitCode, payload)
	}
	if tc.wantJSONResourceType != "" && env.Error.ResourceType != tc.wantJSONResourceType {
		t.Fatalf("%s: envelope resource_type = %q (want %q)\npayload:\n%s",
			tc.name, env.Error.ResourceType, tc.wantJSONResourceType, payload)
	}
	if tc.wantJSONResourceID != "" && env.Error.ResourceID != tc.wantJSONResourceID {
		t.Fatalf("%s: envelope resource_id = %q (want %q)\npayload:\n%s",
			tc.name, env.Error.ResourceID, tc.wantJSONResourceID, payload)
	}
}
