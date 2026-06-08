// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stopSetup wires the docker parent + stop leaf against a hermetic cfg and
// a fake dockerClient. Mirrors startSetup so stop tests follow the same
// shape as start tests for grep-discoverability.
type stopSetup struct {
	fake *fakeDockerClient
	cfg  *clicfg.Config
	cmd  *cmdHandle
}

func newStopSetup(t *testing.T, containers map[string]Container) *stopSetup {
	t.Helper()

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	fake := newFakeDockerClient()
	for name, c := range containers {
		fake.Containers[name] = c
	}
	origFactory := clientFactory
	clientFactory = func(bool) dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	run := func(args string) error {
		// Rebuild the cobra tree on each invocation so flag state never leaks
		// across cases — cobra commands are not safe to re-Execute with a new
		// argv otherwise.
		cmd := NewCmd(cfg)
		cmd.SetOut(out)
		cmd.SetErr(errBuf)
		argv, splitErr := shlex.Split(args)
		require.NoError(t, splitErr)
		cmd.SetArgs(append([]string{"stop"}, argv...))
		return cmd.Execute()
	}

	return &stopSetup{
		fake: fake,
		cfg:  cfg,
		cmd: &cmdHandle{
			out: out,
			err: errBuf,
			run: run,
		},
	}
}

// withShortPollInterval shrinks pollInterval for the duration of the test so
// --wait poll cases don't burn the production 500ms gap on every retry.
func withShortPollInterval(t *testing.T, d time.Duration) {
	t.Helper()
	orig := pollInterval
	pollInterval = d
	t.Cleanup(func() { pollInterval = orig })
}

// withShortStopWaitTimeout shrinks waitTimeout for the duration of the test
// so timeout cases don't burn the production 60s budget.
func withShortStopWaitTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	orig := waitTimeout
	waitTimeout = d
	t.Cleanup(func() { waitTimeout = orig })
}

func managedRunningContainerForStop(name string) Container {
	return Container{
		Name:     name,
		Status:   "Up 2 hours",
		Edition:  "enterprise",
		Version:  "5.20",
		BoltPort: "7687",
		HTTPPort: "7474",
		Image:    "neo4j:5.20-enterprise",
		Managed:  true,
		Running:  true,
	}
}

func TestStop_HappyPath_NoWait(t *testing.T) {
	// REQ-F-040: a managed container is stopped via dockerClient.Stop exactly
	// once. No --wait → no Inspect poll, no narration.
	s := newStopSetup(t, map[string]Container{
		"dev": managedRunningContainerForStop("dev"),
	})

	require.NoError(t, s.cmd.run("dev"))
	require.Len(t, s.fake.StopCalls, 1)
	assert.Equal(t, "dev", s.fake.StopCalls[0])
	assert.Empty(t, s.cmd.err.String(), "no --wait must produce no stderr narration")
	// Exactly one Inspect call — the pre-flight managed check. The --wait
	// poll loop must NOT fire when --wait is not set.
	assert.Len(t, s.fake.InspectCalls, 1)
}

func TestStop_MissingContainer_UnknownError(t *testing.T) {
	// REQ-F-043: missing container → unknown-name usage error; no Stop call.
	s := newStopSetup(t, nil)

	err := s.cmd.run("ghost")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no managed container named "ghost"`)
	assert.Contains(t, err.Error(), "neo4j-cli docker list")
	assert.Empty(t, s.fake.StopCalls, "Stop must not fire when Inspect reports unknown")
}

func TestStop_UnmanagedContainer_UnknownError(t *testing.T) {
	// REQ-F-043: a container that exists in Docker but lacks the managed
	// label is still treated as unknown. The Stop call must not fire.
	s := newStopSetup(t, map[string]Container{
		"someones-postgres": {
			Name:    "someones-postgres",
			Status:  "Up 2 hours",
			Image:   "postgres:16",
			Managed: false,
			Running: true,
		},
	})

	err := s.cmd.run("someones-postgres")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no managed container named "someones-postgres"`)
	assert.Empty(t, s.fake.StopCalls)
}

func TestStop_DockerStopError_Surfaced(t *testing.T) {
	// dockerClient.Stop failure (daemon returned non-zero) is surfaced
	// verbatim — REQ-F-061 wraps stderr in a clierr.UsageError upstream so
	// the leaf only needs to silence cobra's usage banner and return.
	s := newStopSetup(t, map[string]Container{
		"dev": managedRunningContainerForStop("dev"),
	})
	s.fake.StopFn = func(_ context.Context, _ string) error {
		return errors.New("docker stop dev: Error response from daemon: container is dead")
	}

	err := s.cmd.run("dev")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "container is dead")
	require.Len(t, s.fake.StopCalls, 1)
}

func TestStop_Wait_HappyPath_ExitsAfterPolls(t *testing.T) {
	// REQ-F-041: --wait polls Inspect until Running == false. Simulate a
	// container that reports Running for the first two Inspect calls after
	// the pre-flight, then flips to Running=false on the third.
	withShortPollInterval(t, 1*time.Millisecond)
	withShortStopWaitTimeout(t, 2*time.Second)

	s := newStopSetup(t, map[string]Container{
		"dev": managedRunningContainerForStop("dev"),
	})
	var inspectCount int32
	s.fake.InspectFn = func(_ context.Context, name string) (Container, error) {
		n := atomic.AddInt32(&inspectCount, 1)
		c := managedRunningContainerForStop(name)
		// Call 1 is the pre-flight managed check; the poll loop starts at
		// call 2. Flip Running to false on the 4th observation so we exercise
		// at least one ticker tick before exit.
		if n >= 4 {
			c.Running = false
			c.Status = "Exited (0) 1 second ago"
		}
		return c, nil
	}

	require.NoError(t, s.cmd.run("dev --wait"))
	require.Len(t, s.fake.StopCalls, 1)
	assert.Contains(t, s.cmd.err.String(), `info: waiting for container "dev" to exit...`)
	// Pre-flight + at least three poll observations.
	assert.GreaterOrEqual(t, atomic.LoadInt32(&inspectCount), int32(4))
}

func TestStop_Wait_HappyPath_EphemeralCleanedUpAfterStop(t *testing.T) {
	// For an ephemeral container (`--rm`) the daemon removes the container
	// the instant it exits. The pre-flight Inspect must succeed (so we know
	// it's managed) but subsequent Inspect calls during the poll loop
	// return "no such container" — which counts as a successful exit.
	withShortPollInterval(t, 1*time.Millisecond)
	withShortStopWaitTimeout(t, 2*time.Second)

	s := newStopSetup(t, map[string]Container{
		"dev": managedRunningContainerForStop("dev"),
	})
	var inspectCount int32
	s.fake.InspectFn = func(_ context.Context, name string) (Container, error) {
		n := atomic.AddInt32(&inspectCount, 1)
		if n == 1 {
			// Pre-flight: return the managed running container.
			return managedRunningContainerForStop(name), nil
		}
		// After the Stop call the ephemeral container is gone — mirror
		// execClient.Inspect's contract by wrapping ErrNotFound. The bare
		// error fallback in inspectExited is still tested elsewhere; this
		// case exercises the primary errors.Is path the production client
		// surfaces.
		return Container{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	require.NoError(t, s.cmd.run("dev --wait"))
	require.Len(t, s.fake.StopCalls, 1)
	assert.Contains(t, s.cmd.err.String(), `info: waiting for container "dev" to exit...`)

	// Sanity: a subsequent `docker get` should return unknown-name because
	// the ephemeral container has been cleaned up. Drive the get leaf
	// through the same fake to verify the contract end-to-end. The fake's
	// InspectFn now returns ErrNotFound, so get's unknown-name branch fires.
	getCmd := NewCmd(s.cfg)
	getOut := bytes.NewBuffer(nil)
	getErr := bytes.NewBuffer(nil)
	getCmd.SetOut(getOut)
	getCmd.SetErr(getErr)
	getCmd.SetArgs([]string{"get", "dev"})
	err := getCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no managed container named "dev"`)
}

func TestStop_Wait_Timeout_ReturnsError(t *testing.T) {
	// REQ-F-041: on timeout the leaf returns the documented usage error
	// naming the container and the duration. The container is NOT torn
	// down further — stop already fired, the daemon owns the lifecycle.
	withShortPollInterval(t, 1*time.Millisecond)
	withShortStopWaitTimeout(t, 30*time.Millisecond)

	s := newStopSetup(t, map[string]Container{
		"dev": managedRunningContainerForStop("dev"),
	})
	s.fake.InspectFn = func(_ context.Context, name string) (Container, error) {
		// Inspect always reports the container as still running so the
		// poll loop never sees an exit and the deadline trips first.
		return managedRunningContainerForStop(name), nil
	}

	err := s.cmd.run("dev --wait")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `container "dev" did not exit within`)
	assert.Contains(t, err.Error(), "30ms")
	require.Len(t, s.fake.StopCalls, 1, "Stop must have fired even though the wait timed out")
}

func TestStop_Wait_TransientInspectError_RetriesUntilTimeout(t *testing.T) {
	// A non-missing-container Inspect error during the poll loop is
	// treated as transient — the loop keeps polling until the deadline
	// rather than failing fast. Verify by returning a generic daemon
	// error from every post-pre-flight Inspect; we expect the timeout
	// error, not the daemon error.
	withShortPollInterval(t, 1*time.Millisecond)
	withShortStopWaitTimeout(t, 30*time.Millisecond)

	s := newStopSetup(t, map[string]Container{
		"dev": managedRunningContainerForStop("dev"),
	})
	var inspectCount int32
	s.fake.InspectFn = func(_ context.Context, name string) (Container, error) {
		n := atomic.AddInt32(&inspectCount, 1)
		if n == 1 {
			return managedRunningContainerForStop(name), nil
		}
		return Container{}, errors.New("Error response from daemon: connection reset")
	}

	err := s.cmd.run("dev --wait")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `did not exit within`)
	assert.NotContains(t, err.Error(), "connection reset", "transient Inspect errors must not surface")
}

func TestStop_NoArgs_CobraUsageError(t *testing.T) {
	// cobra.ExactArgs(1) — no positional arg → error before RunE fires, so
	// neither Inspect nor Stop is called.
	s := newStopSetup(t, nil)
	err := s.cmd.run("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
	assert.Empty(t, s.fake.InspectCalls)
	assert.Empty(t, s.fake.StopCalls)
}

func TestStop_TooManyArgs_CobraUsageError(t *testing.T) {
	s := newStopSetup(t, nil)
	err := s.cmd.run("a b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
	assert.Empty(t, s.fake.InspectCalls)
	assert.Empty(t, s.fake.StopCalls)
}

func TestStop_InspectDaemonError_Propagated(t *testing.T) {
	// A non-not-found pre-flight Inspect error (daemon down, permission
	// denied, …) propagates verbatim — the operator must see the real
	// cause, not a misleading "no managed container" message. Stop must
	// NOT fire.
	s := newStopSetup(t, nil)
	s.fake.InspectFn = func(_ context.Context, _ string) (Container, error) {
		return Container{}, errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?")
	}

	err := s.cmd.run("dev")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cannot connect to the Docker daemon",
		"daemon errors must surface verbatim so the operator can fix the real cause")
	assert.NotContains(t, err.Error(), "no managed container named",
		"daemon errors must NOT be funneled into the unknown-name message")
	assert.Empty(t, s.fake.StopCalls, "Stop must not fire when Inspect reports a daemon error")
}
