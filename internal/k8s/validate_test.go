// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFakeBinary creates an executable file with the standard 0755 mode.
func writeFakeBinary(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755))
	return path
}

func TestValidateBinary_RejectsRelativePath(t *testing.T) {
	err := validateBinary("kubectl", "bin/kubectl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not absolute")
}

func TestValidateBinary_RejectsMissingFile(t *testing.T) {
	err := validateBinary("kubectl", filepath.Join(t.TempDir(), "kubectl"))
	require.Error(t, err)
}

func TestValidateBinary_RejectsDirectory(t *testing.T) {
	err := validateBinary("kubectl", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")
}

func TestValidateBinary_AcceptsRegularExecutable(t *testing.T) {
	path := writeFakeBinary(t, t.TempDir(), "kubectl")
	assert.NoError(t, validateBinary("kubectl", path))
}

func TestValidateBinary_ResolvesSymlinkToTrustedTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privilege on Windows")
	}
	dir := t.TempDir()
	target := writeFakeBinary(t, dir, "kubectl-real")
	link := filepath.Join(dir, "kubectl")
	require.NoError(t, os.Symlink(target, link))
	assert.NoError(t, validateBinary("kubectl", link))
}

func TestValidateBinary_RejectsSymlinkToUntrustedTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privilege on Windows")
	}
	dir := t.TempDir()
	target := writeFakeBinary(t, dir, "kubectl-real")
	require.NoError(t, os.Chmod(target, 0o777))
	link := filepath.Join(dir, "kubectl")
	require.NoError(t, os.Symlink(target, link))
	err := validateBinary("kubectl", link)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writable by other users")
}

func TestValidateBinary_RejectsNonExecutableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no executable permission bit")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "kubectl")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o644))
	err := validateBinary("kubectl", path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not executable")
}

func TestValidateBinary_RejectsWorldWritableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no unix permission bits")
	}
	path := writeFakeBinary(t, t.TempDir(), "kubectl")
	require.NoError(t, os.Chmod(path, 0o777))
	err := validateBinary("kubectl", path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writable by other users")
}

func TestValidateBinary_RejectsWorldWritableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no unix permission bits")
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "bin")
	require.NoError(t, os.Mkdir(dir, 0o777))
	require.NoError(t, os.Chmod(dir, 0o777))
	path := writeFakeBinary(t, dir, "kubectl")
	err := validateBinary("kubectl", path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writable by other users")
}

func TestValidateBinary_AcceptsRootOwnedSystemBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix ownership rules do not apply on Windows")
	}
	if os.Geteuid() != 0 {
		// /bin/sh belongs to root on a standard unix install.
		assert.NoError(t, validateBinary("sh", "/bin/sh"))
	}
}

func TestResolveKubectl_ValidationFailurePropagates(t *testing.T) {
	SetKubectlLookPathForTest(t, func(string) (string, error) { return "/evil/kubectl", nil })
	SetValidateBinaryForTest(t, func(_, path string) error {
		return untrustedBinaryErr("kubectl", path, "planted")
	})
	_, err := resolveKubectl()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to run kubectl")
}

func TestResolveHelm_ValidationFailurePropagates(t *testing.T) {
	SetHelmLookPathForTest(t, func(string) (string, error) { return "/evil/helm", nil })
	SetValidateBinaryForTest(t, func(_, path string) error {
		return untrustedBinaryErr("helm", path, "planted")
	})
	_, err := resolveHelm()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to run helm")
}
