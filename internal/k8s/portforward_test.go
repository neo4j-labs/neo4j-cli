// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProcess struct {
	exited <-chan error
}

func (p fakeProcess) Wait() error {
	return <-p.exited
}

func captureProcess(t *testing.T, exited <-chan error) (<-chan context.Context, *[]string) {
	t.Helper()
	contexts := make(chan context.Context, 1)
	arguments := &[]string{}
	previous := startProcessFn
	startProcessFn = func(ctx context.Context, _ string, args []string, stderr io.Writer) (process, error) {
		contexts <- ctx
		*arguments = append(*arguments, args...)
		_, _ = io.WriteString(stderr, "Error from server (NotFound): pod not found\n")
		return fakeProcess{exited: exited}, nil
	}
	t.Cleanup(func() { startProcessFn = previous })
	return contexts, arguments
}

func TestPortForward_ArgvAndCancellation(t *testing.T) {
	stubLookPath(t)
	exited := make(chan error, 1)
	contexts, arguments := captureProcess(t, exited)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan int, 1)
	done := make(chan error, 1)
	go func() {
		done <- NewClient(Settings{Kubeconfig: "/tmp/kube", Context: "kind-kind", Namespace: "team-a"}).
			PortForward(ctx, PortForwardOptions{
				Pod: "dev-server-0", RemotePort: 17687, LocalPort: 7688,
				OnReady: func(_ context.Context, port int) error { ready <- port; return nil },
			})
	}()
	assert.Equal(t, 7688, <-ready)
	childCtx := <-contexts
	cancel()
	select {
	case <-childCtx.Done():
	case <-t.Context().Done():
		t.Fatal("the port-forward context did not stop")
	}
	exited <- context.Canceled
	assert.ErrorIs(t, <-done, context.Canceled)
	assert.Equal(t, []string{
		"port-forward", "pod/dev-server-0", "--address", "127.0.0.1", "7688:17687",
		"--kubeconfig", "/tmp/kube", "--context", "kind-kind", "-n", "team-a",
	}, *arguments)
}

func TestPortForward_ReadinessFailureStopsChild(t *testing.T) {
	stubLookPath(t)
	exited := make(chan error, 1)
	contexts, _ := captureProcess(t, exited)
	wantErr := errors.New("cannot write the environment file")
	err := NewClient(Settings{}).PortForward(t.Context(), PortForwardOptions{
		Pod: "dev-server-0", RemotePort: 7687, LocalPort: 7687,
		OnReady: func(ctx context.Context, _ int) error {
			go func() { <-ctx.Done(); exited <- ctx.Err() }()
			return wantErr
		},
	})
	assert.ErrorIs(t, err, wantErr)
	assert.ErrorIs(t, (<-contexts).Err(), context.Canceled)
}

func TestPortForward_ChildFailureCancelsReadiness(t *testing.T) {
	stubLookPath(t)
	exited := make(chan error, 1)
	contexts, _ := captureProcess(t, exited)
	exited <- &exec.ExitError{}
	err := NewClient(Settings{}).PortForward(t.Context(), PortForwardOptions{
		Pod: "dev-server-0", RemotePort: 7687, LocalPort: 7687,
		OnReady: func(ctx context.Context, _ int) error { <-ctx.Done(); return ctx.Err() },
	})
	assert.Truef(t, IsNotFound(err), "the child failure must survive cancellation of the readiness context, got: %v", err)
	assert.ErrorIs(t, (<-contexts).Err(), context.Canceled)
}

func TestPortForward_UnexpectedExitIsAnError(t *testing.T) {
	stubLookPath(t)
	exited := make(chan error, 1)
	_, _ = captureProcess(t, exited)
	exited <- nil
	err := NewClient(Settings{}).PortForward(t.Context(), PortForwardOptions{
		Pod: "dev-server-0", RemotePort: 7687, LocalPort: 7687,
		OnReady: func(ctx context.Context, _ int) error { <-ctx.Done(); return ctx.Err() },
	})
	var cliErr *clierr.CLIError
	require.ErrorAs(t, err, &cliErr)
	assert.Equal(t, "upstream_error", cliErr.CodeName)
	assert.Contains(t, err.Error(), "stopped")
}

func TestPortForward_DebugCapturesChildStderr(t *testing.T) {
	stubLookPath(t)
	exited := make(chan error, 1)
	_, _ = captureProcess(t, exited)
	var debug strings.Builder
	SetDebugWriterForTest(t, &debug)
	exited <- &exec.ExitError{}
	_ = NewClient(Settings{Debug: true}).PortForward(t.Context(), PortForwardOptions{
		Pod: "dev-server-0", RemotePort: 7687, LocalPort: 7687,
		OnReady: func(ctx context.Context, _ int) error { <-ctx.Done(); return ctx.Err() },
	})
	assert.Contains(t, debug.String(), DebugReqPrefix+"kubectl port-forward")
	assert.Contains(t, debug.String(), "pod not found")
}
