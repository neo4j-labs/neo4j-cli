// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"io"
	"testing"
)

// This file exposes the package's test seams to tests in OTHER packages,
// mirroring the docker engine's testhooks.go. Each setter restores the
// previous value via t.Cleanup. The seams are process-global, so callers
// must not use t.Parallel() around them.

// SetDebugWriterForTest redirects --debug diagnostics (debugW) away from
// os.Stderr.
func SetDebugWriterForTest(t *testing.T, w io.Writer) {
	t.Helper()
	prev := debugW
	debugW = w
	t.Cleanup(func() { debugW = prev })
}

// SetKubectlLookPathForTest replaces the kubectl PATH lookup.
func SetKubectlLookPathForTest(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	prev := kubectlLookPathFn
	kubectlLookPathFn = fn
	t.Cleanup(func() { kubectlLookPathFn = prev })
}

// SetHelmLookPathForTest replaces the helm PATH lookup.
func SetHelmLookPathForTest(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	prev := helmLookPathFn
	helmLookPathFn = fn
	t.Cleanup(func() { helmLookPathFn = prev })
}

// SetValidateBinaryForTest replaces the binary-validation seam. Tests whose
// lookPath stub returns a fake path use it to skip the real filesystem rules.
func SetValidateBinaryForTest(t *testing.T, fn func(name, path string) error) {
	t.Helper()
	prev := validateBinaryFn
	validateBinaryFn = fn
	t.Cleanup(func() { validateBinaryFn = prev })
}

// SetRunFnForTest replaces the process-execution seam. Engine tests use it
// to capture the argv the engine builds and return canned output without
// spawning a process.
func SetRunFnForTest(t *testing.T, fn func(ctx context.Context, path string, args []string) runResult) {
	t.Helper()
	prev := runFn
	runFn = fn
	t.Cleanup(func() { runFn = prev })
}
