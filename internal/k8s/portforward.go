// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/tee"
)

// PortForwardOptions describes a foreground Bolt connection. OnReady runs
// once the child has started and must return when its context ends. If it
// returns an error, the client stops the child.
type PortForwardOptions struct {
	Pod        string
	RemotePort int32
	LocalPort  int
	OnReady    func(context.Context) error
}

type process interface {
	Wait() error
}

// startProcessFn owns process creation. Tests replace it to exercise the
// lifecycle without running kubectl or opening a connection to a cluster.
var startProcessFn = startProcess

func startProcess(ctx context.Context, path string, args []string, stderr io.Writer) (process, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = stderr
	return cmd, cmd.Start()
}

// PortForward implements Client. The child gets its own context, so a failed
// readiness check can stop it without cancelling the whole command. Stdin
// stays unattached, and the child's stderr goes only to the redacted debug
// trace, never straight to the user.
func (c *execClient) PortForward(ctx context.Context, opts PortForwardOptions) error {
	path, err := resolveKubectl()
	if err != nil {
		return err
	}
	args := kubectlArgs(c.settings, "port-forward", "pod/"+opts.Pod,
		"--address", "127.0.0.1", fmt.Sprintf("%d:%d", opts.LocalPort, opts.RemotePort))
	if c.settings.Debug {
		debugInvocation("kubectl", args)
	}
	start := time.Now()
	// Keep the failure reason even without --debug, but bound a long-lived
	// child's diagnostics. Only the redacted debug trace displays this data.
	var stderr tee.LimitedBuffer
	trace := func(err error) runResult {
		res := runResult{err: err, stderr: string(stderr.Bytes())}
		if c.settings.Debug {
			debugResult("kubectl", res, time.Since(start))
		}
		return res
	}
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	child, err := startProcessFn(childCtx, path, args, &stderr)
	if err != nil {
		return classifyRunError(ctx, "kubectl", args, trace(err))
	}

	// The child is killed when childCtx ends, so a cancelled command always
	// reaches exited. A child that exits on its own cancels childCtx, which
	// stops the readiness check.
	exited := make(chan error, 1)
	go func() {
		exited <- child.Wait()
		cancel()
	}()
	ready := make(chan error, 1)
	go func() {
		ready <- opts.OnReady(childCtx)
	}()
	finish := func(err error) error {
		res := trace(err)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err == nil {
			return clierr.NewUpstreamError("kubectl port-forward stopped before the command context ended")
		}
		return classifyRunError(ctx, "kubectl", args, res)
	}

	select {
	case err := <-exited:
		<-ready
		return finish(err)
	case readyErr := <-ready:
		if readyErr == nil {
			// Connected. Keep forwarding until Ctrl-C or the child fails.
			return finish(<-exited)
		}
		cancel()
		waitErr := <-exited
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		// When the child exits first, the readiness check sees a cancelled
		// context. The child's own failure is the one worth reporting.
		if errors.Is(readyErr, context.Canceled) {
			return finish(waitErr)
		}
		trace(waitErr)
		return readyErr
	}
}
