// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
)

// installerMacRecorder captures every exec invocation triggered by the
// macOS installer so a single test can assert on argv shape, ordering,
// AND on the absence of forbidden flags (e.g. `-mountrandom`).
type installerMacRecorder struct {
	calls [][]string
	err   error
}

func (r *installerMacRecorder) record(_ context.Context, name string, args ...string) error {
	r.calls = append(r.calls, append([]string{name}, args...))
	return r.err
}

// macInstallerSetup pins all per-OS seams the macOS installer reads so
// the production path is exercised end-to-end without touching the host
// /Applications dir.
func macInstallerSetup(t *testing.T) (*installerMacRecorder, string) {
	t.Helper()
	t.Cleanup(desktop.ClearLastInstalledTargetDirForTest)
	rec := &installerMacRecorder{}
	t.Cleanup(desktop.SetInstallerMacRunCmdFnForTest(rec.record))

	tmpHome := t.TempDir()
	t.Cleanup(desktop.SetInstallerMacHomeDirFnForTest(func() (string, error) {
		return tmpHome, nil
	}))

	// Default mount-point glob returns a single synthetic mount path.
	t.Cleanup(desktop.SetInstallerMacGlobFnForTest(func(pattern string) ([]string, error) {
		return []string{"/Volumes/Neo4j Desktop 2"}, nil
	}))

	// Default mkdir hits the real FS via the home temp dir.
	t.Cleanup(desktop.SetInstallerMacMkdirAllFnForTest(os.MkdirAll))

	return rec, tmpHome
}

func TestInstallerMac_HappyPath_AttachCopyDetach(t *testing.T) {
	rec, _ := macInstallerSetup(t)

	target := t.TempDir()
	var copySrc, copyDst string
	t.Cleanup(desktop.SetInstallerMacCopyFnForTest(func(src, dst string) error {
		copySrc, copyDst = src, dst
		return nil
	}))

	plan := desktop.InstallPlan{
		ArtifactPath: "/tmp/neo4j-desktop-2-darwin-2.0.16.dmg",
		TargetDir:    target,
		Version:      "2.0.16",
	}
	if err := desktop.RunInstallActionForTest_Darwin(t, plan); err != nil {
		t.Fatalf("installMacOS: %v", err)
	}

	// Must have 2 exec calls: attach then detach.
	if len(rec.calls) != 2 {
		t.Fatalf("expected 2 exec calls (attach, detach); got %d: %#v", len(rec.calls), rec.calls)
	}

	attach := rec.calls[0]
	if attach[0] != "hdiutil" {
		t.Fatalf("attach command must be hdiutil; got %q", attach[0])
	}
	wantAttachFlags := []string{"attach", "-nobrowse", "-noverify", "-noautoopen", plan.ArtifactPath}
	for _, want := range wantAttachFlags {
		found := false
		for _, got := range attach[1:] {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("attach argv missing %q; got %v", want, attach)
		}
	}

	detach := rec.calls[1]
	if detach[0] != "hdiutil" || detach[1] != "detach" {
		t.Fatalf("expected detach as second call; got %v", detach)
	}
	hasQuiet := false
	for _, a := range detach {
		if a == "-quiet" {
			hasQuiet = true
		}
	}
	if !hasQuiet {
		t.Fatalf("detach must include -quiet; got %v", detach)
	}

	if !strings.HasSuffix(copySrc, "Neo4j Desktop 2.app") {
		t.Fatalf("copy src must point at the .app bundle on mount; got %q", copySrc)
	}
	if filepath.Dir(copyDst) != target {
		t.Fatalf("copy dst must be inside target dir; got %q want parent %q", copyDst, target)
	}
	if !strings.HasSuffix(copyDst, "Neo4j Desktop 2.app") {
		t.Fatalf("copy dst basename must be the .app bundle; got %q", copyDst)
	}
}

func TestInstallerMac_EACCESFallback_TargetIsApplications(t *testing.T) {
	if runtime.GOOS == "windows" {
		// The test asserts a `/Applications/` prefix on the first copy
		// destination. On Windows, filepath.Join interprets `/Applications`
		// as a relative-from-root path and produces `\Applications\...`,
		// failing the HasPrefix("/Applications/") check. The macOS installer
		// only ever runs on darwin in production.
		t.Skip("macOS installer test uses unix-style absolute paths")
	}
	rec, tmpHome := macInstallerSetup(t)
	_ = rec

	// First copy attempt simulates EACCES on /Applications; the second
	// must land in ~/Applications.
	copyCalls := atomic.Int32{}
	var fallbackDst string
	t.Cleanup(desktop.SetInstallerMacCopyFnForTest(func(src, dst string) error {
		n := copyCalls.Add(1)
		if n == 1 {
			if !strings.HasPrefix(dst, "/Applications/") {
				t.Errorf("first copy must target /Applications; got %q", dst)
			}
			return &os.PathError{Op: "open", Path: dst, Err: syscall.EACCES}
		}
		fallbackDst = dst
		return nil
	}))

	plan := desktop.InstallPlan{
		ArtifactPath: "/tmp/neo4j-desktop-2-darwin-2.0.17.dmg",
		TargetDir:    "/Applications",
		Version:      "2.0.17",
	}
	if err := desktop.RunInstallActionForTest_Darwin(t, plan); err != nil {
		t.Fatalf("installMacOS: %v", err)
	}

	if got := copyCalls.Load(); got != 2 {
		t.Fatalf("expected 2 copy attempts on EACCES fallback; got %d", got)
	}
	wantFallback := filepath.Join(tmpHome, "Applications", "Neo4j Desktop 2.app")
	if fallbackDst != wantFallback {
		t.Fatalf("fallback copy dst mismatch; got %q want %q", fallbackDst, wantFallback)
	}

	// Fallback dir must exist on disk.
	if _, err := os.Stat(filepath.Join(tmpHome, "Applications")); err != nil {
		t.Fatalf("fallback Applications dir must exist after install: %v", err)
	}

	// Breadcrumb cell must reflect the fallback path.
	if got := desktop.LastInstalledTargetDirForTest(); got != filepath.Join(tmpHome, "Applications") {
		t.Fatalf("breadcrumb cell must reflect fallback path; got %q", got)
	}
}

func TestInstallerMac_ExplicitTargetDir_DoesNotFallback(t *testing.T) {
	macInstallerSetup(t)
	t.Cleanup(desktop.SetInstallerMacCopyFnForTest(func(src, dst string) error {
		return &os.PathError{Op: "open", Path: dst, Err: syscall.EACCES}
	}))

	plan := desktop.InstallPlan{
		ArtifactPath: "/tmp/neo4j-desktop-2-darwin-2.0.18.dmg",
		TargetDir:    "/Users/explicit",
		Version:      "2.0.18",
	}
	err := desktop.RunInstallActionForTest_Darwin(t, plan)
	if err == nil {
		t.Fatalf("expected EACCES to surface when user passed explicit target dir")
	}
	if !strings.Contains(err.Error(), "/Users/explicit") {
		t.Fatalf("error must mention the explicit target dir; got %v", err)
	}
}

func TestInstallerMac_AttachFailure_Surfaces(t *testing.T) {
	rec, _ := macInstallerSetup(t)
	rec.err = errors.New("hdiutil exit status 1")

	plan := desktop.InstallPlan{
		ArtifactPath: "/tmp/neo4j-desktop-2-darwin-bad.dmg",
		TargetDir:    "/Applications",
		Version:      "bad",
	}
	err := desktop.RunInstallActionForTest_Darwin(t, plan)
	if err == nil {
		t.Fatalf("expected hdiutil attach failure to surface")
	}
	if !strings.Contains(err.Error(), "hdiutil attach") {
		t.Fatalf("error must mention hdiutil attach; got %v", err)
	}
}

func TestInstallerMac_GlobMiss_Surfaces(t *testing.T) {
	rec, _ := macInstallerSetup(t)
	_ = rec
	t.Cleanup(desktop.SetInstallerMacGlobFnForTest(func(string) ([]string, error) {
		return nil, nil
	}))

	plan := desktop.InstallPlan{
		ArtifactPath: "/tmp/neo4j-desktop-2-darwin-2.0.19.dmg",
		TargetDir:    "/Applications",
		Version:      "2.0.19",
	}
	err := desktop.RunInstallActionForTest_Darwin(t, plan)
	if err == nil {
		t.Fatalf("expected mount-point glob miss to surface as error")
	}
	if !strings.Contains(err.Error(), "mount point") {
		t.Fatalf("error must mention mount point; got %v", err)
	}
}

// TestInstallerMac_IsEACCESHandlesFsErrPermission proves the EACCES helper
// covers both raw syscall.EACCES and the higher-level fs.ErrPermission
// shape — important because Go's os helpers wrap permission errors
// differently across versions.
func TestInstallerMac_IsEACCESHandlesFsErrPermission(t *testing.T) {
	wrapped := &os.PathError{Op: "open", Path: "/Applications/foo", Err: fs.ErrPermission}
	if !desktop.IsEACCESForTest(wrapped) {
		t.Fatalf("isEACCES must recognise fs.ErrPermission")
	}
	if !desktop.IsEACCESForTest(&os.PathError{Op: "open", Path: "/x", Err: syscall.EACCES}) {
		t.Fatalf("isEACCES must recognise syscall.EACCES")
	}
	if desktop.IsEACCESForTest(errors.New("some other error")) {
		t.Fatalf("isEACCES must reject non-permission errors")
	}
}
