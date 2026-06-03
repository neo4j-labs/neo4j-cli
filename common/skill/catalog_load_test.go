// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/skill"
	"github.com/neo4j/cli/common/skill/catalog"
)

// catalogServer wires an httptest.Server that serves a plugin.json and
// tarball over routes matching the production catalog URLs. Hits to
// each route are counted so tests can assert refresh vs cache behaviour.
type catalogServer struct {
	server      *httptest.Server
	pluginBody  []byte
	tarballBody []byte
	pluginHits  int
	tarballHits int
	failPlugin  bool
	failTarball bool
}

func newCatalogServer(t *testing.T) *catalogServer {
	t.Helper()
	cs := &catalogServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/plugin.json", func(w http.ResponseWriter, _ *http.Request) {
		cs.pluginHits++
		if cs.failPlugin || cs.pluginBody == nil {
			http.Error(w, "no body", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(cs.pluginBody)
	})
	mux.HandleFunc("/tarball", func(w http.ResponseWriter, _ *http.Request) {
		cs.tarballHits++
		if cs.failTarball || cs.tarballBody == nil {
			http.Error(w, "no body", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(cs.tarballBody)
	})
	cs.server = httptest.NewServer(mux)
	t.Cleanup(cs.server.Close)
	return cs
}

// catalogRouter is an http.Client-style doer that maps absolute upstream
// URLs to httptest paths so production constants stay frozen.
type catalogRouter struct {
	server *httptest.Server
	routes map[string]string
}

func (r *catalogRouter) Do(req *http.Request) (*http.Response, error) {
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

func (cs *catalogServer) doer() catalog.HTTPDoer {
	return &catalogRouter{
		server: cs.server,
		routes: map[string]string{
			catalog.PluginJSONURL: "/plugin.json",
			catalog.TarballURL:    "/tarball",
		},
	}
}

// makeCatalogTarball builds a gzipped tarball with the conventional
// top-level archive dir + a SKILL.md per named skill.
func makeCatalogTarball(t *testing.T, version string, skills ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	top := "neo4j-skills-" + version + "/"
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: top, Mode: 0o755, Typeflag: tar.TypeDir}))
	for _, s := range skills {
		body := "---\nname: " + s + "\nversion: " + version + "\n---\n# " + s + "\nbody\n"
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

// pluginJSONBody renders a plugin.json with the given version and skill
// paths.
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

// installCatalogCacheRoot is the path the catalog test seam points at —
// kept stable across tests so seeding utilities can compute the same
// content paths the helper writes to.
const installCatalogCacheRoot = "/cache/neo4j-cli/skill-catalog"

// withCatalogSeams installs package-level catalog test seams pointing
// at the given httptest server and at a fixed cache root inside the
// in-memory afero.Fs. Restored on test cleanup.
func withCatalogSeams(t *testing.T, doer catalog.HTTPDoer) {
	t.Helper()
	skill.SetCatalogCacheRootForTest(t, func() (string, error) { return installCatalogCacheRoot, nil })
	skill.SetCatalogHTTPDoerForTest(t, func() catalog.HTTPDoer { return doer })
}

// seedCatalogCache pre-populates the cache with a plugin.json + a tarball-
// extracted content tree so tests for warm-cache / cold-cache branches can
// distinguish "we hit the network" from "we read the cache".
func seedCatalogCache(t *testing.T, fs afero.Fs, version string, skills ...string) {
	t.Helper()
	require.NoError(t, fs.MkdirAll(installCatalogCacheRoot, 0755))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(installCatalogCacheRoot, "plugin.json"),
		pluginJSONBody(version, skills...), 0600))
	// fetched-at: use a known fresh timestamp so default Stale() = false.
	require.NoError(t, afero.WriteFile(fs, filepath.Join(installCatalogCacheRoot, "fetched-at"),
		[]byte("2999-01-01T00:00:00Z"), 0600))
	contentRoot := filepath.Join(installCatalogCacheRoot, "content", version)
	for _, s := range skills {
		dir := filepath.Join(contentRoot, s)
		require.NoError(t, fs.MkdirAll(dir, 0755))
		body := "---\nname: " + s + "\nversion: " + version + "\n---\n# cached " + s + "\n"
		require.NoError(t, afero.WriteFile(fs, filepath.Join(dir, "SKILL.md"), []byte(body), 0600))
	}
}
