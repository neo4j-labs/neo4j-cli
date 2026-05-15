// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"errors"
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
