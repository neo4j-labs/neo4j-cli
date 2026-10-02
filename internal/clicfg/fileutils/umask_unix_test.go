// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build !windows

package fileutils

import (
	"syscall"
	"testing"
)

// umaskZero clamps the process umask to 0 for the duration of the test so
// directory/file mode assertions reflect the bits actually requested. Returns
// a restorer that re-applies the previous umask; t.Cleanup also handles the
// restore so callers can simply `defer umaskZero(t)()`.
func umaskZero(t *testing.T) func() {
	t.Helper()
	prev := syscall.Umask(0)
	restore := func() {
		syscall.Umask(prev)
	}
	t.Cleanup(restore)
	return restore
}
