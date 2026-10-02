// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureBundle returns a fs.FS whose root mirrors a generated bundle
// layout: SKILL.md at the root and references/<sub>.md files. Mirrors
// what fs.Sub(embed.FS, "bundle") would produce in the real generator.
func fixtureBundle() fs.FS {
	return fstest.MapFS{
		"SKILL.md":            {Data: []byte("# top\n")},
		"references/aura.md":  {Data: []byte("# aura ref\n")},
		"references/skill.md": {Data: []byte("# skill ref\n")},
		// deeper nesting still copies (defensive — PRD bans nested
		// references in v1, but the helper itself is generic).
		"references/nested/x": {Data: []byte("nested\n")},
	}
}

func TestCopyBundle(t *testing.T) {
	tests := []struct {
		name    string
		dstDir  string
		want    map[string]string
		wantErr bool
	}{
		{
			name:   "preserves SKILL.md + references/ tree",
			dstDir: filepath.Join("home", "skills", "neo4j-cli"),
			want: map[string]string{
				filepath.Join("home", "skills", "neo4j-cli", "SKILL.md"):                  "# top\n",
				filepath.Join("home", "skills", "neo4j-cli", "references", "aura.md"):     "# aura ref\n",
				filepath.Join("home", "skills", "neo4j-cli", "references", "skill.md"):    "# skill ref\n",
				filepath.Join("home", "skills", "neo4j-cli", "references", "nested", "x"): "nested\n",
			},
		},
		{
			name:    "empty dst dir errors",
			dstDir:  "",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			memFs := afero.NewMemMapFs()
			err := CopyBundle(memFs, tc.dstDir, fixtureBundle())
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			for p, want := range tc.want {
				data, rerr := afero.ReadFile(memFs, p)
				require.NoError(t, rerr, "expected file at %s", p)
				assert.Equal(t, want, string(data), "content at %s", p)
			}
		})
	}
}

func TestCopyBundleNilFS(t *testing.T) {
	memFs := afero.NewMemMapFs()
	err := CopyBundle(memFs, "x", nil)
	require.Error(t, err)
}

func TestCopyBundleOverwrites(t *testing.T) {
	memFs := afero.NewMemMapFs()
	dst := filepath.Join("d", "skills")
	require.NoError(t, memFs.MkdirAll(dst, 0755))
	require.NoError(t, afero.WriteFile(memFs, filepath.Join(dst, "SKILL.md"), []byte("stale"), 0600))

	require.NoError(t, CopyBundle(memFs, dst, fixtureBundle()))

	got, err := afero.ReadFile(memFs, filepath.Join(dst, "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "# top\n", string(got))
}

// Mirrors how the real generator will hand a scoped subtree off to
// CopyBundle: //go:embed bundle ; fs.Sub(Bundle, "bundle").
func TestCopyBundleWithFsSub(t *testing.T) {
	rooted := fstest.MapFS{
		"bundle/SKILL.md":          {Data: []byte("scoped\n")},
		"bundle/references/cmd.md": {Data: []byte("ref\n")},
		"unrelated/ignored.md":     {Data: []byte("nope\n")},
	}
	sub, err := fs.Sub(rooted, "bundle")
	require.NoError(t, err)

	memFs := afero.NewMemMapFs()
	dst := filepath.Join("d", "skills")
	require.NoError(t, CopyBundle(memFs, dst, sub))

	skill, err := afero.ReadFile(memFs, filepath.Join(dst, "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "scoped\n", string(skill))

	ref, err := afero.ReadFile(memFs, filepath.Join(dst, "references", "cmd.md"))
	require.NoError(t, err)
	assert.Equal(t, "ref\n", string(ref))

	// `unrelated/` was outside the sub — must not have been copied.
	exists, _ := afero.DirExists(memFs, filepath.Join(dst, "unrelated"))
	assert.False(t, exists)
}

func TestRemoveDir(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(afero.Fs)
		dir      string
		wantErr  bool
		wantGone bool
	}{
		{
			name: "removes populated dir recursively",
			setup: func(fs afero.Fs) {
				_ = fs.MkdirAll(filepath.Join("d", "skills", "refs"), 0755)
				_ = afero.WriteFile(fs, filepath.Join("d", "skills", "SKILL.md"), []byte("x"), 0600)
				_ = afero.WriteFile(fs, filepath.Join("d", "skills", "refs", "a.md"), []byte("y"), 0600)
			},
			dir:      filepath.Join("d", "skills"),
			wantGone: true,
		},
		{
			name:     "missing dir is a no-op (idempotent)",
			setup:    func(_ afero.Fs) {},
			dir:      filepath.Join("never", "existed"),
			wantGone: true,
		},
		{
			name:    "empty path errors",
			setup:   func(_ afero.Fs) {},
			dir:     "",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			memFs := afero.NewMemMapFs()
			tc.setup(memFs)

			err := RemoveDir(memFs, tc.dir)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tc.wantGone {
				exists, eerr := afero.DirExists(memFs, tc.dir)
				require.NoError(t, eerr)
				assert.False(t, exists, "dir should not exist after RemoveDir")
			}
		})
	}
}

func TestRemoveDirIdempotent(t *testing.T) {
	memFs := afero.NewMemMapFs()
	dir := filepath.Join("d", "skills")
	require.NoError(t, memFs.MkdirAll(dir, 0755))
	require.NoError(t, afero.WriteFile(memFs, filepath.Join(dir, "SKILL.md"), []byte("x"), 0600))

	require.NoError(t, RemoveDir(memFs, dir))
	// Second call must also succeed.
	require.NoError(t, RemoveDir(memFs, dir))
}
