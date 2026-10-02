// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build !windows

package cli

import (
	"bytes"
	"context"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncBuffer lets the signal goroutine write while the test reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestNotifyInterrupt_SIGINTCancelsTheContextAndIsNarrated(t *testing.T) {
	var w syncBuffer
	ctx, stop := notifyInterrupt(context.Background(), &w)
	defer stop()

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGINT))

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("context was not cancelled by SIGINT")
	}

	code, ok := interruptExit(ctx, assert.AnError)
	assert.True(t, ok)
	assert.Equal(t, 130, code)
	assert.Contains(t, w.String(), "press Ctrl-C again to force quit", "the user is told how to force quit")
}
