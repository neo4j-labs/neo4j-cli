// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"testing"
	"time"
)

// This file exposes the package's test seams to tests in OTHER packages (the
// feedback commands drive the engine through its exported API). Each setter
// restores the previous value via t.Cleanup. The seams are process-global, so
// callers must not use t.Parallel().

// SetMixpanelTokenForTest gives the build a Mixpanel project token (or removes
// it with ""), so Send talks to a non-loopback destination or refuses to.
func SetMixpanelTokenForTest(t *testing.T, token string) {
	t.Helper()
	prev := mixpanelToken
	mixpanelToken = token
	t.Cleanup(func() { mixpanelToken = prev })
}

// SetSendTimeoutForTest shortens the total deadline of one submission.
func SetSendTimeoutForTest(t *testing.T, d time.Duration) {
	t.Helper()
	prev := sendTimeout
	sendTimeout = d
	t.Cleanup(func() { sendTimeout = prev })
}

// MixpanelBase returns the compiled-in Mixpanel base URL.
func MixpanelBase() string { return mixpanelBase }
