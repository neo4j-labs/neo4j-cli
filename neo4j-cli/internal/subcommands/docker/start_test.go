// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startSetup wires the docker parent + start leaf against a hermetic cfg and
// a fake dockerClient, optionally seeding containers and dbms credentials.
// Returns the fake (for call assertions), stderr/stdout buffers, and the cmd
// ready to be Execute()d via a separate args set in each table case.
type startSetup struct {
	fake      *fakeDockerClient
	cfg       *clicfg.Config
	cmd       *cmdHandle
	credAdded bool
}

// cmdHandle bundles the runnable cobra root + io buffers a test asserts on.
type cmdHandle struct {
	out, err *bytes.Buffer
	run      func(args string) error
}

func newStartSetup(t *testing.T, containers map[string]Container, creds map[string]string) *startSetup {
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

	credAdded := false
	for name, pass := range creds {
		require.NoError(t, cfg.Credentials.Dbms.Add(name, "neo4j", pass, "neo4j", "neo4j://localhost:7687"))
		credAdded = true
	}

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
		cmd.SetArgs(append([]string{"start"}, argv...))
		return cmd.Execute()
	}

	return &startSetup{
		fake: fake,
		cfg:  cfg,
		cmd: &cmdHandle{
			out: out,
			err: errBuf,
			run: run,
		},
		credAdded: credAdded,
	}
}

// withStartWaitProbe swaps the package-level waitForBoltFn seam so --wait
// tests run deterministically without standing up a real Bolt endpoint.
// Restored on cleanup.
func withStartWaitProbe(t *testing.T, prober func(ctx context.Context, uri, user, pass string, timeout time.Duration) error) {
	t.Helper()
	orig := waitForBoltFn
	waitForBoltFn = prober
	t.Cleanup(func() { waitForBoltFn = orig })
}

// withStartTCPWaitProbe swaps the package-level waitForBoltTCPFn seam so
// --wait fallback tests run deterministically without standing up a real
// listener. Restored on cleanup.
func withStartTCPWaitProbe(t *testing.T, prober func(ctx context.Context, host string, port int, timeout time.Duration) error) {
	t.Helper()
	orig := waitForBoltTCPFn
	waitForBoltTCPFn = prober
	t.Cleanup(func() { waitForBoltTCPFn = orig })
}

// withStartShortWaitTimeout shrinks waitTimeout for the duration of the test
// so timeout cases don't burn the production 60s budget when the fake prober
// echoes WaitForBolt's timeout error directly.
func withStartShortWaitTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	orig := waitTimeout
	waitTimeout = d
	t.Cleanup(func() { waitTimeout = orig })
}

func managedRunningContainer(name string) Container {
	return Container{
		Name:     name,
		Status:   "Exited (0) 5 seconds ago",
		Edition:  "enterprise",
		Version:  "5.20",
		BoltPort: "7687",
		HTTPPort: "7474",
		Image:    "neo4j:5.20-enterprise",
		Managed:  true,
	}
}

func TestStart_HappyPath_NoWait(t *testing.T) {
	// REQ-F-040: a managed container is started via dockerClient.Start exactly
	// once. No --wait → no readiness probe, no narration.
	s := newStartSetup(t, map[string]Container{
		"dev": managedRunningContainer("dev"),
	}, nil)

	require.NoError(t, s.cmd.run("dev"))
	require.Len(t, s.fake.StartCalls, 1)
	assert.Equal(t, "dev", s.fake.StartCalls[0])
	assert.Empty(t, s.cmd.err.String(), "no --wait must produce no stderr narration")
}

func TestStart_MissingContainer_UnknownError(t *testing.T) {
	// REQ-F-043: missing container → unknown-name usage error; no Start call.
	// Also covers REQ-F-042: an already-removed ephemeral container surfaces
	// here identically because Inspect can't distinguish "never existed" from
	// "removed via --rm".
	s := newStartSetup(t, nil, nil)

	err := s.cmd.run("ghost")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no managed container named "ghost"`)
	assert.Contains(t, err.Error(), "neo4j-cli docker list")
	assert.Empty(t, s.fake.StartCalls, "Start must not fire when Inspect reports unknown")
}

func TestStart_UnmanagedContainer_UnknownError(t *testing.T) {
	// REQ-F-043: a container that exists in Docker but lacks the managed
	// label is still treated as unknown. The Start call must not fire.
	s := newStartSetup(t, map[string]Container{
		"someones-postgres": {
			Name:    "someones-postgres",
			Status:  "Exited (0) 5 seconds ago",
			Image:   "postgres:16",
			Managed: false,
		},
	}, nil)

	err := s.cmd.run("someones-postgres")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no managed container named "someones-postgres"`)
	assert.Empty(t, s.fake.StartCalls)
}

func TestStart_Wait_HappyPath(t *testing.T) {
	// REQ-F-041: --wait blocks until WaitForBolt returns nil. Verify the
	// prober receives the uri/user/password from the stored dbms credential
	// and that the TCP fallback probe is NOT invoked when a credential exists.
	var calls int32
	var tcpCalls int32
	withStartWaitProbe(t, func(_ context.Context, uri, user, pass string, _ time.Duration) error {
		atomic.AddInt32(&calls, 1)
		assert.Equal(t, "neo4j://localhost:7687", uri)
		assert.Equal(t, "neo4j", user)
		assert.Equal(t, "secretpw", pass)
		return nil
	})
	withStartTCPWaitProbe(t, func(_ context.Context, _ string, _ int, _ time.Duration) error {
		atomic.AddInt32(&tcpCalls, 1)
		return nil
	})

	s := newStartSetup(t, map[string]Container{
		"dev": managedRunningContainer("dev"),
	}, map[string]string{"dev": "secretpw"})

	require.NoError(t, s.cmd.run("dev --wait"))
	require.Len(t, s.fake.StartCalls, 1)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
	assert.Equal(t, int32(0), atomic.LoadInt32(&tcpCalls), "TCP fallback must not fire when a credential exists")
	assert.Contains(t, s.cmd.err.String(), "info: waiting for Bolt on localhost:7687...")
	assert.NotContains(t, s.cmd.err.String(), "falling back to TCP probe",
		"authenticated path must not announce the TCP fallback")
	// --wait happy path must NOT tear down the container.
	assert.Empty(t, s.fake.StopCalls)
	assert.Empty(t, s.fake.RemoveForceCalls)
}

func TestStart_Wait_Timeout_ReturnsErrorNoTearDown(t *testing.T) {
	// REQ-F-041: on timeout the leaf returns the error and leaves the
	// container running — same contract as create --wait.
	withStartShortWaitTimeout(t, 50*time.Millisecond)
	withStartWaitProbe(t, func(_ context.Context, _, _, _ string, _ time.Duration) error {
		return errors.New("container started but Bolt did not become ready within 50ms; check 'docker logs <name>'")
	})

	s := newStartSetup(t, map[string]Container{
		"dev": managedRunningContainer("dev"),
	}, map[string]string{"dev": "secretpw"})

	err := s.cmd.run("dev --wait")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Bolt did not become ready")
	require.Len(t, s.fake.StartCalls, 1)
	assert.Contains(t, s.cmd.err.String(), "info: waiting for Bolt on localhost:7687...")
	assert.Empty(t, s.fake.StopCalls, "timeout must not stop the container")
	assert.Empty(t, s.fake.RemoveForceCalls, "timeout must not remove the container")
}

func TestStart_Wait_TCPFallback_HappyPath(t *testing.T) {
	// When no dbms credential is stored for the container (e.g. created with
	// --no-store-credential), --wait must fall back to a TCP-only readiness
	// probe rather than hard-erroring. The authenticated probe must NOT run,
	// and stderr must announce the reduced guarantee so the operator knows
	// "port open != Bolt ready".
	var boltCalls int32
	var tcpCalls int32
	withStartWaitProbe(t, func(_ context.Context, _, _, _ string, _ time.Duration) error {
		atomic.AddInt32(&boltCalls, 1)
		return nil
	})
	withStartTCPWaitProbe(t, func(_ context.Context, host string, port int, _ time.Duration) error {
		atomic.AddInt32(&tcpCalls, 1)
		assert.Equal(t, "localhost", host)
		assert.Equal(t, 7687, port)
		return nil
	})

	s := newStartSetup(t, map[string]Container{
		"dev": managedRunningContainer("dev"),
	}, nil)

	require.NoError(t, s.cmd.run("dev --wait"))
	require.Len(t, s.fake.StartCalls, 1)
	assert.Equal(t, int32(0), atomic.LoadInt32(&boltCalls),
		"authenticated probe must NOT run without a credential")
	assert.Equal(t, int32(1), atomic.LoadInt32(&tcpCalls),
		"TCP fallback probe must run exactly once when no credential is stored")
	assert.Contains(t, s.cmd.err.String(), `no stored credential for "dev"`)
	assert.Contains(t, s.cmd.err.String(), "falling back to TCP probe on localhost:7687 (not authenticated)")
	// --wait happy path must NOT tear down the container.
	assert.Empty(t, s.fake.StopCalls)
	assert.Empty(t, s.fake.RemoveForceCalls)
}

func TestStart_Wait_TCPFallback_Timeout_NoTearDown(t *testing.T) {
	// On TCP-fallback timeout the leaf returns the WaitForBoltTCP error and
	// leaves the container running — same contract as the authenticated
	// timeout path (no Stop / RemoveForce calls).
	withStartShortWaitTimeout(t, 50*time.Millisecond)
	withStartTCPWaitProbe(t, func(_ context.Context, _ string, _ int, timeout time.Duration) error {
		return errors.New("container started but TCP port 7687 did not open within " + timeout.String() + "; check 'docker logs <name>'")
	})

	s := newStartSetup(t, map[string]Container{
		"dev": managedRunningContainer("dev"),
	}, nil)

	err := s.cmd.run("dev --wait")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TCP port 7687 did not open")
	require.Len(t, s.fake.StartCalls, 1)
	assert.Contains(t, s.cmd.err.String(), "falling back to TCP probe")
	assert.Empty(t, s.fake.StopCalls, "TCP timeout must not stop the container")
	assert.Empty(t, s.fake.RemoveForceCalls, "TCP timeout must not remove the container")
}

func TestStart_Wait_UnparseableBoltPort_Error(t *testing.T) {
	// If Inspect returns a container with an unparseable bolt-port label
	// (label corruption / version drift), --wait fails loud with a
	// diagnostic naming the offending value. Start has already run so the
	// container itself is fine; the operator can re-run without --wait.
	bad := managedRunningContainer("dev")
	bad.BoltPort = "not-a-port"
	s := newStartSetup(t, map[string]Container{"dev": bad}, nil)

	err := s.cmd.run("dev --wait")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unparseable bolt-port label")
	assert.Contains(t, err.Error(), `"not-a-port"`)
	require.Len(t, s.fake.StartCalls, 1, "Start fires before the port parse")
}

func TestStart_DockerStartError_Surfaced(t *testing.T) {
	// dockerClient.Start failure (daemon returned non-zero) is surfaced
	// verbatim — REQ-F-061 wraps stderr in a clierr.UsageError upstream so
	// the leaf only needs to silence cobra's usage banner and return.
	s := newStartSetup(t, map[string]Container{
		"dev": managedRunningContainer("dev"),
	}, nil)
	s.fake.StartFn = func(_ context.Context, _ string) error {
		return errors.New("docker start dev: Error response from daemon: container is dead")
	}

	err := s.cmd.run("dev")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "container is dead")
	require.Len(t, s.fake.StartCalls, 1)
}

func TestStart_NoArgs_CobraUsageError(t *testing.T) {
	// cobra.ExactArgs(1) — no positional arg → error before RunE fires, so
	// neither Inspect nor Start is called.
	s := newStartSetup(t, nil, nil)
	err := s.cmd.run("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
	assert.Empty(t, s.fake.InspectCalls)
	assert.Empty(t, s.fake.StartCalls)
}

func TestStart_TooManyArgs_CobraUsageError(t *testing.T) {
	s := newStartSetup(t, nil, nil)
	err := s.cmd.run("a b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
	assert.Empty(t, s.fake.InspectCalls)
	assert.Empty(t, s.fake.StartCalls)
}

func TestStart_InspectDaemonError_Propagated(t *testing.T) {
	// A non-not-found Inspect error (daemon down, permission denied, …)
	// propagates verbatim — the operator must see the real cause, not a
	// misleading "no managed container" message. Start must NOT fire.
	s := newStartSetup(t, nil, nil)
	s.fake.InspectFn = func(_ context.Context, _ string) (Container, error) {
		return Container{}, errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?")
	}

	err := s.cmd.run("dev")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cannot connect to the Docker daemon",
		"daemon errors must surface verbatim so the operator can fix the real cause")
	assert.NotContains(t, err.Error(), "no managed container named",
		"daemon errors must NOT be funneled into the unknown-name message")
	assert.Empty(t, s.fake.StartCalls, "Start must not fire when Inspect reports a daemon error")
}
