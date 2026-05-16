// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deleteSetup wires the docker parent + delete leaf against a hermetic cfg
// and a fake dockerClient. Like startSetup / stopSetup but with an extra
// `stdin` knob so the prompt cases can drive y/N/empty input. The TTY
// signal is controlled separately via withStdinIsTerminal.
type deleteSetup struct {
	fake *fakeDockerClient
	cfg  *clicfg.Config
	cmd  *cmdHandle
}

func newDeleteSetup(t *testing.T, containers map[string]Container, creds map[string]string, stdin string) *deleteSetup {
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
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	for name, pass := range creds {
		require.NoError(t, cfg.Credentials.Dbms.Add(name, "neo4j", pass, "neo4j", "neo4j://localhost:7687"))
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
		cmd.SetIn(strings.NewReader(stdin))
		argv, splitErr := shlex.Split(args)
		require.NoError(t, splitErr)
		cmd.SetArgs(append([]string{"delete"}, argv...))
		return cmd.Execute()
	}

	return &deleteSetup{
		fake: fake,
		cfg:  cfg,
		cmd: &cmdHandle{
			out: out,
			err: errBuf,
			run: run,
		},
	}
}

// withStdinIsTerminal swaps the stdinIsTerminal package-level seam so tests
// can deterministically pick the TTY / non-TTY branch without standing up
// a real PTY. Restored on cleanup.
func withStdinIsTerminal(t *testing.T, isTTY bool) {
	t.Helper()
	orig := stdinIsTerminal
	stdinIsTerminal = func() bool { return isTTY }
	t.Cleanup(func() { stdinIsTerminal = orig })
}

func managedContainerForDelete(name string) Container {
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

func TestDelete_TTY_Yes_RemovesContainerAndCredential(t *testing.T) {
	// REQ-F-050/F-051: TTY + `y` confirms; both the container and the
	// stored dbms credential are removed; the prompt is written to stderr.
	withStdinIsTerminal(t, true)
	s := newDeleteSetup(t,
		map[string]Container{"dev": managedContainerForDelete("dev")},
		map[string]string{"dev": "secret"},
		"y\n",
	)

	require.NoError(t, s.cmd.run("dev"))
	require.Len(t, s.fake.RemoveForceCalls, 1)
	assert.Equal(t, "dev", s.fake.RemoveForceCalls[0])
	_, err := s.cfg.Credentials.Dbms.Get("dev")
	require.Error(t, err, "credential should have been removed")
	assert.Contains(t, s.cmd.err.String(), `Delete container dev and its dbms credential? [y/N]`)
}

func TestDelete_TTY_YesUppercase_Confirms(t *testing.T) {
	// `Y` (uppercase) also confirms; case-insensitive match.
	withStdinIsTerminal(t, true)
	s := newDeleteSetup(t,
		map[string]Container{"dev": managedContainerForDelete("dev")},
		map[string]string{"dev": "secret"},
		"Y\n",
	)

	require.NoError(t, s.cmd.run("dev"))
	require.Len(t, s.fake.RemoveForceCalls, 1)
}

func TestDelete_TTY_Yes_Word_Confirms(t *testing.T) {
	// Full word `yes` also confirms.
	withStdinIsTerminal(t, true)
	s := newDeleteSetup(t,
		map[string]Container{"dev": managedContainerForDelete("dev")},
		map[string]string{"dev": "secret"},
		"yes\n",
	)

	require.NoError(t, s.cmd.run("dev"))
	require.Len(t, s.fake.RemoveForceCalls, 1)
}

func TestDelete_TTY_No_Cancels(t *testing.T) {
	// `n` cancels; neither the container nor the credential is touched.
	withStdinIsTerminal(t, true)
	s := newDeleteSetup(t,
		map[string]Container{"dev": managedContainerForDelete("dev")},
		map[string]string{"dev": "secret"},
		"n\n",
	)

	require.NoError(t, s.cmd.run("dev"))
	assert.Empty(t, s.fake.RemoveForceCalls, "RemoveForce must not fire on cancel")
	_, err := s.cfg.Credentials.Dbms.Get("dev")
	require.NoError(t, err, "credential must remain on cancel")
	assert.Contains(t, s.cmd.err.String(), "cancelled.")
}

func TestDelete_TTY_EmptyLine_DefaultsToCancel(t *testing.T) {
	// Empty line (user pressed Enter) is the default N → cancel.
	withStdinIsTerminal(t, true)
	s := newDeleteSetup(t,
		map[string]Container{"dev": managedContainerForDelete("dev")},
		map[string]string{"dev": "secret"},
		"\n",
	)

	require.NoError(t, s.cmd.run("dev"))
	assert.Empty(t, s.fake.RemoveForceCalls)
	assert.Contains(t, s.cmd.err.String(), "cancelled.")
}

func TestDelete_NonTTY_NoForce_UsageError(t *testing.T) {
	// REQ-F-052: scripts / piped callers must pass --force; without it the
	// leaf surfaces a usage error and nothing is touched.
	withStdinIsTerminal(t, false)
	s := newDeleteSetup(t,
		map[string]Container{"dev": managedContainerForDelete("dev")},
		map[string]string{"dev": "secret"},
		"",
	)

	err := s.cmd.run("dev")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-TTY caller must pass --force to confirm deletion")
	assert.Empty(t, s.fake.RemoveForceCalls)
	_, getErr := s.cfg.Credentials.Dbms.Get("dev")
	require.NoError(t, getErr, "credential must remain on non-TTY without --force")
}

func TestDelete_Force_TTY_SkipsPromptAndRemoves(t *testing.T) {
	// --force on a TTY skips the prompt entirely; nothing is read from stdin,
	// no prompt is written to stderr, both container and credential go away.
	withStdinIsTerminal(t, true)
	s := newDeleteSetup(t,
		map[string]Container{"dev": managedContainerForDelete("dev")},
		map[string]string{"dev": "secret"},
		"", // stdin must not be read at all
	)

	require.NoError(t, s.cmd.run("dev --force"))
	require.Len(t, s.fake.RemoveForceCalls, 1)
	_, err := s.cfg.Credentials.Dbms.Get("dev")
	require.Error(t, err)
	assert.NotContains(t, s.cmd.err.String(), "Delete container")
}

func TestDelete_Force_NonTTY_SkipsPromptAndRemoves(t *testing.T) {
	// --force on a non-TTY (the script path) also skips the prompt.
	withStdinIsTerminal(t, false)
	s := newDeleteSetup(t,
		map[string]Container{"dev": managedContainerForDelete("dev")},
		map[string]string{"dev": "secret"},
		"",
	)

	require.NoError(t, s.cmd.run("dev --force"))
	require.Len(t, s.fake.RemoveForceCalls, 1)
	_, err := s.cfg.Credentials.Dbms.Get("dev")
	require.Error(t, err)
}

func TestDelete_NonManagedContainer_UnknownError(t *testing.T) {
	// REQ-F-053: a container that exists in Docker but lacks the managed
	// label gets the unknown-name usage error; no RemoveForce call.
	withStdinIsTerminal(t, true)
	s := newDeleteSetup(t,
		map[string]Container{
			"someones-postgres": {
				Name:    "someones-postgres",
				Status:  "Up 2 hours",
				Image:   "postgres:16",
				Managed: false,
				Running: true,
			},
		},
		nil,
		"y\n",
	)

	err := s.cmd.run("someones-postgres --force")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no managed container named "someones-postgres"`)
	assert.Contains(t, err.Error(), "neo4j-cli docker list")
	assert.Empty(t, s.fake.RemoveForceCalls)
}

func TestDelete_MissingContainer_UnknownError(t *testing.T) {
	// REQ-F-053: missing container → unknown-name usage error; no RemoveForce.
	withStdinIsTerminal(t, true)
	s := newDeleteSetup(t, nil, nil, "y\n")

	err := s.cmd.run("ghost --force")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no managed container named "ghost"`)
	assert.Empty(t, s.fake.RemoveForceCalls)
}

func TestDelete_ContainerExists_CredentialMissing_StillSucceeds(t *testing.T) {
	// REQ-F-050: missing dbms credential is NOT an error — the container is
	// still removed and the leaf returns nil. Drives the "credential never
	// stored" (--no-store-credential) or "credential already removed" path.
	withStdinIsTerminal(t, true)
	s := newDeleteSetup(t,
		map[string]Container{"dev": managedContainerForDelete("dev")},
		nil, // no credential stored
		"y\n",
	)

	require.NoError(t, s.cmd.run("dev"))
	require.Len(t, s.fake.RemoveForceCalls, 1)
	assert.Equal(t, "dev", s.fake.RemoveForceCalls[0])
}

func TestDelete_DockerRemoveError_Surfaced(t *testing.T) {
	// dockerClient.RemoveForce failure (daemon returned non-zero) is surfaced
	// verbatim. The credential is NOT removed when the container removal
	// failed — keeping the stored credential paired with the still-present
	// container is the safer default.
	withStdinIsTerminal(t, true)
	s := newDeleteSetup(t,
		map[string]Container{"dev": managedContainerForDelete("dev")},
		map[string]string{"dev": "secret"},
		"y\n",
	)
	s.fake.RemoveForceFn = func(_ context.Context, _ string) error {
		return errors.New("docker rm -f dev: Error response from daemon: container is in use")
	}

	err := s.cmd.run("dev --force")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "container is in use")
	_, credErr := s.cfg.Credentials.Dbms.Get("dev")
	require.NoError(t, credErr, "credential must remain when RemoveForce failed")
}

func TestDelete_NoArgs_CobraUsageError(t *testing.T) {
	// cobra.ExactArgs(1) — no positional arg → error before RunE fires.
	withStdinIsTerminal(t, true)
	s := newDeleteSetup(t, nil, nil, "")

	err := s.cmd.run("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
	assert.Empty(t, s.fake.RemoveForceCalls)
	assert.Empty(t, s.fake.InspectCalls)
}

func TestDelete_TooManyArgs_CobraUsageError(t *testing.T) {
	withStdinIsTerminal(t, true)
	s := newDeleteSetup(t, nil, nil, "")

	err := s.cmd.run("a b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
	assert.Empty(t, s.fake.RemoveForceCalls)
}

func TestDelete_InspectDaemonError_Propagated(t *testing.T) {
	// A non-not-found Inspect error (daemon down, permission denied, …)
	// propagates verbatim — the operator must see the real cause, not a
	// misleading "no managed container" message. RemoveForce must NOT fire.
	withStdinIsTerminal(t, true)
	s := newDeleteSetup(t, nil, nil, "y\n")
	s.fake.InspectFn = func(_ context.Context, _ string) (Container, error) {
		return Container{}, errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?")
	}

	err := s.cmd.run("dev --force")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cannot connect to the Docker daemon",
		"daemon errors must surface verbatim so the operator can fix the real cause")
	assert.NotContains(t, err.Error(), "no managed container named",
		"daemon errors must NOT be funneled into the unknown-name message")
	assert.Empty(t, s.fake.RemoveForceCalls, "RemoveForce must not fire when Inspect reports a daemon error")
}

func TestDelete_HasWriteAnnotation(t *testing.T) {
	// REQ-F-050: delete is a write operation; the --rw gate relies on the
	// "write" annotation being set on the leaf.
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	root := NewCmd(cfg)

	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "delete" {
			assert.Equal(t, "true", c.Annotations["write"])
			found = true
			break
		}
	}
	require.True(t, found, "delete subcommand must be registered")
}
