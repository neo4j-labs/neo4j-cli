// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// schemaSeam wires a per-statement response/error map for :schema tests via
// the runStatementResponseFn seam. Statements are matched by exact string
// (the schema queries are constants in schema.go).
type schemaSeam struct {
	calls []string
	resp  map[string]*queryResponse
	err   map[string]error
}

func newSchemaSeam() *schemaSeam {
	return &schemaSeam{
		resp: map[string]*queryResponse{},
		err:  map[string]error{},
	}
}

func (s *schemaSeam) handle(_ context.Context, _ *conn, statement string, _ map[string]any) (*queryResponse, error) {
	s.calls = append(s.calls, statement)
	if e, ok := s.err[statement]; ok {
		return nil, e
	}
	if r, ok := s.resp[statement]; ok {
		return r, nil
	}
	// Default empty success — keeps tests resilient to optional schema probes
	// (e.g. dbms.components, SHOW SETTINGS) that swallow errors.
	resp := makeQueryResponse([]string{}, [][]any{})
	return resp, nil
}

func (s *schemaSeam) install(t *testing.T) {
	t.Helper()
	withRunStatementSeam(t, s.handle)
}

// happySchemaSeam returns a schemaSeam pre-configured with canned responses
// for every introspection query the runSchema pipeline issues against a
// minimal Movies-like graph.
func happySchemaSeam() *schemaSeam {
	s := newSchemaSeam()

	s.resp["CALL db.schema.nodeTypeProperties() YIELD nodeType, nodeLabels, propertyName, propertyTypes, mandatory"] = makeQueryResponse(
		[]string{"nodeType", "nodeLabels", "propertyName", "propertyTypes", "mandatory"},
		[][]any{
			{":Person", []any{"Person"}, "name", []any{"String"}, true},
			{":Movie", []any{"Movie"}, "title", []any{"String"}, false},
		},
	)
	s.resp["CALL db.schema.relTypeProperties() YIELD relType, propertyName, propertyTypes, mandatory"] = makeQueryResponse(
		[]string{"relType", "propertyName", "propertyTypes", "mandatory"},
		[][]any{
			{":`ACTED_IN`", "role", []any{"String"}, false},
			{":`DIRECTED`", nil, nil, false},
		},
	)
	s.resp["MATCH (n)-[r:`ACTED_IN`]->(m) WITH DISTINCT labels(n) AS from, labels(m) AS to RETURN from, to"] = makeQueryResponse(
		[]string{"from", "to"},
		[][]any{{[]any{"Person"}, []any{"Movie"}}},
	)
	s.resp["MATCH (n)-[r:`DIRECTED`]->(m) WITH DISTINCT labels(n) AS from, labels(m) AS to RETURN from, to"] = makeQueryResponse(
		[]string{"from", "to"},
		[][]any{{[]any{"Person"}, []any{"Movie"}}},
	)
	s.resp["SHOW INDEXES YIELD name, type, entityType, labelsOrTypes, properties, state, owningConstraint, options"] = makeQueryResponse(
		[]string{"name", "type", "entityType", "labelsOrTypes", "properties", "state", "owningConstraint", "options"},
		[][]any{{"idx_person_name", "RANGE", "NODE", []any{"Person"}, []any{"name"}, "ONLINE", nil, map[string]any{}}},
	)
	s.resp["SHOW CONSTRAINTS YIELD name, type, entityType, labelsOrTypes, properties, ownedIndex, propertyType"] = makeQueryResponse(
		[]string{"name", "type", "entityType", "labelsOrTypes", "properties", "ownedIndex", "propertyType"},
		[][]any{{"uq_person_name", "UNIQUENESS", "NODE", []any{"Person"}, []any{"name"}, "idx_person_name", "STRING"}},
	)
	s.resp["CALL dbms.components()"] = makeQueryResponse(
		[]string{"name", "versions", "edition"},
		[][]any{{"Neo4j Kernel", []any{"5.20.0"}, "community"}},
	)
	s.resp["SHOW SETTINGS YIELD name, value WHERE name = 'db.query.default_language' RETURN value"] = makeQueryResponse(
		[]string{"value"},
		[][]any{{"CYPHER 25"}},
	)

	return s
}

func TestSchema_HappyPath_JSON(t *testing.T) {
	s := happySchemaSeam()
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":schema",
	)
	require.NoError(t, err)

	var got schemaResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))

	require.NotNil(t, got.Database)
	assert.Equal(t, "neo4j", got.Database.Name)
	assert.Equal(t, []string{"5.20.0"}, got.Database.Versions)
	assert.Equal(t, "community", got.Database.Edition)
	assert.Equal(t, "CYPHER 25", got.Database.DefaultLanguage)

	require.Len(t, got.Nodes, 2)
	assert.Equal(t, ":Person", got.Nodes[0].NodeType)
	assert.Equal(t, []string{"Person"}, got.Nodes[0].NodeLabels)
	assert.Equal(t, "name", got.Nodes[0].PropertyName)
	assert.True(t, got.Nodes[0].Mandatory)

	require.Len(t, got.Relationships, 2)
	assert.Equal(t, ":`ACTED_IN`", got.Relationships[0].RelType)

	// Two relTypes → two paths (ACTED_IN, DIRECTED), sorted alphabetically.
	require.Len(t, got.RelationshipPaths, 2)
	assert.Equal(t, "ACTED_IN", got.RelationshipPaths[0].RelType)
	assert.Equal(t, []string{"Person"}, got.RelationshipPaths[0].From)
	assert.Equal(t, []string{"Movie"}, got.RelationshipPaths[0].To)
	assert.Equal(t, "DIRECTED", got.RelationshipPaths[1].RelType)

	require.Len(t, got.Indexes, 1)
	assert.Equal(t, "idx_person_name", got.Indexes[0]["name"])

	require.Len(t, got.Constraints, 1)
	assert.Equal(t, "uq_person_name", got.Constraints[0]["name"])
}

// TestSchema_DefaultOutputNonTTYIsJSON locks the contract that with the
// implicit `default` output and a non-TTY stdout (piped or redirected),
// `:schema` emits JSON. The package-level TestMain seeds the seam to true;
// this test flips it locally to simulate the piped case.
func TestSchema_DefaultOutputNonTTYIsJSON(t *testing.T) {
	withStdoutIsTerminal(t, false)
	s := happySchemaSeam()
	s.install(t)

	h := newRunHarness(t, "default")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":schema",
	)
	require.NoError(t, err)

	// Output should parse as the structured schemaResult JSON envelope.
	var got schemaResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	require.NotNil(t, got.Database)
	assert.Equal(t, "neo4j", got.Database.Name)
	// And NOT contain the H2 table markers.
	assert.NotContains(t, h.stdout.String(), "## Nodes")
}

// TestSchema_DefaultOutputTTYIsTables locks the new TTY-aware default for
// `:schema`: with `default` output and a TTY stdout, the renderer emits the
// five canonical stacked tables (no JSON envelope).
func TestSchema_DefaultOutputTTYIsTables(t *testing.T) {
	s := happySchemaSeam()
	s.install(t)

	h := newRunHarness(t, "default")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":schema",
	)
	require.NoError(t, err)

	out := h.stdout.String()
	assertSectionsInOrder(t, out,
		"## Nodes",
		"## Relationships",
		"## Relationship Paths",
		"## Indexes",
		"## Constraints",
	)
	// Should not be a JSON envelope.
	assert.NotContains(t, out, `"nodes"`)
	assert.NotContains(t, out, `"relationships"`)
}

func TestSchema_HappyPath_Table(t *testing.T) {
	s := happySchemaSeam()
	s.install(t)

	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":schema",
	)
	require.NoError(t, err)

	out := h.stdout.String()
	// All five sub-tables should render with their H2 markers in canonical
	// order. assertSectionsInOrder confirms the documented section ordering.
	// Database info is JSON-only; table mode skips it.
	assertSectionsInOrder(t, out,
		"## Nodes",
		"## Relationships",
		"## Relationship Paths",
		"## Indexes",
		"## Constraints",
	)
	assert.NotContains(t, out, "## Database")

	// Spot-check that body content from the canned data made it to stdout.
	assert.Contains(t, out, "Person")
	assert.Contains(t, out, "ACTED_IN")
	assert.Contains(t, out, "idx_person_name")
	assert.Contains(t, out, "uq_person_name")
}

func TestSchema_RequiredQueryFailureFailsCommand(t *testing.T) {
	// Inject a SHOW INDEXES failure — must propagate as a command error.
	s := happySchemaSeam()
	s.err["SHOW INDEXES YIELD name, type, entityType, labelsOrTypes, properties, state, owningConstraint, options"] =
		errors.New("Neo.ClientError.Statement.SyntaxError: bad indexes")
	delete(s.resp, "SHOW INDEXES YIELD name, type, entityType, labelsOrTypes, properties, state, owningConstraint, options")
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":schema",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Neo.ClientError.Statement.SyntaxError")
	assert.Contains(t, err.Error(), "bad indexes")
}

func TestSchema_OptionalQueryFailureSwallowed(t *testing.T) {
	// Both optional probes (dbms.components + SHOW SETTINGS) fail; the
	// command must still succeed with the rest of the result populated.
	s := happySchemaSeam()
	s.err["CALL dbms.components()"] =
		errors.New("Neo.ClientError.Procedure.ProcedureNotFound: no such procedure")
	delete(s.resp, "CALL dbms.components()")
	s.err["SHOW SETTINGS YIELD name, value WHERE name = 'db.query.default_language' RETURN value"] =
		errors.New("Neo.ClientError.Procedure.ProcedureNotFound: no settings")
	delete(s.resp, "SHOW SETTINGS YIELD name, value WHERE name = 'db.query.default_language' RETURN value")
	s.install(t)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":schema",
	)
	require.NoError(t, err)

	var got schemaResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))

	// Database section is present (we always set Name from the connection)
	// but versions/edition/default_language are empty since both probes failed.
	require.NotNil(t, got.Database)
	assert.Equal(t, "neo4j", got.Database.Name)
	assert.Empty(t, got.Database.Versions)
	assert.Empty(t, got.Database.Edition)
	assert.Empty(t, got.Database.DefaultLanguage)

	// Required sections still populated.
	assert.NotEmpty(t, got.Nodes)
	assert.NotEmpty(t, got.Relationships)
	assert.NotEmpty(t, got.Indexes)
	assert.NotEmpty(t, got.Constraints)
}

// TestSchema_StripRelTypeWrap covers the unwrap helper across the shapes the
// driver / API have been observed to emit.
func TestSchema_StripRelTypeWrap(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{":\"OWNS\"", "OWNS"},
		{":`OWNS`", "OWNS"},
		{":OWNS", "OWNS"},
		{"OWNS", "OWNS"},
		{"", ""},
		{":", ""},
		{"::`X`", "X"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, stripRelTypeWrap(tc.in))
		})
	}
}

// TestSchema_UniqueRelTypes locks the dedup + sort contract so the per-rel
// MATCH calls fire in deterministic order (and tests can rely on it).
func TestSchema_UniqueRelTypes(t *testing.T) {
	rels := []relProperty{
		{RelType: ":`OWNS`"},
		{RelType: ":`OWNS`"}, // duplicate
		{RelType: ":`ACTED_IN`"},
	}
	got := uniqueRelTypes(rels)
	assert.Equal(t, []string{"ACTED_IN", "OWNS"}, got)
}

// TestSchema_NoArgsAccepted asserts cobra rejects positional args on :schema.
func TestSchema_NoArgsAccepted(t *testing.T) {
	s := happySchemaSeam()
	s.install(t)
	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri=neo4j://example:7687",
		"--password=pw",
		":schema",
		"unexpected-arg",
	)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "unknown command")
}

// assertSectionsInOrder fails the test unless every needle appears in body
// in the supplied order, each at strictly higher index than the previous.
func assertSectionsInOrder(t *testing.T, body string, needles ...string) {
	t.Helper()
	last := -1
	for i, n := range needles {
		idx := strings.Index(body, n)
		require.GreaterOrEqualf(t, idx, 0, "section %q (index %d) missing from output", n, i)
		require.Greaterf(t, idx, last, "section %q must appear after the previous section", n)
		last = idx
	}
}

// TestSchema_FetchHelpersCompose drives the runSchema pipeline directly via
// the seam (no cobra) — ensures the fetch helpers compose cleanly when tests
// bypass the cobra layer entirely.
func TestSchema_FetchHelpersCompose(t *testing.T) {
	s := happySchemaSeam()
	s.install(t)

	c := &conn{
		uri:       "neo4j://example:7687",
		username:  "u",
		password:  "pw",
		database:  "neo4j",
		userAgent: "neo4j-cli/vtest",
	}
	ctx := context.Background()

	nodes, err := fetchNodeProperties(ctx, c)
	require.NoError(t, err)
	assert.Len(t, nodes, 2)

	rels, err := fetchRelProperties(ctx, c)
	require.NoError(t, err)
	assert.Len(t, rels, 2)

	paths, err := fetchRelPaths(ctx, c, uniqueRelTypes(rels))
	require.NoError(t, err)
	assert.Len(t, paths, 2)

	idx, err := fetchTabular(ctx, c,
		"SHOW INDEXES YIELD name, type, entityType, labelsOrTypes, properties, state, owningConstraint, options")
	require.NoError(t, err)
	assert.Len(t, idx, 1)
}
