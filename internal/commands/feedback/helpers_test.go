// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/flags"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestConfig returns a Config over an in-memory filesystem with an empty
// config.json and the feedback flag on.
func newTestConfig(t *testing.T) *clicfg.Config {
	t.Helper()
	fs, err := testfs.GetTestFs("{}", "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test-version")
	cfg.Flags.SetForTest("flag.feedback-command", true)
	t.Setenv("DO_NOT_TRACK", "") // a developer's own setting must not leak into tests
	return cfg
}

// runCmd executes a leaf with args, capturing stdout and stderr separately.
func runCmd(t *testing.T, cmd *cobra.Command, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), errb.String(), err
}

// testRoot builds a tiny command tree used by the policy tests.
func testRoot() *cobra.Command {
	root := &cobra.Command{Use: "neo4j-cli"}
	query := &cobra.Command{Use: "query", Run: func(*cobra.Command, []string) {}}
	cred := &cobra.Command{Use: "credential"}
	dbms := &cobra.Command{Use: "dbms"}
	add := &cobra.Command{Use: "add", Run: func(*cobra.Command, []string) {}}
	hidden := &cobra.Command{Use: "secret-thing", Hidden: true, Run: func(*cobra.Command, []string) {}}
	fb := &cobra.Command{Use: "feedback"}
	submit := &cobra.Command{Use: "submit", Run: func(*cobra.Command, []string) {}}
	dbms.AddCommand(add)
	cred.AddCommand(dbms)
	fb.AddCommand(submit)
	root.AddCommand(query, cred, hidden, fb)
	return root
}

// newTestApp mounts the real feedback tree under a stub root with a `query`
// command so the --command allow-list has something to match.
func newTestApp(cfg *clicfg.Config) *cobra.Command {
	root := &cobra.Command{Use: "neo4j-cli", SilenceErrors: true}
	// The real root's --format, --rw and write gate, as in internal/cli/app.go.
	cobra.EnableTraverseRunHooks = true
	flags.RegisterOutputFlag(root, cfg)
	flags.RegisterRwFlag(root)
	root.PersistentPreRunE = flags.ComposeRootPersistentPreRunE(cfg)
	root.AddCommand(&cobra.Command{Use: "query", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(NewCmd(cfg))
	return root
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ce *clierr.CLIError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return 1
}

// enableFor authorizes feedback against endpoint for an hour.
func enableFor(t *testing.T, cfg *clicfg.Config, endpoint string) {
	t.Helper()
	require.NoError(t, cfg.Global.Set(keyEndpoint, endpoint))
	require.NoError(t, cfg.Global.Set(keyAuthorizedUntil, now().Add(time.Hour).UTC().Format(time.RFC3339)))
}

func parseJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m), "stdout must be exactly one JSON document, got %q", s)
	return m
}

// withSeams pins the agent seams for the duration of a test. Without an agent
// the caller is a person at a terminal; asScript overrides that.
func withSeams(t *testing.T, agentDetected bool, harness string) {
	t.Helper()
	od, oh, oi := detectAgent, harnessLabel, invokerLabel
	detectAgent = func() bool { return agentDetected }
	harnessLabel = func() string { return harness }
	invokerLabel = func() string {
		if agentDetected {
			return "agent"
		}
		return "human"
	}
	t.Cleanup(func() { detectAgent, harnessLabel, invokerLabel = od, oh, oi })
}

// lastProps returns the properties of the single event in a recorded /track body.
func lastProps(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var events []struct {
		Event      string         `json:"event"`
		Properties map[string]any `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(body, &events), "body must be a JSON array: %q", body)
	require.Len(t, events, 1)
	assert.Equal(t, "NEO4J-CLI_AGENT_FEEDBACK", events[0].Event)
	return events[0].Properties
}

// withToken gives the build a Mixpanel project token for the test, so Send will
// talk to a non-loopback destination (a test proxy stands in for it).
func withToken(t *testing.T, token string) {
	t.Helper()
	old := mixpanelToken
	mixpanelToken = token
	t.Cleanup(func() { mixpanelToken = old })
}

// asScript makes the caller a script: no agent, and stdin is not a terminal.
func asScript(t *testing.T) {
	t.Helper()
	oi := invokerLabel
	invokerLabel = func() string { return "script" }
	t.Cleanup(func() { invokerLabel = oi })
}
