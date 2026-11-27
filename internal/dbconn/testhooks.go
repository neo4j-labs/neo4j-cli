// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbconn

import (
	"testing"
	"time"
)

// SetBoltProbeForTest replaces the authenticated readiness probe. Tests must
// not run in parallel while this process-global seam is replaced.
func SetBoltProbeForTest(t *testing.T, probe BoltProber) {
	t.Helper()
	previous := probeBoltFn
	probeBoltFn = probe
	t.Cleanup(func() { probeBoltFn = previous })
}

// SetBoltPollIntervalForTest shortens the readiness polling interval.
func SetBoltPollIntervalForTest(t *testing.T, interval time.Duration) {
	t.Helper()
	previous := boltPollInterval
	boltPollInterval = interval
	t.Cleanup(func() { boltPollInterval = previous })
}
