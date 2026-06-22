// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package linter

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureSchema is a small movies-style schema: ACTED_IN only goes
// (Person)-[:ACTED_IN]->(Movie).
func fixtureSchema() *DbSchema {
	return &DbSchema{
		Labels:            []string{"Movie", "Person"},
		RelationshipTypes: []string{"ACTED_IN"},
		PropertyKeys:      []string{"title", "name"},
		GraphSchema: []GraphSchemaRel{
			{From: "Person", To: "Movie", RelType: "ACTED_IN"},
		},
	}
}

func TestArtifactShape(t *testing.T) {
	// Engine init fails loudly on a wrongly-built artifact (newEngine checks
	// the global); this static check just gives a clearer message on a bad
	// refresh, before any test pays the engine-init cost.
	assert.True(t, strings.HasPrefix(cypherLintJS, "var "+artifactGlobal+"="),
		"cypherLint.js must be built with --format=iife --global-name=%s (see README.md)", artifactGlobal)
}

func TestLint_Clean(t *testing.T) {
	diags, err := Lint("MATCH (n) RETURN n", Cypher5, nil)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestLint_SemanticError(t *testing.T) {
	diags, err := Lint("MATCH (n) RETURN m", Cypher5, nil)
	require.NoError(t, err)
	require.NotEmpty(t, diags)
	d := diags[0]
	assert.Equal(t, "error", d.Severity)
	assert.Contains(t, d.Message, "`m`")
	assert.Equal(t, 0, d.Start.Line, "positions are 0-indexed at this layer")
	assert.Greater(t, d.End.Offset, d.Start.Offset)
}

func TestLint_SyntaxError(t *testing.T) {
	diags, err := Lint("MATCH (n RETURN n", Cypher5, nil)
	require.NoError(t, err)
	require.NotEmpty(t, diags, "syntax errors must be reported, not just semantic ones")
	d := diags[0]
	assert.Equal(t, "error", d.Severity)
	assert.Contains(t, strings.ToLower(d.Message), "invalid input")
	assert.GreaterOrEqual(t, d.Start.Offset, 0)
}

func TestLint_Cypher25Accepted(t *testing.T) {
	diags, err := Lint("MATCH (n) RETURN n", Cypher25, nil)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestLint_ParamErrorsSuppressedWithoutDeclarations(t *testing.T) {
	// lintCypherQuery errors on every $param absent from dbSchema.parameters;
	// with no declared parameters the glue filters those with the upstream
	// isNotParamError predicate so parameterized queries lint cleanly.
	diags, err := Lint("RETURN $x", Cypher5, nil)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestLint_DeclaredParametersEnableChecking(t *testing.T) {
	schema := &DbSchema{Parameters: map[string]any{"x": 1}}

	diags, err := Lint("RETURN $x", Cypher5, schema)
	require.NoError(t, err)
	assert.Empty(t, diags, "declared parameter must lint clean")

	diags, err = Lint("RETURN $y", Cypher5, schema)
	require.NoError(t, err)
	require.Len(t, diags, 1)
	assert.Equal(t, "error", diags[0].Severity)
	assert.Contains(t, diags[0].Message, "$y is not defined")
}

func TestLint_NilValuedParameterCountsAsDeclared(t *testing.T) {
	// :embed-modified --param entries are declared with a nil value (the
	// vector is never computed for linting); JSON null must still count as
	// declared upstream.
	schema := &DbSchema{Parameters: map[string]any{"vec": nil}}
	diags, err := Lint("RETURN $vec", Cypher5, schema)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestLint_UnknownLabelWarning(t *testing.T) {
	diags, err := Lint("MATCH (n:NotALabel) RETURN n", Cypher5, fixtureSchema())
	require.NoError(t, err)
	require.Len(t, diags, 1)
	assert.Equal(t, "warning", diags[0].Severity)
	assert.Contains(t, diags[0].Message, "NotALabel")
	assert.Contains(t, diags[0].Message, "not present in the database")
}

func TestLint_KnownLabelClean(t *testing.T) {
	diags, err := Lint("MATCH (n:Movie) RETURN n", Cypher5, fixtureSchema())
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestLint_PathDirectionalityWarning(t *testing.T) {
	// ACTED_IN only goes (Person)->(Movie); the reversed pattern warns.
	diags, err := Lint("MATCH (m:Movie)-[:ACTED_IN]->(p:Person) RETURN p", Cypher5, fixtureSchema())
	require.NoError(t, err)
	require.NotEmpty(t, diags)
	for _, d := range diags {
		assert.Equal(t, "warning", d.Severity)
	}
	assert.Contains(t, diags[0].Message, "has no")
}

func TestLint_NilSchemaNoSchemaDiagnostics(t *testing.T) {
	diags, err := Lint("MATCH (n:NotALabel) RETURN n", Cypher5, nil)
	require.NoError(t, err)
	assert.Empty(t, diags, "schema-aware warnings must not fire without a schema")
}

func TestLint_PartialSchemaDisablesLabelWarnings(t *testing.T) {
	// warnOnUndeclaredLabels requires BOTH labels and relationshipTypes; a
	// partial schema must not fire it.
	diags, err := Lint("MATCH (n:NotALabel) RETURN n", Cypher5, &DbSchema{Labels: []string{"Movie"}})
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestLint_SchemaDefaultLanguageUsed(t *testing.T) {
	// schema.DefaultLanguage takes precedence over the version argument; a
	// prologue in the query would beat both. Legacy octal literals like 0123
	// error in both dialects but with dialect-specific messages: CYPHER 5
	// names the removed octal syntax, CYPHER 25's grammar rejects the token
	// outright.
	q := "RETURN 0123"
	diags5, err := Lint(q, Cypher5, nil)
	require.NoError(t, err)
	require.NotEmpty(t, diags5)
	assert.Contains(t, diags5[0].Message, "octal integer literal")

	diags25, err := Lint(q, Cypher5, &DbSchema{DefaultLanguage: string(Cypher25)})
	require.NoError(t, err)
	require.NotEmpty(t, diags25)
	assert.NotContains(t, diags25[0].Message, "octal integer literal",
		"DefaultLanguage in the schema must change the dialect actually linted against")
	assert.Contains(t, diags25[0].Message, "Invalid input")
}

// procFuncFixture builds procedure/function registries under both dialect
// keys, the way fetchLintSchema does. Rows carry the complete SHOW shape the
// TeaVM signature resolver digests (see the DbSchema doc for why subsetting
// is unsafe).
func procFuncFixture() *DbSchema {
	procRow := func(name string, returns []any, deprecated bool, deprecatedBy string) map[string]any {
		row := map[string]any{
			"name":                name,
			"description":         "a test procedure",
			"mode":                "READ",
			"worksOnSystem":       false,
			"signature":           name + "() :: ()",
			"argumentDescription": []any{},
			"returnDescription":   returns,
			"admin":               false,
			"option":              map[string]any{"deprecated": deprecated},
		}
		if deprecatedBy != "" {
			row["deprecatedBy"] = deprecatedBy
		}
		return row
	}
	fnRow := func(name string, builtIn, deprecated bool, deprecatedBy string) map[string]any {
		row := map[string]any{
			"name":                name,
			"category":            "Scalar",
			"description":         "a test function",
			"signature":           name + "() :: FLOAT",
			"isBuiltIn":           builtIn,
			"argumentDescription": []any{},
			"returnDescription":   "FLOAT",
			"aggregating":         false,
			"isDeprecated":        deprecated,
		}
		if deprecatedBy != "" {
			row["deprecatedBy"] = deprecatedBy
		}
		return row
	}
	labelColumn := []any{map[string]any{
		"name": "label", "description": "", "type": "STRING", "isDeprecated": false,
	}}
	procs := Registry{
		"db.labels":    procRow("db.labels", labelColumn, false, ""),
		"db.oldLabels": procRow("db.oldLabels", []any{}, true, "db.labels"),
	}
	funcs := Registry{
		"pi":     fnRow("pi", true, false, ""),
		"old.fn": fnRow("old.fn", false, true, "pi"),
	}
	return &DbSchema{
		Procedures: map[string]Registry{string(Cypher5): procs, string(Cypher25): procs},
		Functions:  map[string]Registry{string(Cypher5): funcs, string(Cypher25): funcs},
	}
}

func TestLint_UnknownProcedureError(t *testing.T) {
	diags, err := Lint("CALL db.lables()", Cypher5, procFuncFixture())
	require.NoError(t, err)
	require.NotEmpty(t, diags)
	assert.Equal(t, "error", diags[0].Severity)
	assert.Contains(t, diags[0].Message, "db.lables")
	assert.Contains(t, diags[0].Message, "not present in the database")
}

func TestLint_KnownProcedureYieldClean(t *testing.T) {
	// YIELD resolving against the registry's returnDescription proves the
	// rows reached the TeaVM signature resolver intact.
	diags, err := Lint("CALL db.labels() YIELD label RETURN label", Cypher5, procFuncFixture())
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestLint_DeprecatedProcedureWarning(t *testing.T) {
	diags, err := Lint("CALL db.oldLabels()", Cypher5, procFuncFixture())
	require.NoError(t, err)
	require.NotEmpty(t, diags)
	found := false
	for _, d := range diags {
		if d.Severity == "warning" && strings.Contains(d.Message, "deprecated") {
			found = true
			assert.Contains(t, d.Message, "db.labels", "the deprecatedBy alternative must be named")
		}
	}
	assert.True(t, found, "expected a deprecation warning, got: %+v", diags)
}

func TestLint_UnknownFunctionError(t *testing.T) {
	diags, err := Lint("RETURN nosuch()", Cypher5, procFuncFixture())
	require.NoError(t, err)
	require.NotEmpty(t, diags)
	assert.Equal(t, "error", diags[0].Severity)
	assert.Contains(t, diags[0].Message, "nosuch")
	assert.Contains(t, diags[0].Message, "not present in the database")
}

func TestLint_BuiltinFunctionCaseInsensitive(t *testing.T) {
	// Built-in functions match case-insensitively upstream (PI ≡ pi).
	diags, err := Lint("RETURN PI()", Cypher5, procFuncFixture())
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestLint_DeprecatedFunctionWarning(t *testing.T) {
	diags, err := Lint("RETURN old.fn()", Cypher5, procFuncFixture())
	require.NoError(t, err)
	require.NotEmpty(t, diags)
	found := false
	for _, d := range diags {
		if d.Severity == "warning" && strings.Contains(d.Message, "deprecated") {
			found = true
		}
	}
	assert.True(t, found, "expected a deprecation warning, got: %+v", diags)
}

func TestLint_SemanticChecksSurviveRegistries(t *testing.T) {
	// The upstream semantic-analysis wrapper swallows exceptions, so a
	// registry shape the TeaVM resolver cannot digest would silently disable
	// every semantic check. This canary fails if that ever happens.
	diags, err := Lint("MATCH (n) RETURN m", Cypher5, procFuncFixture())
	require.NoError(t, err)
	require.NotEmpty(t, diags, "semantic analysis must stay active with registries present")
	assert.Contains(t, diags[0].Message, "`m`")
}

func TestLint_Cypher25PrologueUsesRegistry(t *testing.T) {
	// A CYPHER 25 prologue must resolve procedures/functions against the
	// registry's "CYPHER 25" key — which fetchLintSchema always populates
	// alongside "CYPHER 5".
	diags, err := Lint("CYPHER 25 CALL db.labels() YIELD label RETURN label", Cypher5, procFuncFixture())
	require.NoError(t, err)
	assert.Empty(t, diags)

	diags, err = Lint("CYPHER 25 CALL db.lables()", Cypher5, procFuncFixture())
	require.NoError(t, err)
	require.NotEmpty(t, diags)
	assert.Equal(t, "error", diags[0].Severity)
	assert.Contains(t, diags[0].Message, "not present in the database")
}

func TestLint_DialectSpecificSignatures(t *testing.T) {
	// Mirrors upstream's functionsValidation test: the same function carries
	// a different signature per dialect (CYPHER 5 requires a MAP argument,
	// CYPHER 25 takes none). The prologue must select the matching registry
	// entry — proving per-dialect rows reach the TeaVM signature resolver,
	// not just the existence check.
	fn := func(signature string, args []any) map[string]any {
		return map[string]any{
			"name":                "test.uuid",
			"category":            "Scalar",
			"description":         "Returns a UUID.",
			"signature":           signature,
			"isBuiltIn":           false,
			"argumentDescription": args,
			"returnDescription":   "STRING",
			"aggregating":         false,
			"isDeprecated":        false,
		}
	}
	configArg := []any{map[string]any{
		"name": "config", "description": "", "type": "MAP", "isDeprecated": false,
	}}
	schema := &DbSchema{
		Functions: map[string]Registry{
			string(Cypher5):  {"test.uuid": fn("test.uuid(config :: MAP) :: STRING", configArg)},
			string(Cypher25): {"test.uuid": fn("test.uuid() :: STRING", []any{})},
		},
	}

	diags, err := Lint("CYPHER 5 RETURN test.uuid()", Cypher5, schema)
	require.NoError(t, err)
	require.NotEmpty(t, diags, "CYPHER 5 signature requires an argument")
	assert.Equal(t, "error", diags[0].Severity)
	assert.Contains(t, diags[0].Message, "required number of arguments")

	diags, err = Lint("CYPHER 25 RETURN test.uuid()", Cypher5, schema)
	require.NoError(t, err)
	assert.Empty(t, diags, "CYPHER 25 signature takes no arguments")
}

func TestLint_MissingDialectKeyFlagsKnownProcedure(t *testing.T) {
	// Locks the upstream trap documented on DbSchema: a populated registry
	// MISSING the resolved dialect's key makes even existing procedures
	// "unknown". This is why fetchLintSchema populates both keys or neither.
	schema := procFuncFixture()
	delete(schema.Procedures, string(Cypher25))
	diags, err := Lint("CYPHER 25 CALL db.labels()", Cypher5, schema)
	require.NoError(t, err)
	require.NotEmpty(t, diags, "missing dialect key must surface as unknown procedure")
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "not present in the database") {
			found = true
		}
	}
	assert.True(t, found, "expected an unknown-procedure diagnostic, got: %+v", diags)
}

func TestLint_NoRegistriesSkipsProcedureChecks(t *testing.T) {
	// Without procedures/functions keys the existence checks stay off: a
	// schema-less lint must not flag unknown CALLs.
	diags, err := Lint("CALL not.a.proc()", Cypher5, fixtureSchema())
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestLint_DiagnosticsSortedByOffset(t *testing.T) {
	diags, err := Lint("MATCH (a) RETURN b, c", Cypher5, nil)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(diags), 2)
	for i := 1; i < len(diags); i++ {
		assert.LessOrEqual(t, diags[i-1].Start.Offset, diags[i].Start.Offset)
	}
}

func TestLint_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			diags, err := Lint("MATCH (n) RETURN m", Cypher5, nil)
			assert.NoError(t, err)
			assert.NotEmpty(t, diags)
		}()
	}
	wg.Wait()
}
