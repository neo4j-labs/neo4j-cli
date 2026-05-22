// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
)

// detectInstalled is unexported; tests reach it indirectly via the public
// `Install` cobra leaf in install_test.go. These tests directly exercise
// the per-OS detection paths by pinning the home dir / glob / stat seams
// and observing the breadcrumb on the install leaf when --target-dir +
// --dry-run prevents network I/O. Pure-Go unit tests of the detector live
// here because hitting the cobra plumbing is overkill for "does this glob
// match correctly".
//
// Behavioural coverage is split as follows:
//   - install_test.go covers the FULL orchestration on the LINUX branch
//     (detect → manifest → verify → action) end-to-end.
//   - This file covers macOS / Windows detection edge cases (Info.plist
//     parse, registry-version fallback) without going through cobra.

func TestDetect_macOS_ReadsVersionFromInfoPlist(t *testing.T) {
	// Set up a fake `.app` directory and an Info.plist with the canonical
	// CFBundleShortVersionString key.
	tmp := t.TempDir()
	appPath := filepath.Join(tmp, "Neo4j Desktop 2.app")
	contentsPath := filepath.Join(appPath, "Contents")
	if err := os.MkdirAll(contentsPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	plistBody := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleShortVersionString</key>
	<string>2.0.10</string>
	<key>CFBundleVersion</key>
	<string>10</string>
</dict>
</plist>`
	if err := os.WriteFile(filepath.Join(contentsPath, "Info.plist"), []byte(plistBody), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}

	// Point macOS detection at tmp by pinning the home dir + stat. The
	// `/Applications/Neo4j Desktop 2.app` path won't exist; the
	// `~/Applications/...` path will.
	t.Cleanup(desktop.SetDetectHomeDirFnForTest(func() (string, error) { return tmp, nil }))
	// Symlink-or-copy the app dir into ~/Applications/Neo4j Desktop 2.app
	// inside tmp (the home-dir seam already points at tmp).
	if err := os.MkdirAll(filepath.Join(tmp, "Applications"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Rename(appPath, filepath.Join(tmp, "Applications", "Neo4j Desktop 2.app")); err != nil {
		t.Fatalf("move app: %v", err)
	}

	// Pin stat so the canonical `/Applications/...` candidate (which may
	// exist on the dev host) is rejected — only the tmp-rooted candidate
	// is allowed to match.
	t.Cleanup(desktop.SetDetectStatFnForTest(func(p string) (os.FileInfo, error) {
		if strings.HasPrefix(p, tmp) {
			return os.Stat(p)
		}
		return nil, os.ErrNotExist
	}))
	t.Cleanup(desktop.SetDetectReadFileFnForTest(os.ReadFile))

	hit, ok := desktop.DetectInstalledForTest("darwin")
	if !ok {
		t.Fatalf("expected macOS detection hit")
	}
	if hit.Version != "2.0.10" {
		t.Fatalf("expected version 2.0.10 from Info.plist, got %q", hit.Version)
	}
	if !strings.Contains(hit.Path, "Neo4j Desktop 2.app") {
		t.Fatalf("expected hit.Path to point at .app, got %q", hit.Path)
	}
}

func TestDetect_Linux_ExtractsVersionFromAppImageFilename(t *testing.T) {
	tmp := t.TempDir()
	appsDir := filepath.Join(tmp, "Applications")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	appImagePath := filepath.Join(appsDir, "neo4j-desktop-2.0.11.AppImage")
	if err := os.WriteFile(appImagePath, []byte("body"), 0o755); err != nil {
		t.Fatalf("write appimage: %v", err)
	}

	t.Cleanup(desktop.SetDetectHomeDirFnForTest(func() (string, error) { return tmp, nil }))
	t.Cleanup(desktop.SetDetectGlobFnForTest(filepath.Glob))

	hit, ok := desktop.DetectInstalledForTest("linux")
	if !ok {
		t.Fatalf("expected linux detection hit")
	}
	if hit.Version != "2.0.11" {
		t.Fatalf("expected version 2.0.11 parsed from filename, got %q", hit.Version)
	}
	if hit.Path != appImagePath {
		t.Fatalf("expected hit.Path %q, got %q", appImagePath, hit.Path)
	}
}

func TestDetect_Linux_NoMatch_NoHit(t *testing.T) {
	tmp := t.TempDir()
	t.Cleanup(desktop.SetDetectHomeDirFnForTest(func() (string, error) { return tmp, nil }))
	t.Cleanup(desktop.SetDetectGlobFnForTest(filepath.Glob))

	if _, ok := desktop.DetectInstalledForTest("linux"); ok {
		t.Fatalf("expected no detection hit when no AppImage is installed")
	}
}

func TestDetect_Windows_StatHitWithoutVersion(t *testing.T) {
	tmp := t.TempDir()
	installDir := filepath.Join(tmp, "Programs", "neo4j-desktop")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Cleanup(desktop.SetDetectLocalAppDataFnForTest(func() string { return tmp }))
	t.Cleanup(desktop.SetDetectStatFnForTest(os.Stat))
	t.Cleanup(desktop.SetDetectReadFileFnForTest(os.ReadFile))

	hit, ok := desktop.DetectInstalledForTest("windows")
	if !ok {
		t.Fatalf("expected windows detection hit")
	}
	if hit.Path != installDir {
		t.Fatalf("expected hit.Path %q, got %q", installDir, hit.Path)
	}
	// No version-hint file present → version is empty; the breadcrumb
	// renders this as "version unknown".
	if hit.Version != "" {
		t.Fatalf("expected empty version when no hint file is present, got %q", hit.Version)
	}
}

func TestDetect_Windows_ReadsVersionFromPackageJSON(t *testing.T) {
	tmp := t.TempDir()
	installDir := filepath.Join(tmp, "Programs", "neo4j-desktop")
	pkgDir := filepath.Join(installDir, "resources", "app.asar.unpacked")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pkgBody := `{
  "name": "neo4j-desktop-2",
  "version": "2.0.12"
}`
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(pkgBody), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	t.Cleanup(desktop.SetDetectLocalAppDataFnForTest(func() string { return tmp }))
	t.Cleanup(desktop.SetDetectStatFnForTest(os.Stat))
	t.Cleanup(desktop.SetDetectReadFileFnForTest(os.ReadFile))

	hit, ok := desktop.DetectInstalledForTest("windows")
	if !ok {
		t.Fatalf("expected windows detection hit")
	}
	if hit.Version != "2.0.12" {
		t.Fatalf("expected version 2.0.12 from package.json, got %q", hit.Version)
	}
}

func TestDetect_UnknownOS_NoHit(t *testing.T) {
	if _, ok := desktop.DetectInstalledForTest("plan9"); ok {
		t.Fatalf("expected no detection hit on unknown OS")
	}
}
