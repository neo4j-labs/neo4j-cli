// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/skill/catalog"
)

func fixtureSelfBundle() fs.FS {
	return fstest.MapFS{
		"SKILL.md": {Data: []byte("---\nname: neo4j-cli\n---\n")},
	}
}

func TestResolveSelf(t *testing.T) {
	bundle := fixtureSelfBundle()
	const binaryName = "neo4j-cli"
	const version = "1.7.0"

	tests := []struct {
		name        string
		input       string
		wantErr     bool
		wantErrSelf bool
	}{
		{name: "canonical self resolves to embedded bundle", input: "self"},
		{name: "binary-name alias resolves to embedded bundle", input: binaryName},
		{name: "unknown catalog name returns ErrNotSelfSkill", input: "neo4j-cypher-skill", wantErr: true, wantErrSelf: true},
		{name: "empty name returns ErrNotSelfSkill", input: "", wantErr: true, wantErrSelf: true},
		{name: "case-mismatched self is NOT aliased (catalog ids are lowercase)", input: "Self", wantErr: true, wantErrSelf: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src, err := ResolveSelf(bundle, version, binaryName, tc.input)
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrSelf {
					assert.ErrorIs(t, err, ErrNotSelfSkill)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, version, src.Version)
			require.NotNil(t, src.FS)
			data, rerr := fs.ReadFile(src.FS, "SKILL.md")
			require.NoError(t, rerr)
			assert.Contains(t, string(data), "name: neo4j-cli")
		})
	}
}

func TestResolveSelfNilBundle(t *testing.T) {
	_, err := ResolveSelf(nil, "1.0.0", "neo4j-cli", "self")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotSelfSkill, "nil-bundle error is a programmer error, not a not-self sentinel")
}

func TestResolveSelfReservedSetMatchesCatalog(t *testing.T) {
	// Sanity: the resolver and catalog must agree on the reserved-name
	// set. If the catalog ever stops reserving the binary name (or
	// `self`), this test fails loudly.
	const binaryName = "neo4j-cli"
	for _, n := range catalog.ReservedNames(binaryName) {
		_, err := ResolveSelf(fixtureSelfBundle(), "1", binaryName, n)
		require.NoError(t, err, "catalog.ReservedNames returned %q but resolver did not accept it", n)
	}
}

func TestResolveSelfEmptyBinaryName(t *testing.T) {
	// An empty binaryName only reserves the canonical `self` id.
	src, err := ResolveSelf(fixtureSelfBundle(), "1", "", "self")
	require.NoError(t, err)
	assert.NotNil(t, src.FS)

	_, err = ResolveSelf(fixtureSelfBundle(), "1", "", "neo4j-cli")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotSelfSkill)
}
