// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"context"
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/relate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// remoteCall is one captured remoteExecute invocation.
type remoteCall struct {
	query  string
	params map[string]any
	write  bool
}

// installFakeRemote swaps the remoteExecute seam: SHOW CONSTRAINTS answers
// the canned constraints, writes return one identity row per input row, and
// everything is captured.
func installFakeRemote(t *testing.T, constraintRows [][]any) *[]remoteCall {
	t.Helper()
	var calls []remoteCall
	orig := remoteExecute
	remoteExecute = func(ctx context.Context, driver neo4j.Driver, database, query string, params map[string]any, write bool) ([]string, [][]any, error) {
		calls = append(calls, remoteCall{query: query, params: params, write: write})
		if query == "SHOW CONSTRAINTS YIELD name, type, entityType, labelsOrTypes, properties" {
			return []string{"name", "type", "entityType", "labelsOrTypes", "properties"}, constraintRows, nil
		}
		// A write answers with one row per input row (everything matched).
		rows := params["rows"].([]any)
		out := make([][]any, len(rows))
		for i := range rows {
			out[i] = []any{"matched"}
		}
		return []string{"key"}, out, nil
	}
	t.Cleanup(func() { remoteExecute = orig })
	return &calls
}

// constraintRow builds one SHOW CONSTRAINTS row.
func constraintRow(name, typ, entity string, labels, props []any) []any {
	return []any{name, typ, entity, labels, props}
}

// pushStore fakes the local store's answers for plan + read queries.
func pushStore(t *testing.T) *fakeStore {
	t.Helper()
	f := &fakeStore{respond: func(stmt string) (relate.StatementResult, error) {
		switch {
		case stmt == "MATCH (a)-[r]->(b) RETURN labels(a) AS sl, type(r) AS t, labels(b) AS el, count(*) AS c":
			return relate.StatementResult{
				Columns: []string{"sl", "t", "el", "c"},
				Rows:    [][]any{{[]any{"Person"}, "KNOWS", []any{"Person"}, int64(2)}},
			}, nil
		case stmt == "CALL db.labels()":
			return relate.StatementResult{Columns: []string{"label"}, Rows: [][]any{{"Person"}}}, nil
		case stmt == "MATCH (n:`Person`) RETURN count(n) AS c":
			return relate.StatementResult{Columns: []string{"c"}, Rows: [][]any{{int64(2)}}}, nil
		case strings.Contains(stmt, "AS key, properties(n) AS props"):
			// Node read for any key property.
			return relate.StatementResult{
				Columns: []string{"key", "props"},
				Rows: [][]any{
					{"ada@x", map[string]any{"name": "Ada"}},
					{"bob@x", map[string]any{"name": "Bob"}},
				},
			}, nil
		case strings.Contains(stmt, "AS start") && strings.Contains(stmt, "AS end"):
			// Rel read for any keys.
			return relate.StatementResult{
				Columns: []string{"start", "end", "props"},
				Rows: [][]any{
					{"ada@x", "bob@x", map[string]any{"since": int64(2020)}},
					{"bob@x", "ada@x", map[string]any{"since": int64(2021)}},
				},
			}, nil
		}
		return relate.StatementResult{}, nil
	}}
	installFakeOpen(t, f)
	return f
}

// seedStoreDir materializes the store-looking directory push requires
// (manifest.json marker included).
func (h *harness) seedStoreDir(t *testing.T, name string) string {
	t.Helper()
	path := h.seedStore(t, name)
	return path
}

func TestRelatePush_UpsertsKeyedByConstraints(t *testing.T) {
	pushStore(t)
	calls := installFakeRemote(t, [][]any{
		constraintRow("c1", "UNIQUENESS", "NODE", []any{"Person"}, []any{"email"}),
	})

	h := newHarness(t, "json")
	h.seedStoreDir(t, "movies")

	err := h.execute(t, "push", "movies",
		"--uri", "neo4j+s://remote:7687", "--username", "neo4j", "--password", "pw",
	)
	require.NoError(t, err)

	// One constraint probe + node upsert + rel upsert.
	require.Len(t, *calls, 3)
	assert.False(t, (*calls)[0].write)
	assert.True(t, (*calls)[1].write)
	assert.Equal(t,
		"UNWIND $rows AS row MERGE (n:`Person` {`email`: row.key}) SET n = row.props RETURN row.key AS key",
		(*calls)[1].query)
	rows := (*calls)[1].params["rows"].([]any)
	require.Len(t, rows, 2)
	first := rows[0].(map[string]any)
	assert.Equal(t, "ada@x", first["key"])

	assert.Equal(t,
		"UNWIND $rows AS row MATCH (a:`Person` {`email`: row.start}), (b:`Person` {`email`: row.end}) "+
			"MERGE (a)-[r:`KNOWS`]->(b) SET r = row.props RETURN row.start AS start, row.end AS end",
		(*calls)[2].query)

	assert.Contains(t, h.stderr.String(), "Pushed 2 nodes and 2 relationships")
}

func TestRelatePush_MergeKeyOverrideAndRelKey(t *testing.T) {
	pushStore(t)
	calls := installFakeRemote(t, nil)

	h := newHarness(t, "json")
	h.seedStoreDir(t, "movies")

	err := h.execute(t, "push", "movies",
		"--uri", "neo4j+s://remote:7687", "--username", "neo4j", "--password", "pw",
		"--merge-key", "Person.handle",
		"--rel-key", "since",
	)
	require.NoError(t, err)

	assert.Equal(t,
		"UNWIND $rows AS row MERGE (n:`Person` {`handle`: row.key}) SET n = row.props RETURN row.key AS key",
		(*calls)[1].query)
	assert.Contains(t, (*calls)[2].query, "MERGE (a)-[r:`KNOWS` {`since`: row.rkey}]->(b)")
}

func TestRelatePush_UnkeyedLabelRefused(t *testing.T) {
	pushStore(t)
	installFakeRemote(t, nil) // no constraints on the remote

	h := newHarness(t, "json")
	h.seedStoreDir(t, "movies")

	err := h.execute(t, "push", "movies",
		"--uri", "neo4j+s://remote:7687", "--username", "neo4j", "--password", "pw",
	)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), "no identity key for label(s) Person")
	assert.Contains(t, err.Error(), "--merge-key")
}

func TestRelatePush_DryRunWritesNothing(t *testing.T) {
	pushStore(t)
	calls := installFakeRemote(t, [][]any{
		constraintRow("c1", "UNIQUENESS", "NODE", []any{"Person"}, []any{"email"}),
	})

	h := newHarness(t, "json")
	h.seedStoreDir(t, "movies")

	err := h.execute(t, "push", "movies",
		"--uri", "neo4j+s://remote:7687", "--username", "neo4j", "--password", "pw",
		"--dry-run",
	)
	require.NoError(t, err)

	// Only the constraint probe ran; no writes.
	require.Len(t, *calls, 1)
	assert.False(t, (*calls)[0].write)

	assert.Contains(t, h.stderr.String(), "dry run")
	assert.Contains(t, h.stderr.String(), "label Person: 2 nodes, merge key email")
	assert.Contains(t, h.stderr.String(), "rel (Person)-[:KNOWS]->(Person): 2 rels")
}

func TestRelatePush_Batching(t *testing.T) {
	pushStore(t)
	calls := installFakeRemote(t, [][]any{
		constraintRow("c1", "UNIQUENESS", "NODE", []any{"Person"}, []any{"email"}),
	})

	h := newHarness(t, "json")
	h.seedStoreDir(t, "movies")

	err := h.execute(t, "push", "movies",
		"--uri", "neo4j+s://remote:7687", "--username", "neo4j", "--password", "pw",
		"--batch-size", "1",
	)
	require.NoError(t, err)

	// probe + 2 node batches + 2 rel batches.
	require.Len(t, *calls, 5)
	assert.Len(t, (*calls)[1].params["rows"].([]any), 1)
	assert.Len(t, (*calls)[2].params["rows"].([]any), 1)
}

func TestRelatePush_MissingStoreIsUsageError(t *testing.T) {
	// The engine hook must be installed (even a fake) so the command gets
	// past the not-compiled gate and reaches the missing-store check.
	installFakeOpen(t, &fakeStore{})
	installFakeRemote(t, nil)

	h := newHarness(t, "json")
	err := h.execute(t, "push", "movies",
		"--uri", "neo4j+s://remote:7687", "--username", "neo4j", "--password", "pw",
	)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), "no Relate store at")
}

func TestRelatePush_InvalidMergeKeySpec(t *testing.T) {
	pushStore(t)
	installFakeRemote(t, nil)

	h := newHarness(t, "json")
	h.seedStoreDir(t, "movies")

	err := h.execute(t, "push", "movies",
		"--uri", "neo4j+s://remote:7687", "--username", "neo4j", "--password", "pw",
		"--merge-key", "no-dots-here",
	)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), "Label.property")
}

func TestSplitCSV(t *testing.T) {
	assert.Equal(t, []string{"A", "B"}, splitCSV([]string{"A,B"}))
	assert.Equal(t, []string{"A", "B", "C"}, splitCSV([]string{"A, B", "C ", ""}))
	assert.Nil(t, splitCSV(nil))
}
