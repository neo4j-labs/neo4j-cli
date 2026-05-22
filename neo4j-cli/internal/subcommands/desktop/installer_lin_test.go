// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
)

// linuxInstallerWriteSinker captures every WriteFile call so a single
// test asserts on the `.desktop` entry contents AND the AppImage destination.
type linuxInstallerWriteSinker struct {
	calls map[string][]byte
}

func newLinuxInstallerWriteSinker() *linuxInstallerWriteSinker {
	return &linuxInstallerWriteSinker{calls: map[string][]byte{}}
}

func (s *linuxInstallerWriteSinker) write(path string, body []byte, _ os.FileMode) error {
	s.calls[path] = append([]byte{}, body...)
	return nil
}

func linuxInstallerSetup(t *testing.T) (string, *linuxInstallerWriteSinker) {
	t.Helper()
	t.Cleanup(desktop.ClearLastInstalledTargetDirForTest)
	tmpHome := t.TempDir()
	t.Cleanup(desktop.SetInstallerLinuxHomeDirFnForTest(func() (string, error) {
		return tmpHome, nil
	}))
	t.Cleanup(desktop.SetInstallerLinuxMkdirAllFnForTest(os.MkdirAll))
	t.Cleanup(desktop.SetInstallerLinuxChmodFnForTest(os.Chmod))

	sinker := newLinuxInstallerWriteSinker()
	t.Cleanup(desktop.SetInstallerLinuxWriteFileFnForTest(sinker.write))

	return tmpHome, sinker
}

func TestInstallerLinux_HappyPath_CopiesAppImageAndWritesDesktopEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows NTFS doesn't honour unix exec bits via os.Chmod, so the
		// `+x must be set` assertion below can't pass. The Linux installer
		// only ever runs on Linux in production; skipping on Windows hosts
		// keeps the CI matrix green without losing coverage.
		t.Skip("Linux installer test requires unix exec-bit semantics")
	}
	tmpHome, sinker := linuxInstallerSetup(t)

	// Build a synthetic AppImage tempfile.
	tempDir := t.TempDir()
	artifactPath := filepath.Join(tempDir, "neo4j-desktop-2-linux-2.0.20.AppImage")
	artifactBody := []byte("fake appimage")
	if err := os.WriteFile(artifactPath, artifactBody, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	targetDir := filepath.Join(tmpHome, "Applications")
	plan := desktop.InstallPlan{
		ArtifactPath: artifactPath,
		TargetDir:    targetDir,
		Version:      "2.0.20",
	}
	if err := desktop.RunInstallActionForTest_Linux(t, plan); err != nil {
		t.Fatalf("installLinux: %v", err)
	}

	wantAppImage := filepath.Join(targetDir, "neo4j-desktop-2.0.20.AppImage")
	stat, err := os.Stat(wantAppImage)
	if err != nil {
		t.Fatalf("AppImage must exist at %s: %v", wantAppImage, err)
	}
	if stat.Mode()&0o111 == 0 {
		t.Fatalf("AppImage must be executable; mode=%o", stat.Mode())
	}

	// `.desktop` entry must have been written with absolute Exec= path.
	wantEntryPath := filepath.Join(tmpHome, ".local", "share", "applications", "neo4j-desktop-2.desktop")
	body, ok := sinker.calls[wantEntryPath]
	if !ok {
		t.Fatalf("XDG .desktop entry must be written at %s; got writes: %v", wantEntryPath, sinker.calls)
	}
	if !strings.Contains(string(body), "Exec="+wantAppImage) {
		t.Fatalf(".desktop entry must contain absolute Exec=%s; got:\n%s", wantAppImage, body)
	}
	if !strings.Contains(string(body), "Name=Neo4j Desktop 2") {
		t.Fatalf(".desktop entry must contain Name=Neo4j Desktop 2; got:\n%s", body)
	}
}

func TestInstallerLinux_StaleAppImageRemoved_BeforeCopy(t *testing.T) {
	tmpHome, _ := linuxInstallerSetup(t)

	tempDir := t.TempDir()
	artifactPath := filepath.Join(tempDir, "neo4j-desktop-2-linux-2.0.21.AppImage")
	newBody := []byte("new appimage body")
	if err := os.WriteFile(artifactPath, newBody, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	targetDir := filepath.Join(tmpHome, "Applications")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	staleBody := []byte("STALE")
	stale := filepath.Join(targetDir, "neo4j-desktop-2.0.21.AppImage")
	if err := os.WriteFile(stale, staleBody, 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	plan := desktop.InstallPlan{
		ArtifactPath: artifactPath,
		TargetDir:    targetDir,
		Version:      "2.0.21",
	}
	if err := desktop.RunInstallActionForTest_Linux(t, plan); err != nil {
		t.Fatalf("installLinux: %v", err)
	}

	got, err := os.ReadFile(stale)
	if err != nil {
		t.Fatalf("read installed: %v", err)
	}
	if string(got) != string(newBody) {
		t.Fatalf("stale AppImage must be overwritten by new body; got %q want %q", got, newBody)
	}
}

func TestInstallerLinux_WriteFileError_Surfaces(t *testing.T) {
	tmpHome, _ := linuxInstallerSetup(t)
	t.Cleanup(desktop.SetInstallerLinuxWriteFileFnForTest(func(string, []byte, os.FileMode) error {
		return errors.New("simulated XDG write failure")
	}))

	tempDir := t.TempDir()
	artifactPath := filepath.Join(tempDir, "neo4j-desktop-2-linux-2.0.22.AppImage")
	if err := os.WriteFile(artifactPath, []byte("body"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	plan := desktop.InstallPlan{
		ArtifactPath: artifactPath,
		TargetDir:    filepath.Join(tmpHome, "Applications"),
		Version:      "2.0.22",
	}
	err := desktop.RunInstallActionForTest_Linux(t, plan)
	if err == nil {
		t.Fatalf("expected XDG write failure to surface")
	}
	if !strings.Contains(err.Error(), "XDG") && !strings.Contains(err.Error(), ".desktop") {
		// The error message mentions the XDG entry path indirectly via the
		// entry filename — accept either signal.
		if !strings.Contains(err.Error(), "simulated XDG write failure") {
			t.Fatalf("error must surface the underlying write failure; got %v", err)
		}
	}
}

func TestInstallerLinux_TargetDirCreated_IfMissing(t *testing.T) {
	// REQ-F-018: ~/Applications must be created if missing. We point
	// home at a fresh tempdir so `~/Applications` definitely does not
	// exist; the action must create it before copying.
	tmpHome, _ := linuxInstallerSetup(t)

	tempDir := t.TempDir()
	artifactPath := filepath.Join(tempDir, "neo4j-desktop-2-linux-2.0.25.AppImage")
	if err := os.WriteFile(artifactPath, []byte("body"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	targetDir := filepath.Join(tmpHome, "Applications")
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Fatalf("test setup: target dir must not exist yet; got err=%v", err)
	}

	plan := desktop.InstallPlan{
		ArtifactPath: artifactPath,
		TargetDir:    targetDir,
		Version:      "2.0.25",
	}
	if err := desktop.RunInstallActionForTest_Linux(t, plan); err != nil {
		t.Fatalf("installLinux: %v", err)
	}
	if _, err := os.Stat(targetDir); err != nil {
		t.Fatalf("target dir must exist after install: %v", err)
	}
}
