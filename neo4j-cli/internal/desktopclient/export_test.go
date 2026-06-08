// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"io"
	"net/http"
	"testing"
	"time"
)

// Test-only re-exports of the emit helpers so package tests (debug_test.go) can
// drive them directly. They also keep the helpers rooted in the usage graph
// until the production call sites in client.go/discovery.go are wired up.
func DebugRequestForTest(method, url string, header http.Header, body []byte) {
	debugRequest(method, url, header, body)
}

func DebugResponseForTest(statusCode int, header http.Header, body []byte, elapsed time.Duration) {
	debugResponse(statusCode, header, body, elapsed)
}

func DebugInfoForTest(format string, args ...any) {
	debugInfo(format, args...)
}

// SetDebugWriterForTest overrides the package-level debug seam (debugW) for the
// duration of the test, restoring the previous value via t.Cleanup. Lets tests
// capture --debug diagnostics instead of writing to os.Stderr.
func SetDebugWriterForTest(t *testing.T, w io.Writer) {
	t.Helper()
	prev := debugW
	debugW = w
	t.Cleanup(func() { debugW = prev })
}

// SetDebugForTest toggles the package-level debugEnabled gate for the duration
// of the test, restoring the previous value via t.Cleanup.
func SetDebugForTest(t *testing.T, enabled bool) {
	t.Helper()
	prev := debugEnabled
	debugEnabled = enabled
	t.Cleanup(func() { debugEnabled = prev })
}
