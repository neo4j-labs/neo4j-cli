// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build cgo && relate

// Real-engine check that --relate-opt reaches the engine: the options map onto
// RELATE_* variables the engine reads at open, which no fake store can prove.

package relate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngineOpenOpts_MarkdownVault(t *testing.T) {
	t.Setenv("RELATE_SNAPSHOT_FORMAT", "")
	t.Setenv("RELATE_MARKDOWN_BODY_PROPERTIES", "")
	path := filepath.Join(t.TempDir(), "vault")

	require.NoError(t, ApplyOpenOpts([]string{
		"snapshot-format=markdown",
		"markdown-body-properties=Person=bio",
	}))
	s, err := OpenStore(path, "")
	require.NoError(t, err)
	_, err = s.Run([]string{"CREATE (:Person {name: 'Ada', bio: 'First programmer'})"}, nil, false)
	require.NoError(t, err)
	require.NoError(t, s.Checkpoint())
	s.Close()

	note, err := os.ReadFile(filepath.Join(path, "Person", "Ada.md"))
	require.NoError(t, err, "a markdown store writes one note per node")
	assert.Contains(t, string(note), "name: Ada")
	assert.Contains(t, string(note), "First programmer", "the configured body property becomes the note body")
	assert.True(t, IsStore(afero.NewOsFs(), path), "a vault keeps its marker under .relate/")

	// Reopen with no option: the codec is detected from the store.
	require.NoError(t, os.Unsetenv("RELATE_SNAPSHOT_FORMAT"))
	require.NoError(t, os.Unsetenv("RELATE_MARKDOWN_BODY_PROPERTIES"))
	s2, err := OpenStore(path, "")
	require.NoError(t, err)
	defer s2.Close()
	res, err := s2.Run([]string{"MATCH (p:Person) RETURN p.name AS name"}, nil, false)
	require.NoError(t, err)
	assert.Equal(t, [][]any{{"Ada"}}, res[0].Rows)
}
