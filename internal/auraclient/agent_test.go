// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
)

func TestAgentPath(t *testing.T) {
	s := agentService{}
	good := Scope{OrgID: "org-1", ProjectID: "proj-1"}

	tests := []struct {
		name    string
		scope   Scope
		id      string
		suffix  []string
		want    string
		wantErr string
	}{
		{name: "collection", scope: good, want: "/organizations/org-1/projects/proj-1/agents"},
		{name: "member", scope: good, id: "a1", want: "/organizations/org-1/projects/proj-1/agents/a1"},
		{name: "invoke", scope: good, id: "a1", suffix: []string{"invoke"}, want: "/organizations/org-1/projects/proj-1/agents/a1/invoke"},
		{name: "agent id traversal", scope: good, id: "../../../x", wantErr: `invalid agent id "../../../x"`},
		{name: "agent id query injection", scope: good, id: "a?admin=1", wantErr: `invalid agent id "a?admin=1"`},
		{name: "org id traversal", scope: Scope{OrgID: "..", ProjectID: "proj-1"}, wantErr: `invalid organization id ".."`},
		{name: "project id slash", scope: Scope{OrgID: "org-1", ProjectID: "a/b"}, wantErr: `invalid project id "a/b"`},
		{name: "empty org", scope: Scope{ProjectID: "proj-1"}, wantErr: `invalid organization id ""`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.path(tc.scope, tc.id, tc.suffix...)
			if tc.wantErr != "" {
				require.Error(t, err)
				var ce *clierr.CLIError
				require.True(t, errors.As(err, &ce))
				assert.Contains(t, ce.Message, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestAgentSpecBody_CarriesEveryField(t *testing.T) {
	body := AgentSpec{
		Name: "n", Description: "d", DBID: "db", IsPrivate: true, Tools: []any{"t"},
		SystemPrompt: "p", IsMCPEnabled: true, Enabled: false,
	}.body()

	assert.Equal(t, map[string]any{
		"name": "n", "description": "d", "dbid": "db", "is_private": true, "tools": []any{"t"},
		"system_prompt": "p", "is_mcp_enabled": true, "enabled": false,
	}, body, "replace/create must send false booleans, not omit them")
}

func TestAgentPatchBody_OnlySendsSetFields(t *testing.T) {
	yes, no, name := true, false, "renamed"

	assert.Empty(t, AgentPatch{}.body())

	assert.Equal(t, map[string]any{"name": "renamed", "enabled": false, "is_private": true},
		AgentPatch{Name: &name, Enabled: &no, IsPrivate: &yes}.body())

	cleared := AgentPatch{Tools: []any{}}.body()
	assert.Contains(t, cleared, "tools", "an explicit empty tools list clears the tools")
	assert.NotContains(t, AgentPatch{}.body(), "tools")
}

func TestWithInvocationID(t *testing.T) {
	assert.NoError(t, withInvocationID(nil, "id-1"))

	plain := errors.New("boom")
	assert.Same(t, plain, withInvocationID(plain, ""), "no id leaves the error untouched")
	assert.EqualError(t, withInvocationID(plain, "id-1"), "boom (invocation id: id-1)")

	ce := clierr.NewUpstreamError("upstream failed")
	got := withInvocationID(fmt.Errorf("wrapped: %w", ce), "id-2")
	var out *clierr.CLIError
	require.True(t, errors.As(got, &out))
	assert.Contains(t, out.Message, "(invocation id: id-2)", "CLIError message is edited so envelope rendering keeps the id")
}

func TestDecodeBareRecords(t *testing.T) {
	rows, err := decodeBareRecords([]byte(`[{"id":"a"},{"id":"b"}]`))
	require.NoError(t, err)
	assert.Len(t, rows, 2)

	rows, err = decodeBareRecords([]byte(`{"id":"a"}`))
	require.NoError(t, err)
	assert.Len(t, rows, 1)

	_, err = decodeBareRecords([]byte(`not json`))
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "malformed body is a clierr, not a panic")

	_, err = decodeBareRecord([]byte(`[{"id":"a"},{"id":"b"}]`))
	assert.Error(t, err, "a single-record decode rejects several rows")
}
