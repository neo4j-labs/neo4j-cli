// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// routingDoer maps absolute upstream URLs to httptest paths so production
// constants stay frozen while tests route through an in-process server.
type routingDoer struct {
	server *httptest.Server
	routes map[string]string // upstream URL -> httptest path
}

func (r *routingDoer) Do(req *http.Request) (*http.Response, error) {
	path, ok := r.routes[req.URL.String()]
	if !ok {
		return nil, fmt.Errorf("no route for %s", req.URL.String())
	}
	rewritten, err := http.NewRequestWithContext(req.Context(), req.Method, r.server.URL+path, req.Body)
	if err != nil {
		return nil, err
	}
	rewritten.Header = req.Header.Clone()
	return r.server.Client().Do(rewritten)
}

// catalogFixture sets up a test server, a doer that routes the two
// production URLs to it, and a Catalog backed by an afero mem fs rooted
// at `cache/`. The plugin handler echoes whatever the test wrote to
// `pluginBody`; the tarball handler streams `tarballBody`.
type catalogFixture struct {
	fs          afero.Fs
	cacheRoot   string
	server      *httptest.Server
	doer        *routingDoer
	pluginBody  []byte
	tarballBody []byte
	pluginHits  int
	tarballHits int
}

func newFixture(t *testing.T) *catalogFixture {
	t.Helper()
	fx := &catalogFixture{
		fs:        afero.NewMemMapFs(),
		cacheRoot: filepath.Join("cache", "neo4j-cli", "skill-catalog"),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/plugin.json", func(w http.ResponseWriter, _ *http.Request) {
		fx.pluginHits++
		if fx.pluginBody == nil {
			http.Error(w, "no body", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(fx.pluginBody)
	})
	mux.HandleFunc("/tarball", func(w http.ResponseWriter, _ *http.Request) {
		fx.tarballHits++
		if fx.tarballBody == nil {
			http.Error(w, "no body", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(fx.tarballBody)
	})
	fx.server = httptest.NewServer(mux)
	fx.doer = &routingDoer{
		server: fx.server,
		routes: map[string]string{
			PluginJSONURL: "/plugin.json",
			TarballURL:    "/tarball",
		},
	}
	t.Cleanup(fx.server.Close)
	return fx
}

func (fx *catalogFixture) catalog() *Catalog {
	return New(Options{
		CacheRoot:     fx.cacheRoot,
		Doer:          fx.doer,
		BinaryVersion: "test-1.2.3",
	})
}

// makeTarball builds an in-memory gzipped tar containing a SKILL.md for
// each named skill plus a top-level archive directory.
func makeTarball(t *testing.T, version string, skills ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	top := "neo4j-skills-" + version + "/"
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: top, Mode: 0o755, Typeflag: tar.TypeDir}))
	for _, s := range skills {
		body := "# " + s + "\n"
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: top + s + "/SKILL.md", Mode: 0o600, Typeflag: tar.TypeReg, Size: int64(len(body)),
		}))
		_, werr := tw.Write([]byte(body))
		require.NoError(t, werr)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func pluginJSONBody(version string, skills ...string) []byte {
	var buf bytes.Buffer
	buf.WriteString(`{"name":"neo4j-skills","version":"`)
	buf.WriteString(version)
	buf.WriteString(`","skills":[`)
	for i, s := range skills {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteByte('"')
		buf.WriteString("./" + s)
		buf.WriteByte('"')
	}
	buf.WriteString("]}")
	return buf.Bytes()
}

func TestRefresh_FetchesPluginAndTarball_OnFirstRun(t *testing.T) {
	fx := newFixture(t)
	fx.pluginBody = pluginJSONBody("1.0.0", "neo4j-cypher-skill")
	fx.tarballBody = makeTarball(t, "1.0.0", "neo4j-cypher-skill")

	origNow := nowFn
	t.Cleanup(func() { nowFn = origNow })
	frozen := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return frozen }

	cat := fx.catalog()
	require.NoError(t, cat.Refresh(context.Background(), fx.fs))

	assert.Equal(t, "1.0.0", cat.Version)
	require.Len(t, cat.Skills, 1)
	assert.Equal(t, "neo4j-cypher-skill", cat.Skills[0].Name)

	plugin, err := afero.ReadFile(fx.fs, filepath.Join(fx.cacheRoot, "plugin.json"))
	require.NoError(t, err)
	assert.Equal(t, fx.pluginBody, plugin)

	ts, err := afero.ReadFile(fx.fs, filepath.Join(fx.cacheRoot, "fetched-at"))
	require.NoError(t, err)
	assert.Equal(t, frozen.UTC().Format(time.RFC3339), string(ts))

	skillFile := filepath.Join(fx.cacheRoot, "content", "1.0.0", "neo4j-cypher-skill", "SKILL.md")
	body, err := afero.ReadFile(fx.fs, skillFile)
	require.NoError(t, err)
	assert.Equal(t, "# neo4j-cypher-skill\n", string(body))

	assert.Equal(t, 1, fx.pluginHits)
	assert.Equal(t, 1, fx.tarballHits)
}

func TestRefresh_SkipsTarball_WhenVersionUnchanged(t *testing.T) {
	fx := newFixture(t)
	fx.pluginBody = pluginJSONBody("1.0.0", "neo4j-cypher-skill")
	fx.tarballBody = makeTarball(t, "1.0.0", "neo4j-cypher-skill")

	cat := fx.catalog()
	cat.Version = "1.0.0" // simulate cached state already at upstream version

	require.NoError(t, cat.Refresh(context.Background(), fx.fs))
	assert.Equal(t, 1, fx.pluginHits)
	assert.Equal(t, 0, fx.tarballHits, "tarball must not be fetched when version is unchanged")
}

func TestRefresh_ReExtracts_WhenVersionChanged(t *testing.T) {
	fx := newFixture(t)
	fx.pluginBody = pluginJSONBody("2.0.0", "neo4j-cypher-skill", "neo4j-gds-skill")
	fx.tarballBody = makeTarball(t, "2.0.0", "neo4j-cypher-skill", "neo4j-gds-skill")

	cat := fx.catalog()
	cat.Version = "1.0.0"

	require.NoError(t, cat.Refresh(context.Background(), fx.fs))
	assert.Equal(t, "2.0.0", cat.Version)

	for _, skill := range []string{"neo4j-cypher-skill", "neo4j-gds-skill"} {
		p := filepath.Join(fx.cacheRoot, "content", "2.0.0", skill, "SKILL.md")
		exists, err := afero.Exists(fx.fs, p)
		require.NoError(t, err)
		assert.True(t, exists, "expected %s after re-extract", p)
	}
	assert.Equal(t, 1, fx.tarballHits)
}

func TestRefresh_SendsUserAgentHeader(t *testing.T) {
	fx := newFixture(t)
	fx.pluginBody = pluginJSONBody("1.0.0", "neo4j-cypher-skill")
	fx.tarballBody = makeTarball(t, "1.0.0", "neo4j-cypher-skill")

	var pluginUA, tarballUA string
	mux := http.NewServeMux()
	mux.HandleFunc("/plugin.json", func(w http.ResponseWriter, r *http.Request) {
		pluginUA = r.Header.Get("User-Agent")
		_, _ = w.Write(fx.pluginBody)
	})
	mux.HandleFunc("/tarball", func(w http.ResponseWriter, r *http.Request) {
		tarballUA = r.Header.Get("User-Agent")
		_, _ = w.Write(fx.tarballBody)
	})
	fx.server.Close()
	fx.server = httptest.NewServer(mux)
	fx.doer.server = fx.server
	t.Cleanup(fx.server.Close)

	cat := fx.catalog()
	require.NoError(t, cat.Refresh(context.Background(), fx.fs))

	assert.Equal(t, "neo4j-cli/test-1.2.3", pluginUA)
	assert.Equal(t, "neo4j-cli/test-1.2.3", tarballUA)
}

func TestRefresh_DefaultsUserAgentToDev(t *testing.T) {
	assert.Equal(t, "neo4j-cli/dev", userAgentFor(""))
	assert.Equal(t, "neo4j-cli/1.2.3", userAgentFor("1.2.3"))
}

func TestRefresh_NetworkFailure_ReturnsError(t *testing.T) {
	fx := newFixture(t)
	// Empty pluginBody causes the handler to return 500.

	cat := fx.catalog()
	err := cat.Refresh(context.Background(), fx.fs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin.json")
}

func TestRefresh_BadPluginJSON_Errors(t *testing.T) {
	fx := newFixture(t)
	fx.pluginBody = []byte("not-json")

	cat := fx.catalog()
	err := cat.Refresh(context.Background(), fx.fs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse upstream plugin.json")
}

func TestRefresh_EmptyUpstreamVersion_Errors(t *testing.T) {
	fx := newFixture(t)
	fx.pluginBody = []byte(`{"name":"neo4j-skills","version":"","skills":[]}`)

	cat := fx.catalog()
	err := cat.Refresh(context.Background(), fx.fs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty version")
}

func TestRefresh_TarballHTTPError_Errors(t *testing.T) {
	fx := newFixture(t)
	fx.pluginBody = pluginJSONBody("1.0.0", "neo4j-cypher-skill")
	// tarballBody nil → handler returns 500.

	cat := fx.catalog()
	err := cat.Refresh(context.Background(), fx.fs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tarball")
}

func TestRefresh_NetworkFailure_LeavesPriorCacheIntact(t *testing.T) {
	fx := newFixture(t)

	// Seed prior cache: plugin.json + fetched-at + content/0.9.0/neo4j-cypher-skill/SKILL.md.
	priorPlugin := pluginJSONBody("0.9.0", "neo4j-cypher-skill")
	require.NoError(t, fx.fs.MkdirAll(fx.cacheRoot, 0755))
	require.NoError(t, afero.WriteFile(fx.fs, filepath.Join(fx.cacheRoot, "plugin.json"), priorPlugin, 0600))
	priorStamp := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	require.NoError(t, afero.WriteFile(fx.fs, filepath.Join(fx.cacheRoot, "fetched-at"), []byte(priorStamp), 0600))
	priorSkill := filepath.Join(fx.cacheRoot, "content", "0.9.0", "neo4j-cypher-skill", "SKILL.md")
	require.NoError(t, fx.fs.MkdirAll(filepath.Dir(priorSkill), 0755))
	require.NoError(t, afero.WriteFile(fx.fs, priorSkill, []byte("prior"), 0600))

	// Server returns 500 for plugin.json — refresh must fail without touching cache.
	cat := fx.catalog()
	cat.Version = "0.9.0"
	err := cat.Refresh(context.Background(), fx.fs)
	require.Error(t, err)

	// Prior cache files unchanged.
	got, rerr := afero.ReadFile(fx.fs, filepath.Join(fx.cacheRoot, "plugin.json"))
	require.NoError(t, rerr)
	assert.Equal(t, priorPlugin, got)
	gotStamp, rerr := afero.ReadFile(fx.fs, filepath.Join(fx.cacheRoot, "fetched-at"))
	require.NoError(t, rerr)
	assert.Equal(t, priorStamp, string(gotStamp))
	gotSkill, rerr := afero.ReadFile(fx.fs, priorSkill)
	require.NoError(t, rerr)
	assert.Equal(t, "prior", string(gotSkill))

	// Caller can still Load the prior catalog and Lookup the prior skill.
	loaded := New(Options{CacheRoot: fx.cacheRoot})
	require.NoError(t, loaded.Load(fx.fs))
	assert.Equal(t, "0.9.0", loaded.Version)
	_, sub, lerr := loaded.Lookup(fx.fs, "neo4j-cypher-skill", "neo4j-cli")
	require.NoError(t, lerr)
	data, rerr := fs.ReadFile(sub, "SKILL.md")
	require.NoError(t, rerr)
	assert.Equal(t, "prior", string(data))
}

func TestRefresh_TarballFailure_LeavesPriorPluginJSONIntact(t *testing.T) {
	fx := newFixture(t)
	fx.pluginBody = pluginJSONBody("1.0.0", "neo4j-cypher-skill")
	// tarballBody nil → 500 from tarball handler.

	priorPlugin := pluginJSONBody("0.9.0", "neo4j-cypher-skill")
	require.NoError(t, fx.fs.MkdirAll(fx.cacheRoot, 0755))
	require.NoError(t, afero.WriteFile(fx.fs, filepath.Join(fx.cacheRoot, "plugin.json"), priorPlugin, 0600))

	cat := fx.catalog()
	cat.Version = "0.9.0"
	err := cat.Refresh(context.Background(), fx.fs)
	require.Error(t, err)

	// plugin.json must not have been overwritten with the new upstream body.
	got, rerr := afero.ReadFile(fx.fs, filepath.Join(fx.cacheRoot, "plugin.json"))
	require.NoError(t, rerr)
	assert.Equal(t, priorPlugin, got, "plugin.json must stay at prior version after tarball failure")
}

func TestRefresh_NilGuards(t *testing.T) {
	var nilCat *Catalog
	require.Error(t, nilCat.Refresh(context.Background(), afero.NewMemMapFs()))

	c := New(Options{}) // empty cacheRoot
	require.Error(t, c.Refresh(context.Background(), afero.NewMemMapFs()))

	c2 := New(Options{CacheRoot: "x"})
	require.Error(t, c2.Refresh(context.Background(), nil))
}

func TestLookup_HappyPath(t *testing.T) {
	fx := newFixture(t)
	fx.pluginBody = pluginJSONBody("1.0.0", "neo4j-cypher-skill")
	fx.tarballBody = makeTarball(t, "1.0.0", "neo4j-cypher-skill")

	cat := fx.catalog()
	require.NoError(t, cat.Refresh(context.Background(), fx.fs))

	entry, sub, err := cat.Lookup(fx.fs, "neo4j-cypher-skill", "neo4j-cli")
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "neo4j-cypher-skill", entry.Name)

	data, err := fs.ReadFile(sub, "SKILL.md")
	require.NoError(t, err)
	assert.Equal(t, "# neo4j-cypher-skill\n", string(data))
}

func TestLookup_RejectsReservedNames(t *testing.T) {
	tests := []struct {
		name       string
		skillName  string
		binaryName string
	}{
		{"self canonical", "self", "neo4j-cli"},
		{"binary name alias", "neo4j-cli", "neo4j-cli"},
		{"custom binary name", "my-binary", "my-binary"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cat := New(Options{CacheRoot: "cache"})
			cat.Version = "1.0.0"
			cat.Skills = []SkillEntry{{Name: tc.skillName, Path: "./" + tc.skillName}}
			_, _, err := cat.Lookup(afero.NewMemMapFs(), tc.skillName, tc.binaryName)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "reserved")
		})
	}
}

func TestLookup_UnknownSkill_Errors(t *testing.T) {
	cat := New(Options{CacheRoot: "cache"})
	cat.Version = "1.0.0"
	cat.Skills = []SkillEntry{{Name: "neo4j-cypher-skill", Path: "./neo4j-cypher-skill"}}
	_, _, err := cat.Lookup(afero.NewMemMapFs(), "missing-skill", "neo4j-cli")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"missing-skill"`)
}

func TestLookup_MissingContent_Errors(t *testing.T) {
	cat := New(Options{CacheRoot: "cache"})
	cat.Version = "1.0.0"
	cat.Skills = []SkillEntry{{Name: "neo4j-cypher-skill", Path: "./neo4j-cypher-skill"}}
	_, _, err := cat.Lookup(afero.NewMemMapFs(), "neo4j-cypher-skill", "neo4j-cli")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestLookup_EmptyVersion_Errors(t *testing.T) {
	cat := New(Options{CacheRoot: "cache"})
	cat.Skills = []SkillEntry{{Name: "x", Path: "./x"}}
	_, _, err := cat.Lookup(afero.NewMemMapFs(), "x", "neo4j-cli")
	require.Error(t, err)
}

func TestLookup_NilAndEmptyGuards(t *testing.T) {
	var nilCat *Catalog
	_, _, err := nilCat.Lookup(afero.NewMemMapFs(), "x", "neo4j-cli")
	require.Error(t, err)

	cat := New(Options{CacheRoot: "cache"})
	cat.Version = "1.0.0"
	_, _, err = cat.Lookup(afero.NewMemMapFs(), "", "neo4j-cli")
	require.Error(t, err)
}

func TestReservedNames(t *testing.T) {
	assert.Equal(t, []string{"self", "neo4j-cli"}, ReservedNames("neo4j-cli"))
	assert.Equal(t, []string{"self"}, ReservedNames(""))
	assert.Equal(t, []string{"self"}, ReservedNames("self"), "binary called 'self' must dedupe")
}

func TestIsReserved(t *testing.T) {
	assert.True(t, IsReserved("self", "neo4j-cli"))
	assert.True(t, IsReserved("neo4j-cli", "neo4j-cli"))
	assert.False(t, IsReserved("neo4j-cypher-skill", "neo4j-cli"))
	assert.False(t, IsReserved("", "neo4j-cli"))
}

// TestRefresh_DropsMaliciousUpstreamSkillNames asserts that the
// extraction-allowlist path (Refresh -> skillEntriesFromPluginJSON)
// silently drops upstream skill entries whose derived name fails
// ValidSkillName. Without the filter a compromised upstream publishing
// `"skills": [".."]` would (a) widen the tarball allowlist to include ".."
// segments and (b) propagate through Lookup -> Install -> RemoveAll on
// the parent skillsRoot directory.
func TestRefresh_DropsMaliciousUpstreamSkillNames(t *testing.T) {
	fx := newFixture(t)
	// Inject path-traversal + hidden-name entries alongside one safe entry.
	fx.pluginBody = []byte(`{"name":"neo4j-skills","version":"1.0.0","skills":["..",".","/",  "foo/..","./..",".git","./safe-skill"]}`)
	// Tarball only carries the safe-skill payload — the extractor will only
	// see "safe-skill" in the allowlist after filtering.
	fx.tarballBody = makeTarball(t, "1.0.0", "safe-skill")

	cat := fx.catalog()
	require.NoError(t, cat.Refresh(context.Background(), fx.fs))

	require.Len(t, cat.Skills, 1, "malicious upstream skill names must be dropped")
	assert.Equal(t, "safe-skill", cat.Skills[0].Name)
}
