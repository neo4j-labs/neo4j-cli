// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// interruptError is the cancellation cause recorded when the process receives
// SIGINT or SIGTERM.
type interruptError struct {
	sig os.Signal
}

func (e interruptError) Error() string { return "interrupted by " + e.sig.String() }

// exitCode follows the shell convention for a process ended by a signal:
// 128 plus the signal number (130 for Ctrl-C, 143 for SIGTERM).
func (e interruptError) exitCode() int {
	if s, ok := e.sig.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return 130
}

// notifyInterrupt returns a context that is cancelled, with an interruptError
// as its cause, when SIGINT or SIGTERM arrives. The first signal asks the
// running operation to stop and says how to force quit; the handler is then
// removed, so a second signal ends the process immediately. That keeps a command
// that does not yet watch its context from becoming impossible to interrupt.
//
// The returned stop function releases the handler and must be called.
func notifyInterrupt(parent context.Context, w io.Writer) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		select {
		case sig := <-ch:
			fmt.Fprintln(w, "\ninterrupted: stopping the current operation (press Ctrl-C again to force quit)") //nolint:errcheck // best-effort narration
			signal.Stop(ch)
			cancel(interruptError{sig: sig})
		case <-done:
		}
	}()

	return ctx, func() {
		signal.Stop(ch)
		close(done)
		cancel(nil)
	}
}

// interruptExit reports the exit code for a command that failed because the
// process was interrupted. It returns false when err is nil or no signal was
// received, so ordinary failures are rendered as usual.
func interruptExit(ctx context.Context, err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	var ie interruptError
	if errors.As(context.Cause(ctx), &ie) {
		return ie.exitCode(), true
	}
	return 0, false
}
