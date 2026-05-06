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

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
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
	origIsTTY := stdinIsTTY
	origStdin := stdinReader
	origPwReader := passwordReader
	t.Cleanup(func() {
		stdinIsTTY = origIsTTY
		stdinReader = origStdin
		passwordReader = origPwReader
	})

	// Default to "TTY" so commands that don't pipe stdin behave like an
	// interactive session (the missing-cypher path returns a usage error
	// rather than blocking on stdin).
	stdinIsTTY = func() bool { return true }
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

// seamRouter is a tiny statement-router used to swap the runStatementResponseFn
// seam during tests. Each entry maps an exact statement string → response or
// error. Tests can append cypher to the calls slice for ordering assertions.
type seamRouter struct {
	calls   []string
	resp    map[string]*queryResponse
	respErr map[string]error
	// onUnexpected fires when a statement does not match any route — defaults
	// to fatal-fail to surface unexpected calls in tests.
	onUnexpected func(statement string) (*queryResponse, error)
}

func (r *seamRouter) handle(_ context.Context, _ *conn, statement string, _ map[string]any) (*queryResponse, error) {
	r.calls = append(r.calls, statement)
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
		resp:    map[string]*queryResponse{},
		respErr: map[string]error{},
	}
}

func makeQueryResponse(fields []string, values [][]any) *queryResponse {
	resp := &queryResponse{}
	resp.Data.Fields = fields
	resp.Data.Values = values
	return resp
}

func makePlan(operatorType string, children ...queryPlan) *queryPlan {
	out := &queryPlan{OperatorType: operatorType}
	if len(children) > 0 {
		out.Children = children
	}
	return out
}

func TestRunQuery_HappyPath_TableOutput(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1 AS n"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n", "m"}, [][]any{})
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
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
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
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
	r.resp["EXPLAIN MATCH (n) RETURN n"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryPlan = makePlan("ProduceResults@neo4j", queryPlan{OperatorType: "NodeByLabelScan@neo4j"})
		return resp
	}()
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
}

func TestRunQuery_WriteCypherWithoutRwErrorsBeforeExecution(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN CREATE (n)"] = func() *queryResponse {
		resp := makeQueryResponse([]string{}, [][]any{})
		resp.QueryPlan = makePlan("ProduceResults@neo4j",
			queryPlan{OperatorType: "EmptyResult@neo4j", Children: []queryPlan{{OperatorType: "Create@neo4j"}}})
		return resp
	}()
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

func TestQueryPlanAllowsWrite(t *testing.T) {
	tests := []struct {
		name string
		plan *queryPlan
		want bool
	}{
		{
			name: "nil plan is read",
			plan: nil,
			want: false,
		},
		{
			name: "read operators stay read",
			plan: &queryPlan{OperatorType: "ProduceResults@neo4j", Children: []queryPlan{{OperatorType: "Filter@neo4j"}}},
			want: false,
		},
		{
			name: "write operator in child marks write",
			plan: &queryPlan{OperatorType: "ProduceResults@neo4j", Children: []queryPlan{{OperatorType: "EmptyResult@neo4j", Children: []queryPlan{{OperatorType: "Create@neo4j"}}}}},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, queryPlanAllowsWrite(tc.plan))
		})
	}
}

func TestOperatorLooksLikeWrite(t *testing.T) {
	tests := []struct {
		operator string
		want     bool
	}{
		{operator: "ProduceResults@neo4j", want: false},
		{operator: "Filter@neo4j", want: false},
		{operator: "Create@neo4j", want: true},
		{operator: "SetProperty@neo4j", want: true},
		{operator: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.operator, func(t *testing.T) {
			assert.Equal(t, tc.want, operatorLooksLikeWrite(tc.operator))
		})
	}
}

func TestRunQuery_RowLimitTruncates_TableOutput(t *testing.T) {
	r := newSeamRouter()
	rows := make([][]any, 10)
	for i := range rows {
		rows[i] = []any{int64(i + 1)}
	}
	r.resp["EXPLAIN RETURN range(1,10)"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
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
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
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
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
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
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
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
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
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
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
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
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
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
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
		return resp
	}()
	r.resp["RETURN 1 AS n"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
	r.install(t)

	h := newRunHarness(t, "json")
	// Override seams: not a TTY; supply Cypher via "stdin".
	stdinIsTTY = func() bool { return false }
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
	// stdinIsTTY default is true via harness.

	err := h.execute(t, "--uri=neo4j://example:7687", "--password=pw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Cypher")
}

func TestRunQuery_EmptyStdinNonTTYReturnsUsageError(t *testing.T) {
	h := newRunHarness(t, "table")
	stdinIsTTY = func() bool { return false }
	stdinReader = func() io.Reader { return strings.NewReader("   \n  ") }

	err := h.execute(t, "--uri=neo4j://example:7687", "--password=pw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Cypher")
}

func TestRunQuery_PasswordFromEnvSkipsPrompt(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
		return resp
	}()
	r.resp["RETURN 1"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
	r.install(t)

	h := newRunHarness(t, "json")
	t.Setenv(envPassword, "from-env")
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envDatabase, "")
	t.Setenv(envInsecure, "")

	// Set passwordReader so a buggy fallthrough would surface as a test
	// failure (returning a sentinel that wouldn't match).
	passwordReader = func() (string, error) {
		t.Fatal("passwordReader must NOT be invoked when env supplies password")
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
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
		return resp
	}()
	r.resp["RETURN 1"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
	r.install(t)

	h := newRunHarness(t, "json")

	// Clear env-based password.
	t.Setenv(envPassword, "")

	called := false
	passwordReader = func() (string, error) {
		called = true
		return "typed-at-prompt", nil
	}

	err := h.execute(t, "--uri=neo4j://example:7687", "--username=u", "RETURN 1")
	require.NoError(t, err)
	assert.True(t, called, "passwordReader must be invoked on TTY when no password is set")
	assert.Contains(t, h.stderr.String(), "Password:")
}

func TestRunQuery_PasswordMissingNonTTYReturnsClearError(t *testing.T) {
	h := newRunHarness(t, "json")
	stdinIsTTY = func() bool { return false }
	// Provide stdin Cypher so the early Cypher check passes.
	stdinReader = func() io.Reader { return strings.NewReader("RETURN 1") }
	t.Setenv(envPassword, "")

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
	withRunStatementSeam(t, func(_ context.Context, _ *conn, statement string, params map[string]any) (*queryResponse, error) {
		if strings.HasPrefix(statement, "EXPLAIN ") {
			resp := makeQueryResponse([]string{"n"}, [][]any{})
			resp.QueryPlan = makePlan("ProduceResults@neo4j")
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
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
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
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
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
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
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
// still emits the documented stderr notice. Bolt-family inputs are rewritten
// to http(s):// (the rewrite logic lives in uri.go and is independent of the
// underlying transport).
func TestRunQuery_URIRewriteEmitsStderrNotice(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
		return resp
	}()
	r.resp["RETURN 1"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=bolt://127.0.0.1:9999",
		"--password=pw",
		"RETURN 1",
	)
	require.NoError(t, err)

	stderr := h.stderr.String()
	assert.Contains(t, stderr, "info: rewrote URI 'bolt://127.0.0.1:9999'",
		"stderr must contain the rewrite notice with the original URI")

	var got decodedResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, []string{"n"}, got.Columns)
}

// TestRunQuery_URIPassthroughEmitsNoNotice verifies that already-correct
// http(s) URIs do NOT trigger the rewrite notice.
func TestRunQuery_URIPassthroughEmitsNoNotice(t *testing.T) {
	r := newSeamRouter()
	r.resp["EXPLAIN RETURN 1"] = func() *queryResponse {
		resp := makeQueryResponse([]string{"n"}, [][]any{})
		resp.QueryPlan = makePlan("ProduceResults@neo4j")
		return resp
	}()
	r.resp["RETURN 1"] = makeQueryResponse([]string{"n"}, [][]any{{int64(1)}})
	r.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=http://example:7474",
		"--password=pw",
		"RETURN 1",
	)
	require.NoError(t, err)

	assert.NotContains(t, h.stderr.String(), "rewrote URI",
		"http(s) URIs must pass through without a rewrite notice")
}

func TestPromptPassword_NonTTYReturnsUsageError(t *testing.T) {
	origTTY := stdinIsTTY
	t.Cleanup(func() { stdinIsTTY = origTTY })
	stdinIsTTY = func() bool { return false }

	fs := afero.NewMemMapFs()
	cfg := clicfg.NewConfig(fs, "test", clicfg.QueryScope)
	cmd := NewCmd(cfg)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetOut(&bytes.Buffer{})

	_, err := promptPassword(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--password")
	assert.Contains(t, err.Error(), "NEO4J_PASSWORD")
	assert.Contains(t, err.Error(), ".env")
}
