// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withRunStatementsSeam swaps the package-level runStatementsResponseFn AND
// driverOpener for the duration of the test, so batch (--atomic) tests can
// inject canned per-statement responses without booting a real Neo4j server.
// Mirrors withRunStatementSeam (connect_test.go) but for the batch seam. The
// seam fn receives the readOnly flag so tests can assert ExecuteRead vs
// ExecuteWrite routing.
func withRunStatementsSeam(t *testing.T, fn func(ctx context.Context, c *conn, statements []string, params map[string]any, readOnly bool) ([]*queryResponse, error)) {
	t.Helper()
	origFn := runStatementsResponseFn
	origOpener := driverOpener
	t.Cleanup(func() {
		runStatementsResponseFn = origFn
		driverOpener = origOpener
	})
	runStatementsResponseFn = fn
	driverOpener = func(target, username, password, userAgent string, debug bool) (neo4j.Driver, error) {
		return &noopDriver{}, nil
	}
}

func TestRunStatementsWithMode_BatchOrdering(t *testing.T) {
	var (
		gotStatements []string
		gotParams     map[string]any
		gotReadOnly   bool
	)
	withRunStatementsSeam(t, func(_ context.Context, _ *conn, statements []string, params map[string]any, readOnly bool) ([]*queryResponse, error) {
		gotStatements = statements
		gotParams = params
		gotReadOnly = readOnly
		resps := make([]*queryResponse, 0, len(statements))
		for i := range statements {
			resp := &queryResponse{}
			resp.Data.Fields = []string{"n"}
			resp.Data.Values = [][]any{{int64(i + 1)}}
			resps = append(resps, resp)
		}
		return resps, nil
	})

	c := &conn{database: "neo4j"}
	statements := []string{"RETURN 1 AS n", "RETURN 2 AS n", "RETURN 3 AS n"}
	results, err := runStatementsWithMode(context.Background(), c, statements, map[string]any{"k": 5}, false)
	require.NoError(t, err)
	require.Len(t, results, 3)

	// Results are unwrapped into one *queryResult per statement, in source order.
	for i, res := range results {
		assert.Equal(t, []string{"n"}, res.Columns)
		assert.Equal(t, [][]any{{int64(i + 1)}}, res.Rows)
	}

	// Statements and params forwarded to the seam unmodified, in order.
	assert.Equal(t, statements, gotStatements)
	assert.Equal(t, map[string]any{"k": 5}, gotParams)
	// readOnly=false routes the batch through ExecuteWrite.
	assert.False(t, gotReadOnly)
}

func TestRunStatementsWithMode_ReadOnlyRouting(t *testing.T) {
	var gotReadOnly bool
	withRunStatementsSeam(t, func(_ context.Context, _ *conn, statements []string, _ map[string]any, readOnly bool) ([]*queryResponse, error) {
		gotReadOnly = readOnly
		resps := make([]*queryResponse, len(statements))
		for i := range statements {
			resps[i] = &queryResponse{}
		}
		return resps, nil
	})

	c := &conn{database: "neo4j"}
	_, err := runStatementsWithMode(context.Background(), c, []string{"RETURN 1", "RETURN 2"}, nil, true)
	require.NoError(t, err)
	assert.True(t, gotReadOnly, "readOnly=true must route the batch through ExecuteRead")
}

func TestRunStatementsResponse_ErrorSurfacesAndCategorised(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantContains string
	}{
		{
			name:         "client error categorised as validation",
			err:          &fakeNeo4jError{code: "Neo.ClientError.Statement.SyntaxError", message: "Invalid input"},
			wantContains: "Neo.ClientError.Statement.SyntaxError",
		},
		{
			name:         "transient error categorised as upstream",
			err:          &fakeNeo4jError{code: "Neo.TransientError.Transaction.Terminated", message: "terminated"},
			wantContains: "Neo.TransientError.Transaction.Terminated",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withRunStatementsSeam(t, func(_ context.Context, _ *conn, _ []string, _ map[string]any, _ bool) ([]*queryResponse, error) {
				// One statement in the batch fails; the managed transaction
				// aborts and the driver rolls back (driver-side). The dispatcher
				// surfaces the error with no partial results.
				return nil, tc.err
			})

			c := &conn{database: "neo4j"}
			results, err := runStatementsWithMode(context.Background(), c, []string{"RETURN 1", "BAD"}, nil, false)
			require.Error(t, err)
			assert.Nil(t, results)
			assert.Contains(t, err.Error(), tc.wantContains)
		})
	}
}
