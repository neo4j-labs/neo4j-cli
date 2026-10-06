// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import "testing"

// SetNowUnixForTest swaps the package-level nowUnix function for the
// duration of the test. Restores the previous value via t.Cleanup.
func SetNowUnixForTest(t *testing.T, fn func() int64) {
	t.Helper()
	prev := nowUnix
	nowUnix = fn
	t.Cleanup(func() { nowUnix = prev })
}

// SetKeyringProviderForTest replaces the package-level keyring provider for
// the duration of the test. Restores the previous provider via t.Cleanup.
// Use this seam together with go-keyring's MockInit() to avoid touching the
// real OS keyring in unit tests.
func SetKeyringProviderForTest(t *testing.T, p KeyringProvider) {
	t.Helper()
	prev := defaultKeyring
	defaultKeyring = p
	t.Cleanup(func() { defaultKeyring = prev })
}
