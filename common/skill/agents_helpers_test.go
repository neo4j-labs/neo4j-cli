// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import "testing"

// setGOOSForTest points the per-platform branches of appSupportDir at
// `goos` for the duration of the test. currentGOOS is a process global, so
// callers must not t.Parallel().
func setGOOSForTest(t *testing.T, goos string) {
	t.Helper()
	prev := currentGOOS
	currentGOOS = goos
	t.Cleanup(func() { currentGOOS = prev })
}
