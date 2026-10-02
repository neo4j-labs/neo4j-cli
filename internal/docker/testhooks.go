// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

// This file exposes the package's test seams to tests in OTHER packages (the
// docker commands drive the engine through its exported API). Each setter
// restores the previous value via t.Cleanup. The seams are process-global, so
// callers must not use t.Parallel().

// SetDebugWriterForTest redirects --debug diagnostics (debugW) away from
// os.Stderr.
func SetDebugWriterForTest(t *testing.T, w io.Writer) {
	t.Helper()
	prev := debugW
	debugW = w
	t.Cleanup(func() { debugW = prev })
}

// SetRandSourceForTest replaces the entropy source used to mint container
// passwords.
func SetRandSourceForTest(t *testing.T, r io.Reader) {
	t.Helper()
	prev := randSource
	randSource = r
	t.Cleanup(func() { randSource = prev })
}

// SetListenerFactoryForTest replaces the host-port probe so port pre-flight
// checks are deterministic and never touch the network.
func SetListenerFactoryForTest(t *testing.T, fn func(int) (net.Listener, error)) {
	t.Helper()
	prev := listenerFactory
	listenerFactory = fn
	t.Cleanup(func() { listenerFactory = prev })
}

// SetStopStartForTest replaces the STOP/START DATABASE executor used by the
// existing-container loader and PushToAura.
func SetStopStartForTest(t *testing.T, fn func(ctx context.Context, uri, user, pass, statement string) error) {
	t.Helper()
	prev := stopStartFn
	stopStartFn = fn
	t.Cleanup(func() { stopStartFn = prev })
}

// SetWaitForBoltForTest replaces the Bolt readiness probe the new-container
// loader uses for --wait.
func SetWaitForBoltForTest(t *testing.T, fn func(ctx context.Context, uri, user, pass string, timeout time.Duration) error) {
	t.Helper()
	prev := waitForBoltFn
	waitForBoltFn = fn
	t.Cleanup(func() { waitForBoltFn = prev })
}
