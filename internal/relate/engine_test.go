// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build cgo && relate

// Engine round-trip tests against the real librelate_ffi (built in the Relate
// checkout the replace directive points at). They create throwaway stores
// under t.TempDir() and exercise the Store contract end to end: creation on
// open, typed values both directions, query-type classification, atomic
// rollback, read-only refusal, and the plan document.

package relate

import (
	"path/filepath"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
)

// openTestStore opens a fresh store in a temp dir and closes it on cleanup.
func openTestStore(t *testing.T) Store {
	t.Helper()
	require.NotNil(t, OpenStore, "engine hooks must be installed under the relate tag")
	s, err := OpenStore(filepath.Join(t.TempDir(), "store"), "")
	require.NoError(t, err)
	t.Cleanup(s.Close)
	return s
}

func TestEngineWriteReadRoundTrip(t *testing.T) {
	s := openTestStore(t)

	res, err := s.Run([]string{
		"CREATE (n:Person {name: $name, age: $age, born: date('1985-03-31'), tags: ['a', 'b']}) RETURN n.name AS name",
	}, map[string]any{"name": "Ada", "age": int64(40)}, false)
	require.NoError(t, err)
	require.Len(t, res, 1)

	assert.Equal(t, []string{"name"}, res[0].Columns)
	assert.Equal(t, [][]any{{"Ada"}}, res[0].Rows)
	assert.Equal(t, QueryTypeReadWrite, res[0].QueryType)

	require.NoError(t, s.Checkpoint())

	res, err = s.Run([]string{
		"MATCH (n:Person) RETURN n.name AS name, n.age AS age, n.born AS born, n.tags AS tags",
	}, nil, false)
	require.NoError(t, err)
	require.Len(t, res, 1)

	assert.Equal(t, QueryTypeRead, res[0].QueryType)
	row := res[0].Rows[0]
	assert.Equal(t, "Ada", row[0])
	assert.EqualValues(t, 40, row[1])
	// A DATE cell converts to the dbtype shape the CLI's Bolt path renders.
	born, ok := row[2].(dbtype.Date)
	require.True(t, ok, "expected dbtype.Date, got %T", row[2])
	assert.Equal(t, "1985-03-31", born.String())
	tags, ok := row[3].([]any)
	require.True(t, ok, "expected []any, got %T", row[3])
	assert.Equal(t, []any{"a", "b"}, tags)
}

func TestEngineNodeConversion(t *testing.T) {
	s := openTestStore(t)

	_, err := s.Run([]string{
		"CREATE (a:Person {name: 'Ada'})-[:KNOWS {since: 2020}]->(b:Person {name: 'Grace'})",
	}, nil, false)
	require.NoError(t, err)

	res, err := s.Run([]string{"MATCH p = (a:Person {name: 'Ada'})-[:KNOWS]->(b) RETURN p"}, nil, false)
	require.NoError(t, err)
	require.Len(t, res[0].Rows, 1)

	path, ok := res[0].Rows[0][0].(dbtype.Path)
	require.True(t, ok, "expected dbtype.Path, got %T", res[0].Rows[0][0])
	require.Len(t, path.Nodes, 2)
	assert.Equal(t, []string{"Person"}, path.Nodes[0].Labels)
	assert.Equal(t, map[string]any{"name": "Ada"}, path.Nodes[0].Props)
	require.Len(t, path.Relationships, 1)
	assert.Equal(t, "KNOWS", path.Relationships[0].Type)
	assert.EqualValues(t, 2020, path.Relationships[0].Props["since"])

	res, err = s.Run([]string{"MATCH (a:Person {name: 'Ada'}) RETURN a"}, nil, false)
	require.NoError(t, err)
	node, ok := res[0].Rows[0][0].(dbtype.Node)
	require.True(t, ok, "expected dbtype.Node, got %T", res[0].Rows[0][0])
	assert.NotEmpty(t, node.ElementId)
	assert.Equal(t, []string{"Person"}, node.Labels)
}

func TestEngineAtomicRollback(t *testing.T) {
	s := openTestStore(t)

	// The second statement fails on a unique-value conflict; the whole
	// atomic run must roll back, so the first statement's node is gone.
	_, err := s.Run([]string{
		"CREATE CONSTRAINT person_id IF NOT EXISTS FOR (p:Person) REQUIRE p.name IS UNIQUE",
	}, nil, false)
	require.NoError(t, err)

	_, err = s.Run([]string{
		"CREATE (:Person {name: 'Ada'})",
		"CREATE (:Person {name: 'Ada'})",
	}, nil, true)
	require.Error(t, err)

	res, err := s.Run([]string{"MATCH (n:Person) RETURN count(n) AS n"}, nil, false)
	require.NoError(t, err)
	assert.EqualValues(t, 0, res[0].Rows[0][0])
}

func TestEngineNonAtomicFailFastKeepsEarlierStatements(t *testing.T) {
	s := openTestStore(t)

	_, err := s.Run([]string{
		"CREATE CONSTRAINT person_id IF NOT EXISTS FOR (p:Person) REQUIRE p.name IS UNIQUE",
	}, nil, false)
	require.NoError(t, err)

	// First statement commits, second fails: the failure is returned and the
	// first statement's node stays (the CLI's default per-statement mode).
	_, err = s.Run([]string{
		"CREATE (:Person {name: 'Ada'})",
		"CREATE (:Person {name: 'Ada'})",
	}, nil, false)
	require.Error(t, err)

	res, err := s.Run([]string{"MATCH (n:Person) RETURN count(n) AS n"}, nil, false)
	require.NoError(t, err)
	assert.EqualValues(t, 1, res[0].Rows[0][0])
}

func TestEngineErrorClassification(t *testing.T) {
	s := openTestStore(t)

	// A syntax failure is an engine *EngineError carrying the Neo4j status
	// code, not a plain error.
	_, err := s.Run([]string{"MATCH (n RETURN n"}, nil, false)
	require.Error(t, err)
	var ee *EngineError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, "Neo.ClientError.Statement.SyntaxError", ee.Code)
	assert.Equal(t, "compile", ee.Stage)

	// A pre-statement refusal (interior NUL in the statement text) is a
	// plain error, not an *EngineError.
	_, err = s.Run([]string{"MATCH (n\x00) RETURN n"}, nil, false)
	require.Error(t, err)
	assert.NotErrorAs(t, err, &ee)

	// The command layer maps the engine error onto a typed CLIError so it
	// renders as a validation failure; mirror that mapping here to pin it.
	var ce *clierr.CLIError
	assert.NotErrorAs(t, err, &ce)
}

func TestEngineExplainReportsPlanAndWrappedType(t *testing.T) {
	s := openTestStore(t)

	res, err := s.Run([]string{"EXPLAIN CREATE (n:Person {name: 'A'}) RETURN n"}, nil, false)
	require.NoError(t, err)
	require.Len(t, res, 1)

	// EXPLAIN reports the wrapped write's type (the --rw guard relies on it)
	// and carries a plan tree without actuals.
	assert.Equal(t, QueryTypeReadWrite, res[0].QueryType)
	require.NotNil(t, res[0].Plan)
	assert.NotEmpty(t, res[0].Plan.Operator)
	assert.Nil(t, res[0].Plan.Actual)
}

func TestEngineReadOnlyRefusesMissingStoreAndWrites(t *testing.T) {
	// A missing store is NOT created by the read-only opener.
	_, err := OpenStoreReadOnly(filepath.Join(t.TempDir(), "missing"), "")
	require.Error(t, err)
	var ce *clierr.CLIError
	assert.NotErrorAs(t, err, &ce) // engine refusal, not a CLI usage error

	s, err := OpenStoreReadOnly(mustWriteStore(t), "")
	require.NoError(t, err)
	t.Cleanup(s.Close)

	_, err = s.Run([]string{"CREATE (:Person {name: 'A'})"}, nil, false)
	require.Error(t, err)

	require.Error(t, s.Checkpoint())
}

func TestEngineNamespaceConnect(t *testing.T) {
	// The system namespace connects and answers a system query.
	s, err := OpenStore(filepath.Join(t.TempDir(), "sys"), "system")
	require.NoError(t, err)
	t.Cleanup(s.Close)

	res, err := s.Run([]string{"SHOW DATABASES"}, nil, false)
	require.NoError(t, err)
	require.NotEmpty(t, res)
}

// mustWriteStore creates a store with one node and returns its path.
func mustWriteStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "seeded")
	s, err := OpenStore(path, "")
	require.NoError(t, err)
	_, err = s.Run([]string{"CREATE (:Person {name: 'Ada'})"}, nil, false)
	require.NoError(t, err)
	s.Close()
	return path
}

func TestEngineStats(t *testing.T) {
	s := openTestStore(t)

	res, err := s.Run([]string{"RETURN 1 AS one"}, nil, false)
	require.NoError(t, err)
	assert.Equal(t, StatementStats{}, res[0].Stats, "a read reports all-zero counters")

	res, err = s.Run([]string{
		"CREATE (a:Person {name: 'Ada'})-[:KNOWS]->(b:Person {name: 'Bob'})",
	}, nil, false)
	require.NoError(t, err)
	assert.Equal(t, StatementStats{
		NodesCreated:         2,
		RelationshipsCreated: 1,
		PropertiesSet:        2,
		LabelsAdded:          2,
	}, res[0].Stats)
}
