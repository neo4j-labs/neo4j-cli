// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop/dbms"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	debugSalt     = "salt-desktop-debug"
	debugClientID = "cid-desktop-debug"
)

// mountDesktopUnderRoot builds the desktop tree under a stand-in neo4j-cli root
// with EnableTraverseRunHooks=true (mirroring app.go), so the desktop-root
// PersistentPreRunE resolves --debug for every nested leaf. --format and the
// --rw write gate live on the root, exactly as the shipped surface wires them.
func mountDesktopUnderRoot(t *testing.T, cfg *clicfg.Config) *cobra.Command {
	t.Helper()
	desktopCmd := desktop.NewCmd(cfg)

	root := &cobra.Command{Use: "neo4j-cli"}
	flags.RegisterOutputFlag(root, cfg)
	flags.RegisterRwFlag(root)
	root.PersistentPreRunE = flags.ComposeRootPersistentPreRunE(cfg)
	root.AddCommand(desktopCmd)

	prev := cobra.EnableTraverseRunHooks
	cobra.EnableTraverseRunHooks = true
	t.Cleanup(func() { cobra.EnableTraverseRunHooks = prev })
	return root
}

// pinDesktopClient backs the dbms-leaf newDesktopClientFn seam with a real
// desktopclient.Client wired to the supplied httptest server, so the leaf
// exercises the production wire-tracing path in doRaw.
func pinDesktopClient(t *testing.T, srvURL string) {
	t.Helper()
	t.Cleanup(desktopclient.SetUUIDFnForTest(func() string { return debugClientID }))
	t.Cleanup(desktopclient.SetNowFnForTest(func() time.Time { return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC) }))
	t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return desktopclient.NewClient(desktopclient.ProbeResult{Origin: srvURL}, debugSalt)
	}))
}

func dbmsListServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[{"id":"a","name":"alpha","connectionUri":"neo4j://localhost:7687"}]`))
		case "/fastify/api/dbmss/a":
			_, _ = w.Write([]byte(`{"id":"a","name":"alpha","version":"5.20.0","status":"started","connectionUri":"neo4j://localhost:7687"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// runDesktopDbmsList drives `desktop dbms list` end-to-end for the given format
// and --debug argument, returning captured stdout and the captured
// desktopclient debug stream.
func runDesktopDbmsList(t *testing.T, srvURL, format, debugArg string) (stdout, debug string) {
	t.Helper()

	var debugBuf bytes.Buffer
	t.Cleanup(desktopclient.SetDebugWriterForTest(&debugBuf))

	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	pinDesktopClient(t, srvURL)
	root := mountDesktopUnderRoot(t, cfg)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"desktop", "dbms", "list", "--format", format, debugArg})
	require.NoError(t, root.Execute())

	return out.String(), debugBuf.String()
}

func TestDbmsList_DebugFlagDoesNotChangeStdout(t *testing.T) {
	t.Setenv("NEO4J_DEBUG", "")
	srv := dbmsListServer(t)

	for _, format := range []string{"json", "table", "toon"} {
		t.Run(format, func(t *testing.T) {
			stdoutOff, debugOff := runDesktopDbmsList(t, srv.URL, format, "--debug=false")
			stdoutOn, debugOn := runDesktopDbmsList(t, srv.URL, format, "--debug=true")

			assert.Equal(t, stdoutOff, stdoutOn,
				"--debug must not alter stdout bytes (debug output is routed to the stderr seam)")
			assert.NotEmpty(t, stdoutOn, "command must produce stdout")
			assert.Contains(t, stdoutOn, "alpha")

			assert.Empty(t, debugOff, "no debug output when --debug=false")

			assert.Contains(t, debugOn, "[desktop-debug] > GET")
			assert.Contains(t, debugOn, "/dbmss")
			assert.Contains(t, debugOn, "[desktop-debug] < 200")
			assert.Contains(t, debugOn, "elapsed")
		})
	}
}

// runDebugResolution mounts the desktop tree under a stub root, replaces the
// `desktop dbms list` leaf RunE so no network is touched, drives resolution
// through all PersistentPreRunE hooks, and returns the package-global debug
// state the desktop-root hook resolved onto the seam.
//
// desktopclient.debugEnabled is a process-global; each case resets it to false
// up front so a prior case can't leak an enabled state into a default-off case.
func runDebugResolution(t *testing.T, args []string) bool {
	t.Helper()
	desktopclient.SetDebug(false)

	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	root := mountDesktopUnderRoot(t, cfg)

	leaf, _, err := root.Find([]string{"desktop", "dbms", "list"})
	require.NoError(t, err)
	leaf.RunE = func(_ *cobra.Command, _ []string) error { return nil }

	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"desktop", "dbms", "list"}, args...))
	require.NoError(t, root.Execute())

	return desktopclient.DebugEnabled()
}

func TestDebugResolvedOnMountedSurface(t *testing.T) {
	testCases := []struct {
		name      string
		args      []string
		envValue  string
		wantDebug bool
	}{
		{name: "default off", args: nil, wantDebug: false},
		{name: "flag on", args: []string{"--debug"}, wantDebug: true},
		{name: "env=1 on", args: nil, envValue: "1", wantDebug: true},
		{name: "env=true leaves off", args: nil, envValue: "true", wantDebug: false},
		{name: "explicit --debug=false overrides env=1", args: []string{"--debug=false"}, envValue: "1", wantDebug: false},
	}

	for _, tc := range testCases {
		// Serial (no t.Parallel): desktopclient.debugEnabled is a process-global.
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NEO4J_DEBUG", tc.envValue)
			assert.Equal(t, tc.wantDebug, runDebugResolution(t, tc.args))
		})
	}
}

func TestDbmsCreate_DebugRedactsPassword(t *testing.T) {
	t.Setenv("NEO4J_DEBUG", "")
	const password = "sup3r-s3cr3t-pw"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			// Pre-flight conflict check finds no other DBMS running.
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			_, _ = w.Write([]byte(`{"id":"new","name":"x","version":"5.21.0","status":"created","connectionUri":"neo4j://localhost:7687"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/new/start":
			_, _ = w.Write([]byte(`"started"`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	var debugBuf bytes.Buffer
	t.Cleanup(desktopclient.SetDebugWriterForTest(&debugBuf))

	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	pinDesktopClient(t, srv.URL)
	t.Cleanup(dbms.SetCreatePollSleepFnForTest(func(_ time.Duration) {}))
	root := mountDesktopUnderRoot(t, cfg)

	args, err := shlex.Split("desktop dbms create --name x --version 5.21.0 --password " + password + " --rw --format json --debug")
	require.NoError(t, err)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	require.NoError(t, root.Execute())

	out := debugBuf.String()
	require.NotEmpty(t, out, "debug stream must carry the create wire dump")
	assert.NotContains(t, out, password, "password must be redacted from the debug trace")
	assert.Contains(t, out, "[desktop-debug] > POST")
	assert.Contains(t, out, "/desktop/dbmss")
	assert.Contains(t, out, "***")
}
