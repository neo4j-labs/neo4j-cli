// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbconn

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitForBolt_ImmediateSuccess(t *testing.T) {
	SetBoltPollIntervalForTest(t, time.Hour)
	calls := 0
	SetBoltProbeForTest(t, func(ctx context.Context, uri, username, password string) error {
		calls++
		assert.Equal(t, "bolt+ssc://localhost:7688", uri)
		assert.Equal(t, "neo4j", username)
		assert.Equal(t, "test-password", password)
		assert.NoError(t, ctx.Err())
		return nil
	})
	require.NoError(t, WaitForBolt(t.Context(), "bolt+ssc://localhost:7688", "neo4j", "test-password", time.Second))
	assert.Equal(t, 1, calls)
}

func TestWaitForBolt_SuccessAfterRetries(t *testing.T) {
	SetBoltPollIntervalForTest(t, time.Millisecond)
	calls := 0
	SetBoltProbeForTest(t, func(context.Context, string, string, string) error {
		calls++
		if calls < 4 {
			return errors.New("connection refused")
		}
		return nil
	})
	require.NoError(t, WaitForBolt(t.Context(), "bolt://localhost:7687", "neo4j", "test-password", time.Second))
	assert.Equal(t, 4, calls)
}

func TestWaitForBolt_TimeoutIsNeutral(t *testing.T) {
	SetBoltPollIntervalForTest(t, time.Millisecond)
	SetBoltProbeForTest(t, func(context.Context, string, string, string) error {
		return errors.New("connection refused")
	})
	err := WaitForBolt(t.Context(), "bolt://localhost:7687", "neo4j", "test-password", 10*time.Millisecond)
	var timeout *BoltTimeoutError
	require.ErrorAs(t, err, &timeout)
	assert.Equal(t, 10*time.Millisecond, timeout.Timeout)
	assert.NotContains(t, err.Error(), "docker")
	assert.NotContains(t, err.Error(), "connection refused")
}

func TestWaitForBolt_PreservesContextCancellation(t *testing.T) {
	SetBoltPollIntervalForTest(t, time.Millisecond)
	SetBoltProbeForTest(t, func(ctx context.Context, _, _, _ string) error { return ctx.Err() })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := WaitForBolt(ctx, "bolt://localhost:7687", "neo4j", "test-password", time.Second)
	assert.ErrorIs(t, err, context.Canceled)
}
