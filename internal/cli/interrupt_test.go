// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
)

func TestInterruptExit(t *testing.T) {
	failure := errors.New("request failed: context canceled")

	t.Run("a command that failed after SIGINT exits 130", func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(interruptError{sig: os.Interrupt})

		code, ok := interruptExit(ctx, failure)

		assert.True(t, ok)
		assert.Equal(t, 130, code)
	})

	t.Run("SIGTERM exits 143", func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(interruptError{sig: syscall.SIGTERM})

		code, ok := interruptExit(ctx, failure)

		assert.True(t, ok)
		assert.Equal(t, 143, code)
	})

	t.Run("a command that still succeeded is not an interrupt", func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(interruptError{sig: os.Interrupt})

		_, ok := interruptExit(ctx, nil)

		assert.False(t, ok)
	})

	t.Run("an ordinary failure with no signal is rendered as usual", func(t *testing.T) {
		_, ok := interruptExit(context.Background(), failure)
		assert.False(t, ok)
	})

	t.Run("a context cancelled for another reason is not an interrupt", func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(errors.New("deadline"))

		_, ok := interruptExit(ctx, failure)

		assert.False(t, ok)
	})
}

func TestGuarded_RecoveredPanicIsAFailure(t *testing.T) {
	args := []string{"aura", "instance", "list", "--client-secret", "s3cret-value"}

	t.Run("an unexpected panic exits 1 with the diagnostic on stderr", func(t *testing.T) {
		var out, errOut bytes.Buffer

		code := guarded(IO{Out: &out, Err: &errOut}, args, func() int { panic("boom") })

		assert.Equal(t, 1, code, "a crash must never look like success")
		assert.Empty(t, out.String(), "a crash report is not program output")
		assert.Contains(t, errOut.String(), "Unexpected error running CLI")
		assert.NotContains(t, errOut.String(), "s3cret-value", "secret flag values stay redacted")
	})

	t.Run("a plain error panic exits 1 and surfaces its text", func(t *testing.T) {
		var out, errOut bytes.Buffer

		code := guarded(IO{Out: &out, Err: &errOut}, args, func() int { panic(fmt.Errorf("handler exploded")) })

		assert.Equal(t, 1, code)
		assert.Contains(t, errOut.String(), "handler exploded")
	})

	t.Run("a panic carrying a typed error is rendered with that error's exit code", func(t *testing.T) {
		var out, errOut bytes.Buffer

		code := guarded(IO{Out: &out, Err: &errOut}, args, func() int {
			panic(clierr.NewUpstreamError("the auth endpoint is down"))
		})

		assert.Equal(t, 8, code)
		assert.Contains(t, out.String()+errOut.String(), "the auth endpoint is down")
		assert.NotContains(t, errOut.String(), "Unexpected error running CLI", "a typed error is not a crash")
	})

	t.Run("no panic passes the exit code through", func(t *testing.T) {
		assert.Equal(t, 7, guarded(IO{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}, args, func() int { return 7 }))
	})
}

func TestNotifyInterrupt_StopReleasesTheHandlerWithoutCancellingOnItsOwnSignal(t *testing.T) {
	var buf bytes.Buffer
	ctx, stop := notifyInterrupt(context.Background(), &buf)

	require.NoError(t, ctx.Err())
	stop()

	assert.Error(t, ctx.Err(), "stop releases the context")
	assert.False(t, strings.Contains(buf.String(), "interrupted"), "no signal arrived, so nothing is narrated")
}
