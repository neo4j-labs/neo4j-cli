// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
	"github.com/spf13/afero"
)

// --- checkInstallPresent ---------------------------------------------------
//
// Tests the install-detection branch on macOS / Linux / Windows by pinning
// the doctor's GOOS seam to each value and seeding the install-detect Fn
// seams to a fixture tree. The detection helpers are reused from
// install_detect.go; this exercise locks the doctor wiring against the
// detect helper's per-OS branches plus the unknown-OS FAIL path.

func TestDoctor_CheckInstallPresent_macOS_Pass(t *testing.T) {
	tmp := t.TempDir()
	appsDir := filepath.Join(tmp, "Applications")
	appPath := filepath.Join(appsDir, "Neo4j Desktop 2.app")
	contentsPath := filepath.Join(appPath, "Contents")
	if err := os.MkdirAll(contentsPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	plistBody := `<?xml version="1.0"?><plist><dict>
	<key>CFBundleShortVersionString</key>
	<string>2.0.42</string>
	</dict></plist>`
	if err := os.WriteFile(filepath.Join(contentsPath, "Info.plist"), []byte(plistBody), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}

	t.Cleanup(desktop.SetDoctorGoosFnForTest(func() string { return "darwin" }))
	t.Cleanup(desktop.SetDetectHomeDirFnForTest(func() (string, error) { return tmp, nil }))
	t.Cleanup(desktop.SetDetectStatFnForTest(func(p string) (os.FileInfo, error) {
		// Reject the canonical `/Applications/...` candidate so only the
		// tmp-rooted candidate matches — dev hosts may have a real install.
		if strings.HasPrefix(p, tmp) {
			return os.Stat(p)
		}
		return nil, os.ErrNotExist
	}))
	t.Cleanup(desktop.SetDetectReadFileFnForTest(os.ReadFile))

	got := desktop.CheckInstallPresentForTest()
	if got.Status != "pass" {
		t.Fatalf("expected status=pass, got %+v", got)
	}
	if !strings.Contains(got.Detail, "2.0.42") {
		t.Fatalf("expected version 2.0.42 in detail, got %q", got.Detail)
	}
	if !strings.Contains(got.Detail, "Neo4j Desktop 2.app") {
		t.Fatalf("expected .app path in detail, got %q", got.Detail)
	}
}

func TestDoctor_CheckInstallPresent_Linux_Pass(t *testing.T) {
	tmp := t.TempDir()
	appsDir := filepath.Join(tmp, "Applications")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	appImagePath := filepath.Join(appsDir, "neo4j-desktop-2.0.7.AppImage")
	if err := os.WriteFile(appImagePath, []byte("body"), 0o755); err != nil {
		t.Fatalf("write appimage: %v", err)
	}

	t.Cleanup(desktop.SetDoctorGoosFnForTest(func() string { return "linux" }))
	t.Cleanup(desktop.SetDetectHomeDirFnForTest(func() (string, error) { return tmp, nil }))
	t.Cleanup(desktop.SetDetectGlobFnForTest(filepath.Glob))

	got := desktop.CheckInstallPresentForTest()
	if got.Status != "pass" {
		t.Fatalf("expected status=pass, got %+v", got)
	}
	if !strings.Contains(got.Detail, "2.0.7") {
		t.Fatalf("expected version 2.0.7 in detail, got %q", got.Detail)
	}
}

func TestDoctor_CheckInstallPresent_Windows_Pass(t *testing.T) {
	tmp := t.TempDir()
	installDir := filepath.Join(tmp, "Programs", "neo4j-desktop")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Cleanup(desktop.SetDoctorGoosFnForTest(func() string { return "windows" }))
	t.Cleanup(desktop.SetDetectLocalAppDataFnForTest(func() string { return tmp }))
	t.Cleanup(desktop.SetDetectStatFnForTest(os.Stat))
	t.Cleanup(desktop.SetDetectReadFileFnForTest(os.ReadFile))

	got := desktop.CheckInstallPresentForTest()
	if got.Status != "pass" {
		t.Fatalf("expected status=pass, got %+v", got)
	}
	if got.Detail != installDir {
		t.Fatalf("expected detail=%q (no version hint file), got %q", installDir, got.Detail)
	}
}

func TestDoctor_CheckInstallPresent_UnknownGOOS_Fail(t *testing.T) {
	t.Cleanup(desktop.SetDoctorGoosFnForTest(func() string { return "plan9" }))

	got := desktop.CheckInstallPresentForTest()
	if got.Status != "fail" {
		t.Fatalf("expected status=fail, got %+v", got)
	}
	if !strings.Contains(got.Detail, "plan9") {
		t.Fatalf("expected GOOS in detail, got %q", got.Detail)
	}
	if got.Hint == "" {
		t.Fatalf("expected non-empty hint on FAIL")
	}
}

// --- checkDataDir -----------------------------------------------------------

func TestDoctor_CheckDataDir_Pass(t *testing.T) {
	tmp := t.TempDir()
	t.Cleanup(desktopclient.SetHomeDirFnForTest(func() (string, error) { return tmp, nil }))
	t.Cleanup(desktopclient.SetGOOSFnForTest(func() string { return "linux" }))

	// Clear NEO4J_DESKTOP_DATA_PATH to avoid env-var override on the dev host.
	t.Setenv("NEO4J_DESKTOP_DATA_PATH", "")
	t.Setenv("NEO4J_DESKTOP_ENV", "")

	got := desktop.CheckDataDirForTest(context.Background(), afero.NewMemMapFs(), desktopclient.ProbeResult{})
	if got.Status != "pass" {
		t.Fatalf("expected status=pass, got %+v", got)
	}
	want := filepath.Join(tmp, ".config", "neo4j-desktop", "Application", "Data")
	if got.Detail != want {
		t.Fatalf("expected detail=%q, got %q", want, got.Detail)
	}
}

func TestDoctor_CheckDataDir_HomeDirError_Fail(t *testing.T) {
	t.Cleanup(desktopclient.SetHomeDirFnForTest(func() (string, error) {
		return "", errors.New("no home dir")
	}))
	t.Cleanup(desktopclient.SetGOOSFnForTest(func() string { return "linux" }))
	t.Setenv("NEO4J_DESKTOP_DATA_PATH", "")
	t.Setenv("NEO4J_DESKTOP_ENV", "")

	got := desktop.CheckDataDirForTest(context.Background(), afero.NewMemMapFs(), desktopclient.ProbeResult{})
	if got.Status != "fail" {
		t.Fatalf("expected status=fail, got %+v", got)
	}
	if !strings.Contains(got.Detail, "no home dir") {
		t.Fatalf("expected underlying error in detail, got %q", got.Detail)
	}
	if got.Hint == "" {
		t.Fatalf("expected non-empty hint on FAIL")
	}
}

// --- checkAuthDataReadable --------------------------------------------------

func TestDoctor_CheckAuthDataReadable_Pass(t *testing.T) {
	fs := afero.NewMemMapFs()
	dataDir := "/data"
	saltPath := filepath.Join(dataDir, desktopclient.SaltFilename)
	if err := afero.WriteFile(fs, saltPath, []byte("uuid-like-payload"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := desktop.CheckAuthDataReadableForTest(fs, dataDir)
	if got.Status != "pass" {
		t.Fatalf("expected status=pass, got %+v", got)
	}
	// REQ-F-008 — label and detail must not leak salt / secret / key / JWT
	// wording. Spot-check the four banned tokens.
	for _, banned := range []string{"secret", "salt", "JWT", " key"} {
		if strings.Contains(strings.ToLower(got.Label), strings.ToLower(banned)) {
			t.Fatalf("label leaks %q wording: %q", banned, got.Label)
		}
		if strings.Contains(strings.ToLower(got.Detail), strings.ToLower(banned)) {
			t.Fatalf("detail leaks %q wording: %q", banned, got.Detail)
		}
	}
	// And the salt payload itself must not appear in the detail.
	if strings.Contains(got.Detail, "uuid-like-payload") {
		t.Fatalf("detail leaks salt payload: %q", got.Detail)
	}
}

func TestDoctor_CheckAuthDataReadable_Missing_Fail(t *testing.T) {
	fs := afero.NewMemMapFs()
	dataDir := "/data"

	got := desktop.CheckAuthDataReadableForTest(fs, dataDir)
	if got.Status != "fail" {
		t.Fatalf("expected status=fail, got %+v", got)
	}
	if got.Hint == "" {
		t.Fatalf("expected non-empty hint on FAIL")
	}
}

func TestDoctor_CheckAuthDataReadable_Empty_Fail(t *testing.T) {
	fs := afero.NewMemMapFs()
	dataDir := "/data"
	saltPath := filepath.Join(dataDir, desktopclient.SaltFilename)
	if err := afero.WriteFile(fs, saltPath, []byte("   \n   "), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := desktop.CheckAuthDataReadableForTest(fs, dataDir)
	if got.Status != "fail" {
		t.Fatalf("expected status=fail on empty salt, got %+v", got)
	}
	if !strings.Contains(strings.ToLower(got.Detail), "empty") {
		t.Fatalf("expected detail to mention empty, got %q", got.Detail)
	}
}

// --- checkStandardProbe -----------------------------------------------------
//
// Reuses the existing probe HTTP-client seam so the scan walks an
// in-memory RoundTripper instead of real localhost ports. Tests cover the
// scan-range happy path, the no-listener default-range FAIL, and the
// `--port 12345` FAIL where the user-supplied port must surface in the
// detail.

type doctorProbeRT struct {
	hits map[int]bool
}

func (rt doctorProbeRT) RoundTrip(req *http.Request) (*http.Response, error) {
	_, portStr, _ := net.SplitHostPort(req.URL.Host)
	port, _ := strconv.Atoi(portStr)
	if !rt.hits[port] {
		return nil, fmt.Errorf("connection refused on port %d (test)", port)
	}
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusOK)
	return rec.Result(), nil
}

func pinProbeForDoctor(t *testing.T, hits map[int]bool) {
	t.Helper()
	t.Cleanup(desktopclient.SetProbeHostFnForTest(func() string { return "127.0.0.1" }))
	t.Cleanup(desktopclient.SetHTTPClientFnForTest(func() *http.Client {
		return &http.Client{Transport: doctorProbeRT{hits: hits}, Timeout: 200 * time.Millisecond}
	}))
}

func TestDoctor_CheckStandardProbe_Pass(t *testing.T) {
	pinProbeForDoctor(t, map[int]bool{desktopclient.ProbePortStart: true})

	got := desktop.CheckStandardProbeForTest(context.Background(), 0)
	if got.Status != "pass" {
		t.Fatalf("expected status=pass, got %+v", got)
	}
	wantPortStr := strconv.Itoa(desktopclient.ProbePortStart)
	if !strings.Contains(got.Detail, wantPortStr) {
		t.Fatalf("expected detail to mention port %s, got %q", wantPortStr, got.Detail)
	}
}

func TestDoctor_CheckStandardProbe_NoListener_DefaultRange_Fail(t *testing.T) {
	pinProbeForDoctor(t, map[int]bool{})

	got := desktop.CheckStandardProbeForTest(context.Background(), 0)
	if got.Status != "fail" {
		t.Fatalf("expected status=fail, got %+v", got)
	}
	// FAIL detail mentions the standard scan range.
	if !strings.Contains(got.Detail, "44222") {
		t.Fatalf("expected detail to mention 44222..44232 range, got %q", got.Detail)
	}
	if got.Hint == "" {
		t.Fatalf("expected non-empty hint on FAIL")
	}
}

func TestDoctor_CheckStandardProbe_PinnedPort_NoListener_Fail(t *testing.T) {
	pinProbeForDoctor(t, map[int]bool{})

	got := desktop.CheckStandardProbeForTest(context.Background(), 12345)
	if got.Status != "fail" {
		t.Fatalf("expected status=fail, got %+v", got)
	}
	// The user-supplied port (12345) must surface in the FAIL detail so the
	// table renderer (task-004) can show users exactly which port was tried.
	if !strings.Contains(got.Detail, "12345") {
		t.Fatalf("expected detail to mention user-supplied port 12345, got %q", got.Detail)
	}
	// And the scan-range hint must NOT appear when --port is set.
	if strings.Contains(got.Detail, "44222") {
		t.Fatalf("expected NO standard-range mention when --port pinned, got %q", got.Detail)
	}
}

// --- checkAuthenticatedProbe ----------------------------------------------
//
// Builds a desktopclient.Client whose transport is pinned to a httptest
// server. The probe is `GET /dbmss` — a no-arg list route that requires
// auth but no UUID, side-stepping the route-validation 400 / handler-500
// ambiguity that a synthetic per-id GET produces against live Desktop.

func newDoctorAuthClient(t *testing.T, handler http.HandlerFunc) *desktopclient.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	t.Cleanup(desktopclient.SetUUIDFnForTest(func() string { return "cid-doctor" }))
	t.Cleanup(desktopclient.SetNowFnForTest(func() time.Time {
		return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	}))

	client, err := desktopclient.NewClient(desktopclient.ProbeResult{Origin: srv.URL}, "salt-doctor")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func TestDoctor_CheckAuthenticatedProbe_2xx_Pass(t *testing.T) {
	client := newDoctorAuthClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Expect the GET against /fastify/api/dbmss (list route, no id).
		if !strings.HasSuffix(r.URL.Path, "/dbmss") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[]`))
	})

	got := desktop.CheckAuthenticatedProbeForTest(context.Background(), client)
	if got.Status != "pass" {
		t.Fatalf("expected status=pass on 2xx, got %+v", got)
	}
}

func TestDoctor_CheckAuthenticatedProbe_401_Fail(t *testing.T) {
	client := newDoctorAuthClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad token"}`))
	})

	got := desktop.CheckAuthenticatedProbeForTest(context.Background(), client)
	if got.Status != "fail" {
		t.Fatalf("expected status=fail on 401, got %+v", got)
	}
	if !strings.Contains(got.Detail, "401") {
		t.Fatalf("expected detail to mention 401, got %q", got.Detail)
	}
	if got.Hint == "" {
		t.Fatalf("expected non-empty hint on auth FAIL")
	}
}

func TestDoctor_CheckAuthenticatedProbe_5xx_Fail(t *testing.T) {
	client := newDoctorAuthClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})

	got := desktop.CheckAuthenticatedProbeForTest(context.Background(), client)
	if got.Status != "fail" {
		t.Fatalf("expected status=fail on 5xx, got %+v", got)
	}
}

// --- checkInfoApp ----------------------------------------------------------
//
// Drives `checkInfoApp` directly via `CheckInfoAppForTest` so the
// httpDoFn seam swap and the probe → AppInfo plumbing are exercised end to
// end without going through the orchestrator. Failure variants (401 / 5xx /
// transport / decode / empty DataPath) all must surface as INFO — the row
// is purely diagnostic and is NEVER allowed to FAIL the doctor report
// (REQ-F-207).

func TestDoctor_CheckInfoApp(t *testing.T) {
	const happyPayload = `{
		"platform":"darwin",
		"version":"2.0.7",
		"appPath":"/Applications/Neo4j Desktop 2.app",
		"logsPath":"/Users/u/Library/Logs/Neo4j Desktop 2",
		"dataPath":"/Users/u/Library/Application Support/com.Neo4j.Relate/Data",
		"cachePath":"/Users/u/Library/Caches/com.Neo4j.Relate",
		"configPath":"/Users/u/Library/Application Support/com.Neo4j.Relate/Config"
	}`

	makeRespFn := func(status int, body string) func(*http.Request) (*http.Response, error) {
		return func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{},
			}, nil
		}
	}

	cases := []struct {
		name       string
		probe      desktopclient.ProbeResult
		httpDoFn   func(*http.Request) (*http.Response, error)
		wantStatus string
		wantDetail string
		wantFields *desktopclient.AppInfo // when set, asserts structured fields
	}{
		{
			name:       "200_pass_populates_structured_fields",
			probe:      desktopclient.ProbeResult{Origin: "http://localhost:44222"},
			httpDoFn:   makeRespFn(http.StatusOK, happyPayload),
			wantStatus: desktop.StatusPass,
			wantFields: &desktopclient.AppInfo{
				Version:  "2.0.7",
				AppPath:  "/Applications/Neo4j Desktop 2.app",
				DataPath: "/Users/u/Library/Application Support/com.Neo4j.Relate/Data",
			},
		},
		{
			name:       "401_info_unavailable",
			probe:      desktopclient.ProbeResult{Origin: "http://localhost:44222"},
			httpDoFn:   makeRespFn(http.StatusUnauthorized, `{"error":"unauthorized"}`),
			wantStatus: desktop.StatusInfo,
			wantDetail: "unavailable (older Desktop)",
		},
		{
			name:       "5xx_info_unavailable",
			probe:      desktopclient.ProbeResult{Origin: "http://localhost:44222"},
			httpDoFn:   makeRespFn(http.StatusInternalServerError, `{"error":"boom"}`),
			wantStatus: desktop.StatusInfo,
			wantDetail: "unavailable (older Desktop)",
		},
		{
			name:  "transport_error_info_unavailable",
			probe: desktopclient.ProbeResult{Origin: "http://localhost:44222"},
			httpDoFn: func(_ *http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			},
			wantStatus: desktop.StatusInfo,
			wantDetail: "unavailable (older Desktop)",
		},
		{
			name:       "decode_error_info_unavailable",
			probe:      desktopclient.ProbeResult{Origin: "http://localhost:44222"},
			httpDoFn:   makeRespFn(http.StatusOK, `not-json`),
			wantStatus: desktop.StatusInfo,
			wantDetail: "unavailable (older Desktop)",
		},
		{
			name:       "empty_data_path_info_unavailable",
			probe:      desktopclient.ProbeResult{Origin: "http://localhost:44222"},
			httpDoFn:   makeRespFn(http.StatusOK, `{"version":"2.0.0","dataPath":""}`),
			wantStatus: desktop.StatusInfo,
			wantDetail: "unavailable (older Desktop)",
		},
		{
			name:       "empty_probe_skips_to_info",
			probe:      desktopclient.ProbeResult{},
			httpDoFn:   nil, // must NOT be invoked when probe.Origin is empty
			wantStatus: desktop.StatusInfo,
			wantDetail: "unavailable (older Desktop)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			if tc.httpDoFn != nil {
				fn := tc.httpDoFn
				t.Cleanup(desktopclient.SetHTTPDoFnForTest(func(req *http.Request) (*http.Response, error) {
					calls++
					return fn(req)
				}))
			} else {
				t.Cleanup(desktopclient.SetHTTPDoFnForTest(func(_ *http.Request) (*http.Response, error) {
					calls++
					return nil, errors.New("httpDoFn unexpectedly called with empty probe")
				}))
			}

			got := desktop.CheckInfoAppForTest(context.Background(), tc.probe)

			if got.Name != desktop.CheckInfoApp {
				t.Errorf("Name = %q, want %q", got.Name, desktop.CheckInfoApp)
			}
			if got.Label != desktop.LabelInfoApp {
				t.Errorf("Label = %q, want %q", got.Label, desktop.LabelInfoApp)
			}
			if got.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q (full row=%+v)", got.Status, tc.wantStatus, got)
			}
			if got.Status == desktop.StatusInfo && got.Detail != tc.wantDetail {
				t.Errorf("Detail = %q, want %q", got.Detail, tc.wantDetail)
			}
			if got.Status == desktop.StatusPass {
				if !strings.Contains(got.Detail, "version=") || !strings.Contains(got.Detail, "appPath=") || !strings.Contains(got.Detail, "dataPath=") {
					t.Errorf("PASS Detail missing version/appPath/dataPath: %q", got.Detail)
				}
			}
			if tc.wantFields != nil {
				if got.Version != tc.wantFields.Version {
					t.Errorf("Version = %q, want %q", got.Version, tc.wantFields.Version)
				}
				if got.AppPath != tc.wantFields.AppPath {
					t.Errorf("AppPath = %q, want %q", got.AppPath, tc.wantFields.AppPath)
				}
				if got.DataPath != tc.wantFields.DataPath {
					t.Errorf("DataPath = %q, want %q", got.DataPath, tc.wantFields.DataPath)
				}
			} else {
				// Failure variants must NOT leak any partially-decoded fields
				// into the structured payload — the row is INFO+detail only.
				if got.Version != "" || got.AppPath != "" || got.DataPath != "" {
					t.Errorf("INFO row unexpectedly carries structured fields: %+v", got)
				}
			}
			// Empty-probe variant: httpDoFn must NOT have been called.
			if tc.httpDoFn == nil && calls != 0 {
				t.Errorf("empty probe should short-circuit FetchAppInfo; saw %d httpDoFn invocations", calls)
			}
		})
	}
}

// TestDoctor_CheckInfoApp_NeverFails — REQ-F-207 invariant: even pathological
// HTTP responses (giant body, 4xx that isn't 401, status 0) must surface as
// INFO, never as FAIL. Guards against future regressions if the underlying
// FetchAppInfo error mapping grows new branches.
func TestDoctor_CheckInfoApp_NeverFails(t *testing.T) {
	t.Cleanup(desktopclient.SetHTTPDoFnForTest(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTeapot, // 418 — funky 4xx
			Body:       io.NopCloser(strings.NewReader(`{"please":"do not brew"}`)),
			Header:     http.Header{},
		}, nil
	}))

	got := desktop.CheckInfoAppForTest(context.Background(), desktopclient.ProbeResult{Origin: "http://localhost:44222"})
	if got.Status == desktop.StatusFail {
		t.Fatalf("info_app must never FAIL; got %+v", got)
	}
	if got.Status != desktop.StatusInfo {
		t.Fatalf("unexpected status %q (want info on 418); got %+v", got.Status, got)
	}
}

// --- checkMDNS -------------------------------------------------------------
//
// Drives checkMDNS directly via CheckMDNSForTest through the desktopclient
// mDNS browse seam (no real multicast) and the doctor GOOS seam. Mirrors the
// info_app pattern: PASS with the advertised origin on a hit, INFO on a miss,
// and NEVER StatusFail.

// TestDoctor_CheckMDNS_Hit — a responder yields PASS with the 127.0.0.1
// origin the new Desktop advertises.
func TestDoctor_CheckMDNS_Hit(t *testing.T) {
	t.Cleanup(desktopclient.SetMDNSBrowseFnForTest(func(_ context.Context) (int, bool) { return 49160, true }))

	got := desktop.CheckMDNSForTest(context.Background())

	if got.Name != desktop.CheckMDNS {
		t.Errorf("Name = %q, want %q", got.Name, desktop.CheckMDNS)
	}
	if got.Status != desktop.StatusPass {
		t.Fatalf("expected PASS on mDNS hit, got %+v", got)
	}
	if !strings.Contains(got.Detail, "http://127.0.0.1:49160") {
		t.Errorf("expected detail to carry the 127.0.0.1 origin+port, got %q", got.Detail)
	}
}

// TestDoctor_CheckMDNS_Miss_NonDarwin — no responder on a non-macOS host
// renders as INFO with no Local Network hint.
func TestDoctor_CheckMDNS_Miss_NonDarwin(t *testing.T) {
	t.Cleanup(desktopclient.SetMDNSBrowseFnForTest(func(_ context.Context) (int, bool) { return 0, false }))
	t.Cleanup(desktopclient.SetGOOSFnForTest(func() string { return "linux" }))
	t.Cleanup(desktop.SetDoctorGoosFnForTest(func() string { return "linux" }))

	got := desktop.CheckMDNSForTest(context.Background())

	if got.Status != desktop.StatusInfo {
		t.Fatalf("expected INFO on mDNS miss, got %+v", got)
	}
	if got.Hint != "" {
		t.Errorf("expected no Local Network hint off-darwin, got %q", got.Hint)
	}
}

// TestDoctor_CheckMDNS_Miss_Darwin — on macOS a miss pairs the INFO with the
// Local Network permission + --port remediation hint.
func TestDoctor_CheckMDNS_Miss_Darwin(t *testing.T) {
	t.Cleanup(desktopclient.SetMDNSBrowseFnForTest(func(_ context.Context) (int, bool) { return 0, false }))
	t.Cleanup(desktopclient.SetGOOSFnForTest(func() string { return "darwin" }))
	t.Cleanup(desktop.SetDoctorGoosFnForTest(func() string { return "darwin" }))

	got := desktop.CheckMDNSForTest(context.Background())

	if got.Status != desktop.StatusInfo {
		t.Fatalf("expected INFO on mDNS miss, got %+v", got)
	}
	if !strings.Contains(got.Hint, "Local Network") || !strings.Contains(got.Hint, "--port") {
		t.Errorf("expected darwin hint to mention Local Network and --port, got %q", got.Hint)
	}
}

// TestDoctor_CheckMDNS_NeverFails — checkMDNS is purely diagnostic and must
// never gate downstream checks via a StatusFail.
func TestDoctor_CheckMDNS_NeverFails(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Cleanup(desktopclient.SetMDNSBrowseFnForTest(func(_ context.Context) (int, bool) { return 0, false }))
		t.Cleanup(desktopclient.SetGOOSFnForTest(func() string { return goos }))
		t.Cleanup(desktop.SetDoctorGoosFnForTest(func() string { return goos }))

		got := desktop.CheckMDNSForTest(context.Background())
		if got.Status == desktop.StatusFail {
			t.Fatalf("mdns_discovery must never FAIL (goos=%s); got %+v", goos, got)
		}
	}
}
