// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
)

func TestScopedResourcePath(t *testing.T) {
	good := Scope{OrgID: "org-1", ProjectID: "proj-1"}

	tests := []struct {
		name     string
		scope    Scope
		resource string
		idType   string
		id       string
		suffix   []string
		want     string
		wantErr  string
	}{
		{name: "collection", scope: good, resource: "agents", idType: "agent",
			want: "/organizations/org-1/projects/proj-1/agents"},
		{name: "member", scope: good, resource: "agents", idType: "agent", id: "a1",
			want: "/organizations/org-1/projects/proj-1/agents/a1"},
		{name: "member sub-resource", scope: good, resource: "agents", idType: "agent", id: "a1", suffix: []string{"invoke"},
			want: "/organizations/org-1/projects/proj-1/agents/a1/invoke"},
		{name: "several suffixes", scope: good, resource: "agents", idType: "agent", id: "a1", suffix: []string{"x", "y"},
			want: "/organizations/org-1/projects/proj-1/agents/a1/x/y"},
		{name: "nested collection", scope: good, resource: "agents/evaluation-datasets", idType: "evaluation-dataset",
			want: "/organizations/org-1/projects/proj-1/agents/evaluation-datasets"},
		{name: "nested member", scope: good, resource: "agents/evaluation-datasets", idType: "evaluation-dataset", id: "ds-1",
			want: "/organizations/org-1/projects/proj-1/agents/evaluation-datasets/ds-1"},
		// An empty id builds the collection path: the id segment is omitted
		// and, unlike a real id, "" is not validated.
		{name: "empty id is the collection path", scope: good, resource: "agents/evaluation-runs", idType: "evaluation-run", id: "",
			want: "/organizations/org-1/projects/proj-1/agents/evaluation-runs"},
		{name: "org traversal", scope: Scope{OrgID: "..", ProjectID: "proj-1"}, resource: "agents", idType: "agent",
			wantErr: `invalid organization id ".."`},
		{name: "empty org", scope: Scope{ProjectID: "proj-1"}, resource: "agents", idType: "agent",
			wantErr: `invalid organization id ""`},
		{name: "project slash", scope: Scope{OrgID: "org-1", ProjectID: "a/b"}, resource: "agents", idType: "agent",
			wantErr: `invalid project id "a/b"`},
		{name: "id traversal", scope: good, resource: "agents", idType: "agent", id: "../../../x",
			wantErr: `invalid agent id "../../../x"`},
		{name: "id query injection", scope: good, resource: "agents", idType: "agent", id: "a?admin=1",
			wantErr: `invalid agent id "a?admin=1"`},
		{name: "id validation names the id type", scope: good, resource: "agents/evaluation-runs", idType: "evaluation-run", id: "a/b",
			wantErr: `invalid evaluation-run id "a/b"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scopedResourcePath(tc.scope, tc.resource, tc.idType, tc.id, tc.suffix...)
			if tc.wantErr != "" {
				require.Error(t, err)
				var ce *clierr.CLIError
				require.True(t, errors.As(err, &ce))
				assert.Equal(t, 6, ce.Code, "a bad ID is a validation error")
				assert.Equal(t, tc.wantErr, ce.Message)
				assert.Equal(t, tc.wantErr, err.Error(), "ValidateResourceID's error is returned unwrapped and unreworded")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
