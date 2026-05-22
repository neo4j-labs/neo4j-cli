// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/neo4j/cli/common/clierr"
)

// newProbeServer spins up an httptest server that responds 200 on ProbePath
// and 404 elsewhere. Returns the server + its host:port for use in the
// probe host/client overrides. Caller is responsible for t.Cleanup-ing.
func newProbeServer(t *testing.T) (*httptest.Server, string, int) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(ProbePath, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	u := strings.TrimPrefix(srv.URL, "http://")
	host, portStr, err := net.SplitHostPort(u)
	if err != nil {
		t.Fatalf("SplitHostPort %q: %v", u, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("atoi: %v", err)
	}
	return srv, host, port
}

// pinProbeTo wires the package-level probe seams at the given host so the
// scan walks the test server's port range. ports[port] = true means
// "respond 200" — anything else means "no answer". Returns nothing; uses
// t.Cleanup to restore the seams.
func pinProbeTo(t *testing.T, host string, hits map[int]bool) {
	t.Helper()
	restoreHost := SetProbeHostFnForTest(func() string { return host })
	t.Cleanup(restoreHost)

	// HTTP client that only succeeds for ports listed as hits — uses a
	// custom RoundTripper so we don't need separate test servers per port.
	restoreClient := SetHTTPClientFnForTest(func() *http.Client {
		return &http.Client{
			Timeout: probeTimeoutPerPort,
			Transport: roundTripperFn(func(req *http.Request) (*http.Response, error) {
				_, portStr, _ := net.SplitHostPort(req.URL.Host)
				port, _ := strconv.Atoi(portStr)
				if !hits[port] {
					return nil, fmt.Errorf("connection refused on port %d (test)", port)
				}
				rec := httptest.NewRecorder()
				rec.WriteHeader(http.StatusOK)
				return rec.Result(), nil
			}),
		}
	})
	t.Cleanup(restoreClient)
}

type roundTripperFn func(*http.Request) (*http.Response, error)

func (f roundTripperFn) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProbePort_FirstPortLive(t *testing.T) {
	pinProbeTo(t, "127.0.0.1", map[int]bool{ProbePortStart: true})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	got, err := ProbePort(ctx, 0)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ProbePort: %v", err)
	}
	if got.Port != ProbePortStart {
		t.Fatalf("got.Port = %d, want %d", got.Port, ProbePortStart)
	}
	if got.Origin != fmt.Sprintf("http://127.0.0.1:%d", ProbePortStart) {
		t.Fatalf("got.Origin = %q", got.Origin)
	}
	// Latency budget: ≤200ms when default port is live (REQ-NF-003).
	if elapsed > 200*time.Millisecond {
		t.Fatalf("probe took %v, expected ≤200ms", elapsed)
	}
}

func TestProbePort_WalksUpOnConflict(t *testing.T) {
	// Simulate Desktop having auto-incremented to port 44225.
	pinProbeTo(t, "127.0.0.1", map[int]bool{44225: true})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := ProbePort(ctx, 0)
	if err != nil {
		t.Fatalf("ProbePort: %v", err)
	}
	if got.Port != 44225 {
		t.Fatalf("got.Port = %d, want 44225", got.Port)
	}
}

func TestProbePort_NoneLive(t *testing.T) {
	pinProbeTo(t, "127.0.0.1", map[int]bool{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	_, err := ProbePort(ctx, 0)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrNoDesktop) {
		t.Fatalf("expected ErrNoDesktop, got %v", err)
	}
	// Worst-case 11 probes * 200ms cap; allow a bit of slack for slow CI.
	if elapsed > 3*time.Second {
		t.Fatalf("scan took %v, expected ≤3s", elapsed)
	}
}

func TestProbePort_TCPOpenAloneDoesNotCount(t *testing.T) {
	// Spin up a real TCP listener that accepts but returns 404 on the
	// probe path — verifies the raw-TCP-open path is rejected.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() }) //nolint:errcheck // listener close error is not actionable
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	}
	go srv.Serve(listener) //nolint:errcheck // server lifetime owned by listener cleanup
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		srv.Shutdown(ctx) //nolint:errcheck // shutdown error during test teardown is not actionable
	})

	_, port, _ := net.SplitHostPort(listener.Addr().String())
	portInt, _ := strconv.Atoi(port)
	restoreHost := SetProbeHostFnForTest(func() string { return "127.0.0.1" })
	t.Cleanup(restoreHost)
	restoreClient := SetHTTPClientFnForTest(func() *http.Client { return &http.Client{Timeout: probeTimeoutPerPort} })
	t.Cleanup(restoreClient)

	_, err = ProbePort(context.Background(), portInt)
	if !errors.Is(err, ErrNoDesktop) {
		t.Fatalf("expected ErrNoDesktop for 404 responder, got %v", err)
	}
}

func TestProbePort_HitsLiveServer(t *testing.T) {
	srv, host, port := newProbeServer(t)
	_ = srv

	restoreHost := SetProbeHostFnForTest(func() string { return host })
	t.Cleanup(restoreHost)
	restoreClient := SetHTTPClientFnForTest(func() *http.Client { return &http.Client{Timeout: probeTimeoutPerPort} })
	t.Cleanup(restoreClient)

	got, err := ProbePort(context.Background(), port)
	if err != nil {
		t.Fatalf("ProbePort pinned %d: %v", port, err)
	}
	if got.Port != port {
		t.Fatalf("got.Port = %d, want %d", got.Port, port)
	}
}

func TestProbePort_PinnedPortMissReturnsErrNoDesktop(t *testing.T) {
	pinProbeTo(t, "127.0.0.1", map[int]bool{})

	_, err := ProbePort(context.Background(), 44222)
	if !errors.Is(err, ErrNoDesktop) {
		t.Fatalf("expected ErrNoDesktop, got %v", err)
	}
}

func TestResolveDataDir_EnvVarWins(t *testing.T) {
	t.Setenv("NEO4J_DESKTOP_DATA_PATH", p("/custom/root"))
	t.Setenv("NEO4J_DESKTOP_ENV", "")
	withUserConfigDir(t, p("/cfg"))

	got, err := ResolveDataDir(context.Background(), afero.NewMemMapFs(), ProbeResult{})
	if err != nil {
		t.Fatalf("ResolveDataDir: %v", err)
	}
	want := filepath.Join(p("/custom/root"), "Application", "Data")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveDataDir_ActiveEnvWins(t *testing.T) {
	t.Setenv("NEO4J_DESKTOP_DATA_PATH", "")
	t.Setenv("NEO4J_DESKTOP_ENV", "")
	withUserConfigDir(t, p("/cfg"))
	fs := afero.NewMemMapFs()
	writeEnvJSON(t, fs, p("/cfg"), "active.json", `{
		"name": "Default",
		"id": "x",
		"active": true,
		"type": "LOCAL",
		"relateDataPath": "/data/relate"
	}`)

	got, err := ResolveDataDir(context.Background(), fs, ProbeResult{})
	if err != nil {
		t.Fatalf("ResolveDataDir: %v", err)
	}
	if got != "/data/relate" {
		t.Fatalf("got %q, want %q", got, "/data/relate")
	}
}

func TestResolveDataDir_NEO4J_DESKTOP_ENVOverride(t *testing.T) {
	t.Setenv("NEO4J_DESKTOP_DATA_PATH", "")
	t.Setenv("NEO4J_DESKTOP_ENV", "Other")
	withUserConfigDir(t, p("/cfg"))
	fs := afero.NewMemMapFs()
	writeEnvJSON(t, fs, p("/cfg"), "a.json", `{
		"name": "Default", "id": "a", "active": true, "type": "LOCAL",
		"relateDataPath": "/data/default"
	}`)
	writeEnvJSON(t, fs, p("/cfg"), "b.json", `{
		"name": "Other", "id": "b", "active": false, "type": "LOCAL",
		"relateDataPath": "/data/other"
	}`)

	got, err := ResolveDataDir(context.Background(), fs, ProbeResult{})
	if err != nil {
		t.Fatalf("ResolveDataDir: %v", err)
	}
	if got != "/data/other" {
		t.Fatalf("got %q, want %q (override should have selected Other)", got, "/data/other")
	}
}

func TestResolveDataDir_PerOSDefaults(t *testing.T) {
	t.Setenv("NEO4J_DESKTOP_DATA_PATH", "")
	t.Setenv("NEO4J_DESKTOP_ENV", "")
	withUserConfigDir(t, p("/cfg")) // No envs on disk → fallback path.
	fs := afero.NewMemMapFs()

	restoreHome := SetHomeDirFnForTest(func() (string, error) { return p("/home/u"), nil })
	t.Cleanup(restoreHome)

	cases := []struct {
		goos string
		want string
	}{
		{"darwin", p("/home/u/Library/Application Support/neo4j-desktop/Application/Data")},
		{"linux", p("/home/u/.config/neo4j-desktop/Application/Data")},
		{"windows", p("/home/u/.Neo4jDesktop2/Data")},
	}
	for _, tc := range cases {
		t.Run(tc.goos, func(t *testing.T) {
			restoreGOOS := SetGOOSFnForTest(func() string { return tc.goos })
			t.Cleanup(restoreGOOS)

			got, err := ResolveDataDir(context.Background(), fs, ProbeResult{})
			if err != nil {
				t.Fatalf("ResolveDataDir: %v", err)
			}
			if got != tc.want {
				t.Fatalf("goos=%s: got %q, want %q", tc.goos, got, tc.want)
			}
		})
	}
}

func TestResolveDataDir_EnvWithoutRelateDataPathFallsThrough(t *testing.T) {
	t.Setenv("NEO4J_DESKTOP_DATA_PATH", "")
	t.Setenv("NEO4J_DESKTOP_ENV", "")
	withUserConfigDir(t, p("/cfg"))
	fs := afero.NewMemMapFs()
	// Active env but no relateDataPath — must fall through to OS default.
	writeEnvJSON(t, fs, p("/cfg"), "x.json", `{"name":"X","id":"x","active":true,"type":"LOCAL"}`)

	restoreHome := SetHomeDirFnForTest(func() (string, error) { return p("/home/u"), nil })
	t.Cleanup(restoreHome)
	restoreGOOS := SetGOOSFnForTest(func() string { return "linux" })
	t.Cleanup(restoreGOOS)

	got, err := ResolveDataDir(context.Background(), fs, ProbeResult{})
	if err != nil {
		t.Fatalf("ResolveDataDir: %v", err)
	}
	want := p("/home/u/.config/neo4j-desktop/Application/Data")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLoadSalt_ReadsFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	dataDir := p("/data")
	if err := fs.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}
	const salt = "11111111-2222-3333-4444-555555555555"
	if err := afero.WriteFile(fs, filepath.Join(dataDir, SaltFilename), []byte(salt+"\n"), 0o600); err != nil {
		t.Fatalf("write salt: %v", err)
	}

	got, err := LoadSalt(fs, dataDir)
	if err != nil {
		t.Fatalf("LoadSalt: %v", err)
	}
	if got != salt {
		t.Fatalf("got %q, want %q (trailing whitespace must be trimmed)", got, salt)
	}
}

func TestLoadSalt_MissingFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	_, err := LoadSalt(fs, p("/nope"))
	if err == nil {
		t.Fatalf("expected error for missing salt file")
	}
}

// TestDefaultDataDir_NativeOS exercises the unmocked runtime.GOOS + real
// os.UserHomeDir path so each CI matrix entry (ubuntu/macos/windows) asserts
// its own native shape. Catches drift in os.UserHomeDir output that the
// mocked seam tests cannot see.
func TestDefaultDataDir_NativeOS(t *testing.T) {
	got, err := defaultDataDir()
	if err != nil {
		t.Fatalf("defaultDataDir: %v", err)
	}
	var wantSuffix string
	switch runtime.GOOS {
	case "darwin":
		wantSuffix = filepath.FromSlash("Library/Application Support/neo4j-desktop/Application/Data")
	case "windows":
		wantSuffix = filepath.FromSlash(".Neo4jDesktop2/Data")
	default:
		wantSuffix = filepath.FromSlash(".config/neo4j-desktop/Application/Data")
	}
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("goos=%s: defaultDataDir = %q, want suffix %q", runtime.GOOS, got, wantSuffix)
	}
	// Windows must NOT have the `\Application\Data` segment — that was the
	// task-016 bug. Defense-in-depth in case the suffix above is loosened.
	if runtime.GOOS == "windows" {
		bad := filepath.FromSlash(".Neo4jDesktop2/Application/Data")
		if strings.HasSuffix(got, bad) {
			t.Fatalf("windows: defaultDataDir = %q still has the buggy %q suffix", got, bad)
		}
	}
}

// fetchAppInfoFullPayload is the canonical 200 reply Desktop dev builds return
// from `GET /fastify/api/info/app`. All seven fields are present.
const fetchAppInfoFullPayload = `{
	"platform": "darwin",
	"version": "2.0.0",
	"appPath": "/Applications/Neo4j Desktop 2.app",
	"logsPath": "/Users/u/Library/Logs/Neo4j Desktop 2",
	"dataPath": "/Users/u/Library/Application Support/com.Neo4j.Relate/Data",
	"cachePath": "/Users/u/Library/Caches/com.Neo4j.Relate",
	"configPath": "/Users/u/Library/Application Support/com.Neo4j.Relate/Config"
}`

func TestFetchAppInfo_200_HappyPathDecodesAllFields(t *testing.T) {
	var seenReq *http.Request
	t.Cleanup(SetHTTPDoFnForTest(func(req *http.Request) (*http.Response, error) {
		seenReq = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(fetchAppInfoFullPayload)),
			Header:     http.Header{},
		}, nil
	}))

	got, err := FetchAppInfo(context.Background(), ProbeResult{Origin: "http://example.invalid"})
	if err != nil {
		t.Fatalf("FetchAppInfo: %v", err)
	}

	// Sanity-check the request shape: URL, method, no auth headers.
	if seenReq == nil {
		t.Fatalf("httpDoFn was not invoked")
	}
	if seenReq.Method != http.MethodGet {
		t.Fatalf("method = %q, want GET", seenReq.Method)
	}
	if seenReq.URL.String() != "http://example.invalid/fastify/api/info/app" {
		t.Fatalf("url = %q, want http://example.invalid/fastify/api/info/app", seenReq.URL.String())
	}
	if v := seenReq.Header.Get(HeaderClientID); v != "" {
		t.Fatalf("FetchAppInfo must NOT send %s, got %q", HeaderClientID, v)
	}
	if v := seenReq.Header.Get(HeaderAPIToken); v != "" {
		t.Fatalf("FetchAppInfo must NOT send %s, got %q", HeaderAPIToken, v)
	}

	want := AppInfo{
		Platform:   "darwin",
		Version:    "2.0.0",
		AppPath:    "/Applications/Neo4j Desktop 2.app",
		LogsPath:   "/Users/u/Library/Logs/Neo4j Desktop 2",
		DataPath:   "/Users/u/Library/Application Support/com.Neo4j.Relate/Data",
		CachePath:  "/Users/u/Library/Caches/com.Neo4j.Relate",
		ConfigPath: "/Users/u/Library/Application Support/com.Neo4j.Relate/Config",
	}
	if got != want {
		t.Fatalf("AppInfo mismatch:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestFetchAppInfo_200_UnknownFieldsIgnored(t *testing.T) {
	// Unknown future fields must NOT cause a decode error — default JSON
	// decoder behaviour. Asserts forward-compat with later Desktop builds.
	payload := `{"platform":"linux","version":"2.1.0","dataPath":"/d","futureField":42,"nested":{"x":1}}`
	t.Cleanup(SetHTTPDoFnForTest(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payload)),
			Header:     http.Header{},
		}, nil
	}))

	got, err := FetchAppInfo(context.Background(), ProbeResult{Origin: "http://example.invalid"})
	if err != nil {
		t.Fatalf("FetchAppInfo: %v", err)
	}
	if got.Platform != "linux" || got.Version != "2.1.0" || got.DataPath != "/d" {
		t.Fatalf("unexpected decoded shape: %+v", got)
	}
}

func TestFetchAppInfo_401_ReturnsAuthError(t *testing.T) {
	// Older Desktop builds without the route-level exemption — middleware
	// 401s the request because no JWT is supplied. FetchAppInfo surfaces a
	// typed auth error so the caller (ResolveDataDir, doctor) can fall
	// through to the next discovery branch without surfacing the failure.
	t.Cleanup(SetHTTPDoFnForTest(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
			Header:     http.Header{},
		}, nil
	}))

	_, err := FetchAppInfo(context.Background(), ProbeResult{Origin: "http://example.invalid"})
	if err == nil {
		t.Fatalf("expected error on 401")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T, want *clierr.CLIError", err)
	}
	if ce.Code != 4 {
		t.Fatalf("exit code = %d, want 4 (auth)", ce.Code)
	}
}

func TestFetchAppInfo_5xx_ReturnsFatalErrorWithTruncatedBody(t *testing.T) {
	bigBody := strings.Repeat("B", 500)
	t.Cleanup(SetHTTPDoFnForTest(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader(bigBody)),
			Header:     http.Header{},
		}, nil
	}))

	_, err := FetchAppInfo(context.Background(), ProbeResult{Origin: "http://example.invalid"})
	if err == nil {
		t.Fatalf("expected error on 500")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T", err)
	}
	if ce.Code != 1 {
		t.Fatalf("exit code = %d, want 1 (fatal)", ce.Code)
	}
	if !strings.Contains(ce.Message, "returned 500") {
		t.Fatalf("message missing status: %q", ce.Message)
	}
	if strings.Contains(ce.Message, bigBody) {
		t.Fatalf("message contains untruncated body")
	}
	if !strings.Contains(ce.Message, "…") {
		t.Fatalf("message missing truncation marker: %q", ce.Message)
	}
}

func TestFetchAppInfo_TransportError_MapsToUnreachable(t *testing.T) {
	// httpDoFn returns a non-deadline error → canonical unreachable hint.
	t.Cleanup(SetHTTPDoFnForTest(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	}))

	_, err := FetchAppInfo(context.Background(), ProbeResult{Origin: "http://example.invalid"})
	if err == nil {
		t.Fatalf("expected transport error")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T", err)
	}
	if !strings.Contains(ce.Message, "doesn't appear to be running") {
		t.Fatalf("missing canonical unreachable message: %q", ce.Message)
	}
}

func TestFetchAppInfo_DeadlineExceeded_MapsToCanonicalTimeout(t *testing.T) {
	// httpDoFn blocks until the context deadline fires. The caller-supplied
	// context cancels first; FetchAppInfo wraps it in context.WithTimeout
	// internally, but the inner timeout never fires here — the outer cancel
	// triggers a DeadlineExceeded that matches errors.Is the same way.
	t.Cleanup(SetHTTPDoFnForTest(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	_, err := FetchAppInfo(ctx, ProbeResult{Origin: "http://example.invalid"})
	if err == nil {
		t.Fatalf("expected deadline error")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T", err)
	}
	if !strings.Contains(ce.Message, "did not respond within") {
		t.Fatalf("deadline did not map to canonicalTimeout: %q", ce.Message)
	}
	if strings.Contains(ce.Message, "doesn't appear to be running") {
		t.Fatalf("deadline-exceeded must NOT collapse to canonicalUnreachable: %q", ce.Message)
	}
}

func TestFetchAppInfo_DecodeError_MapsToFatal(t *testing.T) {
	// 200 with malformed JSON body → fatal decode error. Caller treats this
	// as "fall through to next discovery branch" per REQ-F-204.
	t.Cleanup(SetHTTPDoFnForTest(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{not valid json`)),
			Header:     http.Header{},
		}, nil
	}))

	_, err := FetchAppInfo(context.Background(), ProbeResult{Origin: "http://example.invalid"})
	if err == nil {
		t.Fatalf("expected decode error")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T", err)
	}
	if !strings.Contains(ce.Message, "failed to decode /info/app") {
		t.Fatalf("missing decode-error message: %q", ce.Message)
	}
}

func TestFetchAppInfo_EmptyBody_MapsToDecodeError(t *testing.T) {
	// 200 with empty body — encoding/json rejects "" as invalid JSON. Same
	// fall-through semantics as the decode-error case above.
	t.Cleanup(SetHTTPDoFnForTest(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     http.Header{},
		}, nil
	}))

	_, err := FetchAppInfo(context.Background(), ProbeResult{Origin: "http://example.invalid"})
	if err == nil {
		t.Fatalf("expected decode error on empty body")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T", err)
	}
	if !strings.Contains(ce.Message, "failed to decode /info/app") {
		t.Fatalf("missing decode-error message: %q", ce.Message)
	}
}

func TestFetchAppInfo_RespectsRequestTimeoutBudget(t *testing.T) {
	// The internal context.WithTimeout budget must reflect requestTimeout (90s)
	// — the same default the authenticated Client.doRaw uses. Tests observe it
	// by reading the request context's deadline rather than waiting.
	var observed time.Duration
	t.Cleanup(SetHTTPDoFnForTest(func(req *http.Request) (*http.Response, error) {
		if dl, ok := req.Context().Deadline(); ok {
			observed = time.Until(dl)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(fetchAppInfoFullPayload)),
			Header:     http.Header{},
		}, nil
	}))

	if _, err := FetchAppInfo(context.Background(), ProbeResult{Origin: "http://example.invalid"}); err != nil {
		t.Fatalf("FetchAppInfo: %v", err)
	}
	// requestTimeout is 90s; allow generous slack for capture latency. Must
	// NOT collapse to the 30s of a prior default or anything obviously off.
	if observed <= 60*time.Second {
		t.Fatalf("deadline budget = %s, want > 60s (requestTimeout 90s)", observed)
	}
	if observed > requestTimeout {
		t.Fatalf("deadline budget = %s, must not exceed requestTimeout %s", observed, requestTimeout)
	}
}

// TestResolveDataDir_InfoAppPrecedence is the table-driven gate for the new
// step-2 (/info/app) precedence branch added in task-002. Every case asserts
// the resolved data dir AND the number of httpDoFn invocations so the
// NEO4J_DESKTOP_DATA_PATH short-circuit case can prove /info/app is NOT
// reached. The env-JSON + OS-default fallbacks share scaffolding (a probe
// origin + a fake httpDoFn) so the four-step ladder is exercised end-to-end.
func TestResolveDataDir_InfoAppPrecedence(t *testing.T) {
	const probeOrigin = "http://example.invalid"

	// envJSONActive seeds the env-JSON branch with a relateDataPath of
	// "/data/from-env". Tests that expect this branch to win wire it in.
	envJSONActive := func(t *testing.T) afero.Fs {
		t.Helper()
		fs := afero.NewMemMapFs()
		writeEnvJSON(t, fs, p("/cfg"), "active.json", `{
			"name": "Default", "id": "x", "active": true, "type": "LOCAL",
			"relateDataPath": "/data/from-env"
		}`)
		return fs
	}

	// emptyFS produces an empty in-memory fs — no env-JSON, so a fallback
	// from step 2 falls all the way through to step 4 (OS default).
	emptyFS := func(t *testing.T) afero.Fs {
		t.Helper()
		return afero.NewMemMapFs()
	}

	// makeResp wraps a status + body literal as an http.Response stub for
	// the httpDoFn seam.
	makeResp := func(status int, body string) func(*http.Request) (*http.Response, error) {
		return func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{},
			}, nil
		}
	}

	// happyPayload is a minimal /info/app reply with a populated dataPath.
	const happyPayload = `{"dataPath":"/data/from-info-app"}`
	// emptyDataPathPayload is a 200 reply whose dataPath is empty — must
	// fall through per REQ-F-204.
	const emptyDataPathPayload = `{"dataPath":""}`

	cases := []struct {
		name            string
		probe           ProbeResult
		envOverridePath string // value for NEO4J_DESKTOP_DATA_PATH (empty = unset)
		fs              func(t *testing.T) afero.Fs
		httpDoFn        func(*http.Request) (*http.Response, error)
		wantPath        string
		wantHTTPCalls   int
	}{
		{
			name:          "infoApp 200 wins over env-JSON + OS default",
			probe:         ProbeResult{Origin: probeOrigin},
			fs:            envJSONActive,
			httpDoFn:      makeResp(http.StatusOK, happyPayload),
			wantPath:      "/data/from-info-app",
			wantHTTPCalls: 1,
		},
		{
			name:          "infoApp 401 falls back to env-JSON",
			probe:         ProbeResult{Origin: probeOrigin},
			fs:            envJSONActive,
			httpDoFn:      makeResp(http.StatusUnauthorized, `{"error":"unauthorized"}`),
			wantPath:      "/data/from-env",
			wantHTTPCalls: 1,
		},
		{
			name:          "infoApp 401 + no env-JSON falls back to OS default",
			probe:         ProbeResult{Origin: probeOrigin},
			fs:            emptyFS,
			httpDoFn:      makeResp(http.StatusUnauthorized, `{"error":"unauthorized"}`),
			wantPath:      p("/home/u/.config/neo4j-desktop/Application/Data"),
			wantHTTPCalls: 1,
		},
		{
			name:          "infoApp 5xx falls through to env-JSON",
			probe:         ProbeResult{Origin: probeOrigin},
			fs:            envJSONActive,
			httpDoFn:      makeResp(http.StatusInternalServerError, `boom`),
			wantPath:      "/data/from-env",
			wantHTTPCalls: 1,
		},
		{
			name:  "infoApp transport error falls through to env-JSON",
			probe: ProbeResult{Origin: probeOrigin},
			fs:    envJSONActive,
			httpDoFn: func(_ *http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			},
			wantPath:      "/data/from-env",
			wantHTTPCalls: 1,
		},
		{
			name:          "infoApp decode error falls through to env-JSON",
			probe:         ProbeResult{Origin: probeOrigin},
			fs:            envJSONActive,
			httpDoFn:      makeResp(http.StatusOK, `{not valid json`),
			wantPath:      "/data/from-env",
			wantHTTPCalls: 1,
		},
		{
			name:          "infoApp empty dataPath falls through to env-JSON",
			probe:         ProbeResult{Origin: probeOrigin},
			fs:            envJSONActive,
			httpDoFn:      makeResp(http.StatusOK, emptyDataPathPayload),
			wantPath:      "/data/from-env",
			wantHTTPCalls: 1,
		},
		{
			name:            "NEO4J_DESKTOP_DATA_PATH short-circuits before infoApp",
			probe:           ProbeResult{Origin: probeOrigin},
			envOverridePath: p("/custom/root"),
			fs:              envJSONActive,
			// httpDoFn must NOT be invoked — but we wire one to a counter
			// so we can prove the assertion below.
			httpDoFn:      makeResp(http.StatusOK, happyPayload),
			wantPath:      filepath.Join(p("/custom/root"), "Application", "Data"),
			wantHTTPCalls: 0,
		},
		{
			name:          "empty ProbeResult skips infoApp entirely (no HTTP calls)",
			probe:         ProbeResult{},
			fs:            envJSONActive,
			httpDoFn:      makeResp(http.StatusOK, happyPayload), // never invoked
			wantPath:      "/data/from-env",
			wantHTTPCalls: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NEO4J_DESKTOP_DATA_PATH", tc.envOverridePath)
			t.Setenv("NEO4J_DESKTOP_ENV", "")
			withUserConfigDir(t, p("/cfg"))

			restoreHome := SetHomeDirFnForTest(func() (string, error) { return p("/home/u"), nil })
			t.Cleanup(restoreHome)
			restoreGOOS := SetGOOSFnForTest(func() string { return "linux" })
			t.Cleanup(restoreGOOS)

			// Count httpDoFn invocations so the short-circuit branches can
			// assert zero calls; the underlying response is delegated to the
			// per-case stub.
			var httpCalls int
			t.Cleanup(SetHTTPDoFnForTest(func(req *http.Request) (*http.Response, error) {
				httpCalls++
				return tc.httpDoFn(req)
			}))

			fs := tc.fs(t)
			got, err := ResolveDataDir(context.Background(), fs, tc.probe)
			if err != nil {
				t.Fatalf("ResolveDataDir: %v", err)
			}
			if got != tc.wantPath {
				t.Fatalf("path = %q, want %q", got, tc.wantPath)
			}
			if httpCalls != tc.wantHTTPCalls {
				t.Fatalf("httpDoFn invocations = %d, want %d", httpCalls, tc.wantHTTPCalls)
			}
		})
	}
}
