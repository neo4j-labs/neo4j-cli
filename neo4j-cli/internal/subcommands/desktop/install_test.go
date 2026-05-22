// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop_test

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// installHelper wires desktop.NewCmd against an in-memory FS and a stubbed
// HTTP transport. Tests pin runtime.GOOS / GOARCH and the detect-* seams so
// the install orchestration can be exercised hermetically from any host.
type installHelper struct {
	t   *testing.T
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newInstallHelper(t *testing.T) *installHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	// Default to linux so a single test can pin a specific OS without
	// inheriting host-dependent behaviour from the test runner.
	t.Cleanup(desktop.SetInstallGoosFnForTest(func() string { return "linux" }))
	t.Cleanup(desktop.SetInstallGoarchFnForTest(func() string { return "amd64" }))
	// By default detection misses so tests that don't pin a hit go through
	// the network path. Tests opting INTO detection override the stat seam.
	t.Cleanup(desktop.SetDetectStatFnForTest(func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}))
	t.Cleanup(desktop.SetDetectGlobFnForTest(func(string) ([]string, error) {
		return nil, nil
	}))
	t.Cleanup(desktop.SetDetectHomeDirFnForTest(func() (string, error) { return t.TempDir(), nil }))
	t.Cleanup(desktop.SetDetectLocalAppDataFnForTest(func() string { return t.TempDir() }))
	t.Cleanup(desktop.SetInstallTempDirFnForTest(func() string { return t.TempDir() }))
	// Default install action is a no-op so the post-verify breadcrumb can
	// be asserted without the per-OS (task-010) code path.
	t.Cleanup(desktop.SetRunInstallActionFnForTest(func(_ context.Context, _ desktop.InstallPlan) error {
		return nil
	}))
	return &installHelper{
		t:   t,
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

func (h *installHelper) run(command string) error {
	h.t.Helper()
	args, err := shlex.Split(command)
	if err != nil {
		h.t.Fatalf("shlex: %v", err)
	}
	return h.runArgs(args)
}

// runArgs is the shlex-bypass entry point. Use when an argument contains
// shell metacharacters that shlex would interpret — e.g. Windows paths
// returned by t.TempDir() which contain backslashes (shlex treats `\` as
// an escape).
func (h *installHelper) runArgs(args []string) error {
	h.t.Helper()
	cfg := clicfg.NewConfig(h.fs, "test", clicfg.GlobalScope)
	cmd := desktop.NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetArgs(args)
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)
	return cmd.Execute()
}

// manifestServer returns a stubbed dist.neo4j.org server that serves the
// supplied manifest at the canonical per-OS path and the artifact at the
// resolved relative URL. The artifact bytes are hashed in-test so the
// SHA-512 entry in the manifest matches what we serve.
func manifestServer(t *testing.T, goos, manifestBody, artifactPath string, artifactBytes []byte) *httptest.Server {
	t.Helper()
	manifestPath := manifestPathForOS(goos)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case manifestPath:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(manifestBody))
		case artifactPath:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(artifactBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func manifestPathForOS(goos string) string {
	switch goos {
	case "darwin":
		return "/neo4j-desktop-2/mac/latest-mac.yml"
	case "linux":
		return "/neo4j-desktop-2/linux/latest-linux.yml"
	case "windows":
		return "/neo4j-desktop-2/win/latest.yml"
	}
	return ""
}

// pinManifestURLs rewrites the package-level installManifestURLs map so
// the per-OS lookup points at the test server. Returns a restore closure
// run via t.Cleanup.
func pinManifestURLs(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Cleanup(desktop.SetInstallManifestURLsForTest(map[string]string{
		"darwin":  srv.URL + "/neo4j-desktop-2/mac/latest-mac.yml",
		"linux":   srv.URL + "/neo4j-desktop-2/linux/latest-linux.yml",
		"windows": srv.URL + "/neo4j-desktop-2/win/latest.yml",
	}))
	t.Cleanup(desktop.SetInstallHTTPDoFnForTest(func(req *http.Request) (*http.Response, error) {
		return srv.Client().Do(req)
	}))
}

// makeManifest builds an electron-builder YAML manifest pointing at one
// artifact whose SHA-512 matches the supplied bytes. The artifact's
// relative URL is `<base-name>` so the resolver places it next to the
// manifest URL.
func makeManifest(t *testing.T, version, artifactName string, artifactBytes []byte) (string, string) {
	t.Helper()
	sum := sha512.Sum512(artifactBytes)
	encoded := base64.StdEncoding.EncodeToString(sum[:])
	body := fmt.Sprintf(`version: %s
files:
  - url: %s
    sha512: %s
    size: %d
path: %s
sha512: %s
releaseDate: '2026-05-18T00:00:00.000Z'
`, version, artifactName, encoded, len(artifactBytes), artifactName, encoded)
	return body, encoded
}

func TestInstall_AlreadyInstalled_PrintsHintAndExitsBeforeHTTP(t *testing.T) {
	h := newInstallHelper(t)
	desktop.SetInstallGoosFnForTest(func() string { return "linux" })
	home := t.TempDir()
	appsDir := filepath.Join(home, "Applications")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	appImagePath := filepath.Join(appsDir, "neo4j-desktop-2.0.10.AppImage")
	if err := os.WriteFile(appImagePath, []byte("appimage"), 0o755); err != nil {
		t.Fatalf("write appimage: %v", err)
	}
	desktop.SetDetectHomeDirFnForTest(func() (string, error) { return home, nil })
	desktop.SetDetectGlobFnForTest(func(pattern string) ([]string, error) {
		return filepath.Glob(pattern)
	})

	httpCalls := atomic.Int32{}
	desktop.SetInstallHTTPDoFnForTest(func(*http.Request) (*http.Response, error) {
		httpCalls.Add(1)
		return nil, fmt.Errorf("should not be reached")
	})

	if err := h.run("install"); err != nil {
		t.Fatalf("install: %v", err)
	}
	out := h.out.String()
	if !strings.Contains(out, "already installed") || !strings.Contains(out, appImagePath) || !strings.Contains(out, "2.0.10") {
		t.Fatalf("expected already-installed breadcrumb mentioning path + version, got: %s", out)
	}
	if !strings.Contains(out, "--force") {
		t.Fatalf("breadcrumb must mention --force escape hatch, got: %s", out)
	}
	if got := httpCalls.Load(); got != 0 {
		t.Fatalf("already-installed must short-circuit BEFORE any HTTP call; got %d", got)
	}
}

func TestInstall_Force_SkipsDetection(t *testing.T) {
	h := newInstallHelper(t)
	desktop.SetInstallGoosFnForTest(func() string { return "linux" })

	// Force a detect-hit; the run must still proceed to the network because
	// of --force.
	desktop.SetDetectGlobFnForTest(func(string) ([]string, error) {
		return []string{"/home/test/Applications/neo4j-desktop-2.0.10.AppImage"}, nil
	})

	artifactBytes := []byte("fake-appimage-body")
	manifestBody, _ := makeManifest(t, "2.0.11", "Neo4j-Desktop-2-2.0.11.AppImage", artifactBytes)
	srv := manifestServer(t, "linux", manifestBody, "/neo4j-desktop-2/linux/Neo4j-Desktop-2-2.0.11.AppImage", artifactBytes)
	pinManifestURLs(t, srv)

	if err := h.run("install --force"); err != nil {
		t.Fatalf("install --force: %v", err)
	}
	out := h.out.String()
	if !strings.Contains(out, "Installed Neo4j Desktop 2 (version 2.0.11)") {
		t.Fatalf("expected post-install breadcrumb on --force, got: %s", out)
	}
	if strings.Contains(out, "already installed") {
		t.Fatalf("--force must skip already-installed shortcut; got: %s", out)
	}
}

func TestInstall_DryRun_PrintsURLsAndExits(t *testing.T) {
	h := newInstallHelper(t)
	desktop.SetInstallGoosFnForTest(func() string { return "linux" })

	artifactBytes := []byte("dry-run-body")
	manifestBody, _ := makeManifest(t, "2.0.12", "Neo4j-Desktop-2-2.0.12.AppImage", artifactBytes)

	manifestCalls := atomic.Int32{}
	artifactCalls := atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/neo4j-desktop-2/linux/latest-linux.yml":
			manifestCalls.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(manifestBody))
		case "/neo4j-desktop-2/linux/Neo4j-Desktop-2-2.0.12.AppImage":
			artifactCalls.Add(1)
			t.Errorf("dry-run must NOT download the artifact")
			http.Error(w, "should not be fetched", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	pinManifestURLs(t, srv)

	// Spy on the run-action seam to make sure --dry-run never hits it.
	actionCalls := atomic.Int32{}
	desktop.SetRunInstallActionFnForTest(func(_ context.Context, _ desktop.InstallPlan) error {
		actionCalls.Add(1)
		return nil
	})

	if err := h.run("install --dry-run"); err != nil {
		t.Fatalf("install --dry-run: %v", err)
	}
	out := h.out.String()
	wantManifest := srv.URL + "/neo4j-desktop-2/linux/latest-linux.yml"
	wantArtifact := srv.URL + "/neo4j-desktop-2/linux/Neo4j-Desktop-2-2.0.12.AppImage"
	if !strings.Contains(out, wantManifest) {
		t.Fatalf("dry-run must print manifest URL %q, got: %s", wantManifest, out)
	}
	if !strings.Contains(out, wantArtifact) {
		t.Fatalf("dry-run must print artifact URL %q, got: %s", wantArtifact, out)
	}
	if !strings.Contains(out, "2.0.12") {
		t.Fatalf("dry-run must print resolved version, got: %s", out)
	}
	if got := actionCalls.Load(); got != 0 {
		t.Fatalf("dry-run must NOT invoke the install action; got %d", got)
	}
	if got := manifestCalls.Load(); got == 0 {
		t.Fatalf("dry-run must fetch the manifest (to resolve URLs); got %d", got)
	}
	if got := artifactCalls.Load(); got != 0 {
		t.Fatalf("dry-run must NOT fetch the artifact; got %d", got)
	}
}

func TestInstall_SHA512Mismatch_DeletesTempfileAndErrors(t *testing.T) {
	h := newInstallHelper(t)
	desktop.SetInstallGoosFnForTest(func() string { return "linux" })

	// Build the manifest claiming a hash that does not match the bytes
	// we actually serve so the verify step fails.
	wrongBytes := []byte("wrong-bytes-for-the-manifest-hash")
	manifestBody, _ := makeManifest(t, "2.0.13", "Neo4j-Desktop-2-2.0.13.AppImage", wrongBytes)
	servedBytes := []byte("a different body the server actually sends")
	srv := manifestServer(t, "linux", manifestBody, "/neo4j-desktop-2/linux/Neo4j-Desktop-2-2.0.13.AppImage", servedBytes)
	pinManifestURLs(t, srv)

	tempDir := t.TempDir()
	desktop.SetInstallTempDirFnForTest(func() string { return tempDir })

	actionCalls := atomic.Int32{}
	desktop.SetRunInstallActionFnForTest(func(_ context.Context, _ desktop.InstallPlan) error {
		actionCalls.Add(1)
		return nil
	})

	err := h.run("install")
	if err == nil {
		t.Fatalf("expected sha512 mismatch error; got nil")
	}
	if !strings.Contains(err.Error(), "sha512 mismatch") {
		t.Fatalf("expected sha512 mismatch error, got: %v", err)
	}

	if got := actionCalls.Load(); got != 0 {
		t.Fatalf("install action must NOT be invoked on sha512 mismatch; got %d", got)
	}

	// Tempfile must be removed on mismatch.
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("readdir temp: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "neo4j-desktop-2-") {
			t.Fatalf("sha512 mismatch must delete tempfile; found %q", e.Name())
		}
	}
}

func TestInstall_Linux_arm64_HardErrors(t *testing.T) {
	h := newInstallHelper(t)
	desktop.SetInstallGoosFnForTest(func() string { return "linux" })
	desktop.SetInstallGoarchFnForTest(func() string { return "arm64" })

	// Network must NOT be reached.
	desktop.SetInstallHTTPDoFnForTest(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("network should not be reached")
	})

	err := h.run("install")
	if err == nil {
		t.Fatalf("expected arm64 hard-error; got nil")
	}
	if !strings.Contains(err.Error(), "arm64") {
		t.Fatalf("expected arm64 in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "deployment-center") {
		t.Fatalf("expected deployment-center URL in error, got: %v", err)
	}
}

func TestInstall_DryRun_DoesNotLicensePromptOrAutoLaunch(t *testing.T) {
	h := newInstallHelper(t)
	desktop.SetInstallGoosFnForTest(func() string { return "linux" })

	artifactBytes := []byte("license-check-body")
	manifestBody, _ := makeManifest(t, "2.0.14", "Neo4j-Desktop-2-2.0.14.AppImage", artifactBytes)
	srv := manifestServer(t, "linux", manifestBody, "/neo4j-desktop-2/linux/Neo4j-Desktop-2-2.0.14.AppImage", artifactBytes)
	pinManifestURLs(t, srv)

	if err := h.run("install"); err != nil {
		t.Fatalf("install: %v", err)
	}
	out := h.out.String()
	stderr := h.err.String()
	// REQ-F-021: no license-prompt text should appear on either stream.
	if strings.Contains(out, "Accept") || strings.Contains(stderr, "Accept") {
		t.Fatalf("install must not prompt for license acceptance; stdout=%q stderr=%q", out, stderr)
	}
	// REQ-F-022: stderr next-step hint must appear, NOT an `open -a` line.
	if !strings.Contains(stderr, "Run Neo4j Desktop 2") {
		t.Fatalf("expected stderr next-step hint, got: %q", stderr)
	}
	if strings.Contains(stderr, "open -a") {
		t.Fatalf("install must not auto-launch via open -a; got: %q", stderr)
	}
}

func TestInstall_TargetDir_OverridesDefault(t *testing.T) {
	h := newInstallHelper(t)
	desktop.SetInstallGoosFnForTest(func() string { return "linux" })

	artifactBytes := []byte("target-dir-body")
	manifestBody, _ := makeManifest(t, "2.0.15", "Neo4j-Desktop-2-2.0.15.AppImage", artifactBytes)
	srv := manifestServer(t, "linux", manifestBody, "/neo4j-desktop-2/linux/Neo4j-Desktop-2-2.0.15.AppImage", artifactBytes)
	pinManifestURLs(t, srv)

	customDir := t.TempDir()
	var seenTargetDir string
	desktop.SetRunInstallActionFnForTest(func(_ context.Context, plan desktop.InstallPlan) error {
		seenTargetDir = plan.TargetDir
		return nil
	})

	// customDir comes from t.TempDir(), which on Windows contains backslashes
	// that shlex would interpret as escape characters. Use runArgs to bypass
	// the shlex split.
	if err := h.runArgs([]string{"install", "--target-dir", customDir}); err != nil {
		t.Fatalf("install --target-dir: %v", err)
	}
	if seenTargetDir != customDir {
		t.Fatalf("expected --target-dir override to land in plan.TargetDir; got %q want %q", seenTargetDir, customDir)
	}
}

func TestInstall_MacOS_PicksDMG_NotZip(t *testing.T) {
	h := newInstallHelper(t)
	desktop.SetInstallGoosFnForTest(func() string { return "darwin" })

	artifactBytes := []byte("dmg-body")
	zipBytes := []byte("zip-body-squirrel")
	sumDMG := sha512.Sum512(artifactBytes)
	sumZIP := sha512.Sum512(zipBytes)
	manifestBody := fmt.Sprintf(`version: 2.0.16
files:
  - url: Neo4j-Desktop-2-2.0.16-mac.zip
    sha512: %s
    size: %d
  - url: Neo4j-Desktop-2-2.0.16.dmg
    sha512: %s
    size: %d
path: Neo4j-Desktop-2-2.0.16.dmg
sha512: %s
releaseDate: '2026-05-18T00:00:00.000Z'
`,
		base64.StdEncoding.EncodeToString(sumZIP[:]), len(zipBytes),
		base64.StdEncoding.EncodeToString(sumDMG[:]), len(artifactBytes),
		base64.StdEncoding.EncodeToString(sumDMG[:]))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/neo4j-desktop-2/mac/latest-mac.yml":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(manifestBody))
		case "/neo4j-desktop-2/mac/Neo4j-Desktop-2-2.0.16.dmg":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(artifactBytes)
		case "/neo4j-desktop-2/mac/Neo4j-Desktop-2-2.0.16-mac.zip":
			t.Errorf("install must NOT download the .zip (Squirrel.Mac vehicle)")
			http.Error(w, "should not be fetched", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	pinManifestURLs(t, srv)

	if err := h.run("install"); err != nil {
		t.Fatalf("install (mac): %v", err)
	}
	if !strings.Contains(h.out.String(), "version 2.0.16") {
		t.Fatalf("expected post-install breadcrumb mentioning version, got: %s", h.out.String())
	}
}

func TestInstall_ManifestFetchError_Surfaces(t *testing.T) {
	h := newInstallHelper(t)
	desktop.SetInstallGoosFnForTest(func() string { return "linux" })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	pinManifestURLs(t, srv)

	err := h.run("install")
	if err == nil {
		t.Fatalf("expected manifest fetch error; got nil")
	}
	if !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("expected error to mention manifest, got: %v", err)
	}
}

func TestInstall_HasWriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	root := desktop.NewCmd(cfg)
	var install *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "install" {
			install = c
			break
		}
	}
	if install == nil {
		t.Fatalf("install leaf not registered on desktop tree")
	}
	if got := install.Annotations["write"]; got != "true" {
		t.Fatalf("install must be annotated write=true; got %q", got)
	}
	if install.Example == "" || strings.HasPrefix(install.Example, "  ") {
		t.Fatalf("install Example must be non-empty flush-left; got %q", install.Example)
	}
	if !strings.Contains(install.Example, "--rw") {
		t.Fatalf("install Example must mention --rw on write invocations; got %q", install.Example)
	}
}

func TestInstall_NoArgsAllowed(t *testing.T) {
	h := newInstallHelper(t)
	err := h.run("install some-positional")
	if err == nil {
		t.Fatalf("expected error for unexpected positional arg")
	}
}
