// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"testing"
	"time"
)

// SetHTTPClientTimeoutForTest overrides the package-level httpClientTimeout
// for the duration of the test. Restores the previous value via t.Cleanup.
// Lets timeout-fires assertions run in milliseconds instead of 60 seconds.
func SetHTTPClientTimeoutForTest(t *testing.T, d time.Duration) {
	t.Helper()
	prev := httpClientTimeout
	httpClientTimeout = d
	t.Cleanup(func() { httpClientTimeout = prev })
}

// TokenIdentity exposes the unexported identity hash for env-mode JWT cache tests.
func TokenIdentity(clientID, clientSecret, authURL string) string {
	return tokenIdentity(clientID, clientSecret, authURL)
}

// TokenCachePath exposes the unexported cache-file path for env-mode JWT cache tests.
func TokenCachePath(identity string) string {
	return tokenCachePath(identity)
}
