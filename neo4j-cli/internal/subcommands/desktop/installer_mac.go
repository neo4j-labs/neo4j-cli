// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package desktop — installer_mac.go owns the macOS install action:
// hdiutil attach the verified DMG, copy the `.app` bundle into the target
// dir, and detach. Default target is `/Applications`; on EACCES the action
// falls back to `~/Applications` so non-admin users can install without
// elevation.
package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/neo4j/cli/common/clierr"
)

const installerMacAppName = "Neo4j Desktop 2.app"

// installerMacMountPrefix glob discovers the mount point because
// `hdiutil attach`'s stdout shape has changed without warning historically.
const installerMacMountPrefix = "/Volumes/Neo4j Desktop 2"

var (
	installerMacRunCmdFn = func(ctx context.Context, name string, args ...string) error {
		return exec.CommandContext(ctx, name, args...).Run()
	}

	installerMacCopyFn = copyDirRecursive

	installerMacMkdirAllFn = os.MkdirAll

	installerMacGlobFn = filepath.Glob

	installerMacHomeDirFn = os.UserHomeDir
)

// SetInstallerMacRunCmdFnForTest overrides the exec runner.
func SetInstallerMacRunCmdFnForTest(fn func(context.Context, string, ...string) error) func() {
	prev := installerMacRunCmdFn
	installerMacRunCmdFn = fn
	return func() { installerMacRunCmdFn = prev }
}

// SetInstallerMacCopyFnForTest overrides the directory-copy seam.
func SetInstallerMacCopyFnForTest(fn func(src, dst string) error) func() {
	prev := installerMacCopyFn
	installerMacCopyFn = fn
	return func() { installerMacCopyFn = prev }
}

// SetInstallerMacMkdirAllFnForTest overrides the mkdir seam.
func SetInstallerMacMkdirAllFnForTest(fn func(string, os.FileMode) error) func() {
	prev := installerMacMkdirAllFn
	installerMacMkdirAllFn = fn
	return func() { installerMacMkdirAllFn = prev }
}

// SetInstallerMacGlobFnForTest overrides the glob seam.
func SetInstallerMacGlobFnForTest(fn func(string) ([]string, error)) func() {
	prev := installerMacGlobFn
	installerMacGlobFn = fn
	return func() { installerMacGlobFn = prev }
}

// SetInstallerMacHomeDirFnForTest overrides the home-dir seam.
func SetInstallerMacHomeDirFnForTest(fn func() (string, error)) func() {
	prev := installerMacHomeDirFn
	installerMacHomeDirFn = fn
	return func() { installerMacHomeDirFn = prev }
}

// installMacOS runs hdiutil attach, copies the `.app` (falling back to
// `~/Applications` on EACCES against `/Applications`), then detaches.
// Detach failure is best-effort — the bytes are already on disk.
// `--target-dir` disables the fallback so EACCES surfaces directly.
func installMacOS(ctx context.Context, plan InstallPlan) error {
	if err := installerMacRunCmdFn(ctx,
		"hdiutil", "attach", "-nobrowse", "-noverify", "-noautoopen", plan.ArtifactPath); err != nil {
		return clierr.NewFatalError(
			"desktop install (macOS): hdiutil attach %s failed: %s", plan.ArtifactPath, err.Error())
	}

	mountPoint, err := resolveMacMountPoint()
	if err != nil {
		// Best-effort detach so a leaked mount doesn't linger. Don't shadow
		// the original error.
		_ = installerMacRunCmdFn(ctx, "hdiutil", "detach", installerMacMountPrefix, "-quiet")
		return err
	}

	defer func() {
		// Best-effort detach; a failed detach is not fatal (bytes are on disk).
		_ = installerMacRunCmdFn(ctx, "hdiutil", "detach", mountPoint, "-quiet")
	}()

	src := filepath.Join(mountPoint, installerMacAppName)
	finalTarget, err := copyMacAppWithFallback(plan, src)
	if err != nil {
		return err
	}
	// `plan` is a value; record the resolved target in the process-wide
	// cell so the cobra leaf's breadcrumb sees the actual on-disk path.
	setLastInstalledTargetDir(finalTarget)
	return nil
}

// copyMacAppWithFallback performs the `.app` copy with EACCES fallback to
// `~/Applications`. Returns the path that received the bundle.
func copyMacAppWithFallback(plan InstallPlan, src string) (string, error) {
	// An explicit --target-dir surfaces EACCES directly — the user asked
	// for that directory.
	userPickedTarget := plan.TargetDir != "" && plan.TargetDir != "/Applications"

	if err := installerMacCopyFn(src, filepath.Join(plan.TargetDir, installerMacAppName)); err != nil {
		if userPickedTarget || !isEACCES(err) {
			return "", clierr.NewFatalError(
				"desktop install (macOS): copy %s into %s failed: %s",
				src, plan.TargetDir, err.Error())
		}
		home, herr := installerMacHomeDirFn()
		if herr != nil || home == "" {
			return "", clierr.NewFatalError(
				"desktop install (macOS): cannot resolve home dir for EACCES fallback: %v", herr)
		}
		fallback := filepath.Join(home, "Applications")
		if mkErr := installerMacMkdirAllFn(fallback, 0o755); mkErr != nil {
			return "", clierr.NewFatalError(
				"desktop install (macOS): create fallback dir %s failed: %s", fallback, mkErr.Error())
		}
		if err2 := installerMacCopyFn(src, filepath.Join(fallback, installerMacAppName)); err2 != nil {
			return "", clierr.NewFatalError(
				"desktop install (macOS): copy %s into fallback %s failed: %s",
				src, fallback, err2.Error())
		}
		return fallback, nil
	}
	return plan.TargetDir, nil
}

// resolveMacMountPoint globs the mount prefix and returns the first match.
func resolveMacMountPoint() (string, error) {
	matches, err := installerMacGlobFn(installerMacMountPrefix + "*")
	if err != nil {
		return "", clierr.NewFatalError(
			"desktop install (macOS): glob mount point %s*: %s",
			installerMacMountPrefix, err.Error())
	}
	if len(matches) == 0 {
		return "", clierr.NewFatalError(
			"desktop install (macOS): no mount point matched %s* (hdiutil attach succeeded but volume not found)",
			installerMacMountPrefix)
	}
	return matches[0], nil
}

// copyDirRecursive copies the directory rooted at src to dst preserving
// modes. Pure-Go so it's testable; surfaces EACCES so the caller's
// isEACCES check can drive the fallback.
func copyDirRecursive(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("copyDirRecursive: source %s is not a directory", src)
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return fmt.Errorf("mkdir %s: %w", dst, err)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)

		switch {
		case d.IsDir():
			fi, statErr := d.Info()
			if statErr != nil {
				return statErr
			}
			return os.MkdirAll(target, fi.Mode().Perm())
		case d.Type()&os.ModeSymlink != 0:
			linkTarget, readErr := os.Readlink(path)
			if readErr != nil {
				return readErr
			}
			// Defence in depth: a malicious DMG could contain absolute
			// symlinks or `..`-traversal targets escaping the bundle root.
			// SHA-512 + same-host pin are primary; this is the belt.
			resolved := linkTarget
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(target), resolved)
			}
			resolved = filepath.Clean(resolved)
			rel, relErr := filepath.Rel(filepath.Clean(dst), resolved)
			if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("desktop install: refusing symlink %q whose target %q resolves outside the bundle root %q", target, linkTarget, dst)
			}
			// Remove a stale symlink so re-installs don't trip on EEXIST.
			_ = os.Remove(target)
			return os.Symlink(linkTarget, target)
		default:
			return copyRegularFile(path, target, d)
		}
	})
}

func copyRegularFile(src, dst string, d fs.DirEntry) error {
	in, err := os.Open(src) //nolint:gosec // src is the verified DMG mount path
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	fi, err := d.Info()
	if err != nil {
		return fmt.Errorf("stat entry %s: %w", src, err)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
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

// isEACCES reports whether err is (or wraps) a permission-denied error.
// The string fallback guards against a future Go release that stops
// surfacing the syscall via fs.ErrPermission / syscall.EACCES.
func isEACCES(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EACCES) {
		return true
	}
	return strings.Contains(err.Error(), "permission denied")
}

// lastInstalledTargetDir is a process-wide cell each per-OS install action
// writes after resolving the on-disk target so the post-install breadcrumb
// can reflect a fallback path (e.g. macOS EACCES → `~/Applications`).
// Concurrent installs in a single process are unsupported.
var lastInstalledTargetDir string

func setLastInstalledTargetDir(dir string) { lastInstalledTargetDir = dir }

func clearLastInstalledTargetDir() { lastInstalledTargetDir = "" }

// LastInstalledTargetDirForTest exposes the cell for test assertions.
func LastInstalledTargetDirForTest() string { return lastInstalledTargetDir }

// ClearLastInstalledTargetDirForTest resets the cell between tests.
func ClearLastInstalledTargetDirForTest() { clearLastInstalledTargetDir() }

// IsEACCESForTest exposes the permission-error classifier.
func IsEACCESForTest(err error) bool { return isEACCES(err) }

// runInstallActionForTestT lets RunInstallActionForTest_* call t.Helper()
// without depending on testing.T directly.
type runInstallActionForTestT interface {
	Helper()
}

// RunInstallActionForTest_Darwin invokes the unexported macOS install action.
func RunInstallActionForTest_Darwin(t runInstallActionForTestT, plan InstallPlan) error {
	t.Helper()
	return installMacOS(context.Background(), plan)
}

// RunInstallActionForTest_Linux invokes the unexported Linux install action.
func RunInstallActionForTest_Linux(t runInstallActionForTestT, plan InstallPlan) error {
	t.Helper()
	return installLinux(context.Background(), plan)
}

// RunInstallActionForTest_Windows invokes the unexported Windows install action.
func RunInstallActionForTest_Windows(t runInstallActionForTestT, plan InstallPlan) error {
	t.Helper()
	return installWindows(context.Background(), plan)
}
