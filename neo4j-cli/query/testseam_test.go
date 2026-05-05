// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"io"
	"os"
	"testing"
)

// TestMain seeds the package-level stdoutIsTerminal seam to return true for
// the entire query test suite. Production resolveOutput auto-detects: TTY →
// table, non-TTY → JSON. Existing renderRows / renderSchema tests assert on
// table output without ever attaching a real *os.File, so without this seed
// the new auto-detect default would flip them to JSON and break unrelated
// assertions. Individual tests that want to exercise the non-TTY branch
// override the seam locally via t.Cleanup. Mirrors the stdinIsTTY pattern at
// run.go.
func TestMain(m *testing.M) {
	stdoutIsTerminal = func(io.Writer) bool { return true }
	os.Exit(m.Run())
}
