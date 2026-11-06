// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/relate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelateList_EmptyRendersEmptyArray(t *testing.T) {
	h := newHarness(t, "json")
	require.NoError(t, h.execute(t, "list"))

	assert.JSONEq(t, "[]", h.stdout.String())
}

func TestRelateList_ListsManagedStoresOnly(t *testing.T) {
	h := newHarness(t, "json")
	fs := h.cfg.Fs()

	h.seedStore(t, "movies")
	h.seedStore(t, "people")
	// A .rl bundle in the managed dir counts as a store.
	require.NoError(t, afero.WriteFile(fs, filepath.Join(relate.StoreDir(), "bundle.rl"), []byte("x"), 0o600))
	// Stray files and non-store directories are skipped.
	require.NoError(t, afero.WriteFile(fs, filepath.Join(relate.StoreDir(), "notes.txt"), []byte("x"), 0o600))
	require.NoError(t, fs.MkdirAll(filepath.Join(relate.StoreDir(), "just-a-dir"), 0o700))

	require.NoError(t, h.execute(t, "list"))

	var got []map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	require.Len(t, got, 3)

	byName := map[string]map[string]any{}
	for _, row := range got {
		byName[row["name"].(string)] = row
	}
	assert.Contains(t, byName, "movies")
	assert.Contains(t, byName, "people")
	assert.Contains(t, byName, "bundle.rl")
	assert.Equal(t, "directory", byName["movies"]["kind"])
	assert.Equal(t, "bundle", byName["bundle.rl"]["kind"])
	assert.NotContains(t, byName, "notes.txt")
	assert.NotContains(t, byName, "just-a-dir")
}

func TestRelateDelete_RefusesNonStore(t *testing.T) {
	h := newHarness(t, "json")
	nonInteractive(t)

	err := h.execute(t, "delete", "movies")
	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), "no Relate store at")
}

func TestRelateDelete_RefusesManagedDirItself(t *testing.T) {
	h := newHarness(t, "json")
	nonInteractive(t)

	// A path-shaped arg pointing at the managed directory itself.
	err := h.execute(t, "delete", relate.StoreDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "managed store directory itself")
}

func TestRelateDelete_RequiresYesForceWhenScripted(t *testing.T) {
	h := newHarness(t, "json")
	nonInteractive(t)
	path := h.seedStore(t, "movies")

	err := h.execute(t, "delete", "movies", "--yes")
	require.Error(t, err, "--yes alone must not bypass the non-TTY gate")
	assert.Contains(t, err.Error(), "--force")

	// The store is still there.
	_, statErr := h.cfg.Fs().Stat(path)
	require.NoError(t, statErr)
}

func TestRelateDelete_RemovesStoreWithYesForce(t *testing.T) {
	h := newHarness(t, "json")
	nonInteractive(t)
	path := h.seedStore(t, "movies")

	require.NoError(t, h.execute(t, "delete", "movies", "--yes", "--force"))

	_, statErr := h.cfg.Fs().Stat(path)
	require.Error(t, statErr, "the store directory must be gone")
	assert.Contains(t, h.stderr.String(), "Deleted Relate store")
}

func TestRelateDelete_ExplicitPathStore(t *testing.T) {
	h := newHarness(t, "json")
	nonInteractive(t)

	fs := h.cfg.Fs()
	path := "/tmp/explicit-store"
	require.NoError(t, fs.MkdirAll(path, 0o700))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(path, "manifest.json"), []byte("{}"), 0o600))

	require.NoError(t, h.execute(t, "delete", path, "--yes", "--force"))

	_, statErr := fs.Stat(path)
	require.Error(t, statErr)
}
