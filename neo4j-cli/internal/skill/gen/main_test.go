// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerator_RoundTrip is the local guard that complements CI's
// `make generate-check`: regenerate the bundle into a t.TempDir and
// assert byte-equal with the committed bundle. Failing means someone
// edited the cobra tree, description.txt, or additions.md without
// running `go generate ./...`.
func TestGenerator_RoundTrip(t *testing.T) {
	pkgDir := pkgDirFromTest(t)
	committedBundle := filepath.Join(pkgDir, "bundle")

	tmpRoot := t.TempDir()
	// Copy description.txt and additions.md into the staging dir so
	// generate() reads them from the same package layout.
	for _, name := range []string{"description.txt", "additions.md"} {
		data, err := os.ReadFile(filepath.Join(pkgDir, name))
		require.NoErrorf(t, err, "read %s", name)
		require.NoError(t, os.WriteFile(filepath.Join(tmpRoot, name), data, 0644))
	}

	require.NoError(t, generate(tmpRoot))

	// Walk both trees, compare relative paths and bytes.
	regen := filepath.Join(tmpRoot, "bundle")
	gotFiles := walkRel(t, regen)
	wantFiles := walkRel(t, committedBundle)

	assert.Equal(t, wantFiles, gotFiles,
		"committed bundle file list differs from regenerated; run `go generate ./neo4j-cli/internal/skill/...`")

	for _, rel := range wantFiles {
		want, err := os.ReadFile(filepath.Join(committedBundle, rel))
		require.NoErrorf(t, err, "read committed %s", rel)
		got, err := os.ReadFile(filepath.Join(regen, rel))
		require.NoErrorf(t, err, "read regenerated %s", rel)
		assert.Equalf(t, string(want), string(got),
			"%s differs; run `go generate ./neo4j-cli/internal/skill/...`", rel)
	}
}

func pkgDirFromTest(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// thisFile = .../neo4j-cli/internal/skill/gen/main_test.go
	// parent of gen/ is the skill package dir.
	return filepath.Dir(filepath.Dir(thisFile))
}

func walkRel(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	require.NoError(t, err)
	return out
}
