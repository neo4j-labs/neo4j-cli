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

// PortForwardOptions describes a foreground Bolt connection. OnReady must
// return when its context ends. If it fails, the client stops the child.
type PortForwardOptions struct {
	Pod        string
	RemotePort int32
	LocalPort  int
	OnReady    func(context.Context, int) error
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

// PortForward implements Client. The child receives its own context so a
// failed readiness probe also stops it. Stdin stays unattached and child
// stderr goes through the redacted debug trace, never directly to the user.
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
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	child, err := startProcessFn(childCtx, path, args, &stderr)
	if err != nil {
		res := runResult{err: err, stderr: string(stderr.Bytes())}
		if c.settings.Debug {
			debugResult("kubectl", res, time.Since(start))
		}
		return classifyRunError(ctx, "kubectl", args, res)
	}

	exited := make(chan error, 1)
	go func() {
		exited <- child.Wait()
		cancel()
	}()
	ready := make(chan error, 1)
	go func() {
		ready <- opts.OnReady(childCtx, opts.LocalPort)
	}()
	finish := func(err error) error {
		res := runResult{err: err, stderr: string(stderr.Bytes())}
		if c.settings.Debug {
			debugResult("kubectl", res, time.Since(start))
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err == nil {
			return clierr.NewUpstreamError("kubectl port-forward stopped before the command context ended")
		}
		return classifyRunError(ctx, "kubectl", args, res)
	}

	select {
	case err := <-ready:
		if err != nil {
			cancel()
			waitErr := <-exited
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			// The child can cancel the probe before its failure reaches the
			// select below. Keep that failure rather than the cancelled probe.
			if errors.Is(err, context.Canceled) {
				return finish(waitErr)
			}
			if c.settings.Debug {
				debugResult("kubectl", runResult{err: waitErr, stderr: string(stderr.Bytes())}, time.Since(start))
			}
			return err
		}
	case err := <-exited:
		cancel()
		<-ready
		return finish(err)
	case <-childCtx.Done():
		err := <-exited
		<-ready
		return finish(err)
	}

	select {
	case err = <-exited:
	case <-ctx.Done():
		cancel()
		err = <-exited
	}
	return finish(err)
}
