// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"io"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
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

// SetTokenCacheDirForTest redirects the on-disk token cache to dir for the
// duration of the test, restoring the previous resolver via t.Cleanup.
func SetTokenCacheDirForTest(t *testing.T, dir string) {
	t.Helper()
	prev := cacheDirFn
	cacheDirFn = func() string { return dir }
	t.Cleanup(func() { cacheDirFn = prev })
}

// SetMintTokenForTest replaces the OAuth mint seam, letting tests count mints
// and supply a canned Grant without HTTP. Restores the previous fn via
// t.Cleanup. The supplied fn receives no args and returns the Grant to hand back.
func SetMintTokenForTest(t *testing.T, fn func() (Grant, error)) {
	t.Helper()
	prev := mintToken
	mintToken = func(_ *credentials.AuraCredential, _ *clicfg.Config) (Grant, error) {
		return fn()
	}
	t.Cleanup(func() { mintToken = prev })
}

// TokenCachePathForTest exposes the cache path resolver for assertions.
func TokenCachePathForTest(clientID, clientSecret, authURL string) string {
	_, short := tokenCacheHash(clientID, clientSecret, authURL)
	return tokenCachePath(short)
}

// TokenCacheHashForTest exposes the identity hash for assertions.
func TokenCacheHashForTest(clientID, clientSecret, authURL string) string {
	full, _ := tokenCacheHash(clientID, clientSecret, authURL)
	return full
}

// VersionPathForTest exposes the version->wire-path mapping for assertions.
func VersionPathForTest(v AuraApiVersion) string {
	return getVersionPath(v)
}
