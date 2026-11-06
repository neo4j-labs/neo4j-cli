// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/relate"
)

// fakeRelateStore is the embedded-backend test seam: it records every Run
// call (statements, atomic flag) and answers each statement through the
// responder. It runs the whole command flow without the engine.
type fakeRelateStore struct {
	statements      []string
	atomicFlags     []bool
	params          []map[string]any
	checkpoints     int
	closed          bool
	openedPath      string
	openedNamespace string
	respond         func(stmt string) (relate.StatementResult, error)
	failCheckpoint  error
}

func (f *fakeRelateStore) Run(statements []string, params map[string]any, atomic bool) ([]relate.StatementResult, error) {
	f.statements = append(f.statements, statements...)
	f.atomicFlags = append(f.atomicFlags, atomic)
	f.params = append(f.params, params)
	results := make([]relate.StatementResult, 0, len(statements))
	for _, stmt := range statements {
		res, err := f.respond(stmt)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}

func (f *fakeRelateStore) Checkpoint() error {
	f.checkpoints++
	return f.failCheckpoint
}

func (f *fakeRelateStore) Close() { f.closed = true }

// installFakeStore swaps the relate.OpenStore seam for one returning the
// fake, restoring the previous hook (the engine's, under the relate tag) on
// cleanup.
func installFakeStore(t *testing.T, f *fakeRelateStore) {
	t.Helper()
	orig := relate.OpenStore
	relate.OpenStore = func(path, namespace string) (relate.Store, error) {
		f.openedPath = path
		f.openedNamespace = namespace
		return f, nil
	}
	t.Cleanup(func() { relate.OpenStore = orig })
}

// explainOf shapes a read/write classification answer for an EXPLAIN
// preflight statement.
func explainOf(qt relate.QueryType) relate.StatementResult {
	return relate.StatementResult{QueryType: qt}
}

func resultOf(columns []string, rows [][]any) relate.StatementResult {
	return relate.StatementResult{Columns: columns, Rows: rows, QueryType: relate.QueryTypeRead}
}

func TestRunQueryRelate_RendersJSONEnvelopeAndSkipsServer(t *testing.T) {
	f := &fakeRelateStore{respond: func(stmt string) (relate.StatementResult, error) {
		switch stmt {
		case "EXPLAIN MATCH (n) RETURN n.name AS name":
			return explainOf(relate.QueryTypeRead), nil
		case "MATCH (n) RETURN n.name AS name":
			return resultOf([]string{"name"}, [][]any{{"Ada"}, {"Grace"}}), nil
		}
		return relate.StatementResult{}, errors.New("unexpected statement: " + stmt)
	}}
	installFakeStore(t, f)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--relate=/tmp/some-store",
		"MATCH (n) RETURN n.name AS name",
	)
	require.NoError(t, err)

	// Preflight and real run both reached the embedded store; no password
	// prompt or driver open happened (the run would have failed on either).
	assert.Equal(t, []string{
		"EXPLAIN MATCH (n) RETURN n.name AS name",
		"MATCH (n) RETURN n.name AS name",
	}, f.statements)

	var got decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, []string{"name"}, got.Columns)
	assert.Len(t, got.Rows, 2)
	assert.Equal(t, "Ada", got.Rows[0]["name"])

	// A read run does not checkpoint; the store is closed.
	assert.Equal(t, 0, f.checkpoints)
	assert.True(t, f.closed)
}

func TestRunQueryRelate_BareNameResolvesManagedPath(t *testing.T) {
	f := &fakeRelateStore{respond: func(stmt string) (relate.StatementResult, error) {
		if stmt == "EXPLAIN RETURN 1 AS one" {
			return explainOf(relate.QueryTypeRead), nil
		}
		return resultOf([]string{"one"}, [][]any{{int64(1)}}), nil
	}}
	installFakeStore(t, f)

	h := newRunHarness(t, "json")
	err := h.execute(t, "--relate=movies", "RETURN 1 AS one")
	require.NoError(t, err)

	assert.Equal(t, relate.ResolveStorePath("movies"), f.openedPath)
}

func TestRunQueryRelate_NamespacePassedThrough(t *testing.T) {
	f := &fakeRelateStore{respond: func(stmt string) (relate.StatementResult, error) {
		return resultOf(nil, nil), nil
	}}
	installFakeStore(t, f)

	h := newRunHarness(t, "json")
	require.NoError(t, h.execute(t, "--relate=movies", "--relate-namespace=system", "SHOW USERS"))

	assert.Equal(t, "system", f.openedNamespace)
}

func TestRunQueryRelate_WriteWithoutRwBlocked(t *testing.T) {
	f := &fakeRelateStore{respond: func(stmt string) (relate.StatementResult, error) {
		if stmt == "EXPLAIN CREATE (n:Person {name: 'Ada'})" {
			return explainOf(relate.QueryTypeReadWrite), nil
		}
		return relate.StatementResult{}, errors.New("real statement must not run without --rw")
	}}
	installFakeStore(t, f)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--relate=/tmp/store",
		"CREATE (n:Person {name: 'Ada'})",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pass --rw to allow it")

	// Only the preflight ran; nothing wrote, nothing checkpointed.
	assert.Equal(t, []string{"EXPLAIN CREATE (n:Person {name: 'Ada'})"}, f.statements)
	assert.Equal(t, 0, f.checkpoints)
}

func TestRunQueryRelate_WriteWithRwCheckpoints(t *testing.T) {
	f := &fakeRelateStore{respond: func(stmt string) (relate.StatementResult, error) {
		return relate.StatementResult{
			Columns:   []string{"n"},
			Rows:      [][]any{{"node"}},
			QueryType: relate.QueryTypeReadWrite,
			Bookmark:  "relate:1",
		}, nil
	}}
	installFakeStore(t, f)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--rw",
		"--relate=/tmp/store",
		"CREATE (n:Person {name: 'Ada'}) RETURN n",
	)
	require.NoError(t, err)

	// With --rw there is no preflight; the single write ran auto-commit and
	// triggered exactly one checkpoint.
	assert.Equal(t, []string{"CREATE (n:Person {name: 'Ada'}) RETURN n"}, f.statements)
	assert.Equal(t, []bool{false}, f.atomicFlags)
	assert.Equal(t, 1, f.checkpoints)
}

func TestRunQueryRelate_ExplainStatementDoesNotCheckpoint(t *testing.T) {
	f := &fakeRelateStore{respond: func(stmt string) (relate.StatementResult, error) {
		// EXPLAIN of a write reports the wrapped type but executes nothing.
		return relate.StatementResult{QueryType: relate.QueryTypeReadWrite}, nil
	}}
	installFakeStore(t, f)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--relate=/tmp/store",
		"EXPLAIN CREATE (n:Person {name: 'Ada'})",
	)
	require.NoError(t, err)

	assert.Equal(t, 0, f.checkpoints, "EXPLAIN must not trigger a checkpoint")
}

func TestRunQueryRelate_AtomicBatchRunsSingleTransaction(t *testing.T) {
	f := &fakeRelateStore{respond: func(stmt string) (relate.StatementResult, error) {
		return relate.StatementResult{QueryType: relate.QueryTypeWrite}, nil
	}}
	installFakeStore(t, f)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--rw", "--atomic",
		"--relate=/tmp/store",
		"CREATE (:Person {name: 'Ada'});\nCREATE (:Person {name: 'Bob'})",
	)
	require.NoError(t, err)

	require.Len(t, f.atomicFlags, 1)
	assert.True(t, f.atomicFlags[0], "multi-statement --atomic batch must run in one transaction")
	assert.Equal(t, 2, len(f.statements))
	assert.Equal(t, 1, f.checkpoints)
}

func TestRunQueryRelate_ParamsBoundToEmbeddedRun(t *testing.T) {
	f := &fakeRelateStore{respond: func(stmt string) (relate.StatementResult, error) {
		return resultOf([]string{"name"}, [][]any{{"Ada"}}), nil
	}}
	installFakeStore(t, f)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--relate=/tmp/store",
		"--param", "name=Ada",
		"MATCH (p:Person {name: $name}) RETURN p.name AS name",
	)
	require.NoError(t, err)

	require.Len(t, f.params, 2) // preflight + real run
	assert.Equal(t, map[string]any{"name": "Ada"}, f.params[0])
}

func TestRunQueryRelate_FlagConflictsUsageError(t *testing.T) {
	installFakeStore(t, &fakeRelateStore{respond: func(string) (relate.StatementResult, error) {
		return relate.StatementResult{}, nil
	}})

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"uri", []string{"--relate=/tmp/s", "--uri=neo4j://example:7687", "RETURN 1"}},
		{"username", []string{"--relate=/tmp/s", "--username=neo4j", "RETURN 1"}},
		{"password", []string{"--relate=/tmp/s", "--password=pw", "RETURN 1"}},
		{"database", []string{"--relate=/tmp/s", "--database=neo4j", "RETURN 1"}},
		{"credential", []string{"--relate=/tmp/s", "--credential=local", "RETURN 1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRunHarness(t, "json")
			err := h.execute(t, tc.args...)
			require.Error(t, err)
			var ce *clierr.CLIError
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, 2, ce.Code, "conflicting connection flags are a usage error")
			assert.Contains(t, err.Error(), "--relate cannot be used together with")
		})
	}
}

func TestRunQueryRelate_EngineErrorClassification(t *testing.T) {
	syntaxErr := &relate.EngineError{
		Code:    "Neo.ClientError.Statement.SyntaxError",
		Stage:   "compile",
		Message: "bad syntax",
	}

	t.Run("client-error code maps to validation error", func(t *testing.T) {
		got := categorizeRelateError(syntaxErr)
		var ce *clierr.CLIError
		require.ErrorAs(t, got, &ce)
		assert.Equal(t, 6, ce.Code)
	})

	t.Run("retryable engine error maps to upstream with re-run guidance", func(t *testing.T) {
		retryErr := &relate.EngineError{
			Code:  "Neo.TransientError.Transaction.Conflict",
			Stage: "runtime", Retryable: true, Message: "conflict",
		}
		got := categorizeRelateError(retryErr)
		var ce *clierr.CLIError
		require.ErrorAs(t, got, &ce)
		assert.Equal(t, 8, ce.Code)
		assert.Contains(t, ce.Suggestion, "re-run")
	})

	t.Run("internal stage maps to upstream with reopen guidance", func(t *testing.T) {
		internalErr := &relate.EngineError{
			Code:  "Neo.DatabaseError.General.UnknownFailure",
			Stage: "internal", Message: "handle poisoned",
		}
		got := categorizeRelateError(internalErr)
		var ce *clierr.CLIError
		require.ErrorAs(t, got, &ce)
		assert.Equal(t, 8, ce.Code)
	})

	t.Run("plain pre-statement refusal maps to validation error", func(t *testing.T) {
		got := categorizeRelateError(errors.New("parameter \"x\": unsupported parameter type chan"))
		var ce *clierr.CLIError
		require.ErrorAs(t, got, &ce)
		assert.Equal(t, 6, ce.Code)
	})

	t.Run("nil stays nil", func(t *testing.T) {
		assert.NoError(t, categorizeRelateError(nil))
	})
}

func TestMapRelateQueryType(t *testing.T) {
	assert.Equal(t, neo4j.QueryTypeReadOnly, mapRelateQueryType(relate.QueryTypeRead))
	assert.Equal(t, neo4j.QueryTypeWriteOnly, mapRelateQueryType(relate.QueryTypeWrite))
	assert.Equal(t, neo4j.QueryTypeReadWrite, mapRelateQueryType(relate.QueryTypeReadWrite))
	assert.Equal(t, neo4j.QueryTypeSchemaWrite, mapRelateQueryType(relate.QueryTypeSchema))
}

func TestRelateCounters(t *testing.T) {
	zero := relateCounters{stats: relate.StatementStats{}}
	assert.False(t, zero.ContainsUpdates())
	assert.Nil(t, statsFromCounters(zero), "all-zero counters must yield a nil writeStats")
	assert.False(t, zero.ContainsSystemUpdates())
	assert.Equal(t, 0, zero.SystemUpdates())

	c := relateCounters{stats: relate.StatementStats{
		NodesCreated:       2,
		ConstraintsRemoved: 1,
	}}
	assert.True(t, c.ContainsUpdates())
	got := statsFromCounters(c)
	require.NotNil(t, got)
	assert.Equal(t, &writeStats{NodesCreated: 2, ConstraintsRemoved: 1}, got)
}

func TestRunQueryRelate_WriteStatsRendered(t *testing.T) {
	writeResult := func() relate.StatementResult {
		return relate.StatementResult{
			Columns:   []string{"n"},
			Rows:      [][]any{{"node"}},
			QueryType: relate.QueryTypeReadWrite,
			Stats: relate.StatementStats{
				NodesCreated:         2,
				RelationshipsCreated: 1,
				PropertiesSet:        3,
				LabelsAdded:          2,
			},
		}
	}

	t.Run("json envelope carries the stats object", func(t *testing.T) {
		f := &fakeRelateStore{respond: func(string) (relate.StatementResult, error) {
			return writeResult(), nil
		}}
		installFakeStore(t, f)

		h := newRunHarness(t, "json")
		err := h.execute(t, "--rw", "--relate=/tmp/store", "CREATE (n)")
		require.NoError(t, err)

		var decoded struct {
			Stats map[string]int `json:"stats"`
		}
		require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &decoded))
		assert.Equal(t, map[string]int{
			"nodes_created":         2,
			"relationships_created": 1,
			"properties_set":        3,
			"labels_added":          2,
		}, decoded.Stats)
	})

	t.Run("read envelope omits stats", func(t *testing.T) {
		f := &fakeRelateStore{respond: func(string) (relate.StatementResult, error) {
			return resultOf([]string{"name"}, [][]any{{"Ada"}}), nil
		}}
		installFakeStore(t, f)

		h := newRunHarness(t, "json")
		err := h.execute(t, "--relate=/tmp/store", "MATCH (n) RETURN n.name AS name")
		require.NoError(t, err)
		assert.NotContains(t, h.stdout.String(), "stats")
	})

	t.Run("table mode prints the stats line", func(t *testing.T) {
		f := &fakeRelateStore{respond: func(string) (relate.StatementResult, error) {
			return writeResult(), nil
		}}
		installFakeStore(t, f)

		h := newRunHarness(t, "table")
		err := h.execute(t, "--rw", "--relate=/tmp/store", "CREATE (n)")
		require.NoError(t, err)
		assert.Contains(t, h.stdout.String(),
			"2 nodes created, 1 relationship created, 3 properties set, 2 labels added")
	})
}
