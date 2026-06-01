// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCounters is a driver-free neo4j.Counters stub. The zero value reports no
// updates (ContainsUpdates returns false unless any counter is non-zero), so
// tests can construct a read-like response with fakeCounters{} and a write-like
// one by setting individual fields.
type fakeCounters struct {
	nodesCreated         int
	nodesDeleted         int
	relationshipsCreated int
	relationshipsDeleted int
	propertiesSet        int
	labelsAdded          int
	labelsRemoved        int
	indexesAdded         int
	indexesRemoved       int
	constraintsAdded     int
	constraintsRemoved   int
	systemUpdates        int
}

func (c fakeCounters) ContainsUpdates() bool {
	return c.nodesCreated|c.nodesDeleted|c.relationshipsCreated|c.relationshipsDeleted|
		c.propertiesSet|c.labelsAdded|c.labelsRemoved|c.indexesAdded|c.indexesRemoved|
		c.constraintsAdded|c.constraintsRemoved|c.systemUpdates != 0
}
func (c fakeCounters) NodesCreated() int           { return c.nodesCreated }
func (c fakeCounters) NodesDeleted() int           { return c.nodesDeleted }
func (c fakeCounters) RelationshipsCreated() int   { return c.relationshipsCreated }
func (c fakeCounters) RelationshipsDeleted() int   { return c.relationshipsDeleted }
func (c fakeCounters) PropertiesSet() int          { return c.propertiesSet }
func (c fakeCounters) LabelsAdded() int            { return c.labelsAdded }
func (c fakeCounters) LabelsRemoved() int          { return c.labelsRemoved }
func (c fakeCounters) IndexesAdded() int           { return c.indexesAdded }
func (c fakeCounters) IndexesRemoved() int         { return c.indexesRemoved }
func (c fakeCounters) ConstraintsAdded() int       { return c.constraintsAdded }
func (c fakeCounters) ConstraintsRemoved() int     { return c.constraintsRemoved }
func (c fakeCounters) SystemUpdates() int          { return c.systemUpdates }
func (c fakeCounters) ContainsSystemUpdates() bool { return c.systemUpdates != 0 }

// compile-time check that the stub satisfies the driver interface.
var _ neo4j.Counters = fakeCounters{}

func TestStatsFromCounters(t *testing.T) {
	t.Run("nil counters yields nil", func(t *testing.T) {
		assert.Nil(t, statsFromCounters(nil))
	})

	t.Run("no updates yields nil", func(t *testing.T) {
		assert.Nil(t, statsFromCounters(fakeCounters{}))
	})

	t.Run("updates copied field-for-field", func(t *testing.T) {
		got := statsFromCounters(fakeCounters{
			nodesCreated:         2,
			relationshipsCreated: 1,
			propertiesSet:        5,
			constraintsAdded:     1,
		})
		require.NotNil(t, got)
		assert.Equal(t, &writeStats{
			NodesCreated:         2,
			RelationshipsCreated: 1,
			PropertiesSet:        5,
			ConstraintsAdded:     1,
		}, got)
	})
}

func TestFormatStatsLine(t *testing.T) {
	tests := []struct {
		name  string
		stats *writeStats
		want  string
	}{
		{
			name:  "single counter singular wording",
			stats: &writeStats{NodesCreated: 1},
			want:  "1 node created",
		},
		{
			name:  "plural wording for >1",
			stats: &writeStats{NodesCreated: 3},
			want:  "3 nodes created",
		},
		{
			name: "only non-zero counters, in fixed order",
			stats: &writeStats{
				NodesCreated:         2,
				RelationshipsCreated: 1,
				PropertiesSet:        5,
				ConstraintsAdded:     1,
			},
			want: "2 nodes created, 1 relationship created, 5 properties set, 1 constraint added",
		},
		{
			name:  "schema-only counters",
			stats: &writeStats{IndexesAdded: 1, ConstraintsRemoved: 2},
			want:  "1 index added, 2 constraints removed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, formatStatsLine(tc.stats))
		})
	}
}

// TestRenderResult_JSON_ReadEnvelopeUnchanged locks the read-path JSON shape:
// a result with nil stats must serialise to exactly the four-key envelope with
// no "stats" key.
func TestRenderResult_JSON_ReadEnvelopeUnchanged(t *testing.T) {
	r := renderResult{columns: []string{"n"}, rows: []map[string]any{{"n": float64(1)}}}
	b, err := json.Marshal(r)
	require.NoError(t, err)

	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &keys))
	assert.NotContains(t, keys, "stats")
	assert.ElementsMatch(t, []string{"columns", "rows", "truncated", "arrays_truncated"}, mapKeys(keys))
}

// TestRenderResult_JSON_StatsOnlyWhenUpdates verifies the "stats" object appears
// (and only carries non-zero counters) when the result mutated state.
func TestRenderResult_JSON_StatsOnlyWhenUpdates(t *testing.T) {
	r := renderResult{
		columns: []string{},
		rows:    nil,
		stats:   &writeStats{NodesCreated: 2, PropertiesSet: 3},
	}
	b, err := json.Marshal(r)
	require.NoError(t, err)

	var decoded struct {
		Stats map[string]int `json:"stats"`
	}
	require.NoError(t, json.Unmarshal(b, &decoded))
	assert.Equal(t, map[string]int{"nodes_created": 2, "properties_set": 3}, decoded.Stats)
}

// TestRunStatementWithMode_PopulatesStats proves the seam's Counters flow into
// queryResult.Stats: a write-like response (counters report updates) yields a
// non-nil Stats; a read-like response (nil/empty counters) yields nil.
func TestRunStatementWithMode_PopulatesStats(t *testing.T) {
	t.Run("write response carries stats", func(t *testing.T) {
		withRunStatementSeam(t, func(_ context.Context, _ *conn, _ string, _ map[string]any, _ bool) (*queryResponse, error) {
			resp := &queryResponse{}
			resp.Counters = fakeCounters{nodesCreated: 1, propertiesSet: 2}
			return resp, nil
		})

		c := &conn{database: "neo4j"}
		res, err := runStatementWrite(context.Background(), c, "CREATE (n {x:1})", nil)
		require.NoError(t, err)
		require.NotNil(t, res.Stats)
		assert.Equal(t, 1, res.Stats.NodesCreated)
		assert.Equal(t, 2, res.Stats.PropertiesSet)
	})

	t.Run("read response carries no stats", func(t *testing.T) {
		withRunStatementSeam(t, func(_ context.Context, _ *conn, _ string, _ map[string]any, _ bool) (*queryResponse, error) {
			resp := &queryResponse{}
			resp.Counters = fakeCounters{}
			return resp, nil
		})

		c := &conn{database: "neo4j"}
		res, err := runStatement(context.Background(), c, "RETURN 1", nil)
		require.NoError(t, err)
		assert.Nil(t, res.Stats)
	})

	t.Run("batch path carries per-statement stats", func(t *testing.T) {
		withRunStatementsSeam(t, func(_ context.Context, _ *conn, statements []string, _ map[string]any, _ bool) ([]*queryResponse, error) {
			resps := make([]*queryResponse, len(statements))
			for i := range statements {
				resp := &queryResponse{}
				resp.Counters = fakeCounters{nodesCreated: i + 1}
				resps[i] = resp
			}
			return resps, nil
		})

		c := &conn{database: "neo4j"}
		results, err := runStatementsWithMode(context.Background(), c, []string{"CREATE (a)", "CREATE (b)"}, nil, false)
		require.NoError(t, err)
		require.Len(t, results, 2)
		require.NotNil(t, results[0].Stats)
		assert.Equal(t, 1, results[0].Stats.NodesCreated)
		assert.Equal(t, 2, results[1].Stats.NodesCreated)
	})
}

// TestRenderStatsLine_TableModeOnly verifies a stats line is printed after the
// table for a write result, prefixed per-statement when multiple statements
// ran, and that read results emit no line.
func TestRenderStatsLine_TableMode(t *testing.T) {
	t.Run("single write prints unprefixed stats line", func(t *testing.T) {
		cmd, cfg, stdout := newRenderCmd(t, "table")
		renderResults(cmd, cfg, []renderResult{
			{columns: []string{}, stats: &writeStats{NodesCreated: 2}},
		})
		out := stdout.String()
		assert.Contains(t, out, "2 nodes created")
		assert.NotContains(t, out, "statement 1:")
	})

	t.Run("multi prefixes each stats line", func(t *testing.T) {
		cmd, cfg, stdout := newRenderCmd(t, "table")
		renderResults(cmd, cfg, []renderResult{
			{columns: []string{}, stats: &writeStats{NodesCreated: 1}},
			{columns: []string{"n"}, rows: []map[string]any{{"n": float64(1)}}},
			{columns: []string{}, stats: &writeStats{PropertiesSet: 3}},
		})
		out := stdout.String()
		assert.Contains(t, out, "statement 1: 1 node created")
		assert.Contains(t, out, "statement 3: 3 properties set")
		assert.NotContains(t, out, "statement 2:")
	})

	t.Run("json mode prints no stats line", func(t *testing.T) {
		cmd, cfg, stdout := newRenderCmd(t, "json")
		renderResults(cmd, cfg, []renderResult{
			{columns: []string{}, stats: &writeStats{NodesCreated: 2}},
		})
		out := stdout.String()
		assert.NotContains(t, out, "nodes created")
	})
}

func mapKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
