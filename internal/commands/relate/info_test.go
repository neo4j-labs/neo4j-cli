// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/relate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelateInfo_MissingStoreIsUsageError(t *testing.T) {
	installFakeOpenReadOnly(t, &fakeStore{})

	h := newHarness(t, "json")
	err := h.execute(t, "info", "movies")
	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), "no Relate store at")
}

func TestRelateInfo_GathersMetadata(t *testing.T) {
	f := &fakeStore{respond: func(stmt string) (relate.StatementResult, error) {
		switch stmt {
		case "CALL dbms.components()":
			return relate.StatementResult{
				Columns: []string{"name", "versions", "edition"},
				Rows:    [][]any{{"Relate", []any{"0.16.1"}, "enterprise"}},
			}, nil
		case "MATCH (n) RETURN count(n) AS nodes":
			return relate.StatementResult{Columns: []string{"nodes"}, Rows: [][]any{{int64(42)}}}, nil
		case "MATCH ()-[r]->() RETURN count(r) AS relationships":
			return relate.StatementResult{Columns: []string{"relationships"}, Rows: [][]any{{int64(7)}}}, nil
		case "CALL db.labels() YIELD label WITH label ORDER BY label RETURN collect(label) AS labels":
			return relate.StatementResult{Columns: []string{"labels"}, Rows: [][]any{{[]any{"Movie", "Person"}}}}, nil
		case "CALL db.relationshipTypes() YIELD relationshipType WITH relationshipType ORDER BY relationshipType RETURN collect(relationshipType) AS relationship_types":
			return relate.StatementResult{Columns: []string{"relationship_types"}, Rows: [][]any{{[]any{"ACTED_IN"}}}}, nil
		case "SHOW INDEXES":
			return relate.StatementResult{Columns: []string{"name"}, Rows: [][]any{{"i1"}, {"i2"}}}, nil
		case "SHOW CONSTRAINTS":
			return relate.StatementResult{Columns: []string{"name"}, Rows: [][]any{{"c1"}}}, nil
		}
		return relate.StatementResult{}, errors.New("unexpected statement: " + stmt)
	}}
	installFakeOpenReadOnly(t, f)

	h := newHarness(t, "json")
	path := h.seedStore(t, "movies")

	require.NoError(t, h.execute(t, "info", "movies"))

	assert.Equal(t, path, f.openedPath)

	var got map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	row := got
	assert.Equal(t, "movies", row["name"])
	assert.Equal(t, "Relate", row["engine"])
	assert.Equal(t, "0.16.1", row["version"])
	assert.EqualValues(t, 42, row["nodes"])
	assert.EqualValues(t, 7, row["relationships"])
	assert.Equal(t, []any{"Movie", "Person"}, row["labels"])
	assert.Equal(t, []any{"ACTED_IN"}, row["relationship_types"])
	assert.EqualValues(t, 2, row["indexes"])
	assert.EqualValues(t, 1, row["constraints"])
	assert.EqualValues(t, 2, row["size_bytes"], "manifest.json is 2 bytes")
}

func TestRelateInfo_ToleratesFailingProbe(t *testing.T) {
	f := &fakeStore{respond: func(stmt string) (relate.StatementResult, error) {
		if stmt == "MATCH (n) RETURN count(n) AS nodes" {
			return relate.StatementResult{}, errors.New("engine refused")
		}
		return relate.StatementResult{Columns: []string{"x"}, Rows: [][]any{{int64(1)}}}, nil
	}}
	installFakeOpenReadOnly(t, f)

	h := newHarness(t, "json")
	h.seedStore(t, "movies")

	require.NoError(t, h.execute(t, "info", "movies"), "a failing probe must not fail the whole info")

	var got map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.NotContains(t, got, "nodes", "the failed probe's field is absent")
	assert.Contains(t, got, "name")
}
