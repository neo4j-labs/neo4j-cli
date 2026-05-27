// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalog

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const samplePluginJSON = `{
  "name": "neo4j-skills",
  "version": "1.0.0",
  "description": "Agent skills for Neo4j",
  "skills": [
    "./neo4j-cypher-skill",
    "./neo4j-aura-agent-skill",
    "./neo4j-gds-skill"
  ]
}`

func TestLoad_PopulatesVersionAndSkills(t *testing.T) {
	memFs := afero.NewMemMapFs()
	cacheRoot := filepath.Join("cache", "neo4j-cli", "skill-catalog")
	require.NoError(t, memFs.MkdirAll(cacheRoot, 0755))
	require.NoError(t, afero.WriteFile(memFs, filepath.Join(cacheRoot, "plugin.json"), []byte(samplePluginJSON), 0600))

	cat := New(Options{CacheRoot: cacheRoot})
	require.NoError(t, cat.Load(memFs))

	assert.Equal(t, "1.0.0", cat.Version)
	require.Len(t, cat.Skills, 3)
	assert.Equal(t, "neo4j-cypher-skill", cat.Skills[0].Name)
	assert.Equal(t, "./neo4j-cypher-skill", cat.Skills[0].Path)
	assert.Equal(t, "neo4j-aura-agent-skill", cat.Skills[1].Name)
	assert.Equal(t, "neo4j-gds-skill", cat.Skills[2].Name)
	assert.Equal(t, cacheRoot, cat.cacheRoot)
}

func TestLoad_Errors(t *testing.T) {
	tests := []struct {
		name      string
		cacheRoot string
		setup     func(afero.Fs, string)
	}{
		{
			name:      "empty cache root",
			cacheRoot: "",
			setup:     func(_ afero.Fs, _ string) {},
		},
		{
			name:      "missing plugin.json",
			cacheRoot: filepath.Join("cache", "neo4j-cli", "skill-catalog"),
			setup:     func(_ afero.Fs, _ string) {},
		},
		{
			name:      "malformed plugin.json",
			cacheRoot: filepath.Join("cache", "neo4j-cli", "skill-catalog"),
			setup: func(fs afero.Fs, root string) {
				_ = fs.MkdirAll(root, 0755)
				_ = afero.WriteFile(fs, filepath.Join(root, "plugin.json"), []byte("not-json"), 0600)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			memFs := afero.NewMemMapFs()
			tc.setup(memFs, tc.cacheRoot)

			cat := New(Options{CacheRoot: tc.cacheRoot})
			err := cat.Load(memFs)
			assert.Error(t, err)
		})
	}
}

func TestLoad_NilReceiver_Errors(t *testing.T) {
	var nilCat *Catalog
	assert.Error(t, nilCat.Load(afero.NewMemMapFs()))
}

func TestStale(t *testing.T) {
	const ttl = 24 * time.Hour
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		setup func(afero.Fs, string)
		want  bool
	}{
		{
			name:  "missing fetched-at is stale",
			setup: func(_ afero.Fs, _ string) {},
			want:  true,
		},
		{
			name: "recent fetched-at is fresh",
			setup: func(fs afero.Fs, root string) {
				_ = fs.MkdirAll(root, 0755)
				ts := now.Add(-1 * time.Hour).Format(time.RFC3339)
				_ = afero.WriteFile(fs, filepath.Join(root, "fetched-at"), []byte(ts), 0600)
			},
			want: false,
		},
		{
			name: "old fetched-at is stale",
			setup: func(fs afero.Fs, root string) {
				_ = fs.MkdirAll(root, 0755)
				ts := now.Add(-25 * time.Hour).Format(time.RFC3339)
				_ = afero.WriteFile(fs, filepath.Join(root, "fetched-at"), []byte(ts), 0600)
			},
			want: true,
		},
		{
			name: "unparseable fetched-at is stale",
			setup: func(fs afero.Fs, root string) {
				_ = fs.MkdirAll(root, 0755)
				_ = afero.WriteFile(fs, filepath.Join(root, "fetched-at"), []byte("garbage"), 0600)
			},
			want: true,
		},
	}

	origNow := nowFn
	t.Cleanup(func() { nowFn = origNow })
	nowFn = func() time.Time { return now }

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			memFs := afero.NewMemMapFs()
			cacheRoot := filepath.Join("cache", "neo4j-cli", "skill-catalog")
			tc.setup(memFs, cacheRoot)

			got := Stale(memFs, cacheRoot, ttl)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestStale_EmptyCacheRoot(t *testing.T) {
	assert.True(t, Stale(afero.NewMemMapFs(), "", 24*time.Hour))
}

func TestCacheRoot_HonoursTestSeam(t *testing.T) {
	origFn := userCacheDirFn
	t.Cleanup(func() { userCacheDirFn = origFn })

	userCacheDirFn = func() (string, error) {
		return filepath.Join("tmp", "user-cache"), nil
	}

	got, err := CacheRoot()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("tmp", "user-cache", "neo4j-cli", "skill-catalog"), got)
}

func TestSkillNameFromPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"./neo4j-cypher-skill", "neo4j-cypher-skill"},
		{"neo4j-cypher-skill", "neo4j-cypher-skill"},
		{"./sub/dir/foo-skill", "foo-skill"},
		{"  ./trim-me  ", "trim-me"},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, skillNameFromPath(tc.in))
		})
	}
}

func TestValidSkillName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"happy path", "neo4j-cypher-skill", true},
		{"alphanumeric", "skill123", true},
		{"underscore", "neo4j_skill", true},
		{"empty", "", false},
		{"dot", ".", false},
		{"double dot", "..", false},
		{"forward slash", "foo/bar", false},
		{"backslash", "foo\\bar", false},
		{"leading dot", ".hidden", false},
		{"leading dot git", ".git", false},
		{"nul byte", "foo\x00bar", false},
		{"embedded space", "foo bar", false},
		{"embedded newline", "foo\nbar", false},
		{"embedded tab", "foo\tbar", false},
		{"embedded carriage return", "foo\rbar", false},
		{"leading space", " foo", false},
		{"trailing newline", "foo\n", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ValidSkillName(tc.in))
		})
	}
}

func TestLoad_DropsMaliciousUpstreamSkillNames(t *testing.T) {
	// path.Base("..") == "..", path.Base("/") == "/",
	// path.Base("foo/..") == "..", path.Base("./..") == "..", etc.
	// All of these would otherwise reach Install where filepath.Join
	// cleans `..` segments and RemoveAll escapes skillsRoot.
	// Each entry's path.Base (after TrimSpace + "./" strip) lands on a name
	// that must be rejected. "foo/.." -> "..", "./.." -> "..", "  ..  " ->
	// "..", "/" -> "/", "." -> ".", ".git" -> ".git".
	malicious := []string{
		"..",
		".",
		"/",
		"foo/..",
		"./..",
		"  ..  ",
		".git",
	}
	var skillsJSON strings.Builder
	for i, s := range malicious {
		if i > 0 {
			skillsJSON.WriteString(",")
		}
		// JSON-encode as a raw string entry.
		skillsJSON.WriteString(`"`)
		skillsJSON.WriteString(s)
		skillsJSON.WriteString(`"`)
	}
	body := `{"name":"neo4j-skills","version":"1.0.0","skills":[` + skillsJSON.String() + `,"./safe-skill"]}`

	memFs := afero.NewMemMapFs()
	cacheRoot := filepath.Join("cache", "neo4j-cli", "skill-catalog")
	require.NoError(t, memFs.MkdirAll(cacheRoot, 0755))
	require.NoError(t, afero.WriteFile(memFs, filepath.Join(cacheRoot, "plugin.json"), []byte(body), 0600))

	cat := New(Options{CacheRoot: cacheRoot})
	require.NoError(t, cat.Load(memFs))

	// Only the safe entry survives.
	require.Len(t, cat.Skills, 1)
	assert.Equal(t, "safe-skill", cat.Skills[0].Name)
}
