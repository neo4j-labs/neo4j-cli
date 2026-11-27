// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsStore_RecognizesBothMarkerLayouts(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "/s/parquet/manifest.json", []byte("{}"), 0o644))
	require.NoError(t, afero.WriteFile(fs, "/s/vault/.relate/manifest.json", []byte("{}"), 0o644)) // markdown vault
	require.NoError(t, fs.MkdirAll("/s/empty", 0o755))
	require.NoError(t, afero.WriteFile(fs, "/s/notes/a.md", []byte("x"), 0o644))

	assert.True(t, IsStore(fs, "/s/parquet"))
	assert.True(t, IsStore(fs, "/s/vault"), "a markdown vault keeps its marker under .relate/")
	assert.False(t, IsStore(fs, "/s/empty"))
	assert.False(t, IsStore(fs, "/s/notes"), "a plain notes directory is not a store")
}
