// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build cgo && relate

// End-to-end tests of the embedded backend against the real engine: the full
// command flow (flags, preflight, params, render) over an actual store
// created under t.TempDir(), including `query :schema --relate`.

package query

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelateEngineEndToEnd_WriteReadSchema(t *testing.T) {
	store := filepath.Join(t.TempDir(), "e2e")

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--rw",
		"--relate="+store,
		"CREATE (n:Person {name: $name})-[:KNOWS]->(m:Person {name: 'Grace'}) RETURN n.name AS name",
		"--param", "name=Ada",
	)
	require.NoError(t, err)

	var created decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &created))
	assert.Equal(t, []string{"name"}, created.Columns)
	assert.Equal(t, "Ada", created.Rows[0]["name"])

	h2 := newRunHarness(t, "json")
	err = h2.execute(t,
		"--relate="+store,
		"MATCH (p:Person) RETURN p.name AS name ORDER BY name",
	)
	require.NoError(t, err)

	var read decodedResult
	require.NoError(t, json.Unmarshal(h2.stdout.Bytes(), &read))
	assert.Len(t, read.Rows, 2)
	assert.Equal(t, "Ada", read.Rows[0]["name"])
	assert.Equal(t, "Grace", read.Rows[1]["name"])

	// Write guard over the embedded engine: a write without --rw is blocked
	// by the EXPLAIN preflight and the engine classifies it as a write.
	h3 := newRunHarness(t, "json")
	err = h3.execute(t,
		"--relate="+store,
		"CREATE (n:Person {name: 'Blocked'})",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--rw")

	// :schema introspects the embedded store through the same read path.
	h4 := newRunHarness(t, "json")
	err = h4.execute(t, ":schema", "--relate="+store)
	require.NoError(t, err)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(h4.stdout.Bytes(), &schema))
	nodes, ok := schema["nodes"].([]any)
	require.True(t, ok, "schema JSON must carry a nodes array")
	require.NotEmpty(t, nodes)
}

func TestRelateEngineEndToEnd_WriteStats(t *testing.T) {
	store := filepath.Join(t.TempDir(), "stats")

	// Table mode: the write prints the cypher-shell style line after the table.
	h := newRunHarness(t, "table")
	err := h.execute(t, "--rw", "--relate="+store,
		"CREATE (a:Person {name: 'Ada'})-[:KNOWS]->(b:Person {name: 'Bob'})")
	require.NoError(t, err)
	out := h.stdout.String()
	assert.Contains(t, out, "2 nodes created")
	assert.Contains(t, out, "1 relationship created")

	// JSON mode: the envelope carries the stats object with the non-zero counters.
	h2 := newRunHarness(t, "json")
	err = h2.execute(t, "--rw", "--relate="+store,
		"CREATE (c:Person {name: 'Cyd'})")
	require.NoError(t, err)

	var decoded struct {
		Stats map[string]int `json:"stats"`
	}
	require.NoError(t, json.Unmarshal(h2.stdout.Bytes(), &decoded))
	assert.Equal(t, map[string]int{
		"nodes_created":  1,
		"properties_set": 1,
		"labels_added":   1,
	}, decoded.Stats)

	// A read emits no stats at all.
	h3 := newRunHarness(t, "json")
	err = h3.execute(t, "--relate="+store, "MATCH (p:Person) RETURN count(p) AS c")
	require.NoError(t, err)
	assert.NotContains(t, h3.stdout.String(), "stats")
}
