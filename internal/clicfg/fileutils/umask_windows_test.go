// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build windows

package fileutils

import "testing"

// umaskZero is a no-op on Windows; the POSIX umask doesn't apply.
func umaskZero(t *testing.T) func() {
	t.Helper()
	return func() {}
}
