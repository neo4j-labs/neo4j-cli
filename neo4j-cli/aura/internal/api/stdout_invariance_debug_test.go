// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/aura"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// invarianceServer serves a single v2 organization-list endpoint. The seeded
// credential carries a far-future token so no /oauth/token round-trip happens —
// the command issues exactly one GET.
func invarianceServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v2beta1/organizations", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[{"id":"org-1","name":"acme"},{"id":"org-2","name":"globex"}]}`)) //nolint:errcheck
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// runOrgListInvariance drives `aura organization list` end-to-end against the
// stub server for the given format and --debug argument, returning the captured
// stdout and the captured api-package debug stream. The aura tree is mounted
// under a stand-in neo4j-cli root with EnableTraverseRunHooks=true (mirroring
// the shipped surface) so the aura-root PersistentPreRunE resolves --debug onto
// cfg before the leaf RunE executes.
func runOrgListInvariance(t *testing.T, serverURL, format, debugArg string) (stdout, debug string) {
	t.Helper()

	var debugBuf bytes.Buffer
	api.SetDebugWriterForTest(t, &debugBuf)

	cfg := buildTestConfig(t, serverURL, debugTestCredJSON)

	auraCmd := aura.NewCmd(cfg)
	auraCmd.Use = "aura"

	// Mirror the shipped neo4j-cli root: --format lives on the root and its
	// PersistentPreRunE binds the format; the aura-root PersistentPreRunE
	// resolves --debug. EnableTraverseRunHooks runs both up the ancestry.
	root := &cobra.Command{Use: "neo4j-cli"}
	flags.RegisterOutputFlag(root, cfg)
	root.PersistentPreRunE = flags.ComposeRootPersistentPreRunE(cfg)
	root.AddCommand(auraCmd)

	prev := cobra.EnableTraverseRunHooks
	cobra.EnableTraverseRunHooks = true
	t.Cleanup(func() { cobra.EnableTraverseRunHooks = prev })

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"aura", "organization", "list", "--format", format, debugArg})
	require.NoError(t, root.Execute())

	return out.String(), debugBuf.String()
}

// TestOrgList_DebugFlagDoesNotChangeStdout is the aura analog of query's
// TestRunQuery_DebugFlagDoesNotChangeStdout: stdout under --debug=true must be
// byte-identical to stdout under --debug=false for every --format, and the
// debug stream must actually carry wire diagnostics under --debug=true (proving
// debug ran, not that it was a no-op).
func TestOrgList_DebugFlagDoesNotChangeStdout(t *testing.T) {
	t.Setenv("NEO4J_DEBUG", "")
	srv := invarianceServer(t)

	for _, format := range []string{"json", "table", "toon"} {
		t.Run(format, func(t *testing.T) {
			stdoutOff, debugOff := runOrgListInvariance(t, srv.URL, format, "--debug=false")
			stdoutOn, debugOn := runOrgListInvariance(t, srv.URL, format, "--debug=true")

			assert.Equal(t, stdoutOff, stdoutOn,
				"--debug must not alter stdout bytes (debug output is routed to the stderr seam)")
			assert.NotEmpty(t, stdoutOn, "command must produce stdout")
			assert.Contains(t, stdoutOn, "acme")

			// Off path: nothing on the debug seam.
			assert.Empty(t, debugOff, "no debug output when --debug=false")

			// On path: debug seam carries the request/response wire dump.
			assert.Contains(t, debugOn, "[aura-debug] > GET")
			assert.Contains(t, debugOn, "/v2beta1/organizations")
			assert.Contains(t, debugOn, "[aura-debug] < 200")
			assert.Contains(t, debugOn, "elapsed")
		})
	}
}

// TestOrgList_DebugStdoutIsRealJSON guards against the byte-identity assertion
// degenerating into a comparison of two empty strings: it confirms the json
// stdout under --debug actually carries the rendered org data.
func TestOrgList_DebugStdoutIsRealJSON(t *testing.T) {
	t.Setenv("NEO4J_DEBUG", "")
	srv := invarianceServer(t)

	stdout, _ := runOrgListInvariance(t, srv.URL, "json", "--debug=true")
	require.NotEmpty(t, stdout)
	// json format renders the data envelope; confirm both org ids land on stdout.
	assert.Contains(t, stdout, "org-1")
	assert.Contains(t, stdout, "org-2")
}
