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

func (s *schemaSeam) handle(_ context.Context, _ *conn, statement string, _ map[string]any, _ bool) (*queryResponse, error) {
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
		[][]any{
			{"Neo4j Kernel", []any{"5.20.0"}, "community"},
			{"Cypher", []any{"5", "25"}, ""},
		},
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
		// Explicit --database so the rendered database.name is asserted; with no
		// --database the server resolves the home DB and name is left unset.
		"--database=neo4j",
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
	assert.Equal(t, []string{"5", "25"}, got.Database.CypherVersions)
	assert.Nil(t, got.Database.GraphEngine)

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
		// Explicit --database so the rendered database.name is asserted below.
		"--database=neo4j",
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
	// Belt-and-suspenders: TestMain seeds StdoutIsTerminal=true, but make the
	// TTY expectation explicit here since the H2 markers are now TTY-gated
	// (CLI-94). Without TTY=true this test would lose its `## <Section>`
	// assertions to the new gate.
	withStdoutIsTerminal(t, true)

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

// TestSchema_TableNonTTYSuppressesHeaders pins the CLI-94 behaviour: under
// `--format table` with a non-TTY stdout (piped, redirected), the H2 section
// markers (`## Nodes`, `## Relationships`, etc.) are suppressed. The table
// rows themselves still render unconditionally — only the decorative labels
// are TTY-gated.
func TestSchema_TableNonTTYSuppressesHeaders(t *testing.T) {
	withStdoutIsTerminal(t, false)

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

	// None of the five H2 markers should appear in non-TTY stdout.
	assert.NotContains(t, out, "## Nodes")
	assert.NotContains(t, out, "## Relationships")
	assert.NotContains(t, out, "## Relationship Paths")
	assert.NotContains(t, out, "## Indexes")
	assert.NotContains(t, out, "## Constraints")

	// Table rows still render. Spot-check the canonical column headers in
	// canonical render order — confirms each section's table reached stdout
	// even with the H2 labels suppressed. go-pretty upper-cases header text
	// by default, so anchors are matched against the upper-case form. Each
	// anchor is unique to one of the five section tables (or the first
	// occurrence in canonical order).
	assertSectionsInOrder(t, out,
		"NODETYPE",         // renderNodesTable header (only)
		"RELTYPE",          // renderRelsTable header (first; also in paths)
		"FROM",             // renderPathsTable header (only)
		"OWNINGCONSTRAINT", // renderMapsTable indexes header (only)
		"OWNEDINDEX",       // renderMapsTable constraints header (only)
	)

	// Body content from the canned data still made it to stdout.
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
		// Explicit --database so database.name is set from the connection.
		"--database=neo4j",
		":schema",
	)
	require.NoError(t, err)

	var got schemaResult
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))

	// Database section is present (Name comes from the explicit --database) but
	// versions/edition/default_language are empty since both probes failed.
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

// TestSchema_DetectsGraphEngine asserts that a non-kernel, non-Cypher row
// returned by dbms.components() populates databaseInfo.GraphEngine with the
// row's literal name and versions, while kernel and Cypher fields continue to
// populate from their own rows. Both sub-cases pin the invariant that
// fetchDatabaseInfo matches by literal `name`, not row index.
func TestSchema_DetectsGraphEngine(t *testing.T) {
	kernel := []any{"Neo4j Kernel", []any{"5.20.0"}, "community"}
	cypher := []any{"Cypher", []any{"5", "25"}, ""}
	vg := []any{"Virtual Graph", []any{"1.2.3"}, ""}

	cases := []struct {
		name string
		rows [][]any
	}{
		{"vg_row_last", [][]any{kernel, cypher, vg}},
		// vg_row_first locks name-based, not index-based, matching.
		{"vg_row_first", [][]any{vg, kernel, cypher}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := happySchemaSeam()
			s.resp["CALL dbms.components()"] = makeQueryResponse(
				[]string{"name", "versions", "edition"}, tc.rows)
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
			assert.Equal(t, []string{"5.20.0"}, got.Database.Versions)
			assert.Equal(t, "community", got.Database.Edition)
			assert.Equal(t, []string{"5", "25"}, got.Database.CypherVersions)
			require.NotNil(t, got.Database.GraphEngine)
			assert.Equal(t, "Virtual Graph", got.Database.GraphEngine.Name)
			assert.Equal(t, []string{"1.2.3"}, got.Database.GraphEngine.Versions)
		})
	}
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

// TestSchema_RenderTablesStripControl asserts the schema render functions
// scrub C0 / DEL bytes from cells that bypass formatCell (NodeType,
// PropertyName, RelType). Without the strip a malicious label could inject
// ANSI escape sequences into the table output.
func TestSchema_RenderTablesStripControl(t *testing.T) {
	nodes := []nodeProperty{{
		NodeType:      "ev\x1bil",
		NodeLabels:    []string{"L"},
		PropertyName:  "p\x7fname",
		PropertyTypes: []string{"String"},
		Mandatory:     false,
	}}
	got := renderNodesTable(nodes)
	assert.Contains(t, got, "ev?il", "nodeType ANSI escape must be replaced with ?")
	assert.Contains(t, got, "p?name", "propertyName DEL must be replaced with ?")
	assert.NotContains(t, got, "\x1b", "no raw ESC may reach the rendered table")
	assert.NotContains(t, got, "\x7f", "no raw DEL may reach the rendered table")

	rels := []relProperty{{
		RelType:       ":`A\x1bB`",
		PropertyName:  "x\x07y",
		PropertyTypes: []string{"String"},
		Mandatory:     true,
	}}
	got = renderRelsTable(rels)
	assert.Contains(t, got, "A?B", "relType ESC must be replaced with ?")
	assert.Contains(t, got, "x?y", "propertyName BEL must be replaced with ?")
	assert.NotContains(t, got, "\x1b")
	assert.NotContains(t, got, "\x07")

	paths := []relPath{{
		RelType: ":`A\x1bB`",
		From:    []string{"From"},
		To:      []string{"To"},
	}}
	got = renderPathsTable(paths)
	assert.Contains(t, got, "A?B", "relType ESC must be replaced with ? in paths table")
	assert.NotContains(t, got, "\x1b")
}

// TestSchema_FetchRelPathsEscapesBacktick guards the Cypher injection vector
// where a relType containing a literal backtick could otherwise close the
// backtick-quoted identifier early and inject statements. The escape rule is
// to double every backtick inside the identifier; the surrounding wrapper
// backticks remain a balanced pair.
func TestSchema_FetchRelPathsEscapesBacktick(t *testing.T) {
	s := newSchemaSeam()
	s.install(t)

	c := &conn{
		uri:       "neo4j://example:7687",
		username:  "u",
		password:  "pw",
		database:  "neo4j",
		userAgent: "neo4j-cli/vtest",
	}

	// Driver wrap form: leading ":`" + payload + trailing "`". stripRelTypeWrap
	// peels the outer wrap, leaving the payload "Foo`]->()-[r:DROP " (note the
	// internal backtick that must be escaped before interpolation).
	relType := ":`Foo`]->()-[r:DROP `"

	_, err := fetchRelPaths(context.Background(), c, []string{relType})
	require.NoError(t, err)
	require.Len(t, s.calls, 1)

	got := s.calls[0]

	// The doubled-backtick form survives the round-trip.
	assert.Contains(t, got, "Foo``]->()-[r:DROP ",
		"internal backtick must be doubled (Cypher escape) inside the identifier")

	// The unescaped payload (single backtick before "]") must NOT appear — that
	// shape is what an injection would look like.
	assert.NotContains(t, got, "Foo`]->()-[r:DROP ",
		"unescaped single-backtick form would close the identifier early")

	// Wrapper backticks must remain a balanced pair around the identifier:
	// total backtick count = 2 wrapper + 2*(internal-literal-backticks).
	// One internal literal backtick → 4 total.
	assert.Equal(t, 4, strings.Count(got, "`"),
		"wrapper backticks must balance and internal backticks must be doubled")

	// And the standard pattern shell stays intact end-to-end.
	const wantSuffix = "]->(m) WITH DISTINCT labels(n) AS from, labels(m) AS to RETURN from, to"
	assert.True(t, strings.HasSuffix(got, wantSuffix),
		"escaped statement must keep the canonical MATCH suffix; got: %q", got)
	assert.True(t, strings.HasPrefix(got, "MATCH (n)-[r:`"),
		"escaped statement must keep the canonical MATCH prefix; got: %q", got)
}
