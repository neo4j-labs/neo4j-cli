// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/query/embed"
	"github.com/neo4j/cli/test/utils/testfs"
)

// runHarness wires a query parent command, captured stdout/stderr, a
// configurable stdin reader, and the test seam overrides. It restores the
// package-level seams to their production defaults via t.Cleanup.
type runHarness struct {
	cfg            *clicfg.Config
	stdout, stderr *bytes.Buffer
}

func newRunHarness(t *testing.T, output string) *runHarness {
	t.Helper()

	// Reset stdin/password seams between tests; production behaviour is
	// re-installed at the end via t.Cleanup.
	origIsTTY := dbconn.StdinIsTTY
	origStdin := stdinReader
	origPwReader := dbconn.PasswordReader
	t.Cleanup(func() {
		dbconn.StdinIsTTY = origIsTTY
		stdinReader = origStdin
		dbconn.PasswordReader = origPwReader
	})

	// Default to "TTY" so commands that don't pipe stdin behave like an
	// interactive session (the missing-cypher path returns a usage error
	// rather than blocking on stdin).
	dbconn.StdinIsTTY = func() bool { return true }
	stdinReader = func() io.Reader { return strings.NewReader("") }

	cfgJSON := `{"format":"` + output + `"}`
	fs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)
	return &runHarness{
		cfg:    clicfg.NewConfig(fs, "test", clicfg.QueryScope),
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
	}
}

func (h *runHarness) execute(t *testing.T, args ...string) error {
	t.Helper()
	cmd := NewCmd(h.cfg)
	cmd.PersistentFlags().Bool("rw", false, "")
	cmd.SetOut(h.stdout)
	cmd.SetErr(h.stderr)
	cmd.SetContext(context.Background())
	cmd.SetArgs(args)
	return cmd.Execute()
}

// TestRunQuery_MultiStatement_DefaultReadsOrderedJSONArray verifies the default
// (non-atomic) multi-statement path: two read statements execute in source
// order through the per-statement seam (each preceded by its EXPLAIN
// preflight), all calls route through ExecuteRead, and the JSON output is an
// array of result envelopes in order.
func TestRunQuery_MultiStatement_DefaultReadsOrderedJSONArray(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1 AS a"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
	r.resp["RETURN 1 AS a"] = makeQueryResponse([]string{"a"}, [][]any{{int64(1)}})
	r.resp["EXPLAIN RETURN 2 AS b"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
	r.resp["RETURN 2 AS b"] = makeQueryResponse([]string{"b"}, [][]any{{int64(2)}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"RETURN 1 AS a;\nRETURN 2 AS b",
	)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"EXPLAIN RETURN 1 AS a", "RETURN 1 AS a",
		"EXPLAIN RETURN 2 AS b", "RETURN 2 AS b",
	}, r.calls)
	assert.True(t, r.readOnlyCalls["RETURN 1 AS a"])
	assert.True(t, r.readOnlyCalls["RETURN 2 AS b"])

	var got []decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got), "multi-statement output must be a JSON array")
	require.Len(t, got, 2)
	assert.Equal(t, []string{"a"}, got[0].Columns)
	assert.Equal(t, []string{"b"}, got[1].Columns)
}

// TestRunQuery_MultiStatement_DefaultFailFast verifies that when the second
// statement errors, the first statement has already executed and the error is
// returned (fail-fast) — the third statement never runs.
func TestRunQuery_MultiStatement_DefaultFailFast(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1 AS a"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
	r.resp["RETURN 1 AS a"] = makeQueryResponse([]string{"a"}, [][]any{{int64(1)}})
	r.resp["EXPLAIN RETURN 2 AS b"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
	r.respErr["RETURN 2 AS b"] = errors.New("Neo.ClientError.Statement.SyntaxError: boom")
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"RETURN 1 AS a;\nRETURN 2 AS b;\nRETURN 3 AS c",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	// First statement executed; failing statement reached; third never ran.
	assert.Equal(t, []string{
		"EXPLAIN RETURN 1 AS a", "RETURN 1 AS a",
		"EXPLAIN RETURN 2 AS b", "RETURN 2 AS b",
	}, r.calls)
	assert.NotContains(t, r.calls, "RETURN 3 AS c")
}

// TestRunQuery_MultiStatement_WriteWithoutRwBlocked verifies that without --rw a
// write statement among the batch is blocked via the per-statement EXPLAIN
// preflight with the existing --rw usage error.
func TestRunQuery_MultiStatement_WriteWithoutRwBlocked(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1 AS a"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
	r.resp["RETURN 1 AS a"] = makeQueryResponse([]string{"a"}, [][]any{{int64(1)}})
	r.resp["EXPLAIN CREATE (n)"] = makeExplainResponse(neo4j.QueryTypeReadWrite)
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"RETURN 1 AS a;\nCREATE (n)",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "this command writes; pass --rw to allow it")
}

// TestRunQuery_MultiStatement_RwWritesExecuteWritePerStatement verifies that
// with --rw each statement skips preflight and routes through ExecuteWrite, in
// order.
func TestRunQuery_MultiStatement_RwWritesExecuteWritePerStatement(t *testing.T) {
	r := newSeamRouter()
	r.resp["CREATE (a)"] = makeQueryResponse([]string{}, [][]any{})
	r.resp["CREATE (b)"] = makeQueryResponse([]string{}, [][]any{})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--rw",
		"CREATE (a);\nCREATE (b)",
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"CREATE (a)", "CREATE (b)"}, r.calls)
	assert.False(t, r.readOnlyCalls["CREATE (a)"])
	assert.False(t, r.readOnlyCalls["CREATE (b)"])
}

// TestRunQuery_Atomic_HappyPath verifies the --atomic path runs the whole batch
// through the single-transaction seam (one invocation returning N responses →
// N rendered envelopes).
func TestRunQuery_Atomic_HappyPath(t *testing.T) {
	var (
		gotStatements []string
		gotReadOnly   bool
		batchCalls    int
	)
	withRunStatementsSeam(t, func(_ context.Context, _ *conn, statements []string, _ map[string]any, readOnly bool) ([]*queryResponse, error) {
		batchCalls++
		gotStatements = statements
		gotReadOnly = readOnly
		return []*queryResponse{
			makeQueryResponse([]string{"a"}, [][]any{{int64(1)}}),
			makeQueryResponse([]string{"b"}, [][]any{{int64(2)}}),
		}, nil
	})
	// EXPLAIN preflight (non-rw) routes through the single-statement seam.
	origFn := runStatementResponseFn
	t.Cleanup(func() { runStatementResponseFn = origFn })
	runStatementResponseFn = func(_ context.Context, _ *conn, _ string, _ map[string]any, _ bool) (*queryResponse, error) {
		return makeExplainResponse(neo4j.QueryTypeReadOnly), nil
	}

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--atomic",
		"RETURN 1 AS a;\nRETURN 2 AS b",
	)
	require.NoError(t, err)
	assert.Equal(t, 1, batchCalls, "atomic path must invoke the batch seam exactly once")
	assert.Equal(t, []string{"RETURN 1 AS a", "RETURN 2 AS b"}, gotStatements)
	assert.True(t, gotReadOnly, "non-rw atomic batch must run read-only")

	var got []decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	require.Len(t, got, 2)
	assert.Equal(t, []string{"a"}, got[0].Columns)
	assert.Equal(t, []string{"b"}, got[1].Columns)
}

// TestRunQuery_Atomic_ErrorSurfaces verifies an error from the atomic batch
// surfaces and the batch seam is invoked exactly once (the transaction
// rolls back driver-side; we can only assert the single invocation here).
func TestRunQuery_Atomic_ErrorSurfaces(t *testing.T) {
	batchCalls := 0
	withRunStatementsSeam(t, func(_ context.Context, _ *conn, _ []string, _ map[string]any, _ bool) ([]*queryResponse, error) {
		batchCalls++
		return nil, errors.New("Neo.ClientError.Statement.SyntaxError: atomic boom")
	})

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--rw",
		"--atomic",
		"CREATE (a);\nCREATE (b)",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "atomic boom")
	assert.Equal(t, 1, batchCalls, "batch seam must be invoked exactly once")
}

// TestRunQuery_SingleStatement_TrailingSemicolonParity verifies a single
// statement with a trailing semicolon renders identically to the same statement
// without one — output stays a single JSON object (not an array).
func TestRunQuery_SingleStatement_TrailingSemicolonParity(t *testing.T) {
	run := func(t *testing.T, cypher string) string {
		t.Helper()
		r := newSeamRouter()
		r.resp["EXPLAIN RETURN 42 AS n"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
		r.resp["RETURN 42 AS n"] = makeQueryResponse([]string{"n"}, [][]any{{int64(42)}})
		r.install(t)

		h := newRunHarness(t, "json")
		err := h.execute(t,
			"--uri=neo4j://example:7687",
			"--password=pw",
			cypher,
		)
		require.NoError(t, err)
		return h.stdout.String()
	}

	withSemi := run(t, "RETURN 42 AS n;")
	withoutSemi := run(t, "RETURN 42 AS n")
	assert.Equal(t, withoutSemi, withSemi, "trailing ; must not change single-statement output")

	// Confirm it's a single object, not an array.
	var obj decodedResult
	require.NoError(t, json.Unmarshal([]byte(withSemi), &obj))
	assert.Equal(t, []string{"n"}, obj.Columns)
}

// TestRunQuery_ContinueOnError_WithAtomic_UsageErrorNoDBCalls verifies that
// combining --continue-on-error with --atomic is rejected as a usage error
// (exit 2) before any DB call is made.
func TestRunQuery_ContinueOnError_WithAtomic_UsageError(t *testing.T) {
	calls := 0
	withRunStatementsSeam(t, func(_ context.Context, _ *conn, _ []string, _ map[string]any, _ bool) ([]*queryResponse, error) {
		calls++
		return nil, nil
	})
	origFn := runStatementResponseFn
	t.Cleanup(func() { runStatementResponseFn = origFn })
	runStatementResponseFn = func(_ context.Context, _ *conn, _ string, _ map[string]any, _ bool) (*queryResponse, error) {
		calls++
		return nil, nil
	}

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--atomic",
		"--continue-on-error",
		"RETURN 1 AS a;\nRETURN 2 AS b",
	)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T", err)
	assert.Equal(t, 2, ce.Code, "must be a usage error")
	assert.Contains(t, err.Error(), "mutually exclusive")
	assert.Equal(t, 0, calls, "no DB calls may be made when the flags conflict")
}

// TestRunQuery_ContinueOnError_FailingMiddleStatement verifies that with the
// flag a failing middle statement does not abort: every statement renders, the
// failed one carries an "error" key at its index, stderr reports the failure,
// and the process exits 6 (validation error).
func TestRunQuery_ContinueOnError_FailingMiddleStatement(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1 AS a"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
	r.resp["RETURN 1 AS a"] = makeQueryResponse([]string{"a"}, [][]any{{int64(1)}})
	r.resp["EXPLAIN RETURN 2 AS b"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
	r.respErr["RETURN 2 AS b"] = errors.New("Neo.ClientError.Statement.SyntaxError: boom")
	r.resp["EXPLAIN RETURN 3 AS c"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
	r.resp["RETURN 3 AS c"] = makeQueryResponse([]string{"c"}, [][]any{{int64(3)}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--continue-on-error",
		"RETURN 1 AS a;\nRETURN 2 AS b;\nRETURN 3 AS c",
	)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T", err)
	assert.Equal(t, 6, ce.Code, "overall failure must be a validation error (exit 6)")
	assert.Contains(t, err.Error(), "1 of 3 statements failed")

	// Third statement still ran despite the second failing.
	assert.Contains(t, r.calls, "RETURN 3 AS c")
	assert.Contains(t, h.stderr.String(), "statement 2: ")
	assert.Contains(t, h.stderr.String(), "boom")

	var got []decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got), "output must be a JSON array of all statements")
	require.Len(t, got, 3)
	assert.Equal(t, []string{"a"}, got[0].Columns)
	assert.Empty(t, got[0].Error)
	assert.Contains(t, got[1].Error, "boom", "failed statement keeps its slot with an error key")
	assert.Equal(t, []string{"c"}, got[2].Columns)
	assert.Empty(t, got[2].Error)
}

// TestRunQuery_NoContinueOnError_StillFailsFast verifies that without the flag,
// the default mode still aborts on the first error and renders no output (the
// pre-flag behaviour).
func TestRunQuery_NoContinueOnError_StillFailsFast(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1 AS a"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
	r.resp["RETURN 1 AS a"] = makeQueryResponse([]string{"a"}, [][]any{{int64(1)}})
	r.resp["EXPLAIN RETURN 2 AS b"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
	r.respErr["RETURN 2 AS b"] = errors.New("Neo.ClientError.Statement.SyntaxError: boom")
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"RETURN 1 AS a;\nRETURN 2 AS b;\nRETURN 3 AS c",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	assert.NotContains(t, r.calls, "RETURN 3 AS c", "third statement must never run on fail-fast")
	assert.Empty(t, h.stdout.String(), "fail-fast renders no stdout")
}

// seamRouter is a tiny statement-router used to swap the runStatementResponseFn
// seam during tests. Each entry maps an exact statement string → response or
// error. Tests can append cypher to the calls slice for ordering assertions
// and inspect readOnlyCalls to verify ExecuteRead vs ExecuteWrite routing.
type seamRouter struct {
	calls         []string
	readOnlyCalls map[string]bool
	resp          map[string]*queryResponse
	respErr       map[string]error
	// onUnexpected fires when a statement does not match any route — defaults
	// to fatal-fail to surface unexpected calls in tests.
	onUnexpected func(statement string) (*queryResponse, error)
}

func (r *seamRouter) handle(_ context.Context, _ *conn, statement string, _ map[string]any, readOnly bool) (*queryResponse, error) {
	r.calls = append(r.calls, statement)
	r.readOnlyCalls[statement] = readOnly
	if err, ok := r.respErr[statement]; ok {
		return nil, err
	}
	if resp, ok := r.resp[statement]; ok {
		return resp, nil
	}
	if r.onUnexpected != nil {
		return r.onUnexpected(statement)
	}
	return nil, errors.New("unexpected statement: " + statement)
}

// installSeam swaps runStatementResponseFn + driverOpener for the duration of
// the test using the supplied router as the response source. Mirrors
// withRunStatementSeam (connect_test.go) but takes the router for batch
// configuration.
func (r *seamRouter) install(t *testing.T) {
	t.Helper()
	withRunStatementSeam(t, r.handle)
}

func newSeamRouter() *seamRouter {
	return &seamRouter{
		resp:          map[string]*queryResponse{},
		respErr:       map[string]error{},
		readOnlyCalls: map[string]bool{},
	}
}

func makeQueryResponse(fields []string, values [][]any) *queryResponse {
	resp := &queryResponse{}
	resp.Data.Fields = fields
	resp.Data.Values = values
	return resp
}

// makeExplainResponse builds the canned EXPLAIN preflight envelope a test
// expects when the cypher should classify as the supplied QueryType.
// makeQueryResponse is the run-time variant — preflight responses must carry
// a QueryType for the classifier to inspect.
func makeExplainResponse(qt neo4j.QueryType) *queryResponse {
	resp := makeQueryResponse([]string{}, [][]any{})
	resp.QueryType = qt
	return resp
}

func TestRunQuery_HappyPath_TableOutput(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1 AS n"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n", "m"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN 1 AS n"] = makeQueryResponse(
		[]string{"n", "m"},
		[][]any{{int64(1), "alice"}, {int64(2), "bob"}},
	)
	r.install(t)

	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--username=neo4j",
		"--password=secret",
		"RETURN 1 AS n",
	)
	require.NoError(t, err)
	out := h.stdout.String()
	assert.Contains(t, out, "alice")
	assert.Contains(t, out, "bob")
	// No truncation warning expected.
	assert.NotContains(t, h.stderr.String(), "truncated")
}

func TestRunQuery_HappyPath_JSONOutput(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 42 AS n"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN 42 AS n"] = makeQueryResponse([]string{"n"}, [][]any{{int64(42)}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"RETURN 42 AS n",
	)
	require.NoError(t, err)

	var got decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, []string{"n"}, got.Columns)
	assert.False(t, got.Truncated)
	require.Len(t, got.Rows, 1)
	// JSON marshal renders int64 as a number; unmarshal as float64.
	assert.Equal(t, float64(42), got.Rows[0]["n"])
}

// TestRunQuery_EmptyResult_ExitsZero locks the exit-0 contract for runQuery
// when the underlying Cypher returns zero rows. Mirrors
// TestRunQuery_HappyPath_JSONOutput shape: EXPLAIN classifies the statement
// as read-only, the run-time response carries the column list with no values,
// and the JSON envelope decodes with the column header, an empty rows slice,
// and truncated=false.
func TestRunQuery_EmptyResult_ExitsZero(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1 AS n WHERE false"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN 1 AS n WHERE false"] = makeQueryResponse([]string{"n"}, [][]any{})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"RETURN 1 AS n WHERE false",
	)
	require.NoError(t, err)

	var got decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, []string{"n"}, got.Columns)
	assert.Empty(t, got.Rows)
	assert.False(t, got.Truncated)
}

func TestRunQuery_ServerErrorSurfacesError(t *testing.T) {
	r := newSeamRouter()
	r.respErr["EXPLAIN BAD CYPHER"] = errors.New("Neo.ClientError.Statement.SyntaxError: Invalid input")
	r.install(t)

	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"BAD CYPHER",
	)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "Neo.ClientError.Statement.SyntaxError")
	assert.Contains(t, msg, "Invalid input")
}

func TestRunQuery_ReadOnlyCypherWithoutRwRunsExplainThenExecutes(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN MATCH (n) RETURN n"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
	r.resp["MATCH (n) RETURN n"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"MATCH (n) RETURN n",
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"EXPLAIN MATCH (n) RETURN n", "MATCH (n) RETURN n"}, r.calls)
	// Both calls must route through ExecuteRead — preflight never mutates and
	// the classifier proved the real cypher is read-only.
	assert.True(t, r.readOnlyCalls["EXPLAIN MATCH (n) RETURN n"], "preflight must use ExecuteRead")
	assert.True(t, r.readOnlyCalls["MATCH (n) RETURN n"], "read-only execution must use ExecuteRead")
}

func TestRunQuery_WriteCypherWithoutRwErrorsBeforeExecution(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN CREATE (n)"] = makeExplainResponse(neo4j.QueryTypeReadWrite)
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"CREATE (n)",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "this command writes; pass --rw to allow it")
	assert.Equal(t, []string{"EXPLAIN CREATE (n)"}, r.calls)
	// Pin focused-error behavior: write-gate rejection must NOT leak cobra's
	// usage block to stdout — cmd.SilenceUsage=true at run.go:50 guarantees this.
	assert.Equal(t, "", h.stdout.String(), "write-gate rejection must not print usage block to stdout")
}

func TestRunQuery_WriteCypherWithRwSkipsPreflight(t *testing.T) {
	r := newSeamRouter()
	r.resp["CREATE (n)"] = makeQueryResponse([]string{}, [][]any{})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--rw",
		"CREATE (n)",
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"CREATE (n)"}, r.calls)
	// With --rw the user opted in to writing; the call must route through
	// ExecuteWrite (readOnly=false) so the driver picks up write-server routing.
	assert.False(t, r.readOnlyCalls["CREATE (n)"], "--rw execution must use ExecuteWrite")
}

func TestRunQuery_ExplainErrorSurfacesVerbatim(t *testing.T) {
	r := newSeamRouter()
	r.respErr["EXPLAIN RETURN 1"] = errors.New("Neo.ClientError.Statement.SyntaxError: Invalid input")
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"RETURN 1",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Neo.ClientError.Statement.SyntaxError")
	assert.Contains(t, err.Error(), "Invalid input")
}

// TestRejectWriteCypher_QueryTypeClassifier locks the contract that
// rejectWriteCypher uses summary.QueryType() (forwarded via
// queryResponse.QueryType) as the sole classifier — only QueryTypeReadOnly
// passes through; everything else (including QueryTypeUnknown) demands --rw.
func TestRejectWriteCypher_QueryTypeClassifier(t *testing.T) {
	tests := []struct {
		name    string
		qt      neo4j.QueryType
		wantErr bool
	}{
		{name: "read-only proceeds", qt: neo4j.QueryTypeReadOnly, wantErr: false},
		{name: "read-write rejected", qt: neo4j.QueryTypeReadWrite, wantErr: true},
		{name: "write-only rejected", qt: neo4j.QueryTypeWriteOnly, wantErr: true},
		{name: "schema-write rejected", qt: neo4j.QueryTypeSchemaWrite, wantErr: true},
		{name: "unknown rejected", qt: neo4j.QueryTypeUnknown, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withRunStatementSeam(t, func(_ context.Context, _ *conn, _ string, _ map[string]any, _ bool) (*queryResponse, error) {
				return makeExplainResponse(tc.qt), nil
			})
			cmd := NewCmd(clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.QueryScope))
			cmd.SetContext(context.Background())
			err := rejectWriteCypher(cmd, &conn{}, "MATCH (n) RETURN n", nil)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "this command writes; pass --rw to allow it")
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestExecutionMode locks the contract of the executionMode helper: it reports
// WHICH leading keyword (EXPLAIN or PROFILE) matched — case-insensitively,
// tolerating leading whitespace and honouring a word boundary — together with
// the statement body after the keyword and its separating whitespace. The body
// must be preserved byte-for-byte (internal whitespace untouched) because the
// write-guard rebuilds its EXPLAIN-classification statement from it.
func TestExecutionMode(t *testing.T) {
	tests := []struct {
		name     string
		cypher   string
		wantMode string
		wantRest string
	}{
		{name: "explain", cypher: "EXPLAIN RETURN 1", wantMode: "EXPLAIN", wantRest: "RETURN 1"},
		{name: "profile", cypher: "PROFILE RETURN 1", wantMode: "PROFILE", wantRest: "RETURN 1"},
		{name: "mixed case", cypher: "PrOfIlE RETURN 1", wantMode: "PROFILE", wantRest: "RETURN 1"},
		{name: "lowercase", cypher: "profile return 1", wantMode: "PROFILE", wantRest: "return 1"},
		{name: "leading whitespace", cypher: "  \tEXPLAIN RETURN 1", wantMode: "EXPLAIN", wantRest: "RETURN 1"},
		{name: "internal whitespace preserved", cypher: "PROFILE MATCH (n)   RETURN  n", wantMode: "PROFILE", wantRest: "MATCH (n)   RETURN  n"},
		{name: "bare keyword no body", cypher: "EXPLAIN", wantMode: "EXPLAIN", wantRest: ""},
		{name: "bare keyword trailing whitespace", cypher: "PROFILE  ", wantMode: "PROFILE", wantRest: ""},
		{name: "empty", cypher: "", wantMode: "", wantRest: ""},
		{name: "whitespace only", cypher: "   \n", wantMode: "", wantRest: ""},
		{name: "explainer is not explain", cypher: "EXPLAINER RETURN 1", wantMode: "", wantRest: ""},
		{name: "profiled is not profile", cypher: "PROFILED RETURN 1", wantMode: "", wantRest: ""},
		{name: "mid-statement keyword ignored", cypher: "RETURN 1 EXPLAIN", wantMode: "", wantRest: ""},
		{name: "bare statement", cypher: "RETURN 1", wantMode: "", wantRest: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mode, rest := executionMode(tc.cypher)
			assert.Equal(t, tc.wantMode, mode)
			assert.Equal(t, tc.wantRest, rest)
		})
	}
}

// TestRejectWriteCypher_ProfileClassifiedViaExplainBody rewrites the old
// "already-moded statement sent verbatim for classification" contract. A
// PROFILE statement carries its own execution mode, but PROFILE EXECUTES —
// sending it verbatim as the write-guard's classification step would run the
// very write the guard is meant to block. The guard must instead classify
// "EXPLAIN " + the body (leading PROFILE keyword stripped), and r.calls must
// never contain the raw PROFILE statement.
func TestRejectWriteCypher_ProfileClassifiedViaExplainBody(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN MATCH (n)   RETURN  n"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
	r.install(t)

	cmd := NewCmd(clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.QueryScope))
	cmd.SetContext(context.Background())
	err := rejectWriteCypher(cmd, &conn{}, "PROFILE MATCH (n)   RETURN  n", nil)
	require.NoError(t, err)
	// Classification must be EXPLAIN of the body with internal whitespace
	// preserved — the raw PROFILE statement is never sent for classification.
	assert.Equal(t, []string{"EXPLAIN MATCH (n)   RETURN  n"}, r.calls)
	assert.NotContains(t, r.calls, "PROFILE", "the raw PROFILE statement must never be sent for classification")
	assert.True(t, r.readOnlyCalls["EXPLAIN MATCH (n)   RETURN  n"], "preflight must use ExecuteRead")
}

// TestRejectWriteCypher_ExplainSkipsClassification verifies that an
// EXPLAIN-prefixed statement triggers NO classification round trip at all: the
// guard passes it through (EXPLAIN cannot mutate, so forcing --rw just to view
// a write plan would be a false positive) with an empty r.calls.
func TestRejectWriteCypher_ExplainSkipsClassification(t *testing.T) {
	r := newSeamRouter()
	r.install(t)

	cmd := NewCmd(clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.QueryScope))
	cmd.SetContext(context.Background())
	err := rejectWriteCypher(cmd, &conn{}, "EXPLAIN MATCH (n)   RETURN  n", nil)
	require.NoError(t, err)
	assert.Empty(t, r.calls, "no classification round trip may be issued for an EXPLAIN-prefixed statement")
}

// TestRunQuery_ExplainWriteWithoutRwSkipsGateEntirely runs an EXPLAIN of a
// write statement through the full command WITHOUT --rw and locks acceptance:
// the gate passes it without a classification round trip and the verbatim
// EXPLAIN statement is the only statement ever sent (the router's
// onUnexpected default would fail the test on any extra call).
func TestRunQuery_ExplainWriteWithoutRwSkipsGateEntirely(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN CREATE (n)"] = makeQueryResponse([]string{}, [][]any{})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"EXPLAIN CREATE (n)",
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"EXPLAIN CREATE (n)"}, r.calls,
		"the verbatim EXPLAIN statement must be the only call — no classification and no --rw requirement")
	assert.True(t, r.readOnlyCalls["EXPLAIN CREATE (n)"], "EXPLAIN execution must use ExecuteRead")
}

// TestRunQuery_ProfileWriteWithoutRwBlockedWithoutExecuting locks acceptance
// for a write-bearing PROFILE statement without --rw: the guard classifies via
// "EXPLAIN "+body, returns the --rw usage error, and the raw PROFILE statement
// is never sent (which would have executed the write).
func TestRunQuery_ProfileWriteWithoutRwBlockedWithoutExecuting(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN CREATE (n)"] = makeExplainResponse(neo4j.QueryTypeReadWrite)
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"PROFILE CREATE (n)",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "this command writes; pass --rw to allow it")
	assert.Equal(t, []string{"EXPLAIN CREATE (n)"}, r.calls)
	assert.NotContains(t, r.calls, "PROFILE CREATE (n)",
		"the raw PROFILE statement must never be sent — it would execute the write as the classification step")
	assert.Empty(t, h.stdout.String(), "write-gate rejection must not print output")
}

// TestRunQuery_ProfileReadWithoutRwClassifiesViaExplain locks acceptance for a
// read-bearing PROFILE statement without --rw: the preflight classifies
// "EXPLAIN "+body (never the raw PROFILE statement), the verbatim PROFILE
// statement then executes read-only, and the command succeeds.
func TestRunQuery_ProfileReadWithoutRwClassifiesViaExplain(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN MATCH (n) RETURN n"] = makeExplainResponse(neo4j.QueryTypeReadOnly)
	r.resp["PROFILE MATCH (n) RETURN n"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"PROFILE MATCH (n) RETURN n",
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"EXPLAIN MATCH (n) RETURN n", "PROFILE MATCH (n) RETURN n"}, r.calls)
	assert.True(t, r.readOnlyCalls["EXPLAIN MATCH (n) RETURN n"], "classification must use ExecuteRead")
	assert.True(t, r.readOnlyCalls["PROFILE MATCH (n) RETURN n"], "read-only PROFILE execution must use ExecuteRead")
}

// TestRunQuery_Atomic_ProfileWriteWithoutRwBlocked verifies the --atomic path
// inherits the fix through preflightAll: a write-bearing PROFILE statement is
// blocked by the per-statement guard before any batch transaction opens, so the
// raw PROFILE statement is never sent.
func TestRunQuery_Atomic_ProfileWriteWithoutRwBlocked(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN CREATE (n)"] = makeExplainResponse(neo4j.QueryTypeReadWrite)
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--atomic",
		"PROFILE CREATE (n)",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "this command writes; pass --rw to allow it")
	assert.Equal(t, []string{"EXPLAIN CREATE (n)"}, r.calls)
	assert.NotContains(t, r.calls, "PROFILE CREATE (n)",
		"the raw PROFILE statement must never be sent on the atomic path either")
}

func TestRunQuery_RowLimitTruncates_TableOutput(t *testing.T) {
	r := newSeamRouter()
	rows := make([][]any, 10)
	for i := range rows {
		rows[i] = []any{int64(i + 1)}
	}
	r.resp["EXPLAIN RETURN range(1,10)"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN range(1,10)"] = makeQueryResponse([]string{"n"}, rows)
	r.install(t)

	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--max-rows=2",
		"RETURN range(1,10)",
	)
	require.NoError(t, err)

	stderr := h.stderr.String()
	assert.Contains(t, stderr, "truncated to 2 rows")
	assert.Contains(t, stderr, "--max-rows 0 for unlimited")

	out := h.stdout.String()
	assert.Contains(t, out, "1")
	assert.Contains(t, out, "2")
	assert.NotContains(t, out, "10")
}

func TestRunQuery_RowLimitTruncates_JSONSetsTruncatedTrueAndPrintsWarning(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN range(1,3)"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN range(1,3)"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}, {int64(2)}, {int64(3)}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--max-rows=1",
		"RETURN range(1,3)",
	)
	require.NoError(t, err)

	var got decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.True(t, got.Truncated, "JSON envelope must report truncated:true")
	assert.Len(t, got.Rows, 1)

	assert.Contains(t, h.stderr.String(), "truncated to 1 rows")
}

func TestRunQuery_RowLimitZeroMeansUnlimited(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN x"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN x"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}, {int64(2)}, {int64(3)}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--max-rows=0",
		"RETURN x",
	)
	require.NoError(t, err)

	var got decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.False(t, got.Truncated)
	assert.Len(t, got.Rows, 3)
	assert.NotContains(t, h.stderr.String(), "truncated")
}

func TestRunQuery_TruncateArraysAppliesBeforeRowCap(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN xs"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"xs"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN xs"] = makeQueryResponse([]string{"xs"}, [][]any{{[]any{int64(1), int64(2), int64(3), int64(4), int64(5)}}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--truncate-arrays-over=3",
		"RETURN xs",
	)
	require.NoError(t, err)

	var got decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	require.Len(t, got.Rows, 1)
	xs, ok := got.Rows[0]["xs"].([]any)
	require.True(t, ok, "xs must be []any after truncation")
	assert.Empty(t, xs, "over-limit array must be elided to []")
}

// TestRunQuery_TruncateArrays_JSONOutputContainsEmptyArray verifies the
// rendered JSON literally contains `"xs": []` for an over-limit top-level
// array — closes the gap where in-memory shape was tested but not the
// actual `--format json` byte stream.
func TestRunQuery_TruncateArrays_JSONOutputContainsEmptyArray(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN xs"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"xs"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	bigArr := make([]any, 10)
	for i := range bigArr {
		bigArr[i] = int64(i + 1)
	}
	r.resp["RETURN xs"] = makeQueryResponse([]string{"xs"}, [][]any{{bigArr}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--truncate-arrays-over=3",
		"RETURN xs",
	)
	require.NoError(t, err)

	out := h.stdout.String()
	assert.Contains(t, out, `"xs": []`,
		"rendered JSON must contain literal empty-array for the elided value")
	assert.NotContains(t, out, "<truncated:",
		"output must NOT contain any placeholder string")
}

// TestRunQuery_TruncateArrays_NestedArray_JSONOutputContainsEmptyArray covers
// an array nested inside a map value (e.g. `{"data": [...]}` returned as a
// row column) — the recursion must elide the nested array end-to-end.
func TestRunQuery_TruncateArrays_NestedArray_JSONOutputContainsEmptyArray(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN obj"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"obj"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	bigArr := make([]any, 10)
	for i := range bigArr {
		bigArr[i] = int64(i + 1)
	}
	r.resp["RETURN obj"] = makeQueryResponse([]string{"obj"}, [][]any{{map[string]any{"data": bigArr}}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--truncate-arrays-over=3",
		"RETURN obj",
	)
	require.NoError(t, err)

	out := h.stdout.String()
	assert.Contains(t, out, `"data": []`,
		"nested array must render as empty-array literal in JSON")
	assert.NotContains(t, out, "<truncated:",
		"output must NOT contain any placeholder string")
}

// TestRunQuery_TruncateArrays_TableOutputCellIsEmptyArray covers --format
// table: the cell rendering for an over-limit array must be `[]` (the
// JSON-stringified empty array), not the legacy placeholder string.
func TestRunQuery_TruncateArrays_TableOutputCellIsEmptyArray(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN xs"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"xs"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	bigArr := make([]any, 10)
	for i := range bigArr {
		bigArr[i] = int64(i + 1)
	}
	r.resp["RETURN xs"] = makeQueryResponse([]string{"xs"}, [][]any{{bigArr}})
	r.install(t)

	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--truncate-arrays-over=3",
		"RETURN xs",
	)
	require.NoError(t, err)

	out := h.stdout.String()
	assert.Contains(t, out, "[]",
		"table cell must render the elided value as []")
	assert.NotContains(t, out, "<truncated:",
		"output must NOT contain any placeholder string")
}

func TestRunQuery_StdinInputWhenNoArg(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1 AS n"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN 1 AS n"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
	r.install(t)

	h := newRunHarness(t, "json")
	// Override seams: not a TTY; supply Cypher via "stdin".
	dbconn.StdinIsTTY = func() bool { return false }
	stdinReader = func() io.Reader { return strings.NewReader("RETURN 1 AS n") }

	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
	)
	require.NoError(t, err)

	var got decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, []string{"n"}, got.Columns)
}

func TestRunQuery_NoCypherOnTTYReturnsUsageError(t *testing.T) {
	h := newRunHarness(t, "table")
	// dbconn.StdinIsTTY default is true via harness.

	err := h.execute(t, "--uri=neo4j://example:7687", "--password=pw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Cypher")
}

func TestRunQuery_EmptyStdinNonTTYReturnsUsageError(t *testing.T) {
	h := newRunHarness(t, "table")
	dbconn.StdinIsTTY = func() bool { return false }
	stdinReader = func() io.Reader { return strings.NewReader("   \n  ") }

	err := h.execute(t, "--uri=neo4j://example:7687", "--password=pw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Cypher")
}

func TestRunQuery_PasswordFromEnvSkipsPrompt(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN 1"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
	r.install(t)

	h := newRunHarness(t, "json")
	t.Setenv("NEO4J_CLI_ACCEPT_ENV_VARS", "1")
	t.Setenv(dbconn.EnvPassword, "from-env")
	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvDatabase, "")

	// Set PasswordReader so a buggy fallthrough would surface as a test
	// failure (returning a sentinel that wouldn't match).
	dbconn.PasswordReader = func() (string, error) {
		t.Fatal("PasswordReader must NOT be invoked when env supplies password")
		return "", nil
	}

	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--username=u",
		"RETURN 1",
	)
	require.NoError(t, err)
}

func TestRunQuery_PasswordPromptedOnTTY(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN 1"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
	r.install(t)

	h := newRunHarness(t, "json")

	// Clear env-based password.
	t.Setenv(dbconn.EnvPassword, "")

	called := false
	dbconn.PasswordReader = func() (string, error) {
		called = true
		return "typed-at-prompt", nil
	}

	err := h.execute(t, "--uri=neo4j://example:7687", "--username=u", "RETURN 1")
	require.NoError(t, err)
	assert.True(t, called, "PasswordReader must be invoked on TTY when no password is set")
	assert.Contains(t, h.stderr.String(), "Password:")
}

func TestRunQuery_PasswordMissingNonTTYReturnsClearError(t *testing.T) {
	h := newRunHarness(t, "json")
	dbconn.StdinIsTTY = func() bool { return false }
	// Provide stdin Cypher so the early Cypher check passes.
	stdinReader = func() io.Reader { return strings.NewReader("RETURN 1") }
	t.Setenv(dbconn.EnvPassword, "")

	err := h.execute(t, "--uri=neo4j://example:7687", "--username=u")
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "--password")
	assert.Contains(t, msg, "NEO4J_PASSWORD")
	assert.Contains(t, msg, ".env")
}

func TestRunQuery_InvalidParamReturnsUsageError(t *testing.T) {
	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--param=missing-equals",
		"RETURN 1",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key=value")
}

func TestRunQuery_ParamsForwardedAsRequestBody(t *testing.T) {
	var seenParams map[string]any
	withRunStatementSeam(t, func(_ context.Context, _ *conn, statement string, params map[string]any, _ bool) (*queryResponse, error) {
		if strings.HasPrefix(statement, "EXPLAIN ") {
			resp := makeQueryResponse([]string{"n"}, [][]any{})
			resp.QueryType = neo4j.QueryTypeReadOnly
			return resp, nil
		}
		seenParams = params
		return makeQueryResponse([]string{"n"}, [][]any{{int64(1)}}), nil
	})

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--param=n=5",
		"--param=name=alice",
		"RETURN $n, $name",
	)
	require.NoError(t, err)
	require.NotNil(t, seenParams)
	assert.Equal(t, float64(5), seenParams["n"])
	assert.Equal(t, "alice", seenParams["name"])
}

// TestRunQuery_TruncateValues_PassThrough_WhenMaxZero is a focused unit test
// for the truncateValues helper to lock the max<=0 short-circuit semantics.
func TestRunQuery_TruncateValues_PassThrough_WhenMaxZero(t *testing.T) {
	in := [][]any{{[]any{1, 2, 3, 4, 5}}}
	out, count := truncateValues(in, 0)
	// Returned slice should be the same backing array (untouched).
	require.Len(t, out, 1)
	require.Len(t, out[0], 1)
	xs, ok := out[0][0].([]any)
	require.True(t, ok)
	assert.Len(t, xs, 5)
	assert.Equal(t, 0, count, "max=0 must report zero truncations")
}

// TestRunQuery_CapRows_Behaviour locks the table-driven contract for the
// row-cap helper covering the three semantic regimes.
func TestRunQuery_CapRows_Behaviour(t *testing.T) {
	tests := []struct {
		name      string
		rows      [][]any
		max       int
		wantLen   int
		wantTrunc bool
	}{
		{"unlimited zero", [][]any{{1}, {2}, {3}}, 0, 3, false},
		{"unlimited negative", [][]any{{1}, {2}, {3}}, -1, 3, false},
		{"limit not exceeded", [][]any{{1}, {2}}, 5, 2, false},
		{"limit equal", [][]any{{1}, {2}}, 2, 2, false},
		{"limit exceeded", [][]any{{1}, {2}, {3}}, 2, 2, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, trunc := capRows(tc.rows, tc.max)
			assert.Len(t, out, tc.wantLen)
			assert.Equal(t, tc.wantTrunc, trunc)
		})
	}
}

// TestRunQuery_TruncateArrays_JSON_AggregateWarningAndField verifies that
// when at least one array is elided, JSON output includes
// `"arrays_truncated": <N>` AND stderr contains the exact aggregate
// warning line. The row-cap (`truncated:true`) is a separate concern and
// must remain false here because --max-rows is unset.
func TestRunQuery_TruncateArrays_JSON_AggregateWarningAndField(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN xs"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"xs"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	bigArr := make([]any, 10)
	for i := range bigArr {
		bigArr[i] = int64(i + 1)
	}
	r.resp["RETURN xs"] = makeQueryResponse([]string{"xs"}, [][]any{{bigArr}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--truncate-arrays-over=3",
		"RETURN xs",
	)
	require.NoError(t, err)

	stderr := h.stderr.String()
	assert.Contains(t, stderr,
		"warning: truncated 1 arrays larger than 3 items (use --truncate-arrays-over 0 to disable)",
		"stderr must contain the exact aggregate warning line")

	var got decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, 1, got.ArraysTruncated, "JSON envelope must report arrays_truncated=1")
	assert.False(t, got.Truncated, "row-cap signal must remain false")
}

// TestRunQuery_TruncateArrays_Table_AggregateWarning verifies that table
// output emits the exact aggregate warning to stderr while leaving the
// table body unchanged (cells render as `[]` per task-011).
func TestRunQuery_TruncateArrays_Table_AggregateWarning(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN xs"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"xs"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	bigArr := make([]any, 10)
	for i := range bigArr {
		bigArr[i] = int64(i + 1)
	}
	r.resp["RETURN xs"] = makeQueryResponse([]string{"xs"}, [][]any{{bigArr}})
	r.install(t)

	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--truncate-arrays-over=3",
		"RETURN xs",
	)
	require.NoError(t, err)

	assert.Contains(t, h.stderr.String(),
		"warning: truncated 1 arrays larger than 3 items (use --truncate-arrays-over 0 to disable)")

	out := h.stdout.String()
	assert.Contains(t, out, "[]", "table cell must render the elided value as []")
}

// TestRunQuery_TruncateArrays_NoTruncation_NoWarningAndZeroField verifies
// that when no arrays exceed the threshold, stderr is silent of the
// array-truncation warning AND JSON `arrays_truncated` is `0`.
func TestRunQuery_TruncateArrays_NoTruncation_NoWarningAndZeroField(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN xs"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"xs"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN xs"] = makeQueryResponse([]string{"xs"}, [][]any{{[]any{int64(1), int64(2), int64(3)}}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--truncate-arrays-over=10",
		"RETURN xs",
	)
	require.NoError(t, err)

	stderr := h.stderr.String()
	assert.NotContains(t, stderr, "arrays larger than",
		"stderr must be silent for the array-truncation warning when nothing was elided")

	var got decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, 0, got.ArraysTruncated, "arrays_truncated must be 0 when nothing was elided")
}

// TestRunQuery_URIRewriteEmitsStderrNotice verifies the URI auto-rewrite path
// still emits the documented stderr notice. http(s):// inputs are rewritten
// to neo4j(+s):// (the rewrite logic lives in uri.go and is independent of the
// underlying transport).
func TestRunQuery_URIRewriteEmitsStderrNotice(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN 1"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=http://127.0.0.1:9999",
		"--password=pw",
		"RETURN 1",
	)
	require.NoError(t, err)

	stderr := h.stderr.String()
	assert.Contains(t, stderr, "info: rewrote URI 'http://127.0.0.1:9999'",
		"stderr must contain the rewrite notice with the original URI")
	assert.Contains(t, stderr, "neo4j://127.0.0.1:7687",
		"stderr must mention the rewritten neo4j:// URI")

	var got decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, []string{"n"}, got.Columns)
}

// TestRunQuery_URIPassthroughEmitsNoNotice verifies that already-correct
// neo4j(+s) URIs do NOT trigger the rewrite notice.
func TestRunQuery_URIPassthroughEmitsNoNotice(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN 1"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"RETURN 1",
	)
	require.NoError(t, err)

	assert.NotContains(t, h.stderr.String(), "rewrote URI",
		"neo4j(+s) URIs must pass through without a rewrite notice")
}

// TestRunQuery_CleartextNonLoopbackWarning verifies that a cleartext bolt or
// neo4j URI to a non-loopback host emits the cleartext warning to stderr;
// loopback hosts and encrypted (+s/+ssc) variants stay silent.
func TestRunQuery_CleartextNonLoopbackWarning(t *testing.T) {
	cases := []struct {
		name        string
		uri         string
		wantWarning bool
	}{
		{name: "neo4j to non-loopback warns", uri: "neo4j://prod.example:7687", wantWarning: true},
		{name: "bolt to non-loopback warns", uri: "bolt://prod.example:7687", wantWarning: true},
		{name: "bolt to localhost silent", uri: "bolt://localhost:7687", wantWarning: false},
		{name: "bolt to 127.0.0.1 silent", uri: "bolt://127.0.0.1:7687", wantWarning: false},
		{name: "neo4j+s to non-loopback silent", uri: "neo4j+s://prod.example:7687", wantWarning: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newSeamRouter()
			r.resp["EXPLAIN RETURN 1"] = func() *queryResponse {
				resp := makeQueryResponse([]string{"n"}, [][]any{})
				resp.QueryType = neo4j.QueryTypeReadOnly
				return resp
			}()
			r.resp["RETURN 1"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
			r.install(t)

			h := newRunHarness(t, "json")
			err := h.execute(t,
				"--uri="+tc.uri,
				"--password=pw",
				"RETURN 1",
			)
			require.NoError(t, err)

			stderr := h.stderr.String()
			if tc.wantWarning {
				assert.Contains(t, stderr, "warning:",
					"stderr must contain the cleartext warning")
				assert.Contains(t, stderr, "cleartext",
					"warning must mention cleartext")
			} else {
				assert.NotContains(t, stderr, "warning:",
					"stderr must not contain a cleartext warning")
			}
		})
	}
}

// TestRunQuery_CleartextWarningRedactsUserinfoPassword verifies the
// userinfo password embedded in the URI is masked via (*url.URL).Redacted()
// when the cleartext warning is emitted.
func TestRunQuery_CleartextWarningRedactsUserinfoPassword(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryType = neo4j.QueryTypeReadOnly
		return resp
	}()
	r.resp["RETURN 1"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://user:supersecret@prod.example:7687",
		"--password=pw",
		"RETURN 1",
	)
	require.NoError(t, err)

	stderr := h.stderr.String()
	assert.Contains(t, stderr, "warning:")
	assert.NotContains(t, stderr, "supersecret",
		"userinfo password must be masked in the warning")
}

func TestPromptPassword_NonTTYReturnsUsageError(t *testing.T) {
	origTTY := dbconn.StdinIsTTY
	t.Cleanup(func() { dbconn.StdinIsTTY = origTTY })
	dbconn.StdinIsTTY = func() bool { return false }

	fs := afero.NewMemMapFs()
	cfg := clicfg.NewConfig(fs, "test", clicfg.QueryScope)
	cmd := NewCmd(cfg)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetOut(&bytes.Buffer{})

	_, err := promptPassword(cmd, cfg)
	require.Error(t, err)
	// accept-env-vars is off (default): the gated env var must NOT be advertised
	// as an effective remedy; point at --flags / .env / the gate instead.
	assert.Contains(t, err.Error(), "--password")
	assert.Contains(t, err.Error(), ".env")
	assert.Contains(t, err.Error(), "accept-env-vars")
	assert.NotContains(t, err.Error(), "set --password, NEO4J_PASSWORD")
}

func TestPromptPassword_NonTTY_AcceptEnvVarsOn_NamesEnvVar(t *testing.T) {
	origTTY := dbconn.StdinIsTTY
	t.Cleanup(func() { dbconn.StdinIsTTY = origTTY })
	dbconn.StdinIsTTY = func() bool { return false }

	t.Setenv("NEO4J_CLI_ACCEPT_ENV_VARS", "1")

	fs := afero.NewMemMapFs()
	cfg := clicfg.NewConfig(fs, "test", clicfg.QueryScope)
	cmd := NewCmd(cfg)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetOut(&bytes.Buffer{})

	_, err := promptPassword(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NEO4J_PASSWORD")
}

// stubEmbedProvider is a deterministic Provider used by the runQuery embed
// tests below. Embed appends a sentinel float so each invocation can be
// inspected, and bumps `calls` for cardinality checks. When `failOn` matches
// the input text exactly, Embed returns an error instead of a vector — used
// to verify provider errors abort the command before any Cypher is sent.
type stubEmbedProvider struct {
	calls  int
	inputs []string
	failOn string
}

func (s *stubEmbedProvider) Embed(_ context.Context, text string) ([]float32, error) {
	s.calls++
	s.inputs = append(s.inputs, text)
	if s.failOn != "" && text == s.failOn {
		return nil, errors.New("provider boom")
	}
	return []float32{1, 2, 3}, nil
}

// TestRunQuery_EmbedParam_VectorForwardedToBothPreflightAndRun verifies that
// `--param NAME:embed=TEXT` produces exactly one provider call AND the same
// vector lands in both the EXPLAIN preflight params map and the real run
// params map. Locks REQ-F-019: embedding happens once before EXPLAIN.
func TestRunQuery_EmbedParam_VectorForwardedToBothPreflightAndRun(t *testing.T) {
	stub := &stubEmbedProvider{}
	restore := embed.WithFactory(func(_ embed.Config) (embed.Provider, error) {
		return stub, nil
	})
	t.Cleanup(restore)
	t.Setenv("NEO4J_EMBED_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "k")

	var explainParams, runParams map[string]any
	withRunStatementSeam(t, func(_ context.Context, _ *conn, statement string, params map[string]any, _ bool) (*queryResponse, error) {
		if strings.HasPrefix(statement, "EXPLAIN ") {
			explainParams = params
			resp := makeQueryResponse([]string{"n"}, [][]any{})
			resp.QueryType = neo4j.QueryTypeReadOnly
			return resp, nil
		}
		runParams = params
		return makeQueryResponse([]string{"n"}, [][]any{{int64(1)}}), nil
	})

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--param=q:embed=hello",
		"RETURN $q",
	)
	require.NoError(t, err)
	assert.Equal(t, 1, stub.calls, "provider must be called exactly once per :embed param")
	assert.Equal(t, []string{"hello"}, stub.inputs)
	assert.Equal(t, []float32{1, 2, 3}, explainParams["q"])
	assert.Equal(t, []float32{1, 2, 3}, runParams["q"])
}

// TestRunQuery_EmbedParam_RWStillEmbedsOnce verifies that --rw routing (which
// skips EXPLAIN preflight) still results in exactly one provider call: the
// embed step lives upstream of the preflight branch.
func TestRunQuery_EmbedParam_RWStillEmbedsOnce(t *testing.T) {
	stub := &stubEmbedProvider{}
	restore := embed.WithFactory(func(_ embed.Config) (embed.Provider, error) {
		return stub, nil
	})
	t.Cleanup(restore)
	t.Setenv("NEO4J_EMBED_PROVIDER", "ollama")

	var runParams map[string]any
	withRunStatementSeam(t, func(_ context.Context, _ *conn, _ string, params map[string]any, _ bool) (*queryResponse, error) {
		runParams = params
		return makeQueryResponse([]string{}, [][]any{}), nil
	})

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--rw",
		"--param=q:embed=hello",
		"CREATE (n {v: $q}) RETURN n",
	)
	require.NoError(t, err)
	assert.Equal(t, 1, stub.calls, "provider must be called exactly once even with --rw")
	assert.Equal(t, []float32{1, 2, 3}, runParams["q"])
}

// TestRunQuery_EmbedParam_MixedLiteralAndEmbed verifies a query with both a
// literal `--param k=v` and a `--param k:embed=...` ends up with both keys in
// the same params map, with the literal kept as a JSON-typed value and the
// embed slot replaced by the resolved vector.
func TestRunQuery_EmbedParam_MixedLiteralAndEmbed(t *testing.T) {
	stub := &stubEmbedProvider{}
	restore := embed.WithFactory(func(_ embed.Config) (embed.Provider, error) {
		return stub, nil
	})
	t.Cleanup(restore)
	t.Setenv("NEO4J_EMBED_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "k")

	var runParams map[string]any
	withRunStatementSeam(t, func(_ context.Context, _ *conn, statement string, params map[string]any, _ bool) (*queryResponse, error) {
		if strings.HasPrefix(statement, "EXPLAIN ") {
			resp := makeQueryResponse([]string{"n"}, [][]any{})
			resp.QueryType = neo4j.QueryTypeReadOnly
			return resp, nil
		}
		runParams = params
		return makeQueryResponse([]string{"n"}, [][]any{{int64(1)}}), nil
	})

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--param=limit=10",
		"--param=q:embed=hello",
		"RETURN $q, $limit",
	)
	require.NoError(t, err)
	assert.Equal(t, 1, stub.calls)
	assert.Equal(t, []float32{1, 2, 3}, runParams["q"])
	// JSON-typed literal: 10 unmarshals as float64 (encoding/json's default).
	assert.Equal(t, float64(10), runParams["limit"])
}

// TestRunQuery_EmbedParam_ProviderErrorAbortsBeforeExplain verifies a provider
// error short-circuits the command before any Cypher (including EXPLAIN) is
// sent to the driver. A panicking driverOpener AND a panicking statement seam
// confirm we never reach driver construction or statement execution.
func TestRunQuery_EmbedParam_ProviderErrorAbortsBeforeExplain(t *testing.T) {
	stub := &stubEmbedProvider{failOn: "boom-input"}
	restore := embed.WithFactory(func(_ embed.Config) (embed.Provider, error) {
		return stub, nil
	})
	t.Cleanup(restore)
	t.Setenv("NEO4J_EMBED_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "k")

	// Swap driverOpener to panic so we prove no Bolt driver is ever opened.
	origOpener := driverOpener
	t.Cleanup(func() { driverOpener = origOpener })
	driverOpener = func(_, _, _, _ string, _ bool) (neo4j.Driver, error) {
		panic("driverOpener must not be called when embed errors before driver-open")
	}

	origFn := runStatementResponseFn
	t.Cleanup(func() { runStatementResponseFn = origFn })
	runStatementResponseFn = func(_ context.Context, _ *conn, statement string, _ map[string]any, _ bool) (*queryResponse, error) {
		t.Fatalf("statement seam must not be called when provider errors: got %q", statement)
		return nil, nil
	}

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--param=q:embed=boom-input",
		"RETURN $q",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embed")
	assert.Contains(t, err.Error(), "provider boom")
	assert.Equal(t, 1, stub.calls, "provider was the failing call site; should be hit exactly once")
}

// TestRunQuery_EmbedParam_MultipleJobsPreserveOrder verifies multiple
// `:embed` params produce one provider call per job in command-line order;
// each entry's vector lands in its own params slot.
func TestRunQuery_EmbedParam_MultipleJobsPreserveOrder(t *testing.T) {
	stub := &stubEmbedProvider{}
	restore := embed.WithFactory(func(_ embed.Config) (embed.Provider, error) {
		return stub, nil
	})
	t.Cleanup(restore)
	t.Setenv("NEO4J_EMBED_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "k")

	withRunStatementSeam(t, func(_ context.Context, _ *conn, statement string, _ map[string]any, _ bool) (*queryResponse, error) {
		if strings.HasPrefix(statement, "EXPLAIN ") {
			resp := makeQueryResponse([]string{"n"}, [][]any{})
			resp.QueryType = neo4j.QueryTypeReadOnly
			return resp, nil
		}
		return makeQueryResponse([]string{"n"}, [][]any{{int64(1)}}), nil
	})

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		"--param=a:embed=first",
		"--param=b:embed=second",
		"RETURN $a, $b",
	)
	require.NoError(t, err)
	assert.Equal(t, 2, stub.calls)
	assert.Equal(t, []string{"first", "second"}, stub.inputs)
}

// TestRunQuery_DebugFlagDoesNotChangeStdout locks REQ-F-009: running with
// --debug=true vs --debug=false against the same seam-stubbed response must
// produce byte-identical stdout. Driver-side debug output is routed to stderr
// (see TestBuildDriverConfigurer_DebugOnAttachesStderrLogger) — stdout stays
// reserved for the rendered query result so `--format json` pipelines remain
// safe under --debug.
func TestRunQuery_DebugFlagDoesNotChangeStdout(t *testing.T) {
	runOnce := func(t *testing.T, debugArg string) string {
		t.Helper()
		t.Setenv("NEO4J_DEBUG", "")

		r := newSeamRouter()
		r.resp["EXPLAIN RETURN 1 AS n"] = func() *queryResponse {
			resp := makeQueryResponse([]string{"n"}, [][]any{})
			resp.QueryType = neo4j.QueryTypeReadOnly
			return resp
		}()
		r.resp["RETURN 1 AS n"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
		r.install(t)

		h := newRunHarness(t, "json")
		err := h.execute(t,
			"--uri=neo4j://example:7687",
			"--password=pw",
			debugArg,
			"RETURN 1 AS n",
		)
		require.NoError(t, err)
		return h.stdout.String()
	}

	debugOff := runOnce(t, "--debug=false")
	debugOn := runOnce(t, "--debug=true")
	assert.Equal(t, debugOff, debugOn,
		"--debug must not alter stdout bytes (driver output is routed to stderr)")
}
