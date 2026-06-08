// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"io"
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
