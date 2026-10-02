// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
)

func TestNewInstance_Normalises(t *testing.T) {
	tests := []struct {
		name       string
		raw        map[string]any
		wantStatus string
		wantProj   string
	}{
		{
			name:       "legacy_status and tenant_id are renamed",
			raw:        map[string]any{"id": "a", "legacy_status": "running", "tenant_id": "p1"},
			wantStatus: "running",
			wantProj:   "p1",
		},
		{
			name:       "native status and project_id win over legacy keys",
			raw:        map[string]any{"status": "paused", "legacy_status": "running", "project_id": "p2", "tenant_id": "p1"},
			wantStatus: "paused",
			wantProj:   "p2",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := newInstance(tc.raw)
			assert.Equal(t, tc.wantStatus, inst.Status)
			assert.Equal(t, tc.wantProj, inst.ProjectID)
			assert.Equal(t, tc.wantStatus, inst.Record["status"])
			assert.Equal(t, tc.wantProj, inst.Record["project_id"])
			assert.NotContains(t, inst.Record, "legacy_status")
			assert.NotContains(t, inst.Record, "tenant_id")
		})
	}
}

func TestNewInstance_RecordKeepsUnmodelledFieldsAndDoesNotMutateInput(t *testing.T) {
	raw := map[string]any{"id": "a", "tenant_id": "p1", "brand_new_field": map[string]any{"x": 1.0}, "storage": nil}

	inst := newInstance(raw)

	assert.Equal(t, map[string]any{"x": 1.0}, inst.Record["brand_new_field"], "unmodelled server fields must reach JSON output")
	assert.Contains(t, inst.Record, "storage", "explicit nulls are preserved")
	assert.Empty(t, inst.Storage)
	assert.Contains(t, raw, "tenant_id", "input must not be mutated")
}

func TestDecodeRows(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    int
		wantErr bool
	}{
		{name: "array", body: `{"data":[{"id":"a"},{"id":"b"}]}`, want: 2},
		{name: "empty array", body: `{"data":[]}`, want: 0},
		{name: "single object", body: `{"data":{"id":"a"}}`, want: 1},
		{name: "malformed json is an error, not a panic", body: `not json`, wantErr: true},
		{name: "missing data is an error", body: `{}`, wantErr: true},
		{name: "scalar data is an error", body: `{"data":"x"}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := decodeRows([]byte(tc.body))
			if tc.wantErr {
				require.Error(t, err)
				var ce *clierr.CLIError
				assert.True(t, errors.As(err, &ce), "want *clierr.CLIError, got %T", err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, rows, tc.want)
		})
	}
}
