// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"net/http"
	"time"
)

// Test-only re-exports of the emit helpers so package tests (debug_test.go) can
// drive them directly (production call sites live in client.go/discovery.go).
func DebugRequestForTest(method, url string, header http.Header, body []byte) {
	debugRequest(method, url, header, body)
}

func DebugResponseForTest(statusCode int, header http.Header, body []byte, elapsed time.Duration) {
	debugResponse(statusCode, header, body, elapsed)
}

func DebugInfoForTest(format string, args ...any) {
	debugInfo(format, args...)
}
