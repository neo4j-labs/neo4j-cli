// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/query/linter"
)

// withLintSeam swaps the lintFn seam for the duration of the test so policy
// paths (warnings-only exit 0, engine failure → fatal) run deterministically
// without the real engine.
func withLintSeam(t *testing.T, fn func(query string, version linter.Version) ([]linter.Diagnostic, error)) {
	t.Helper()
	orig := lintFn
	t.Cleanup(func() { lintFn = orig })
	lintFn = fn
}

// TestQueryLint_CleanQueryEmptyJSONArray verifies a clean query renders as an
// empty JSON array (never null) and exits zero. Uses the real engine.
func TestQueryLint_CleanQueryEmptyJSONArray(t *testing.T) {
	h := newRunHarness(t, "json")
	err := h.execute(t, ":lint", "MATCH (n) RETURN n")
	require.NoError(t, err)
	assert.Equal(t, "[]", strings.TrimSpace(h.stdout.String()))
}

// TestQueryLint_ErrorDiagnosticsAndExitCode6 verifies an error-severity
// diagnostic renders a row AND fails the command with the validation exit
// code (6). Diagnostics must hit stdout before the error returns. Uses the
// real engine.
func TestQueryLint_ErrorDiagnosticsAndExitCode6(t *testing.T) {
	h := newRunHarness(t, "json")
	err := h.execute(t, ":lint", "MATCH (n) RETURN m")
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T", err)
	assert.Equal(t, 6, ce.Code)
	assert.Contains(t, ce.Message, "1 error(s) found")

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &rows), "diagnostics must print before the error returns")
	require.Len(t, rows, 1)
	assert.Equal(t, "error", rows[0]["severity"])
	assert.Contains(t, rows[0]["message"], "`m`")
	assert.Equal(t, float64(1), rows[0]["line"], "line must be 1-indexed")
	assert.Equal(t, float64(17), rows[0]["offset"], "offset stays 0-indexed")
}

// TestQueryLint_JSONShapeSnakeCase locks the exact JSON key set of a
// diagnostic row.
func TestQueryLint_JSONShapeSnakeCase(t *testing.T) {
	h := newRunHarness(t, "json")
	err := h.execute(t, ":lint", "MATCH (n) RETURN m")
	require.Error(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &rows))
	require.NotEmpty(t, rows)
	keys := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t,
		[]string{"severity", "message", "line", "column", "offset", "end_line", "end_column", "end_offset"},
		keys)
}

// TestQueryLint_SyntaxErrorReported verifies syntactically invalid Cypher is
// reported (the analyzer covers parsing too, not only semantics).
func TestQueryLint_SyntaxErrorReported(t *testing.T) {
	h := newRunHarness(t, "json")
	err := h.execute(t, ":lint", "MATCH (n RETURN n")
	require.Error(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &rows))
	require.NotEmpty(t, rows)
	assert.Contains(t, strings.ToLower(rows[0]["message"].(string)), "invalid input")
}

// TestQueryLint_StdinInput verifies piped stdin is consumed when no
// positional argument is supplied, mirroring the parent and `:embed`.
func TestQueryLint_StdinInput(t *testing.T) {
	h := newRunHarness(t, "json")
	stdinIsTTY = func() bool { return false }
	stdinReader = func() io.Reader { return strings.NewReader("MATCH (n) RETURN m\n") }

	err := h.execute(t, ":lint")
	require.Error(t, err, "stdin query has an undefined variable, lint must fail")

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "error", rows[0]["severity"])
}

// TestQueryLint_TTYStdinNoArgReturnsUsageError verifies a TTY stdin with no
// positional arg surfaces the shared "no Cypher" usage error.
func TestQueryLint_TTYStdinNoArgReturnsUsageError(t *testing.T) {
	h := newRunHarness(t, "json")
	// stdinIsTTY default is true via harness.

	err := h.execute(t, ":lint")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Cypher")
}

// TestQueryLint_InvalidCypherVersionUsageError verifies flag validation maps
// to a usage error (exit 2).
func TestQueryLint_InvalidCypherVersionUsageError(t *testing.T) {
	h := newRunHarness(t, "json")
	err := h.execute(t, ":lint", "RETURN 1", "--cypher-version", "4")

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T", err)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, ce.Message, "cypher-version")
}

// TestQueryLint_Cypher25FlagAccepted verifies --cypher-version 25 reaches the
// analyzer and lints cleanly.
func TestQueryLint_Cypher25FlagAccepted(t *testing.T) {
	h := newRunHarness(t, "json")
	err := h.execute(t, ":lint", "MATCH (n) RETURN n", "--cypher-version", "25")
	require.NoError(t, err)
	assert.Equal(t, "[]", strings.TrimSpace(h.stdout.String()))
}

// TestQueryLint_WarningsOnlyExitZero verifies warnings render but do NOT fail
// the command. Driven through the seam: a stable warnings-only query would
// couple the test to analyzer release behavior.
func TestQueryLint_WarningsOnlyExitZero(t *testing.T) {
	withLintSeam(t, func(_ string, _ linter.Version) ([]linter.Diagnostic, error) {
		return []linter.Diagnostic{{
			Severity: "warning",
			Message:  "deprecated thing",
			Start:    linter.Position{Offset: 0, Line: 0, Column: 0},
			End:      linter.Position{Offset: 3, Line: 0, Column: 3},
		}}, nil
	})

	h := newRunHarness(t, "json")
	err := h.execute(t, ":lint", "RETURN 1")
	require.NoError(t, err, "warnings alone must not fail the command")

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "warning", rows[0]["severity"])
}

// TestQueryLint_EngineErrorIsFatal verifies an engine failure maps to a fatal
// error (exit 1) with the package's `query: lint:` prefix.
func TestQueryLint_EngineErrorIsFatal(t *testing.T) {
	withLintSeam(t, func(_ string, _ linter.Version) ([]linter.Diagnostic, error) {
		return nil, errors.New("boom")
	})

	h := newRunHarness(t, "json")
	err := h.execute(t, ":lint", "RETURN 1")

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T", err)
	assert.Equal(t, 1, ce.Code)
	assert.Contains(t, ce.Message, "query: lint: boom")
}

// TestQueryLint_NoBoltConnection_NoPasswordPrompt locks the offline contract:
// `:lint` must NOT open a Bolt driver, run statements, or prompt for a
// password (panicking/fataling seams prove all three).
func TestQueryLint_NoBoltConnection_NoPasswordPrompt(t *testing.T) {
	t.Setenv(envPassword, "")

	origOpener := driverOpener
	t.Cleanup(func() { driverOpener = origOpener })
	driverOpener = func(_, _, _, _ string, _ bool) (neo4j.Driver, error) {
		panic("driverOpener must not be called from `:lint`")
	}

	origRunFn := runStatementResponseFn
	t.Cleanup(func() { runStatementResponseFn = origRunFn })
	runStatementResponseFn = func(_ context.Context, _ *conn, statement string, _ map[string]any, _ bool) (*queryResponse, error) {
		t.Fatalf("statement seam must not be called from `:lint`: got %q", statement)
		return nil, nil
	}

	h := newRunHarness(t, "json")
	passwordReader = func() (string, error) {
		t.Fatal("passwordReader must NOT be invoked from `:lint`")
		return "", nil
	}

	err := h.execute(t, ":lint", "MATCH (n) RETURN n")
	require.NoError(t, err)
	assert.NotContains(t, h.stderr.String(), "Password:")
}

// TestQueryLint_TableOutput verifies table rendering shows the field headers
// and the diagnostic message.
func TestQueryLint_TableOutput(t *testing.T) {
	h := newRunHarness(t, "table")
	err := h.execute(t, ":lint", "MATCH (n) RETURN m")
	require.Error(t, err)

	out := strings.ToLower(h.stdout.String())
	assert.Contains(t, out, "severity")
	assert.Contains(t, out, "message")
	assert.Contains(t, out, "not defined")
}

// TestQueryLint_ToonOutput verifies --format toon yields a TOON document that
// does NOT parse as JSON.
func TestQueryLint_ToonOutput(t *testing.T) {
	h := newRunHarness(t, "toon")
	err := h.execute(t, ":lint", "MATCH (n) RETURN m")
	require.Error(t, err)

	out := h.stdout.String()
	assert.NotEmpty(t, out)
	var v any
	jsonErr := json.Unmarshal([]byte(out), &v)
	assert.Error(t, jsonErr, "toon output must not parse as JSON, got: %s", out)
}

// TestLintDiagnostics_MarshalJSON locks nil/empty rendering to `[]`.
func TestLintDiagnostics_MarshalJSON(t *testing.T) {
	var d lintDiagnostics
	b, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Equal(t, "[]", string(b))
}
