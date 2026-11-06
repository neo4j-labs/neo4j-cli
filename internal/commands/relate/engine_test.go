// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build cgo && relate

// End-to-end cycle of the relate tree against the real engine: the CLI's
// state tree (managed store directory) is pointed at a throwaway directory
// on the real filesystem, then create → list → populate → info → delete runs
// through the actual commands.

package relate

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/relate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBuffer keeps the harness literal in the e2e test short.
func newBuffer() *bytes.Buffer { return &bytes.Buffer{} }

func TestRelateTreeEndToEnd_CreateListInfoDelete(t *testing.T) {
	// Point the whole CLI state tree at a throwaway directory on the real
	// filesystem (the engine writes through cgo to OS paths, so the in-memory
	// test fs cannot back it).
	origPrefix := clicfg.ConfigPrefix
	clicfg.ConfigPrefix = t.TempDir()
	t.Cleanup(func() { clicfg.ConfigPrefix = origPrefix })

	// Seed the config with an explicit json format: a fresh config would
	// auto-detect (toon under an agent harness) and break the JSON decode
	// assertions below.
	osFs := afero.NewOsFs()
	require.NoError(t, osFs.MkdirAll(filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli"), 0o700))
	require.NoError(t, afero.WriteFile(osFs, filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "config.json"),
		[]byte(`{"format":"json"}`), 0o600))

	h := &harness{
		cfg:    clicfg.NewConfig(afero.NewOsFs(), "test"),
		stdout: newBuffer(),
		stderr: newBuffer(),
	}
	nonInteractive(t)

	// create
	require.NoError(t, h.execute(t, "create", "movies"))
	var created map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &created))
	storePath := created["path"].(string)
	assert.Equal(t, filepath.Join(relate.StoreDir(), "movies"), storePath)
	assert.FileExists(t, filepath.Join(storePath, "manifest.json"))

	// A second create without --force refuses.
	err := h.execute(t, "create", "movies")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// list shows it
	h.stdout.Reset()
	require.NoError(t, h.execute(t, "list"))
	var listed []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &listed))
	require.Len(t, listed, 1)
	assert.Equal(t, "movies", listed[0]["name"])
	assert.Equal(t, "directory", listed[0]["kind"])
	assert.Greater(t, listed[0]["size"].(float64), float64(0))

	// populate one node through the engine directly
	store, err := relate.OpenStore(storePath, "")
	require.NoError(t, err)
	_, err = store.Run([]string{"CREATE (:Movie {title: 'The Matrix'})"}, nil, false)
	require.NoError(t, err)
	store.Close()

	// info reports the engine identity and the node
	h.stdout.Reset()
	require.NoError(t, h.execute(t, "info", "movies"))
	var info map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &info))
	row := info
	assert.NotEmpty(t, row["engine"], "dbms.components must report an engine name (the compat identity, e.g. Neo4j Kernel)")
	assert.NotEmpty(t, row["version"], "dbms.components must report a version")
	assert.EqualValues(t, 1, row["nodes"])
	assert.Equal(t, []any{"Movie"}, row["labels"])
	assert.EqualValues(t, 0, row["relationships"])

	// delete (scripted gate: --yes --force)
	require.NoError(t, h.execute(t, "delete", "movies", "--yes", "--force"))
	assert.NoDirExists(t, storePath)

	// list is empty again
	h.stdout.Reset()
	require.NoError(t, h.execute(t, "list"))
	assert.JSONEq(t, "[]", h.stdout.String())
}
