// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

	cfgJSON := `{"aura":{"output":"` + output + `"}}`
	fs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)
	return &runHarness{
		cfg:    clicfg.NewConfig(fs, "test"),
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
	}
}

// startServer boots a one-handler httptest server returning the given
// response body. Status defaults to 200 when 0.
func startServer(t *testing.T, status int, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (h *runHarness) execute(t *testing.T, args ...string) error {
	t.Helper()
	// Use afero.NewMemMapFs for the cobra command itself; the harness's cfg
	// owns the testfs filesystem, which is what cfg.Aura.Output() reads.
	cmd := NewCmd(h.cfg)
	cmd.SetOut(h.stdout)
	cmd.SetErr(h.stderr)
	cmd.SetContext(context.Background())
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestRunQuery_HappyPath_TableOutput(t *testing.T) {
	srv := startServer(t, 0, []byte(`{"data":{"fields":["n","m"],"values":[[1,"alice"],[2,"bob"]]}}`))

	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri="+srv.URL,
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
	srv := startServer(t, 0, []byte(`{"data":{"fields":["n"],"values":[[42]]}}`))

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
		"--password=pw",
		"RETURN 42 AS n",
	)
	require.NoError(t, err)

	var got jsonRowsResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, []string{"n"}, got.Columns)
	assert.False(t, got.Truncated)
	require.Len(t, got.Rows, 1)
	assert.Equal(t, float64(42), got.Rows[0]["n"])
}

func TestRunQuery_ServerErrorSurfacesCodeAndMessage(t *testing.T) {
	srv := startServer(t, http.StatusBadRequest,
		[]byte(`{"errors":[{"code":"Neo.ClientError.Statement.SyntaxError","message":"Invalid input"}]}`))

	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri="+srv.URL,
		"--password=pw",
		"BAD CYPHER",
	)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "Neo.ClientError.Statement.SyntaxError")
	assert.Contains(t, msg, "Invalid input")
}

func TestRunQuery_RowLimitTruncates_TableOutput(t *testing.T) {
	// 10 rows, --max-rows=2 → expect the warning regardless of output mode.
	srv := startServer(t, 0, []byte(`{"data":{"fields":["n"],"values":[[1],[2],[3],[4],[5],[6],[7],[8],[9],[10]]}}`))

	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri="+srv.URL,
		"--password=pw",
		"--max-rows=2",
		"RETURN range(1,10)",
	)
	require.NoError(t, err)

	stderr := h.stderr.String()
	assert.Contains(t, stderr, "truncated to 2 rows")
	assert.Contains(t, stderr, "--max-rows 0 for unlimited")

	// Body should contain rows 1 and 2 but not later rows; check via the
	// rendered body cells. "10" would be the only multi-digit cell.
	out := h.stdout.String()
	assert.Contains(t, out, "1")
	assert.Contains(t, out, "2")
	// Render order is row-major; absence of "10" anywhere in the table body
	// is a sufficient proxy for the cap being applied. (The stderr warning
	// also asserts the cap, but this confirms the rendered body honours it.)
	assert.NotContains(t, out, "10")
}

func TestRunQuery_RowLimitTruncates_JSONSetsTruncatedTrueAndPrintsWarning(t *testing.T) {
	srv := startServer(t, 0, []byte(`{"data":{"fields":["n"],"values":[[1],[2],[3]]}}`))

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
		"--password=pw",
		"--max-rows=1",
		"RETURN range(1,3)",
	)
	require.NoError(t, err)

	var got jsonRowsResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.True(t, got.Truncated, "JSON envelope must report truncated:true")
	assert.Len(t, got.Rows, 1)

	// Warning fires regardless of output mode.
	assert.Contains(t, h.stderr.String(), "truncated to 1 rows")
}

func TestRunQuery_RowLimitZeroMeansUnlimited(t *testing.T) {
	srv := startServer(t, 0, []byte(`{"data":{"fields":["n"],"values":[[1],[2],[3]]}}`))

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
		"--password=pw",
		"--max-rows=0",
		"RETURN x",
	)
	require.NoError(t, err)

	var got jsonRowsResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.False(t, got.Truncated)
	assert.Len(t, got.Rows, 3)
	assert.NotContains(t, h.stderr.String(), "truncated")
}

func TestRunQuery_TruncateArraysAppliesBeforeRowCap(t *testing.T) {
	// One row with a 5-element array; --truncate-arrays-over=3 should rewrite
	// the value to an empty slice; --max-rows=10 leaves the row intact.
	srv := startServer(t, 0, []byte(`{"data":{"fields":["xs"],"values":[[[1,2,3,4,5]]]}}`))

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
		"--password=pw",
		"--truncate-arrays-over=3",
		"RETURN xs",
	)
	require.NoError(t, err)

	var got jsonRowsResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	require.Len(t, got.Rows, 1)
	xs, ok := got.Rows[0]["xs"].([]any)
	require.True(t, ok, "xs must be []any after truncation")
	assert.Empty(t, xs, "over-limit array must be elided to []")
}

// TestRunQuery_TruncateArrays_JSONOutputContainsEmptyArray verifies the
// rendered JSON literally contains `"xs": []` for an over-limit top-level
// array — closes the gap where in-memory shape was tested but not the
// actual `--output json` byte stream.
func TestRunQuery_TruncateArrays_JSONOutputContainsEmptyArray(t *testing.T) {
	// 10-item array; --truncate-arrays-over=3 → emit empty array.
	srv := startServer(t, 0, []byte(`{"data":{"fields":["xs"],"values":[[[1,2,3,4,5,6,7,8,9,10]]]}}`))

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
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
	// One row, one column "obj" whose value is {"data": [1..10]}.
	srv := startServer(t, 0, []byte(
		`{"data":{"fields":["obj"],"values":[[{"data":[1,2,3,4,5,6,7,8,9,10]}]]}}`))

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
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

// TestRunQuery_TruncateArrays_TableOutputCellIsEmptyArray covers --output
// table: the cell rendering for an over-limit array must be `[]` (the
// JSON-stringified empty array), not the legacy placeholder string.
func TestRunQuery_TruncateArrays_TableOutputCellIsEmptyArray(t *testing.T) {
	srv := startServer(t, 0, []byte(`{"data":{"fields":["xs"],"values":[[[1,2,3,4,5,6,7,8,9,10]]]}}`))

	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri="+srv.URL,
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
	srv := startServer(t, 0, []byte(`{"data":{"fields":["n"],"values":[[1]]}}`))

	h := newRunHarness(t, "json")
	// Override seams: not a TTY; supply Cypher via "stdin".
	stdinIsTTY = func() bool { return false }
	stdinReader = func() io.Reader { return strings.NewReader("RETURN 1 AS n") }

	err := h.execute(t,
		"--uri="+srv.URL,
		"--password=pw",
	)
	require.NoError(t, err)

	var got jsonRowsResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, []string{"n"}, got.Columns)
}

func TestRunQuery_NoCypherOnTTYReturnsUsageError(t *testing.T) {
	h := newRunHarness(t, "table")
	// stdinIsTTY default is true via harness.

	err := h.execute(t, "--uri=http://localhost:0", "--password=pw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Cypher")
}

func TestRunQuery_EmptyStdinNonTTYReturnsUsageError(t *testing.T) {
	h := newRunHarness(t, "table")
	stdinIsTTY = func() bool { return false }
	stdinReader = func() io.Reader { return strings.NewReader("   \n  ") }

	err := h.execute(t, "--uri=http://localhost:0", "--password=pw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Cypher")
}

func TestRunQuery_PasswordFromEnvSkipsPrompt(t *testing.T) {
	srv := startServer(t, 0, []byte(`{"data":{"fields":["n"],"values":[[1]]}}`))

	h := newRunHarness(t, "json")
	t.Setenv(envPassword, "from-env")
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envDatabase, "")
	t.Setenv(envInsecure, "")

	// Set passwordReader so a buggy fallthrough would surface as a test
	// failure (returning a sentinel that wouldn't match basic auth).
	passwordReader = func() (string, error) {
		t.Fatal("passwordReader must NOT be invoked when env supplies password")
		return "", nil
	}

	err := h.execute(t,
		"--uri="+srv.URL,
		"--username=u",
		"RETURN 1",
	)
	require.NoError(t, err)
}

func TestRunQuery_PasswordPromptedOnTTY(t *testing.T) {
	srv := startServer(t, 0, []byte(`{"data":{"fields":["n"],"values":[[1]]}}`))

	h := newRunHarness(t, "json")

	// Clear env-based password.
	t.Setenv(envPassword, "")

	called := false
	passwordReader = func() (string, error) {
		called = true
		return "typed-at-prompt", nil
	}

	err := h.execute(t, "--uri="+srv.URL, "--username=u", "RETURN 1")
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

	err := h.execute(t, "--uri=http://localhost:0", "--username=u")
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "--password")
	assert.Contains(t, msg, "NEO4J_PASSWORD")
	assert.Contains(t, msg, ".env")
}

func TestRunQuery_InvalidParamReturnsUsageError(t *testing.T) {
	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri=http://localhost:0",
		"--password=pw",
		"--param=missing-equals",
		"RETURN 1",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key=value")
}

func TestRunQuery_ParamsForwardedAsRequestBody(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = w.Write([]byte(`{"data":{"fields":["n"],"values":[[1]]}}`))
	}))
	t.Cleanup(srv.Close)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
		"--password=pw",
		"--param=n=5",
		"--param=name=alice",
		"RETURN $n, $name",
	)
	require.NoError(t, err)
	require.Contains(t, gotBody, "parameters")
	params, ok := gotBody["parameters"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(5), params["n"])
	assert.Equal(t, "alice", params["name"])
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
	srv := startServer(t, 0, []byte(`{"data":{"fields":["xs"],"values":[[[1,2,3,4,5,6,7,8,9,10]]]}}`))

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
		"--password=pw",
		"--truncate-arrays-over=3",
		"RETURN xs",
	)
	require.NoError(t, err)

	stderr := h.stderr.String()
	assert.Contains(t, stderr,
		"warning: truncated 1 arrays larger than 3 items (use --truncate-arrays-over 0 to disable)",
		"stderr must contain the exact aggregate warning line")

	var got jsonRowsResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, 1, got.ArraysTruncated, "JSON envelope must report arrays_truncated=1")
	assert.False(t, got.Truncated, "row-cap signal must remain false")
}

// TestRunQuery_TruncateArrays_Table_AggregateWarning verifies that table
// output emits the exact aggregate warning to stderr while leaving the
// table body unchanged (cells render as `[]` per task-011).
func TestRunQuery_TruncateArrays_Table_AggregateWarning(t *testing.T) {
	srv := startServer(t, 0, []byte(`{"data":{"fields":["xs"],"values":[[[1,2,3,4,5,6,7,8,9,10]]]}}`))

	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri="+srv.URL,
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
	srv := startServer(t, 0, []byte(`{"data":{"fields":["xs"],"values":[[[1,2,3]]]}}`))

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
		"--password=pw",
		"--truncate-arrays-over=10",
		"RETURN xs",
	)
	require.NoError(t, err)

	stderr := h.stderr.String()
	assert.NotContains(t, stderr, "arrays larger than",
		"stderr must be silent for the array-truncation warning when nothing was elided")

	var got jsonRowsResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, 0, got.ArraysTruncated, "arrays_truncated must be 0 when nothing was elided")
}

// TestRunQuery_URIRewriteEmitsStderrNotice verifies the auto-rewrite path:
// passing a bolt:// URI rewrites to http:// before the HTTP request fires,
// the request reaches the test server (custom port preserved), and exactly
// one info-line is emitted to stderr.
func TestRunQuery_URIRewriteEmitsStderrNotice(t *testing.T) {
	srv := startServer(t, 0, []byte(`{"data":{"fields":["n"],"values":[[1]]}}`))

	// httptest.NewServer URL is `http://127.0.0.1:<port>`. Strip the scheme
	// and feed it back as `bolt://127.0.0.1:<port>`. The custom port (not
	// 7687) is preserved by the rewriter, so the request still lands on the
	// test server after the scheme rewrite to http://.
	hostPort := strings.TrimPrefix(srv.URL, "http://")
	boltURI := "bolt://" + hostPort

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+boltURI,
		"--password=pw",
		"RETURN 1",
	)
	require.NoError(t, err)

	stderr := h.stderr.String()
	assert.Contains(t, stderr, "info: rewrote URI '"+boltURI+"' to 'http://"+hostPort+"'",
		"stderr must contain the rewrite notice with the original and rewritten URIs")
	assert.Contains(t, stderr,
		"the query command uses Neo4j's HTTP Query API; pass --uri https://... to silence",
		"stderr notice must include the explanation suffix")

	// The request must have actually hit the test server (response parsed OK).
	var got jsonRowsResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, []string{"n"}, got.Columns)
}

// TestRunQuery_URIPassthroughEmitsNoNotice verifies that already-correct
// http(s) URIs do NOT trigger the rewrite notice.
func TestRunQuery_URIPassthroughEmitsNoNotice(t *testing.T) {
	srv := startServer(t, 0, []byte(`{"data":{"fields":["n"],"values":[[1]]}}`))

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
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
	cfg := clicfg.NewConfig(fs, "test")
	cmd := NewCmd(cfg)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetOut(&bytes.Buffer{})

	_, err := promptPassword(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--password")
	assert.Contains(t, err.Error(), "NEO4J_PASSWORD")
	assert.Contains(t, err.Error(), ".env")
}
