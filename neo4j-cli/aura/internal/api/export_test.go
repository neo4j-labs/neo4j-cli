// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"testing"
	"time"

	"github.com/spf13/afero"
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

// LoadTokenCache and SaveTokenCache expose the env-mode JWT cache helpers to
// same-package tests. They also mark the (not-yet-wired) cache chain as used so
// the unused linter stays green until getToken wiring lands.
func LoadTokenCache(fs afero.Fs, clientID, clientSecret, authURL string) (string, int64, bool) {
	return loadTokenCache(fs, clientID, clientSecret, authURL)
}

func SaveTokenCache(fs afero.Fs, clientID, clientSecret, authURL, accessToken string, tokenExpiry int64) error {
	return saveTokenCache(fs, clientID, clientSecret, authURL, accessToken, tokenExpiry)
}
