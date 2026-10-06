// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withTestProbe swaps probeBoltFn and pollInterval for the duration of the
// test and restores them on cleanup. Tests use a sub-millisecond pollInterval
// so timeout scenarios complete in <100ms even on loaded CI.
func withTestProbe(t *testing.T, interval time.Duration, prober boltProber) {
	t.Helper()
	origFn := probeBoltFn
	origInterval := pollInterval
	probeBoltFn = prober
	pollInterval = interval
	t.Cleanup(func() {
		probeBoltFn = origFn
		pollInterval = origInterval
	})
}

// TestWaitForBolt_ImmediateSuccess covers the happy path where the first
// probe already finds Bolt accepting sessions. WaitForBolt must return nil
// without ever sleeping.
func TestWaitForBolt_ImmediateSuccess(t *testing.T) {
	var calls int32
	withTestProbe(t, 10*time.Millisecond, func(_ context.Context, _, _, _ string) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	err := WaitForBolt(ctx, "neo4j://localhost:7687", "neo4j", "password", 500*time.Millisecond)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "expected exactly one probe call")
	assert.Less(t, elapsed, 100*time.Millisecond, "immediate success must not wait for a tick")
}

// TestWaitForBolt_SuccessAfterRetries covers the case where Bolt rejects the
// first N handshakes (container still booting) and accepts on the (N+1)th.
// WaitForBolt must keep polling and return nil once the prober succeeds.
func TestWaitForBolt_SuccessAfterRetries(t *testing.T) {
	const failuresBeforeSuccess = 3
	var calls int32
	withTestProbe(t, 5*time.Millisecond, func(_ context.Context, _, _, _ string) error {
		n := atomic.AddInt32(&calls, 1)
		if n <= failuresBeforeSuccess {
			return errors.New("connection refused")
		}
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := WaitForBolt(ctx, "neo4j://localhost:7687", "neo4j", "password", 500*time.Millisecond)
	require.NoError(t, err)

	got := atomic.LoadInt32(&calls)
	assert.Equal(t, int32(failuresBeforeSuccess+1), got,
		"expected exactly failuresBeforeSuccess+1 probe calls (got %d)", got)
}

// TestWaitForBolt_Timeout covers the case where the prober never succeeds.
// WaitForBolt must return a clierr.UsageError naming the timeout duration and
// pointing at `docker logs <name>`.
func TestWaitForBolt_Timeout(t *testing.T) {
	withTestProbe(t, 5*time.Millisecond, func(_ context.Context, _, _, _ string) error {
		return errors.New("connection refused")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	timeout := 50 * time.Millisecond
	start := time.Now()
	err := WaitForBolt(ctx, "neo4j://localhost:7687", "neo4j", "password", timeout)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Bolt did not become ready")
	assert.Contains(t, err.Error(), timeout.String())
	assert.Contains(t, err.Error(), "docker logs <name>")
	assert.GreaterOrEqual(t, elapsed, timeout, "WaitForBolt should not return before the timeout")
	assert.Less(t, elapsed, 500*time.Millisecond, "timeout path took implausibly long")
}

// withTestTCPDialer swaps tcpDialFn and pollInterval for the duration of the
// test and restores them on cleanup. Tests use a sub-millisecond pollInterval
// so timeout scenarios complete in <100ms even on loaded CI.
func withTestTCPDialer(t *testing.T, interval time.Duration, dialer tcpDialer) {
	t.Helper()
	origFn := tcpDialFn
	origInterval := pollInterval
	tcpDialFn = dialer
	pollInterval = interval
	t.Cleanup(func() {
		tcpDialFn = origFn
		pollInterval = origInterval
	})
}

// fakeConn returns a *net.TCPConn-shaped pair via net.Pipe; only Close is
// exercised by WaitForBoltTCP so the half-duplex pipe is more than enough.
func fakeConn() net.Conn {
	c, _ := net.Pipe()
	return c
}

// TestWaitForBoltTCP_ImmediateSuccess covers the happy path: the first dial
// already connects. WaitForBoltTCP must return nil without sleeping.
func TestWaitForBoltTCP_ImmediateSuccess(t *testing.T) {
	var calls int32
	withTestTCPDialer(t, 10*time.Millisecond, func(network, address string, _ time.Duration) (net.Conn, error) {
		atomic.AddInt32(&calls, 1)
		assert.Equal(t, "tcp", network)
		assert.Equal(t, "localhost:7687", address)
		return fakeConn(), nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	err := WaitForBoltTCP(ctx, "localhost", 7687, 500*time.Millisecond)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "expected exactly one dial call")
	assert.Less(t, elapsed, 100*time.Millisecond, "immediate success must not wait for a tick")
}

// TestWaitForBoltTCP_SuccessAfterRetries covers the case where Dial fails for
// the first N attempts (container booting, port not bound yet) and accepts on
// the (N+1)th. The polling loop must keep dialing until it succeeds.
func TestWaitForBoltTCP_SuccessAfterRetries(t *testing.T) {
	const failuresBeforeSuccess = 2
	var calls int32
	withTestTCPDialer(t, 5*time.Millisecond, func(_, _ string, _ time.Duration) (net.Conn, error) {
		n := atomic.AddInt32(&calls, 1)
		if n <= failuresBeforeSuccess {
			return nil, errors.New("connection refused")
		}
		return fakeConn(), nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := WaitForBoltTCP(ctx, "localhost", 7687, 500*time.Millisecond)
	require.NoError(t, err)

	got := atomic.LoadInt32(&calls)
	assert.Equal(t, int32(failuresBeforeSuccess+1), got,
		"expected exactly failuresBeforeSuccess+1 dial calls (got %d)", got)
}

// TestWaitForBoltTCP_Timeout covers the case where the dialer never succeeds.
// WaitForBoltTCP must return a clierr.UsageError naming the timeout duration,
// the port number, and pointing at `docker logs <name>`.
func TestWaitForBoltTCP_Timeout(t *testing.T) {
	withTestTCPDialer(t, 5*time.Millisecond, func(_, _ string, _ time.Duration) (net.Conn, error) {
		return nil, errors.New("connection refused")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	timeout := 50 * time.Millisecond
	start := time.Now()
	err := WaitForBoltTCP(ctx, "localhost", 7687, timeout)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "TCP port 7687 did not open")
	assert.Contains(t, err.Error(), timeout.String())
	assert.Contains(t, err.Error(), "docker logs <name>")
	assert.GreaterOrEqual(t, elapsed, timeout, "WaitForBoltTCP should not return before the timeout")
	assert.Less(t, elapsed, 500*time.Millisecond, "timeout path took implausibly long")
}

// TestWaitForBoltTCP_ContextCancelled verifies that cancelling the outer ctx
// while WaitForBoltTCP is polling causes the probe to bail out promptly
// (via the same probeCtx-driven exit as the timeout path).
func TestWaitForBoltTCP_ContextCancelled(t *testing.T) {
	withTestTCPDialer(t, 5*time.Millisecond, func(_, _ string, _ time.Duration) (net.Conn, error) {
		return nil, errors.New("connection refused")
	})

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel shortly after the first dial fails so the ticker branch sees
	// the cancelled probeCtx and returns.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	timeout := 5 * time.Second
	start := time.Now()
	err := WaitForBoltTCP(ctx, "localhost", 7687, timeout)
	elapsed := time.Since(start)

	require.Error(t, err)
	// Either the cancellation propagated via probeCtx.Done() — surface is
	// the same UsageError shape — or the outer context's cancellation
	// landed first; in both cases the elapsed time must be well under
	// `timeout`.
	assert.Less(t, elapsed, 1*time.Second, "ctx cancellation should bail out before the 5s timeout")
}
