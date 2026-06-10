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

func TestRewriteExports(t *testing.T) {
	t.Run("marker missing", func(t *testing.T) {
		_, err := rewriteExports("var x = 1;")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "export marker not found")
		assert.Contains(t, err.Error(), "README.md")
	})

	t.Run("vendored artifact rewrites cleanly", func(t *testing.T) {
		out, err := rewriteExports(semanticAnalysisJS)
		require.NoError(t, err)
		assert.NotContains(t, out, "export{")
		assert.Contains(t, out, "globalThis.analyzeQuery=")
	})
}

func TestLint_Clean(t *testing.T) {
	diags, err := Lint("MATCH (n) RETURN n", Cypher5)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestLint_SemanticError(t *testing.T) {
	diags, err := Lint("MATCH (n) RETURN m", Cypher5)
	require.NoError(t, err)
	require.NotEmpty(t, diags)
	d := diags[0]
	assert.Equal(t, "error", d.Severity)
	assert.Contains(t, d.Message, "`m`")
	assert.Equal(t, 0, d.Start.Line, "positions are 0-indexed at this layer")
	assert.Greater(t, d.End.Offset, d.Start.Offset)
}

func TestLint_SyntaxError(t *testing.T) {
	diags, err := Lint("MATCH (n RETURN n", Cypher5)
	require.NoError(t, err)
	require.NotEmpty(t, diags, "syntax errors must be reported, not just semantic ones")
	d := diags[0]
	assert.Equal(t, "error", d.Severity)
	assert.Contains(t, strings.ToLower(d.Message), "invalid input")
	assert.GreaterOrEqual(t, d.Start.Offset, 0)
}

func TestLint_Cypher25Accepted(t *testing.T) {
	diags, err := Lint("MATCH (n) RETURN n", Cypher25)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestLint_DiagnosticsSortedByOffset(t *testing.T) {
	diags, err := Lint("MATCH (a) RETURN b, c", Cypher5)
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
			diags, err := Lint("MATCH (n) RETURN m", Cypher5)
			assert.NoError(t, err)
			assert.NotEmpty(t, diags)
		}()
	}
	wg.Wait()
}
