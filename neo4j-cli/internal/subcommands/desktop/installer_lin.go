// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package desktop — installer_lin.go owns the Linux install action: copy
// the AppImage into the target dir, chmod +x, and write an XDG `.desktop`
// launcher entry. arm64 is rejected at the orchestration layer.
//
// Filename uses `_lin` (not `_linux`) so Go's filename-based GOOS constraint
// doesn't restrict it to GOOS=linux — seam-mocked tests need to build it
// on every host.
package desktop

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/neo4j/cli/common/clierr"
)

// installerLinuxAppImageBase. The version suffix is appended at install
// time so the detector's linuxAppImagePattern regex keeps matching.
const installerLinuxAppImageBase = "neo4j-desktop-"

// installerLinuxDesktopEntryName stays stable across installs so launchers
// refresh the entry rather than leaving stale `.desktop` files.
const installerLinuxDesktopEntryName = "neo4j-desktop-2.desktop"

// installerLinuxDesktopEntryTemplate uses an absolute Exec= so launchers
// don't resolve PATH. Icon= is the bundle name; Desktop 2 sets a proper
// icon once it launches.
const installerLinuxDesktopEntryTemplate = `[Desktop Entry]
Name=Neo4j Desktop 2
Comment=Neo4j Desktop 2 — local graph database workbench
Exec=%s
Icon=neo4j-desktop-2
Terminal=false
Type=Application
Categories=Development;Database;
`

var (
	installerLinuxHomeDirFn = os.UserHomeDir

	installerLinuxMkdirAllFn = os.MkdirAll

	installerLinuxWriteFileFn = os.WriteFile

	installerLinuxChmodFn = os.Chmod
)

// SetInstallerLinuxHomeDirFnForTest overrides the home-dir resolver.
func SetInstallerLinuxHomeDirFnForTest(fn func() (string, error)) func() {
	prev := installerLinuxHomeDirFn
	installerLinuxHomeDirFn = fn
	return func() { installerLinuxHomeDirFn = prev }
}

// SetInstallerLinuxMkdirAllFnForTest overrides the mkdir seam.
func SetInstallerLinuxMkdirAllFnForTest(fn func(string, os.FileMode) error) func() {
	prev := installerLinuxMkdirAllFn
	installerLinuxMkdirAllFn = fn
	return func() { installerLinuxMkdirAllFn = prev }
}

// SetInstallerLinuxWriteFileFnForTest overrides the write-file seam.
func SetInstallerLinuxWriteFileFnForTest(fn func(string, []byte, os.FileMode) error) func() {
	prev := installerLinuxWriteFileFn
	installerLinuxWriteFileFn = fn
	return func() { installerLinuxWriteFileFn = prev }
}

// SetInstallerLinuxChmodFnForTest overrides the chmod seam.
func SetInstallerLinuxChmodFnForTest(fn func(string, os.FileMode) error) func() {
	prev := installerLinuxChmodFn
	installerLinuxChmodFn = fn
	return func() { installerLinuxChmodFn = prev }
}

// installLinux copies the AppImage into the target dir, chmod +x's it, and
// writes the XDG `.desktop` launcher entry. Uses copy + Chmod rather than
// rename so a per-installer-tempdir filesystem (e.g. `/tmp` on a separate
// mount) still works.
func installLinux(_ context.Context, plan InstallPlan) error {
	if err := installerLinuxMkdirAllFn(plan.TargetDir, 0o755); err != nil {
		return clierr.NewFatalError(
			"desktop install (linux): create target dir %s: %s", plan.TargetDir, err.Error())
	}

	appImagePath := filepath.Join(plan.TargetDir,
		fmt.Sprintf("%s%s.AppImage", installerLinuxAppImageBase, plan.Version))

	// Remove a stale AppImage from a prior --force run so the copy
	// doesn't trip on a non-overwriteable existing file.
	_ = os.Remove(appImagePath)

	if err := copyFilePreservingDirs(plan.ArtifactPath, appImagePath); err != nil {
		return clierr.NewFatalError(
			"desktop install (linux): copy AppImage to %s: %s", appImagePath, err.Error())
	}

	if err := installerLinuxChmodFn(appImagePath, 0o755); err != nil {
		return clierr.NewFatalError(
			"desktop install (linux): chmod %s: %s", appImagePath, err.Error())
	}

	home, err := installerLinuxHomeDirFn()
	if err != nil || home == "" {
		return clierr.NewFatalError(
			"desktop install (linux): resolve home dir: %v", err)
	}
	xdgAppsDir := filepath.Join(home, ".local", "share", "applications")
	if err := installerLinuxMkdirAllFn(xdgAppsDir, 0o755); err != nil {
		return clierr.NewFatalError(
			"desktop install (linux): create XDG apps dir %s: %s", xdgAppsDir, err.Error())
	}
	entryPath := filepath.Join(xdgAppsDir, installerLinuxDesktopEntryName)
	entryBody := fmt.Sprintf(installerLinuxDesktopEntryTemplate, appImagePath)
	if err := installerLinuxWriteFileFn(entryPath, []byte(entryBody), 0o644); err != nil {
		return clierr.NewFatalError(
			"desktop install (linux): write XDG entry %s: %s", entryPath, err.Error())
	}

	setLastInstalledTargetDir(appImagePath)
	return nil
}

// copyFilePreservingDirs copies a flat file; macOS uses its own recursive
// helper because it copies a `.app` bundle directory tree.
func copyFilePreservingDirs(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", dst, err)
	}
	in, err := os.Open(src) //nolint:gosec // src is a verified tempfile path
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy bytes to %s: %w", dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", dst, err)
	}
	return nil
}
