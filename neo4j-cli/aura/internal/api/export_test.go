// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"io"
	"testing"
	"time"
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

// SetHTTPClientTimeoutForTest overrides the package-level httpClientTimeout
// for the duration of the test. Restores the previous value via t.Cleanup.
// Lets timeout-fires assertions run in milliseconds instead of 60 seconds.
func SetHTTPClientTimeoutForTest(t *testing.T, d time.Duration) {
	t.Helper()
	prev := httpClientTimeout
	httpClientTimeout = d
	t.Cleanup(func() { httpClientTimeout = prev })
}
