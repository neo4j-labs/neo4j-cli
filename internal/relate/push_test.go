// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// planStore answers the shape queries BuildPushPlan issues.
type planStore struct {
	responses  map[string]StatementResult
	statements []string
}

func (s *planStore) Run(statements []string, params map[string]any, atomic bool) ([]StatementResult, error) {
	results := make([]StatementResult, 0, len(statements))
	for _, stmt := range statements {
		s.statements = append(s.statements, stmt)
		res, ok := s.responses[stmt]
		if !ok {
			res = StatementResult{Rows: [][]any{}}
		}
		results = append(results, res)
	}
	return results, nil
}

func (s *planStore) Checkpoint() error { return nil }
func (s *planStore) Close()            {}

// shapeResponse is the (labels(a), type, labels(b), count) row shape.
func relShapeRow(startLabels []string, typ string, endLabels []string, count int64) []any {
	return []any{toAny(startLabels), typ, toAny(endLabels), count}
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func labelsResult(labels ...string) StatementResult {
	rows := make([][]any, len(labels))
	for i, l := range labels {
		rows[i] = []any{l}
	}
	return StatementResult{Columns: []string{"label"}, Rows: rows}
}

const shapeQuery = "MATCH (a)-[r]->(b) RETURN labels(a) AS sl, type(r) AS t, labels(b) AS el, count(*) AS c"

func TestBuildPushPlan(t *testing.T) {
	keys := PushKeySpec{"Person": "email", "Movie": "title"}

	t.Run("full store scope with keyed labels", func(t *testing.T) {
		s := &planStore{responses: map[string]StatementResult{
			shapeQuery: {Columns: []string{"sl", "t", "el", "c"}, Rows: [][]any{
				relShapeRow([]string{"Person"}, "ACTED_IN", []string{"Movie"}, 3),
			}},
			"CALL db.labels()":                        labelsResult("Movie", "Person"),
			"MATCH (n:`Movie`) RETURN count(n) AS c":  {Columns: []string{"c"}, Rows: [][]any{{int64(2)}}},
			"MATCH (n:`Person`) RETURN count(n) AS c": {Columns: []string{"c"}, Rows: [][]any{{int64(5)}}},
		}}

		plan, err := BuildPushPlan(s, nil, nil, keys)
		require.NoError(t, err)
		require.Len(t, plan.Labels, 2)
		assert.Equal(t, "Movie", plan.Labels[0].Label)
		assert.Equal(t, "title", plan.Labels[0].Key)
		assert.EqualValues(t, 2, plan.Labels[0].Count)
		assert.Equal(t, "Person", plan.Labels[1].Label)
		require.Len(t, plan.Rels, 1)
		g := plan.Rels[0]
		assert.Equal(t, "ACTED_IN", g.Type)
		assert.Equal(t, "Person", g.StartLabel)
		assert.Equal(t, "email", g.StartKey)
		assert.Equal(t, "Movie", g.EndLabel)
		assert.Equal(t, "title", g.EndKey)
		assert.Empty(t, plan.UnkeyedLabels)
		assert.Zero(t, plan.UnkeyedRels)
	})

	t.Run("unkeyed labels are named, not failed", func(t *testing.T) {
		s := &planStore{responses: map[string]StatementResult{
			shapeQuery:         {Rows: [][]any{}},
			"CALL db.labels()": labelsResult("Person", "Temp"),
		}}

		plan, err := BuildPushPlan(s, nil, nil, keys)
		require.NoError(t, err)
		assert.Equal(t, []string{"Temp"}, plan.UnkeyedLabels)
		require.Len(t, plan.Labels, 1)
	})

	t.Run("type filter narrows rel groups", func(t *testing.T) {
		s := &planStore{responses: map[string]StatementResult{
			shapeQuery: {Columns: []string{"sl", "t", "el", "c"}, Rows: [][]any{
				relShapeRow([]string{"Person"}, "ACTED_IN", []string{"Movie"}, 3),
				relShapeRow([]string{"Person"}, "KNOWS", []string{"Person"}, 1),
			}},
			"MATCH (n:`Person`) RETURN count(n) AS c": {Rows: [][]any{{int64(5)}}},
		}}

		plan, err := BuildPushPlan(s, []string{"Person"}, []string{"KNOWS"}, keys)
		require.NoError(t, err)
		require.Len(t, plan.Rels, 1)
		assert.Equal(t, "KNOWS", plan.Rels[0].Type)
	})

	t.Run("rels with unkeyed endpoints count as skipped", func(t *testing.T) {
		s := &planStore{responses: map[string]StatementResult{
			shapeQuery: {Columns: []string{"sl", "t", "el", "c"}, Rows: [][]any{
				relShapeRow([]string{"Person"}, "LIKES", []string{"Temp"}, 7),
			}},
			"MATCH (n:`Person`) RETURN count(n) AS c": {Rows: [][]any{{int64(5)}}},
		}}

		plan, err := BuildPushPlan(s, []string{"Person"}, nil, keys)
		require.NoError(t, err)
		assert.Empty(t, plan.Rels)
		assert.EqualValues(t, 7, plan.UnkeyedRels)
	})
}

func TestParseMergeKey(t *testing.T) {
	label, key, err := ParseMergeKey("Person.email")
	require.NoError(t, err)
	assert.Equal(t, "Person", label)
	assert.Equal(t, "email", key)

	for _, bad := range []string{"Person", ".email", "Person.", "", "Person.email.extra"} {
		_, _, err := ParseMergeKey(bad)
		require.Error(t, err, bad)
		assert.Contains(t, err.Error(), "Label.property")
	}
}

func TestPushStatements(t *testing.T) {
	assert.Equal(t,
		"UNWIND $rows AS row MERGE (n:`Person` {`email`: row.key}) SET n = row.props RETURN row.key AS key",
		PushNodesStatement("Person", "email"))

	g := PushRelGroup{Type: "ACTED_IN", StartLabel: "Person", StartKey: "email", EndLabel: "Movie", EndKey: "title"}
	assert.Equal(t,
		"UNWIND $rows AS row MATCH (a:`Person` {`email`: row.start}), (b:`Movie` {`title`: row.end}) "+
			"MERGE (a)-[r:`ACTED_IN`]->(b) SET r = row.props RETURN row.start AS start, row.end AS end",
		PushRelsStatement(g, ""))

	assert.Equal(t,
		"UNWIND $rows AS row MATCH (a:`Person` {`email`: row.start}), (b:`Movie` {`title`: row.end}) "+
			"MERGE (a)-[r:`ACTED_IN` {`roles`: row.rkey}]->(b) SET r = row.props RETURN row.start AS start, row.end AS end",
		PushRelsStatement(g, "roles"))
}

func TestReadPushNodes(t *testing.T) {
	t.Run("rows carry key and props", func(t *testing.T) {
		s := &planStore{responses: map[string]StatementResult{
			"MATCH (n:`Person`) RETURN n.`email` AS key, properties(n) AS props": {
				Columns: []string{"key", "props"},
				Rows: [][]any{
					{"ada@x", map[string]any{"name": "Ada"}},
					{"bob@x", map[string]any{"name": "Bob"}},
				},
			},
		}}
		rows, err := ReadPushNodes(s, "Person", "email")
		require.NoError(t, err)
		require.Len(t, rows, 2)
		assert.Equal(t, "ada@x", rows[0]["key"])
		assert.Equal(t, map[string]any{"name": "Ada"}, rows[0]["props"])
	})

	t.Run("null key is an error naming label and key", func(t *testing.T) {
		s := &planStore{responses: map[string]StatementResult{
			"MATCH (n:`Person`) RETURN n.`email` AS key, properties(n) AS props": {
				Columns: []string{"key", "props"},
				Rows:    [][]any{{"ada@x", map[string]any{}}, {nil, map[string]any{}}},
			},
		}}
		_, err := ReadPushNodes(s, "Person", "email")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "1 nodes of label Person")
		assert.Contains(t, err.Error(), "email")
	})
}

func TestReadPushRels(t *testing.T) {
	s := &planStore{responses: map[string]StatementResult{
		"MATCH (a:`Person`)-[r:`KNOWS`]->(b:`Person`) RETURN a.`email` AS start, b.`email` AS end, properties(r) AS props": {
			Columns: []string{"start", "end", "props"},
			Rows:    [][]any{{"ada@x", "bob@x", map[string]any{"since": int64(2020)}}},
		},
	}}
	rows, err := ReadPushRels(s, PushRelGroup{Type: "KNOWS", StartLabel: "Person", StartKey: "email", EndLabel: "Person", EndKey: "email"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "ada@x", rows[0]["start"])
	assert.Equal(t, "bob@x", rows[0]["end"])
	assert.Equal(t, map[string]any{"since": int64(2020)}, rows[0]["props"])
}

// keyedLabel picks the first sorted label with a key.
func TestKeyedLabel(t *testing.T) {
	keys := PushKeySpec{"Movie": "title", "Person": "email"}
	label, key := keyedLabel([]string{"Person", "Movie"}, keys)
	assert.Equal(t, "Movie", label)
	assert.Equal(t, "title", key)

	label, key = keyedLabel([]string{"Temp"}, keys)
	assert.Equal(t, "", label)
	assert.Equal(t, "", key)
}
