// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
)

// recordingStore records every statement and answers CREATE … RETURN runs
// with synthetic local element ids, one per row, extracted from the
// statement's inlined src literals so the identity wiring is exercised.
type recordingStore struct {
	statements []string
	next       int
}

var srcPattern = regexp.MustCompile(`src: '([^']*)'`)

func (s *recordingStore) Run(statements []string, params map[string]any, atomic bool) ([]StatementResult, error) {
	results := make([]StatementResult, 0, len(statements))
	for _, stmt := range statements {
		s.statements = append(s.statements, stmt)
		res := StatementResult{Rows: [][]any{}}
		if strings.Contains(stmt, "RETURN elementId") {
			matches := srcPattern.FindAllStringSubmatch(stmt, -1)
			for _, m := range matches {
				s.next++
				res.Columns = []string{"eid", "src"}
				res.Rows = append(res.Rows, []any{"local-" + itoa(s.next), m[1]})
			}
		}
		results = append(results, res)
	}
	return results, nil
}

func (s *recordingStore) Checkpoint() error { return nil }
func (s *recordingStore) Close()            {}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// pullFixture builds the two-node one-rel subgraph the round-trip tests use.
func pullFixture() *Subgraph {
	g := NewSubgraph()
	g.Nodes["4:a:0"] = &SubgraphNode{ElementID: "4:a:0", Labels: []string{"Person"}, Props: map[string]any{"name": "Ada"}}
	g.Nodes["4:a:1"] = &SubgraphNode{ElementID: "4:a:1", Labels: []string{"Person"}, Props: map[string]any{"name": "Bob", "born": dbtype.Date(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))}}
	g.Rels["5:a:9"] = &SubgraphRel{ElementID: "5:a:9", StartElementID: "4:a:0", EndElementID: "4:a:1", Type: "KNOWS", Props: map[string]any{"since": int64(2020)}}
	return g
}

func TestMaterialize_CreatesAndRecordsIdentity(t *testing.T) {
	store := &recordingStore{}
	m := &IdentityMap{Sources: map[string]SourceIdentities{}}

	stats, err := Materialize(store, pullFixture(), m, "neo4j://remote:7687/neo4j", 500)
	require.NoError(t, err)

	assert.Equal(t, 2, stats.NodesCreated)
	assert.Equal(t, 1, stats.RelsCreated)
	assert.Equal(t, 0, stats.NodesReused+stats.RelsReused+stats.RelsSkipped)

	ids := m.Sources["neo4j://remote:7687/neo4j"]
	assert.Len(t, ids.Nodes, 2)
	assert.Len(t, ids.Rels, 1)
	assert.Contains(t, ids.Nodes["4:a:0"], "local-")

	// Node creation statement: backticked labels, literal props (temporals
	// as constructor literals), identity capture.
	createNode := store.statements[0]
	assert.Contains(t, createNode, "CREATE (n:`Person`)")
	assert.Contains(t, createNode, "SET n = row.props")
	assert.Contains(t, createNode, "'name': 'Ada'")
	assert.Contains(t, createNode, "date('1980-01-01')")
	assert.Contains(t, createNode, "RETURN elementId(n) AS eid, row.src AS src")

	// Relationship creation: backticked type, elementId-matched endpoints.
	relStmt := store.statements[len(store.statements)-1]
	assert.Contains(t, relStmt, "CREATE (a)-[r:`KNOWS`]->(b)")
	assert.Contains(t, relStmt, "MATCH (a), (b) WHERE elementId(a) = row.start AND elementId(b) = row.end")
}

func TestMaterialize_SecondPullReuses(t *testing.T) {
	store := &recordingStore{}
	m := &IdentityMap{Sources: map[string]SourceIdentities{}}

	_, err := Materialize(store, pullFixture(), m, "src", 500)
	require.NoError(t, err)

	// The subgraph comes back with changed props — same source identities.
	g := pullFixture()
	g.Nodes["4:a:0"].Props = map[string]any{"name": "Ada", "age": int64(40)}

	stats, err := Materialize(store, g, m, "src", 500)
	require.NoError(t, err)

	assert.Equal(t, 2, stats.NodesReused)
	assert.Equal(t, 1, stats.RelsReused)
	assert.Equal(t, 0, stats.NodesCreated+stats.RelsCreated)

	// Updates match by local element id and replace props wholesale.
	updateStmt := store.statements[len(store.statements)-3]
	assert.Contains(t, updateStmt, "MATCH (n) WHERE elementId(n) = row.local SET n = row.props")
	assert.Contains(t, updateStmt, "'age': 40")

	relUpdate := store.statements[len(store.statements)-1]
	assert.Contains(t, relUpdate, "MATCH ()-[r]->() WHERE elementId(r) = row.local SET r = row.props")
}

func TestMaterialize_SkipsDanglingRelationship(t *testing.T) {
	store := &recordingStore{}
	m := &IdentityMap{Sources: map[string]SourceIdentities{}}
	g := pullFixture()
	// The rel's end node never appears in the pulled set.
	delete(g.Nodes, "4:a:1")

	stats, err := Materialize(store, g, m, "src", 500)
	require.NoError(t, err)

	assert.Equal(t, 1, stats.RelsSkipped)
	assert.Equal(t, 0, stats.RelsCreated)
}

func TestMaterialize_BatchesByLabelSignatureAndSize(t *testing.T) {
	store := &recordingStore{}
	m := &IdentityMap{Sources: map[string]SourceIdentities{}}
	g := NewSubgraph()
	for i := 0; i < 5; i++ {
		g.Nodes["n"+itoa(i)] = &SubgraphNode{ElementID: "n" + itoa(i), Labels: []string{"Person"}, Props: map[string]any{"i": int64(i)}}
	}
	for i := 5; i < 7; i++ {
		g.Nodes["n"+itoa(i)] = &SubgraphNode{ElementID: "n" + itoa(i), Labels: []string{"Movie"}, Props: map[string]any{"i": int64(i)}}
	}

	stats, err := Materialize(store, g, m, "src", 2)
	require.NoError(t, err)
	assert.Equal(t, 7, stats.NodesCreated)

	// 3 Person batches (2+2+1) + 1 Movie batch = 4 create statements.
	creates := 0
	for _, stmt := range store.statements {
		if strings.Contains(stmt, "CREATE (n") {
			creates++
		}
	}
	assert.Equal(t, 4, creates)

	// Each statement carries at most batchSize rows.
	for _, stmt := range store.statements {
		assert.LessOrEqual(t, len(srcPattern.FindAllString(stmt, -1)), 2)
	}
}

func TestExtractEntities(t *testing.T) {
	g := NewSubgraph()
	ada := dbtype.Node{ElementId: "4:a:0", Labels: []string{"Person"}, Props: map[string]any{"name": "Ada"}}
	bob := dbtype.Node{ElementId: "4:a:1", Labels: []string{"Person"}, Props: map[string]any{"name": "Bob"}}
	rel := dbtype.Relationship{ElementId: "5:a:9", StartElementId: "4:a:0", EndElementId: "4:a:1", Type: "KNOWS", Props: map[string]any{}}

	t.Run("dedupes repeated nodes across rows", func(t *testing.T) {
		assert.Equal(t, 3, ExtractEntities(g, []any{ada, rel, bob}), "first row: 2 nodes + 1 rel, all new")
		assert.Equal(t, 0, ExtractEntities(g, []any{ada, bob}))
		nodes, rels := g.EntityCounts()
		assert.Equal(t, 2, nodes)
		assert.Equal(t, 1, rels)
	})

	t.Run("path cells contribute all elements", func(t *testing.T) {
		g2 := NewSubgraph()
		path := dbtype.Path{Nodes: []dbtype.Node{ada, bob}, Relationships: []dbtype.Relationship{rel}}
		assert.Equal(t, 3, ExtractEntities(g2, []any{path}))
		nodes, rels := g2.EntityCounts()
		assert.Equal(t, 2, nodes)
		assert.Equal(t, 1, rels)
	})

	t.Run("scalar row yields nothing", func(t *testing.T) {
		g3 := NewSubgraph()
		assert.Equal(t, 0, ExtractEntities(g3, []any{"Ada", int64(1), nil, []any{1, 2}}))
	})
}

func TestIdentityMapRoundTrip(t *testing.T) {
	fs := afero.NewMemMapFs()
	storePath := "/store"

	m, err := LoadIdentityMap(fs, storePath)
	require.NoError(t, err)
	assert.Empty(t, m.Sources, "a missing sidecar is an empty map")

	m.Sources["src"] = SourceIdentities{
		Nodes: map[string]string{"4:a:0": "local-1"},
		Rels:  map[string]string{"5:a:9": "local-3"},
	}
	require.NoError(t, m.Save(fs, storePath))

	m2, err := LoadIdentityMap(fs, storePath)
	require.NoError(t, err)
	assert.Equal(t, "local-1", m2.Sources["src"].Nodes["4:a:0"])
	assert.Equal(t, "local-3", m2.Sources["src"].Rels["5:a:9"])
}

func TestLoadIdentityMap_CorruptFileErrors(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, filepath.Join("/store", identityMapName), []byte("not json"), 0o600))

	_, err := LoadIdentityMap(fs, "/store")
	require.Error(t, err)
	assert.Contains(t, err.Error(), identityMapName)
}
