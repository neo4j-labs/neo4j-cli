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

func TestRewriteExports(t *testing.T) {
	t.Run("marker missing", func(t *testing.T) {
		_, err := rewriteExports("var x = 1;")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "export marker not found")
		assert.Contains(t, err.Error(), "README.md")
	})

	t.Run("vendored artifact rewrites cleanly", func(t *testing.T) {
		out, err := rewriteExports(cypherLintJS)
		require.NoError(t, err)
		assert.NotContains(t, out, "export{")
		assert.Contains(t, out, "globalThis.lintCypherQuery=")
	})
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
