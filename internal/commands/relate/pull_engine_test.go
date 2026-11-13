// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build cgo && relate

// Real-engine pull round-trip: the remote stream is faked (dbtype records,
// exactly what the Bolt driver would deliver), the local store is the
// actual engine — verifying the materialized Cypher parses and executes,
// identities round-trip through the sidecar, and a re-pull updates in place
// instead of duplicating.

package relate

import (
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
	"github.com/spf13/afero"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/relate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelatePullEndToEnd_RealEngine(t *testing.T) {
	origPrefix := clicfg.ConfigPrefix
	clicfg.ConfigPrefix = t.TempDir()
	t.Cleanup(func() { clicfg.ConfigPrefix = origPrefix })

	h := &harness{
		cfg:    clicfg.NewConfig(afero.NewOsFs(), "test"),
		stdout: newBuffer(),
		stderr: newBuffer(),
	}

	pullOnce := func(props map[string]any) {
		t.Helper()
		installFakeStream(t, [][]any{{
			dbtype.Node{ElementId: "4:a:0", Labels: []string{"Person"}, Props: map[string]any{"name": "Ada", "age": props["age"]}},
			dbtype.Relationship{ElementId: "5:a:9", StartElementId: "4:a:0", EndElementId: "4:a:1", Type: "KNOWS", Props: map[string]any{"since": int64(2020)}},
			dbtype.Node{ElementId: "4:a:1", Labels: []string{"Person"}, Props: map[string]any{"name": "Bob"}},
		}}, nil)
		require.NoError(t, h.execute(t, "pull", "movies",
			"--uri", "neo4j+s://remote:7687", "--username", "neo4j", "--password", "pw",
			"--query", "MATCH (a)-[r:KNOWS]->(b) RETURN a, r, b",
		))
	}

	// First pull creates everything.
	pullOnce(map[string]any{"age": int64(40)})

	// Open-verify-close: the verification handle must release the store lock
	// before the next pull opens the store again.
	query := func(stmts ...string) []relate.StatementResult {
		t.Helper()
		store, err := relate.OpenStore(relate.ResolveStorePath("movies"), "")
		require.NoError(t, err)
		defer store.Close()
		res, err := store.Run(stmts, nil, false)
		require.NoError(t, err)
		return res
	}

	res := query("MATCH (a:Person {name: 'Ada'})-[r:KNOWS]->(b:Person {name: 'Bob'}) RETURN a.age AS age, r.since AS since")
	require.Len(t, res[0].Rows, 1, "the pulled rel must connect the pulled nodes")
	assert.EqualValues(t, 40, res[0].Rows[0][0])
	assert.EqualValues(t, 2020, res[0].Rows[0][1])

	// Second pull with a changed property updates in place.
	pullOnce(map[string]any{"age": int64(41)})

	res = query(
		"MATCH (a:Person {name: 'Ada'}) RETURN a.age AS age",
		"MATCH (n:Person) RETURN count(n) AS nodes",
		"MATCH ()-[r:KNOWS]->() RETURN count(r) AS rels",
	)
	assert.EqualValues(t, 41, res[0].Rows[0][0], "the re-pull must update the property")
	assert.EqualValues(t, 2, res[1].Rows[0][0], "the re-pull must not duplicate nodes")
	assert.EqualValues(t, 1, res[2].Rows[0][0], "the re-pull must not duplicate rels")

	// The identity sidecar sits beside the store.
	m, err := relate.LoadIdentityMap(h.cfg.Fs(), relate.ResolveStorePath("movies"))
	require.NoError(t, err)
	ids := m.Sources["neo4j+s://remote:7687/neo4j"]
	assert.Len(t, ids.Nodes, 2)
	assert.Len(t, ids.Rels, 1)
}
