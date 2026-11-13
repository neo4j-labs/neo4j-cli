// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build cgo && relate

// Real-engine push round-trip: the local store is the actual engine
// (populated through it, including a temporal property), the remote is the
// fake remoteExecute seam — verifying the plan's local reads against real
// Cypher and the parameterized MERGE batches the remote would receive.

package relate

import (
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/relate"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelatePushEndToEnd_RealEngine(t *testing.T) {
	origPrefix := clicfg.ConfigPrefix
	clicfg.ConfigPrefix = t.TempDir()
	t.Cleanup(func() { clicfg.ConfigPrefix = origPrefix })

	h := &harness{
		cfg:    clicfg.NewConfig(afero.NewOsFs(), "test"),
		stdout: newBuffer(),
		stderr: newBuffer(),
	}

	// Populate the local store through the engine.
	store, err := relate.OpenStore(relate.ResolveStorePath("movies"), "")
	require.NoError(t, err)
	_, err = store.Run([]string{
		"CREATE (:Person {name: 'Ada', email: 'ada@x', born: date('1985-03-31')}), " +
			"(:Person {name: 'Bob', email: 'bob@x'})",
		"MATCH (a:Person {email: 'ada@x'}), (b:Person {email: 'bob@x'}) " +
			"CREATE (a)-[:KNOWS {since: 2020}]->(b)",
	}, nil, false)
	require.NoError(t, err)
	require.NoError(t, store.Checkpoint())
	store.Close()

	// Fake remote: one uniqueness constraint on Person.email, writes captured.
	calls := installFakeRemote(t, [][]any{
		constraintRow("c1", "UNIQUENESS", "NODE", []any{"Person"}, []any{"email"}),
	})

	require.NoError(t, h.execute(t, "push", "movies",
		"--uri", "neo4j+s://remote:7687", "--username", "neo4j", "--password", "pw",
	))

	// Constraint probe + node MERGE + rel MERGE.
	require.Len(t, *calls, 3)

	nodeRows := (*calls)[1].params["rows"].([]any)
	require.Len(t, nodeRows, 2)
	// Find Ada's row; the temporal prop must cross as a driver-native
	// dbtype.Date (parameterized, no literal rendering on this path).
	var ada map[string]any
	for _, r := range nodeRows {
		row := r.(map[string]any)
		if row["key"] == "ada@x" {
			ada = row
		}
	}
	require.NotNil(t, ada, "Ada's row must be pushed")
	props := ada["props"].(map[string]any)
	born, ok := props["born"].(dbtype.Date)
	require.True(t, ok, "temporal property must cross as dbtype.Date, got %T", props["born"])
	assert.Equal(t, "1985-03-31", born.String())

	relRows := (*calls)[2].params["rows"].([]any)
	require.Len(t, relRows, 1)
	rel := relRows[0].(map[string]any)
	assert.Equal(t, "ada@x", rel["start"])
	assert.Equal(t, "bob@x", rel["end"])

	assert.Contains(t, h.stderr.String(), "Pushed 2 nodes and 1 relationships")
}
