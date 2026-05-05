// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// schemaServerCanned wires a routing httptest server that returns canned
// responses keyed by a substring match against the inbound statement. A nil
// or missing match returns an empty result. A response value of "" signals
// the server should reply with a 4xx + errors[] envelope (used to simulate
// a required-query failure).
type cannedResponse struct {
	body   string // JSON body for 2xx; ignored when errBody is set
	status int    // 0 → 200
	// errBody, when non-empty, is the JSON envelope returned as a 4xx so the
	// runStatement error path fires (status defaults to 400 in that case).
	errBody string
}

// schemaServer routes by the Cypher statement substring → cannedResponse.
// Order of map entries does not matter; the first matching key wins (a tie
// is impossible because real introspection statements have unique prefixes).
func schemaServer(t *testing.T, routes map[string]cannedResponse) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		stmt, _ := req["statement"].(string)

		w.Header().Set("Content-Type", "application/json")
		for needle, resp := range routes {
			if !strings.Contains(stmt, needle) {
				continue
			}
			if resp.errBody != "" {
				status := resp.status
				if status == 0 {
					status = http.StatusBadRequest
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(resp.errBody))
				return
			}
			if resp.status != 0 {
				w.WriteHeader(resp.status)
			}
			_, _ = w.Write([]byte(resp.body))
			return
		}
		// Default empty success.
		_, _ = w.Write([]byte(`{"data":{"fields":[],"values":[]}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// canonical canned bodies for the happy-path test
const (
	nodePropsBody = `{"data":{"fields":["nodeType","nodeLabels","propertyName","propertyTypes","mandatory"],"values":[
		[":Person", ["Person"], "name", ["String"], true],
		[":Movie",  ["Movie"],  "title", ["String"], false]
	]}}`
	relPropsBody = "{\"data\":{\"fields\":[\"relType\",\"propertyName\",\"propertyTypes\",\"mandatory\"]," +
		"\"values\":[" +
		"[\":`ACTED_IN`\", \"role\", [\"String\"], false]," +
		"[\":`DIRECTED`\", null, null, false]" +
		"]}}"
	pathActedInBody  = `{"data":{"fields":["from","to"],"values":[[["Person"],["Movie"]]]}}`
	pathDirectedBody = `{"data":{"fields":["from","to"],"values":[[["Person"],["Movie"]]]}}`
	indexesBody      = `{"data":{"fields":["name","type","entityType","labelsOrTypes","properties","state","owningConstraint","options"],"values":[
		["idx_person_name","RANGE","NODE",["Person"],["name"],"ONLINE",null,{}]
	]}}`
	constraintsBody = `{"data":{"fields":["name","type","entityType","labelsOrTypes","properties","ownedIndex","propertyType"],"values":[
		["uq_person_name","UNIQUENESS","NODE",["Person"],["name"],"idx_person_name","STRING"]
	]}}`
	dbmsComponentsBody = `{"data":{"fields":["name","versions","edition"],"values":[["Neo4j Kernel",["5.20.0"],"community"]]}}`
	settingsBody       = `{"data":{"fields":["value"],"values":[["CYPHER 25"]]}}`
)

// happyRoutes is the canned route table used by tests that don't need a
// failure injection. Stripped relType lookup uses the substring after
// stripRelTypeWrap, so the route table keys here use the bare relType.
func happyRoutes() map[string]cannedResponse {
	return map[string]cannedResponse{
		"db.schema.nodeTypeProperties":          {body: nodePropsBody},
		"db.schema.relTypeProperties":           {body: relPropsBody},
		"MATCH (n)-[r:`ACTED_IN`]->(m)":         {body: pathActedInBody},
		"MATCH (n)-[r:`DIRECTED`]->(m)":         {body: pathDirectedBody},
		"SHOW INDEXES":                          {body: indexesBody},
		"SHOW CONSTRAINTS":                      {body: constraintsBody},
		"CALL dbms.components":                  {body: dbmsComponentsBody},
		"SHOW SETTINGS YIELD name, value WHERE": {body: settingsBody},
	}
}

func TestSchema_HappyPath_JSON(t *testing.T) {
	srv := schemaServer(t, happyRoutes())

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
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
	srv := schemaServer(t, happyRoutes())

	h := newRunHarness(t, "default")
	err := h.execute(t,
		"--uri="+srv.URL,
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
// five canonical stacked tables (no JSON envelope). TestMain seeds the seam
// to true so this test does not toggle it explicitly.
func TestSchema_DefaultOutputTTYIsTables(t *testing.T) {
	srv := schemaServer(t, happyRoutes())

	h := newRunHarness(t, "default")
	err := h.execute(t,
		"--uri="+srv.URL,
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
	srv := schemaServer(t, happyRoutes())

	h := newRunHarness(t, "table")
	err := h.execute(t,
		"--uri="+srv.URL,
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
	routes := happyRoutes()
	routes["SHOW INDEXES"] = cannedResponse{
		errBody: `{"errors":[{"code":"Neo.ClientError.Statement.SyntaxError","message":"bad indexes"}]}`,
	}
	srv := schemaServer(t, routes)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
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
	routes := happyRoutes()
	routes["CALL dbms.components"] = cannedResponse{
		errBody: `{"errors":[{"code":"Neo.ClientError.Procedure.ProcedureNotFound","message":"no such procedure"}]}`,
	}
	routes["SHOW SETTINGS YIELD name, value WHERE"] = cannedResponse{
		errBody: `{"errors":[{"code":"Neo.ClientError.Procedure.ProcedureNotFound","message":"no settings"}]}`,
	}
	srv := schemaServer(t, routes)

	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
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
	srv := schemaServer(t, happyRoutes())
	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--uri="+srv.URL,
		"--password=pw",
		":schema",
		"unexpected-arg",
	)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "unknown command")
}

// TestSchema_ConnectionError surfaces transport-level failures (e.g. unreachable
// host) before any introspection query runs.
func TestSchema_ConnectionError(t *testing.T) {
	h := newRunHarness(t, "json")
	// 127.0.0.1:1 is reliably refused on every supported platform.
	err := h.execute(t,
		"--uri=http://127.0.0.1:1",
		"--password=pw",
		":schema",
	)
	require.Error(t, err)
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

// Sanity test for the runSchema pipeline directly (no cobra) — ensures the
// fetch helpers compose cleanly when tests bypass the cobra layer entirely.
func TestSchema_FetchHelpersCompose(t *testing.T) {
	srv := schemaServer(t, happyRoutes())

	c := &conn{
		uri:      srv.URL,
		username: "u",
		password: "pw",
		database: "neo4j",
		doer:     newHTTPClient(false),
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
