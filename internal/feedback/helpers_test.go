// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// withToken gives the build a Mixpanel project token for the test.
func withToken(t *testing.T, token string) {
	t.Helper()
	prev := mixpanelToken
	mixpanelToken = token
	t.Cleanup(func() { mixpanelToken = prev })
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
