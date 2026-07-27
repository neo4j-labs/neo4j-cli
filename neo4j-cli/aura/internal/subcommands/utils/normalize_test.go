// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package utils_test

import (
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeV2Beta1Response(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    api.ResponseData
		wantRows []map[string]any
	}{
		{
			name: "single: renames legacy_status and tenant_id",
			input: api.NewSingleValueResponseData(map[string]any{
				"id": "1", "name": "foo", "legacy_status": "running", "tenant_id": "proj-1",
			}),
			wantRows: []map[string]any{
				{"id": "1", "name": "foo", "status": "running", "project_id": "proj-1"},
			},
		},
		{
			name: "list: renames in every row",
			input: api.NewListResponseData([]map[string]any{
				{"id": "1", "legacy_status": "running", "tenant_id": "proj-1"},
				{"id": "2", "legacy_status": "paused", "tenant_id": "proj-2"},
			}),
			wantRows: []map[string]any{
				{"id": "1", "status": "running", "project_id": "proj-1"},
				{"id": "2", "status": "paused", "project_id": "proj-2"},
			},
		},
		{
			name: "native status wins over legacy_status",
			input: api.NewSingleValueResponseData(map[string]any{
				"id": "1", "status": "running", "legacy_status": "creating",
			}),
			wantRows: []map[string]any{
				{"id": "1", "status": "running"},
			},
		},
		{
			name: "native project_id wins over tenant_id",
			input: api.NewSingleValueResponseData(map[string]any{
				"id": "1", "project_id": "proj-native", "tenant_id": "proj-legacy",
			}),
			wantRows: []map[string]any{
				{"id": "1", "project_id": "proj-native"},
			},
		},
		{
			name: "no-op when neither legacy key is present",
			input: api.NewSingleValueResponseData(map[string]any{
				"id": "1", "name": "foo", "status": "running", "project_id": "proj-1",
			}),
			wantRows: []map[string]any{
				{"id": "1", "name": "foo", "status": "running", "project_id": "proj-1"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := utils.NormalizeV2Beta1Response(tc.input)
			assert.Equal(t, tc.wantRows, got.AsArray())
		})
	}
}

func TestNormalizeV2Beta1Response_PreservesType(t *testing.T) {
	t.Parallel()

	t.Run("single-item input returns single-item ResponseData", func(t *testing.T) {
		t.Parallel()
		input := api.NewSingleValueResponseData(map[string]any{"legacy_status": "running"})
		got := utils.NormalizeV2Beta1Response(input)
		_, isSingle := got.(api.SingleValueResponseData)
		assert.True(t, isSingle, "expected SingleValueResponseData but got %T", got)
	})

	t.Run("list input returns list ResponseData", func(t *testing.T) {
		t.Parallel()
		input := api.NewListResponseData([]map[string]any{{"legacy_status": "running"}})
		got := utils.NormalizeV2Beta1Response(input)
		_, isList := got.(api.ListResponseData)
		assert.True(t, isList, "expected ListResponseData but got %T", got)
	})
}

func TestNormalizeV2Beta1Response_DoesNotMutateInput(t *testing.T) {
	t.Parallel()

	src := map[string]any{"id": "1", "legacy_status": "running", "tenant_id": "proj-1"}
	input := api.NewSingleValueResponseData(src)

	_ = utils.NormalizeV2Beta1Response(input)

	assert.Equal(t, "running", src["legacy_status"], "legacy_status should be untouched on the source map")
	assert.Equal(t, "proj-1", src["tenant_id"], "tenant_id should be untouched on the source map")
	_, hasStatus := src["status"]
	assert.False(t, hasStatus, "source map should not gain a status key")
}
