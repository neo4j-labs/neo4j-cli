// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package update

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestMain installs a guard on the package-level httpDoFn seam that blocks
// requests to non-localhost hosts. Any test that reaches real GitHub without
// stubbing httpDoFn (via withHttpDo) or pointing apiBaseURL at a test server
// (via withApiBaseURL) fails loudly instead of silently hitting
// api.github.com.
//
// Tests that use withHttpDo save the guard and restore it via t.Cleanup, so
// per-test stubbing still wins. Tests that use withApiBaseURL point at an
// httptest server (localhost / 127.0.0.1), so the guard passes through to
// the real client.
func TestMain(m *testing.M) {
	orig := httpDoFn
	httpDoFn = guardHTTPDo(orig)
	os.Exit(m.Run())
}

// guardHTTPDo wraps an httpDoFn to reject requests to non-localhost hosts.
// Requests to localhost / 127.0.0.1 / ::1 are forwarded to the wrapped fn
// so existing httptest-based tests continue to work.
func guardHTTPDo(next func(*http.Request) (*http.Response, error)) func(*http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		if req.URL != nil {
			host := req.URL.Hostname()
			if host != "" && !isLocalhost(host) {
				return nil, fmt.Errorf(
					"httpDoFn guard: unexpected network call to %s; "+
						"stub httpDoFn via withHttpDo or use withApiBaseURL to point at a test server",
					req.URL,
				)
			}
		}
		return next(req)
	}
}

// isLocalhost reports whether host is a loopback address.
func isLocalhost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasPrefix(host, "127.")
}
