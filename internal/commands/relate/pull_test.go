// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/dbconn"
	"github.com/neo4j/cli/internal/relate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// installFakeStream swaps the remote streaming seam for one replaying the
// given record value rows (each entry is one record's positional values).
func installFakeStream(t *testing.T, records [][]any, streamErr error) *[]string {
	t.Helper()
	var queries []string
	orig := streamRemoteRecords
	streamRemoteRecords = func(ctx context.Context, driver neo4j.Driver, database, query string, params map[string]any, onRow func(values []any) error) error {
		queries = append(queries, query)
		if streamErr != nil {
			return streamErr
		}
		for _, rec := range records {
			if err := onRow(rec); err != nil {
				return err
			}
		}
		return nil
	}
	t.Cleanup(func() { streamRemoteRecords = orig })
	return &queries
}

// tripleRecord builds an (a, rel, b) record.
func tripleRecord(aEid, relEid, bEid string) []any {
	return []any{
		dbtype.Node{ElementId: aEid, Labels: []string{"Person"}, Props: map[string]any{"name": "Ada"}},
		dbtype.Relationship{ElementId: relEid, StartElementId: aEid, EndElementId: bEid, Type: "KNOWS", Props: map[string]any{"since": int64(2020)}},
		dbtype.Node{ElementId: bEid, Labels: []string{"Person"}, Props: map[string]any{"name": "Bob"}},
	}
}

func TestRelatePull_MaterializesAndReports(t *testing.T) {
	f := &fakeStore{}
	installFakeOpen(t, f)
	installFakeStream(t, [][]any{
		tripleRecord("4:a:0", "5:a:9", "4:a:1"),
	}, nil)

	h := newHarness(t, "json")
	// The fake opener does not create the directory; seed it so the identity
	// sidecar can be written beside the store.
	storePath := relate.ResolveStorePath("movies")
	require.NoError(t, h.cfg.Fs().MkdirAll(storePath, 0o700))

	err := h.execute(t, "pull", "movies",
		"--uri", "neo4j://remote:7687", "--username", "neo4j", "--password", "pw",
		"--query", "MATCH (a)-[r:KNOWS]->(b) RETURN a, r, b",
	)
	require.NoError(t, err)

	// The store was opened (created), checkpointed, closed, and the identity
	// sidecar written.
	assert.Equal(t, storePath, f.openedPath)
	assert.Equal(t, 1, f.checkpoints)
	assert.True(t, f.closed)

	m, err := relate.LoadIdentityMap(h.cfg.Fs(), storePath)
	require.NoError(t, err)
	ids := m.Sources["neo4j://remote:7687/neo4j"]
	assert.Len(t, ids.Nodes, 2)
	assert.Len(t, ids.Rels, 1)

	assert.Contains(t, h.stderr.String(), "2 nodes (2 new, 0 reused)")
	assert.Contains(t, h.stderr.String(), "1 relationships (1 new, 0 reused, 0 skipped: endpoint not pulled)")
}

// Regression: a pull that failed after committing its node batch left no
// identity map, so the retry re-created every node.
func TestRelatePull_FailurePartwayStillSavesIdentities(t *testing.T) {
	f := &fakeStore{failCreateAfter: 1} // nodes commit, the relationship batch fails
	installFakeOpen(t, f)
	installFakeStream(t, [][]any{tripleRecord("4:a:0", "5:a:9", "4:a:1")}, nil)

	h := newHarness(t, "json")
	storePath := relate.ResolveStorePath("movies")
	require.NoError(t, h.cfg.Fs().MkdirAll(storePath, 0o700))

	err := h.execute(t, "pull", "movies",
		"--uri", "neo4j://remote:7687", "--username", "neo4j", "--password", "pw",
		"--query", "MATCH (a)-[r:KNOWS]->(b) RETURN a, r, b",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated failure mid-pull")

	m, loadErr := relate.LoadIdentityMap(h.cfg.Fs(), storePath)
	require.NoError(t, loadErr)
	assert.Len(t, m.Sources["neo4j://remote:7687/neo4j"].Nodes, 2, "the committed nodes keep their identities for the retry")
}

// A Cypher mistake in --query is a validation failure (exit 6), not an upstream
// one (exit 8): retry-on-upstream scripts must not loop on an unfixable query.
func TestRelatePull_RemoteErrorsAreClassifiedLikeQuery(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantExit int
	}{
		{"cypher syntax error", &neo4j.Neo4jError{Code: "Neo.ClientError.Statement.SyntaxError", Msg: "Invalid input"}, 6},
		{"transient failure stays upstream", &neo4j.Neo4jError{Code: "Neo.TransientError.General.DatabaseUnavailable", Msg: "down"}, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installFakeOpen(t, &fakeStore{})
			installFakeStream(t, nil, tt.err)

			h := newHarness(t, "json")
			require.NoError(t, h.cfg.Fs().MkdirAll(relate.ResolveStorePath("movies"), 0o700))
			err := h.execute(t, "pull", "movies",
				"--uri", "neo4j://remote:7687", "--username", "neo4j", "--password", "pw",
				"--query", "MATCH (n RETURN n",
			)
			require.Error(t, err)
			var ce *clierr.CLIError
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, tt.wantExit, ce.Code)
		})
	}
}

func TestRelatePull_ScalarOnlyIsUsageError(t *testing.T) {
	installFakeOpen(t, &fakeStore{})
	installFakeStream(t, [][]any{{"Ada", int64(1)}}, nil)

	h := newHarness(t, "json")
	err := h.execute(t, "pull", "movies",
		"--uri", "neo4j://remote:7687", "--username", "neo4j", "--password", "pw",
		"--query", "MATCH (n) RETURN n.name AS name",
	)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), "only scalar values")
}

func TestRelatePull_EmptyResultIsNoOp(t *testing.T) {
	f := &fakeStore{}
	installFakeOpen(t, f)
	installFakeStream(t, nil, nil)

	h := newHarness(t, "json")
	require.NoError(t, h.cfg.Fs().MkdirAll(relate.ResolveStorePath("movies"), 0o700))

	err := h.execute(t, "pull", "movies",
		"--uri", "neo4j://remote:7687", "--username", "neo4j", "--password", "pw",
		"--query", "MATCH (n:Nothing) RETURN n",
	)
	require.NoError(t, err)
	assert.Empty(t, f.statements, "an empty remote result writes nothing")
	assert.Equal(t, 1, f.checkpoints, "the pull still checkpoints (and creates the store)")
}

func TestRelatePull_ParamsPassedToRemoteQuery(t *testing.T) {
	installFakeOpen(t, &fakeStore{})
	queries := installFakeStream(t, nil, nil)

	h := newHarness(t, "json")
	require.NoError(t, h.cfg.Fs().MkdirAll(relate.ResolveStorePath("movies"), 0o700))

	err := h.execute(t, "pull", "movies",
		"--uri", "neo4j://remote:7687", "--username", "neo4j", "--password", "pw",
		"--query", "MATCH (n) WHERE n.k > $min RETURN n",
		"--param", "min=5",
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"MATCH (n) WHERE n.k > $min RETURN n"}, *queries)
}

func TestRelatePull_EmbedModifierRefused(t *testing.T) {
	installFakeOpen(t, &fakeStore{})

	h := newHarness(t, "json")
	err := h.execute(t, "pull", "movies",
		"--uri", "neo4j://remote:7687", "--username", "neo4j", "--password", "pw",
		"--query", "MATCH (n) RETURN n",
		"--param", "v:embed=hello",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ":embed is not supported")
}

func TestRelatePull_RemoteFailureIsUpstreamError(t *testing.T) {
	installFakeOpen(t, &fakeStore{})
	installFakeStream(t, nil, errors.New("connection refused"))

	h := newHarness(t, "json")
	err := h.execute(t, "pull", "movies",
		"--uri", "neo4j://remote:7687", "--username", "neo4j", "--password", "pw",
		"--query", "MATCH (n) RETURN n",
	)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 8, ce.Code)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestRelatePull_MissingPasswordNonTTYIsUsageError(t *testing.T) {
	installFakeOpen(t, &fakeStore{})
	installFakeStream(t, nil, nil)
	nonInteractive(t)

	h := newHarness(t, "json")
	err := h.execute(t, "pull", "movies",
		"--uri", "neo4j://remote:7687", "--username", "neo4j",
		"--query", "MATCH (n) RETURN n",
	)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 2, ce.Code)
}

func TestRelatePull_QueryRequired(t *testing.T) {
	h := newHarness(t, "json")
	err := h.execute(t, "pull", "movies", "--uri", "neo4j://remote:7687")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `required flag(s) "query" not set`)
}

// identitySourceOf separates per remote URI and database.
func TestIdentitySourceOf(t *testing.T) {
	c := &dbconn.Conn{URI: "neo4j://remote:7687", Database: "movies"}
	assert.Equal(t, "neo4j://remote:7687/movies", identitySourceOf(c))

	c2 := &dbconn.Conn{URI: "neo4j://other:7687", Database: ""}
	assert.Equal(t, "neo4j://other:7687/neo4j", identitySourceOf(c2))
}

func TestRelatePull_BatchSizeBounds(t *testing.T) {
	// Out-of-bounds values are usage errors before any store or remote work:
	// only the not-compiled gate precedes the check, so the fake exists merely
	// to get past it.
	installFakeOpen(t, &fakeStore{})

	for _, size := range []int{0, -1, relate.MaxPullBatchSize + 1} {
		h := newHarness(t, "json")
		err := h.execute(t, "pull", "movies",
			"--uri", "neo4j://remote:7687", "--username", "neo4j", "--password", "pw",
			"--query", "MATCH (n) RETURN n",
			"--batch-size", strconv.Itoa(size),
		)
		require.Error(t, err, "batch size %d must be rejected", size)
		var ce *clierr.CLIError
		require.ErrorAs(t, err, &ce)
		assert.Equal(t, 2, ce.Code)
		assert.Contains(t, err.Error(), "--batch-size")
	}

	// The upper bound itself is accepted: an empty stream then succeeds, so
	// the rejection above is the bound, not the stream.
	installFakeStream(t, nil, nil)
	h := newHarness(t, "json")
	require.NoError(t, h.cfg.Fs().MkdirAll(relate.ResolveStorePath("movies"), 0o700))
	err := h.execute(t, "pull", "movies",
		"--uri", "neo4j://remote:7687", "--username", "neo4j", "--password", "pw",
		"--query", "MATCH (n:Nothing) RETURN n",
		"--batch-size", strconv.Itoa(relate.MaxPullBatchSize),
	)
	require.NoError(t, err)
}
