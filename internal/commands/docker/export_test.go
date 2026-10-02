// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"io"
	"net"
	"testing"
)

// SetDebugWriterForTest overrides the package-level debug seam (debugW) for the
// duration of the test, restoring the previous value via t.Cleanup. Lets tests
// capture --debug diagnostics instead of writing to os.Stderr.
func SetDebugWriterForTest(t *testing.T, w io.Writer) {
	t.Helper()
	prev := debugW
	debugW = w
	t.Cleanup(func() { debugW = prev })
}

// SetRandSourceForTest overrides the package-level randSource seam for the
// duration of the test, restoring the previous value via t.Cleanup.
func SetRandSourceForTest(t *testing.T, r io.Reader) {
	t.Helper()
	prev := randSource
	randSource = r
	t.Cleanup(func() { randSource = prev })
}

// SetListenerFactoryForTest overrides the package-level listenerFactory seam
// for the duration of the test so port pre-flight checks are deterministic.
func SetListenerFactoryForTest(t *testing.T, fn func(int) (net.Listener, error)) {
	t.Helper()
	prev := listenerFactory
	listenerFactory = fn
	t.Cleanup(func() { listenerFactory = prev })
}
