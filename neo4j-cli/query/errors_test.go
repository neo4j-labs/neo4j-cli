// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clierr"
)

// TestCategorizeBoltError_Mapping locks the Bolt → exit-code mapping:
// Neo.ClientError.* (cypher syntax/semantic rejections) surface as
// validation errors (exit 6); transport / Neo.TransientError.* /
// Neo.DatabaseError.* failures surface as upstream errors (exit 8). Both
// the typed-driver (*neo4j.Neo4jError) path and the plain-text path
// (errors.New("Neo.ClientError…")) are covered because production hits the
// typed path while tests inject plain strings via the seam.
func TestCategorizeBoltError_Mapping(t *testing.T) {
	cases := []struct {
		name     string
		in       error
		wantCode int
	}{
		{
			name:     "nil passes through unchanged",
			in:       nil,
			wantCode: 0,
		},
		{
			name:     "typed Neo4jError ClientError → validation (6)",
			in:       &neo4j.Neo4jError{Code: "Neo.ClientError.Statement.SyntaxError", Msg: "Invalid input"},
			wantCode: 6,
		},
		{
			name:     "typed Neo4jError TransientError → upstream (8)",
			in:       &neo4j.Neo4jError{Code: "Neo.TransientError.Transaction.DeadlockDetected", Msg: "deadlock"},
			wantCode: 8,
		},
		{
			name:     "typed Neo4jError DatabaseError → upstream (8)",
			in:       &neo4j.Neo4jError{Code: "Neo.DatabaseError.General.UnknownError", Msg: "db boom"},
			wantCode: 8,
		},
		{
			name:     "plain ClientError prefix → validation (6)",
			in:       errors.New("Neo.ClientError.Statement.SyntaxError: Invalid input"),
			wantCode: 6,
		},
		{
			name:     "plain TransientError prefix → upstream (8)",
			in:       errors.New("Neo.TransientError.Transaction.LockClientStopped: stopped"),
			wantCode: 8,
		},
		{
			name:     "plain DatabaseError prefix → upstream (8)",
			in:       errors.New("Neo.DatabaseError.General.UnknownError: boom"),
			wantCode: 8,
		},
		{
			name:     "unrecognised transport-level error → upstream (8)",
			in:       errors.New("connection refused"),
			wantCode: 8,
		},
		{
			name:     "fmt.Errorf-wrapped Neo4jError still classifies (errors.As walks the chain)",
			in:       fmt.Errorf("query: %w", &neo4j.Neo4jError{Code: "Neo.ClientError.Procedure.ProcedureCallFailed", Msg: "bad call"}),
			wantCode: 6,
		},
		{
			name:     "already-typed CLIError is left untouched",
			in:       clierr.NewValidationError("already typed"),
			wantCode: 6,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := categorizeBoltError(tc.in)
			if tc.in == nil {
				assert.NoError(t, got)
				return
			}
			require.Error(t, got)
			var ce *clierr.CLIError
			require.True(t, errors.As(got, &ce), "expected *clierr.CLIError, got %T", got)
			assert.Equal(t, tc.wantCode, ce.Code)
			// Original message content is preserved through the wrap so
			// terminal users still see the driver's code+msg verbatim.
			assert.Contains(t, got.Error(), tc.in.Error())
		})
	}
}

// TestCategorizeBoltError_PreservesWrappedChain locks that errors.As reaches
// the original *neo4j.Neo4jError through the CLIError wrap so future code that
// inspects driver-specific error types (e.g. retry decisions) keeps working.
func TestCategorizeBoltError_PreservesWrappedChain(t *testing.T) {
	driverErr := &neo4j.Neo4jError{Code: "Neo.ClientError.Statement.SyntaxError", Msg: "Invalid input"}
	wrapped := categorizeBoltError(driverErr)

	var ne *neo4j.Neo4jError
	require.True(t, errors.As(wrapped, &ne), "wrapped CLIError must still expose *neo4j.Neo4jError via errors.As")
	assert.Equal(t, "Neo.ClientError.Statement.SyntaxError", ne.Code)
}

// TestRunStatementResponse_CategorizesDriverErrors exercises the dispatch
// boundary end-to-end through the test seam: the seam returns a raw driver
// error, runStatementResponse must surface it as a typed *CLIError with the
// right exit code. This is the contract that the run.go / schema.go callers
// rely on.
func TestRunStatementResponse_CategorizesDriverErrors(t *testing.T) {
	cases := []struct {
		name     string
		seamErr  error
		wantCode int
	}{
		{
			name:     "cypher ClientError → exit 6",
			seamErr:  errors.New("Neo.ClientError.Statement.SyntaxError: Invalid input"),
			wantCode: 6,
		},
		{
			name:     "cypher TransientError → exit 8",
			seamErr:  errors.New("Neo.TransientError.Transaction.LockClientStopped: lock dropped"),
			wantCode: 8,
		},
		{
			name:     "transport-level failure → exit 8",
			seamErr:  errors.New("connection refused"),
			wantCode: 8,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withRunStatementSeam(t, func(_ context.Context, _ *conn, _ string, _ map[string]any, _ bool) (*queryResponse, error) {
				return nil, tc.seamErr
			})

			_, err := runStatementResponse(context.Background(), &conn{database: "neo4j"}, "RETURN 1", nil, true)
			require.Error(t, err)
			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T", err)
			assert.Equal(t, tc.wantCode, ce.Code)
		})
	}
}
