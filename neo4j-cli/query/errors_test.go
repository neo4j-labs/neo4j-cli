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

// TestCategorizeBoltError_ServerRespondedHTTP locks the wrong-port-family
// reclassification: when the driver reports "server responded HTTP" the
// failure is user-input misconfiguration (URI points at an HTTP listener,
// not a Bolt one), so categorizeBoltError must surface it as a UsageError
// with an actionable hint naming the Bolt port and the https:// alternative.
// The branch fires for both plain-text errors and fmt.Errorf-wrapped chains.
func TestCategorizeBoltError_ServerRespondedHTTP(t *testing.T) {
	cases := []struct {
		name string
		in   error
	}{
		{
			name: "plain driver text",
			in:   errors.New("server responded HTTP. Make sure you are not trying to connect to the http endpoint (HTTP defaults to port 7474 whereas BOLT defaults to port 7687)"),
		},
		{
			name: "wrapped via fmt.Errorf",
			in:   fmt.Errorf("query: open driver: %w", errors.New("server responded HTTP. wrong port")),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := categorizeBoltError(tc.in)
			require.Error(t, got)

			var ce *clierr.CLIError
			require.True(t, errors.As(got, &ce), "expected *clierr.CLIError, got %T", got)
			// NewUsageError → exit code 2 (usage_error). This is a
			// user-input misconfiguration, not a transport-level retryable
			// failure, so neither validation (6) nor upstream (8) fit.
			assert.Equal(t, 2, ce.Code, "wrong-port-family must surface as UsageError")

			msg := got.Error()
			assert.Contains(t, msg, "server responded HTTP", "message names the driver symptom")
			assert.Contains(t, msg, "7687", "hint names the default Bolt port")
			assert.Contains(t, msg, "https://", "hint mentions the https:// alternative for the auto-rewrite path")
			// Original driver error chain is preserved so errors.Is/As still
			// reaches the inner sentinel.
			assert.Contains(t, msg, tc.in.Error(), "wrap preserves the driver error text")
		})
	}
}

// TestCategorizeBoltError_SchemaTxForbidden locks the --atomic remediation
// path: ForbiddenDueToTransactionType (schema change mixed with data writes in
// one transaction) stays a validation error (exit 6) but gains a Suggestion
// pointing the user at dropping --atomic. Both the typed-driver path and the
// plain-text seam path must attach it; an ordinary SyntaxError must NOT, so the
// generic validation mapping stays suggestion-free.
func TestCategorizeBoltError_SchemaTxForbidden(t *testing.T) {
	cases := []struct {
		name           string
		in             error
		wantSuggestion bool
	}{
		{
			name:           "typed Neo4jError forbidden-tx-type → validation + suggestion",
			in:             &neo4j.Neo4jError{Code: forbiddenTxTypeCode, Msg: "Tried to execute Write query after executing Schema modification"},
			wantSuggestion: true,
		},
		{
			name:           "plain forbidden-tx-type text → validation + suggestion",
			in:             errors.New("Neo.ClientError.Transaction.ForbiddenDueToTransactionType: Tried to execute Write query after executing Schema modification"),
			wantSuggestion: true,
		},
		{
			name:           "ordinary SyntaxError → validation, no suggestion (regression)",
			in:             errors.New("Neo.ClientError.Statement.SyntaxError: Invalid input"),
			wantSuggestion: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := categorizeBoltError(tc.in)
			require.Error(t, got)
			var ce *clierr.CLIError
			require.True(t, errors.As(got, &ce), "expected *clierr.CLIError, got %T", got)
			assert.Equal(t, 6, ce.Code, "stays a validation error regardless of suggestion")
			if tc.wantSuggestion {
				assert.Contains(t, ce.Suggestion, "without --atomic", "remediation must name the --atomic fix")
			} else {
				assert.Empty(t, ce.Suggestion, "ordinary validation errors carry no suggestion")
			}
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
