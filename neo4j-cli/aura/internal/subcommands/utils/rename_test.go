// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package utils_test

import (
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/stretchr/testify/assert"
)

func TestRenameResponseField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    api.ResponseData
		from     string
		to       string
		wantRows []map[string]any
	}{
		{
			name:  "single-item response: renames key",
			input: api.NewSingleValueResponseData(map[string]any{"id": "1", "tenant_id": "proj-1", "name": "foo"}),
			from:  "tenant_id",
			to:    "project_id",
			wantRows: []map[string]any{
				{"id": "1", "project_id": "proj-1", "name": "foo"},
			},
		},
		{
			name: "list response: renames key in every row",
			input: api.NewListResponseData([]map[string]any{
				{"id": "1", "tenant_id": "proj-1"},
				{"id": "2", "tenant_id": "proj-2"},
			}),
			from: "tenant_id",
			to:   "project_id",
			wantRows: []map[string]any{
				{"id": "1", "project_id": "proj-1"},
				{"id": "2", "project_id": "proj-2"},
			},
		},
		{
			name:     "single-item response: no-op when key is absent",
			input:    api.NewSingleValueResponseData(map[string]any{"id": "1", "name": "foo"}),
			from:     "tenant_id",
			to:       "project_id",
			wantRows: []map[string]any{{"id": "1", "name": "foo"}},
		},
		{
			name: "list response: no-op when key is absent",
			input: api.NewListResponseData([]map[string]any{
				{"id": "1", "name": "foo"},
			}),
			from:     "tenant_id",
			to:       "project_id",
			wantRows: []map[string]any{{"id": "1", "name": "foo"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := utils.RenameResponseField(tc.input, tc.from, tc.to)
			assert.Equal(t, tc.wantRows, got.AsArray())
		})
	}
}

func TestRenameResponseField_PreservesType(t *testing.T) {
	t.Parallel()

	t.Run("single-item input returns single-item ResponseData", func(t *testing.T) {
		t.Parallel()
		input := api.NewSingleValueResponseData(map[string]any{"tenant_id": "proj-1"})
		got := utils.RenameResponseField(input, "tenant_id", "project_id")
		_, isSingle := got.(api.SingleValueResponseData)
		assert.True(t, isSingle, "expected SingleValueResponseData but got %T", got)
	})

	t.Run("list input returns list ResponseData", func(t *testing.T) {
		t.Parallel()
		input := api.NewListResponseData([]map[string]any{{"tenant_id": "proj-1"}})
		got := utils.RenameResponseField(input, "tenant_id", "project_id")
		_, isList := got.(api.ListResponseData)
		assert.True(t, isList, "expected ListResponseData but got %T", got)
	})
}
