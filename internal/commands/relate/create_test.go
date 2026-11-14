// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"encoding/json"
	"testing"

	"github.com/spf13/afero"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/relate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelateCreate_BareNameResolvesManagedPath(t *testing.T) {
	f := &fakeStore{}
	installFakeOpen(t, f)

	h := newHarness(t, "json")
	require.NoError(t, h.execute(t, "create", "movies"))

	assert.Equal(t, relate.ResolveStorePath("movies"), f.openedPath)
	assert.Equal(t, "", f.openedNamespace)
	assert.Equal(t, 1, f.checkpoints, "create must checkpoint the new store")
	assert.True(t, f.closed, "create must close the store again")

	var got map[string]any
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, "movies", got["name"])
	assert.Equal(t, relate.ResolveStorePath("movies"), got["path"])
}

func TestRelateCreate_ExplicitPathTakenVerbatim(t *testing.T) {
	f := &fakeStore{}
	installFakeOpen(t, f)

	h := newHarness(t, "json")
	require.NoError(t, h.execute(t, "create", "./mydb"))

	assert.Equal(t, "./mydb", f.openedPath)
}

func TestRelateCreate_ExistingStoreRefusedWithoutForce(t *testing.T) {
	f := &fakeStore{}
	installFakeOpen(t, f)

	h := newHarness(t, "json")
	path := h.seedStore(t, "movies")

	err := h.execute(t, "create", "movies")
	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), "already exists")
	assert.Contains(t, err.Error(), path)
	assert.Equal(t, 0, f.checkpoints, "a refused create must not touch the engine")
}

func TestRelateCreate_ForceKeepsExistingStore(t *testing.T) {
	f := &fakeStore{}
	installFakeOpen(t, f)

	h := newHarness(t, "json")
	h.seedStore(t, "movies")

	require.NoError(t, h.execute(t, "create", "movies", "--force"))

	assert.Equal(t, "", f.openedPath, "--force must not re-open the existing store")
	assert.Equal(t, 0, f.checkpoints)
}

func TestRelateCreate_RefusesForeignNonEmptyPath(t *testing.T) {
	f := &fakeStore{}
	installFakeOpen(t, f)

	h := newHarness(t, "json")
	fs := h.cfg.Fs()
	require.NoError(t, fs.MkdirAll("/tmp/foreign", 0o700))
	require.NoError(t, afero.WriteFile(fs, "/tmp/foreign/somefile.txt", []byte("x"), 0o600))

	err := h.execute(t, "create", "/tmp/foreign")
	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), "neither a Relate store nor an empty directory")
}

func TestRelateCreate_AcceptsExistingEmptyDir(t *testing.T) {
	f := &fakeStore{}
	installFakeOpen(t, f)

	h := newHarness(t, "json")
	require.NoError(t, h.cfg.Fs().MkdirAll("/tmp/empty", 0o700))

	require.NoError(t, h.execute(t, "create", "/tmp/empty"))
	assert.Equal(t, "/tmp/empty", f.openedPath)
}
