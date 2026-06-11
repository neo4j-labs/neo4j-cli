// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/query/linter"
)

// withLintSeam swaps the lintFn seam for the duration of the test so policy
// paths (warnings-only exit 0, engine failure → fatal) run deterministically
// without the real engine.
func withLintSeam(t *testing.T, fn func(query string, version linter.Version, schema *linter.DbSchema) ([]linter.Diagnostic, error)) {
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
	withLintSeam(t, func(_ string, _ linter.Version, _ *linter.DbSchema) ([]linter.Diagnostic, error) {
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
	withLintSeam(t, func(_ string, _ linter.Version, _ *linter.DbSchema) ([]linter.Diagnostic, error) {
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

// lintFetchSeam returns a schemaSeam preloaded with the three --fetch-schema
// statements over a minimal movies-like graph: ACTED_IN only goes
// (Person)-[:ACTED_IN]->(Movie), default language CYPHER 5.
func lintFetchSeam() *schemaSeam {
	s := newSchemaSeam()
	s.resp[lintSummaryQuery] = makeQueryResponse(
		[]string{"result"},
		[][]any{
			{[]any{"Movie", "Person"}},
			{[]any{"ACTED_IN"}},
			{[]any{"title", "name"}},
		},
	)
	s.resp[lintGraphSchemaQuery] = makeQueryResponse(
		[]string{"nodes", "relationships"},
		[][]any{{
			[]any{
				dbtype.Node{ElementId: "n1", Labels: []string{"Person"}},
				dbtype.Node{ElementId: "n2", Labels: []string{"Movie"}},
			},
			[]any{
				dbtype.Relationship{ElementId: "r1", StartElementId: "n1", EndElementId: "n2", Type: "ACTED_IN"},
			},
		}},
	)
	s.resp["SHOW DATABASES YIELD name, home, defaultLanguage WHERE home RETURN defaultLanguage"] = makeQueryResponse(
		[]string{"defaultLanguage"},
		[][]any{{"CYPHER 5"}},
	)
	s.resp[lintProceduresQuery] = makeQueryResponse(
		[]string{"name", "description", "mode", "worksOnSystem", "argumentDescription", "returnDescription", "signature", "admin", "option"},
		[][]any{{
			"db.labels", "lists labels", "READ", false, []any{},
			[]any{map[string]any{"name": "label", "description": "", "type": "STRING", "isDeprecated": false}},
			"db.labels() :: (label :: STRING)", false, map[string]any{"deprecated": false},
		}},
	)
	s.resp[lintFunctionsQuery] = makeQueryResponse(
		[]string{"name", "category", "description", "signature", "isBuiltIn", "argumentDescription", "returnDescription", "aggregating", "isDeprecated"},
		[][]any{{
			"pi", "Numeric", "pi", "pi() :: FLOAT", true, []any{}, "FLOAT", false, false,
		}},
	)
	return s
}

// TestQueryLint_FetchSchema_UnknownLabelWarning verifies the full
// --fetch-schema path: schema statements run through the seam, the unknown
// label warns, and warnings alone keep exit 0. Uses the real engine.
func TestQueryLint_FetchSchema_UnknownLabelWarning(t *testing.T) {
	s := lintFetchSeam()
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":lint", "MATCH (n:NotALabel) RETURN n", "--fetch-schema",
	)
	require.NoError(t, err, "schema warnings must not affect the exit code")

	assert.Contains(t, s.calls, lintSummaryQuery)
	assert.Contains(t, s.calls, lintGraphSchemaQuery)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "warning", rows[0]["severity"])
	assert.Contains(t, rows[0]["message"], "NotALabel")
}

// TestQueryLint_FetchSchema_PathDirectionalityWarning verifies graphSchema
// triples from db.schema.visualization reach the analyzer: a pattern against
// the relationship's actual direction warns.
func TestQueryLint_FetchSchema_PathDirectionalityWarning(t *testing.T) {
	s := lintFetchSeam()
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":lint", "MATCH (m:Movie)-[:ACTED_IN]->(p:Person) RETURN p", "--fetch-schema",
	)
	require.NoError(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &rows))
	require.NotEmpty(t, rows)
	for _, row := range rows {
		assert.Equal(t, "warning", row["severity"])
	}
	assert.Contains(t, rows[0]["message"], "has no")
}

// TestQueryLint_FetchSchema_FetchedDefaultLanguageApplies verifies the
// database's default language drives the dialect when --cypher-version is
// not set: legacy octal literals get CYPHER 25's generic parse error, not
// CYPHER 5's octal-specific one.
func TestQueryLint_FetchSchema_FetchedDefaultLanguageApplies(t *testing.T) {
	s := lintFetchSeam()
	s.resp["SHOW DATABASES YIELD name, home, defaultLanguage WHERE home RETURN defaultLanguage"] = makeQueryResponse(
		[]string{"defaultLanguage"},
		[][]any{{"CYPHER 25"}},
	)
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":lint", "RETURN 0123", "--fetch-schema",
	)
	require.Error(t, err, "octal literal is an error in both dialects")

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &rows))
	require.NotEmpty(t, rows)
	assert.NotContains(t, rows[0]["message"], "octal integer literal",
		"fetched CYPHER 25 default must apply, not the CYPHER 5 fallback")
}

// TestQueryLint_FetchSchema_ExplicitVersionBeatsFetched verifies an explicit
// --cypher-version overrides the database's default language — and that the
// SHOW DATABASES probe is not even issued (its result would be overwritten).
func TestQueryLint_FetchSchema_ExplicitVersionBeatsFetched(t *testing.T) {
	s := lintFetchSeam()
	s.resp["SHOW DATABASES YIELD name, home, defaultLanguage WHERE home RETURN defaultLanguage"] = makeQueryResponse(
		[]string{"defaultLanguage"},
		[][]any{{"CYPHER 25"}},
	)
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":lint", "RETURN 0123", "--fetch-schema", "--cypher-version", "5",
	)
	require.Error(t, err)

	assert.NotContains(t, s.calls, "SHOW DATABASES YIELD name, home, defaultLanguage WHERE home RETURN defaultLanguage",
		"the default-language probe must be skipped when --cypher-version is explicit")

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &rows))
	require.NotEmpty(t, rows)
	assert.Contains(t, rows[0]["message"], "octal integer literal",
		"explicit --cypher-version 5 must beat the fetched CYPHER 25 default")
}

// TestQueryLint_FetchSchema_OptionalProbesSwallowed verifies failures of the
// visualization and default-language probes (old server, restricted role)
// leave the corresponding checks inactive without failing the command.
func TestQueryLint_FetchSchema_OptionalProbesSwallowed(t *testing.T) {
	s := lintFetchSeam()
	delete(s.resp, lintGraphSchemaQuery)
	s.err[lintGraphSchemaQuery] = errors.New("Neo.ClientError.Procedure.ProcedureNotFound: nope")
	delete(s.resp, "SHOW DATABASES YIELD name, home, defaultLanguage WHERE home RETURN defaultLanguage")
	s.err["SHOW DATABASES YIELD name, home, defaultLanguage WHERE home RETURN defaultLanguage"] =
		errors.New("Neo.ClientError.Statement.SyntaxError: no defaultLanguage column")
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":lint", "MATCH (n:NotALabel) RETURN n", "--fetch-schema",
	)
	require.NoError(t, err, "optional probe failures must not fail the command")

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &rows))
	require.Len(t, rows, 1, "label warnings still work from the summary query alone")
	assert.Contains(t, rows[0]["message"], "NotALabel")
}

// TestQueryLint_FetchSchema_SummaryFailureFails verifies the required summary
// query failing fails the command with the categorized error.
func TestQueryLint_FetchSchema_SummaryFailureFails(t *testing.T) {
	s := lintFetchSeam()
	delete(s.resp, lintSummaryQuery)
	s.err[lintSummaryQuery] = errors.New("Neo.ClientError.Security.Forbidden: no db.labels for you")
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":lint", "RETURN 1", "--fetch-schema",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Neo.ClientError.Security.Forbidden")
	assert.Empty(t, h.stdout.String(), "no diagnostics must print when the fetch fails")
}

// TestQueryLint_FetchSchema_LargeSchemaStillFetchesVisualization locks the
// deliberate departure from CLS: no >=200 labels+relTypes threshold — the
// visualization query is issued regardless of schema size (one-shot fetch,
// not a poll loop; see the note in fetchLintSchema).
func TestQueryLint_FetchSchema_LargeSchemaStillFetchesVisualization(t *testing.T) {
	labels := make([]any, 200)
	for i := range labels {
		labels[i] = fmt.Sprintf("Label%03d", i)
	}
	s := lintFetchSeam()
	s.resp[lintSummaryQuery] = makeQueryResponse(
		[]string{"result"},
		[][]any{{labels}, {[]any{"ACTED_IN"}}, {[]any{}}},
	)
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":lint", "MATCH (n:Label000) RETURN n", "--fetch-schema",
	)
	require.NoError(t, err)
	assert.Contains(t, s.calls, lintGraphSchemaQuery,
		"visualization must be fetched even for large schemas")
}

// TestQueryLint_FetchSchema_UnknownProcedureError verifies the fetched
// procedure registry makes a misspelled CALL an error-severity diagnostic
// (exit 6) — unlike label/relType problems, an unknown procedure would fail
// at run time.
func TestQueryLint_FetchSchema_UnknownProcedureError(t *testing.T) {
	s := lintFetchSeam()
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":lint", "CALL db.lables()", "--fetch-schema",
	)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T", err)
	assert.Equal(t, 6, ce.Code)
	assert.Contains(t, s.calls, lintProceduresQuery)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &rows))
	require.NotEmpty(t, rows)
	assert.Equal(t, "error", rows[0]["severity"])
	assert.Contains(t, rows[0]["message"], "db.lables")
	assert.Contains(t, rows[0]["message"], "not present in the database")
}

// TestQueryLint_FetchSchema_KnownProcedureYieldClean verifies a registry
// procedure lints clean including YIELD column resolution (the fetched rows
// reached the analyzer's signature resolver intact).
func TestQueryLint_FetchSchema_KnownProcedureYieldClean(t *testing.T) {
	s := lintFetchSeam()
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":lint", "CALL db.labels() YIELD label RETURN label", "--fetch-schema",
	)
	require.NoError(t, err)
	assert.Equal(t, "[]", strings.TrimSpace(h.stdout.String()))
}

// TestQueryLint_FetchSchema_RegistryProbeFailureSwallowed verifies failing
// SHOW PROCEDURES/FUNCTIONS probes (restricted role, old server) leave the
// procedure/function checks inactive without failing the command — no false
// "not present" errors from a half-fetched schema.
func TestQueryLint_FetchSchema_RegistryProbeFailureSwallowed(t *testing.T) {
	s := lintFetchSeam()
	delete(s.resp, lintProceduresQuery)
	delete(s.resp, lintFunctionsQuery)
	s.err[lintProceduresQuery] = errors.New("Neo.ClientError.Security.Forbidden: no SHOW PROCEDURES for you")
	s.err[lintFunctionsQuery] = errors.New("Neo.ClientError.Security.Forbidden: no SHOW FUNCTIONS for you")
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":lint", "CALL db.lables()", "--fetch-schema",
	)
	require.NoError(t, err, "registry probe failures must not fail the command")
	assert.Equal(t, "[]", strings.TrimSpace(h.stdout.String()))
}

// TestQueryLint_ParamDeclarationsEnableChecking verifies --param switches
// parameter checking on, fully offline: undeclared $parameters error (exit
// 6), declared ones lint clean. No connection is needed or attempted (no
// seam installed — a Bolt attempt against example:7687 would fail).
func TestQueryLint_ParamDeclarationsEnableChecking(t *testing.T) {
	h := newRunHarness(t, "json")
	err := h.execute(t, ":lint", "RETURN $known + $unknown", "--param", "known=1")

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T", err)
	assert.Equal(t, 6, ce.Code)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "error", rows[0]["severity"])
	assert.Contains(t, rows[0]["message"], "$unknown is not defined")
}

// TestQueryLint_NoParamFlagsSuppressesParamErrors locks the default:
// without --param, parameterized queries lint clean (params assumed
// external).
func TestQueryLint_NoParamFlagsSuppressesParamErrors(t *testing.T) {
	h := newRunHarness(t, "json")
	err := h.execute(t, ":lint", "RETURN $x")
	require.NoError(t, err)
	assert.Equal(t, "[]", strings.TrimSpace(h.stdout.String()))
}

// TestQueryLint_EmbedParamDeclaredWithoutProviderCall verifies a
// `key:embed=` --param declares the key for checking without invoking any
// embedding provider (no provider is configured in the harness, so a call
// would fail).
func TestQueryLint_EmbedParamDeclaredWithoutProviderCall(t *testing.T) {
	h := newRunHarness(t, "json")
	err := h.execute(t, ":lint", "RETURN $vec", "--param", "vec:embed=some text")
	require.NoError(t, err)
	assert.Equal(t, "[]", strings.TrimSpace(h.stdout.String()))
}

// TestQueryLint_FetchSchema_UnexpectedSummaryShapeFails verifies a summary
// result that is not exactly 3 rows fails loudly instead of silently
// linting schema-less.
func TestQueryLint_FetchSchema_UnexpectedSummaryShapeFails(t *testing.T) {
	s := lintFetchSeam()
	s.resp[lintSummaryQuery] = makeQueryResponse([]string{"result"}, [][]any{{[]any{"Movie"}}})
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":lint", "RETURN 1", "--fetch-schema",
	)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T", err)
	assert.Equal(t, 8, ce.Code)
	assert.Contains(t, ce.Message, "unexpected schema summary shape")
}

// TestQueryLint_FetchSchema_PasswordRequiredNonTTY verifies the scripted
// no-password case fails with the standard usage error instead of hanging on
// a prompt (stdin may already be consumed by the piped query).
func TestQueryLint_FetchSchema_PasswordRequiredNonTTY(t *testing.T) {
	t.Setenv(envPassword, "")
	s := lintFetchSeam()
	s.install(t)

	h := newRunHarness(t, "json")
	stdinIsTTY = func() bool { return false }

	err := h.execute(t,
		"--uri=neo4j://example:7687",
		":lint", "RETURN 1", "--fetch-schema",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "password is required")
}
