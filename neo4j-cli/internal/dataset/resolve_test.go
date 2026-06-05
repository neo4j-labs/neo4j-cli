// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dataset

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const moviesManifest = `{
  "name": "movies",
  "dbms": [
    {"dumpFile": "data/movies-35.dump", "targetNeo4jVersion": ">=3.5.0 <4.0.0"},
    {"dumpFile": "data/movies-43.dump", "targetNeo4jVersion": ">=4.3.0 <5.0.0"},
    {"dumpFile": "data/movies-50.dump", "targetNeo4jVersion": ">=5.0.0 <6.0.0", "plugins": ["apoc"]},
    {"scriptFile": "scripts/movies.cypher", "targetNeo4jVersion": ">=4.0.0"}
  ]
}`

// fakeServer maps "<branch>/<path>" -> body for GET, and tracks which paths
// respond 200 to HEAD (dump-existence probe). Missing GET paths 404.
type fakeServer struct {
	manifests map[string]string // branch -> manifest body ("" => 404)
	dumps     map[string]bool   // "<branch>/<dumpPath>" -> exists
}

func (f *fakeServer) start(t *testing.T) func() {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Path shape: /owner/repo/<branch>/<rest...>
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 4)
		if len(parts) < 4 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		branch, rest := parts[2], parts[3]
		if rest == "relate.project-install.json" {
			body, ok := f.manifests[branch]
			if !ok || body == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(body))
			return
		}
		if r.Method == http.MethodHead {
			if f.dumps[branch+"/"+rest] {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))

	origBase, origDo := rawBaseURL, httpDoFn
	rawBaseURL = srv.URL
	httpDoFn = srv.Client().Do
	return func() {
		rawBaseURL, httpDoFn = origBase, origDo
		srv.Close()
	}
}

func TestResolve_SelectsMatchingEntry(t *testing.T) {
	f := &fakeServer{
		manifests: map[string]string{"main": moviesManifest},
		dumps:     map[string]bool{"main/data/movies-50.dump": true},
	}
	defer f.start(t)()

	spec, err := Resolve(context.Background(), "neo4j-graph-examples/movies", "5.13.0")
	require.NoError(t, err)
	assert.Equal(t, "neo4j-graph-examples", spec.Owner)
	assert.Equal(t, "movies", spec.Repo)
	assert.Equal(t, "main", spec.Branch)
	assert.Equal(t, "data/movies-50.dump", spec.DumpPath)
	assert.Equal(t, []string{"apoc"}, spec.Plugins)
	assert.Equal(t, ">=5.0.0 <6.0.0", spec.MatchedVersionRange)
}

func TestResolve_ConcreteAndCalverTargets(t *testing.T) {
	// Manifest with 3.5 / 4.3 / 5.0 dump entries, plus a script entry.
	for _, tc := range []struct {
		name     string
		target   string
		wantDump string
	}{
		{name: "5 picks 5.x", target: "5", wantDump: "data/movies-50.dump"},
		{name: "5.26 picks 5.x", target: "5.26", wantDump: "data/movies-50.dump"},
		{name: "5.26.0 picks 5.x", target: "5.26.0", wantDump: "data/movies-50.dump"},
		{name: "4.4 picks 4.3 entry", target: "4.4", wantDump: "data/movies-43.dump"},
		{name: "3.5.1 picks 3.5 entry", target: "3.5.1", wantDump: "data/movies-35.dump"},
		{name: "calver 2025.01.0 picks 5.x", target: "2025.01.0", wantDump: "data/movies-50.dump"},
		{name: "calver 2026.05.0 picks 5.x", target: "2026.05.0", wantDump: "data/movies-50.dump"},
		{name: "latest picks 5.x", target: "latest", wantDump: "data/movies-50.dump"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeServer{
				manifests: map[string]string{"main": moviesManifest},
				dumps:     map[string]bool{"main/" + tc.wantDump: true},
			}
			defer f.start(t)()

			spec, err := Resolve(context.Background(), "neo4j-graph-examples/movies", tc.target)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDump, spec.DumpPath)
		})
	}
}

func TestResolve_CalverPrefersExplicitCalverEntry(t *testing.T) {
	mixed := `{"dbms": [
		{"dumpFile": "data/v5.dump", "targetNeo4jVersion": ">=5.0.0 <6.0.0"},
		{"dumpFile": "data/calver.dump", "targetNeo4jVersion": ">=2025.0.0"}
	]}`
	for _, tc := range []struct {
		name     string
		target   string
		wantDump string
	}{
		{name: "calver prefers newest lower bound", target: "2026.05.0", wantDump: "data/calver.dump"},
		{name: "latest prefers newest lower bound", target: "latest", wantDump: "data/calver.dump"},
		{name: "5.26 still picks 5.x entry", target: "5.26", wantDump: "data/v5.dump"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeServer{
				manifests: map[string]string{"main": mixed},
				dumps:     map[string]bool{"main/" + tc.wantDump: true},
			}
			defer f.start(t)()

			spec, err := Resolve(context.Background(), "owner/repo", tc.target)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDump, spec.DumpPath)
		})
	}
}

func TestResolve_CalverNoCompatibleWhenOnlyPre5(t *testing.T) {
	pre5 := `{"dbms": [
		{"dumpFile": "data/v4.dump", "targetNeo4jVersion": ">=4.0.0 <5.0.0"}
	]}`
	for _, target := range []string{"2026.05.0", "latest"} {
		t.Run(target, func(t *testing.T) {
			f := &fakeServer{manifests: map[string]string{"main": pre5}}
			defer f.start(t)()

			_, err := Resolve(context.Background(), "owner/repo", target)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no dump entry compatible")
		})
	}
}

func TestResolve_FallsBackToMaster(t *testing.T) {
	f := &fakeServer{
		manifests: map[string]string{"master": moviesManifest},
		dumps:     map[string]bool{"master/data/movies-43.dump": true},
	}
	defer f.start(t)()

	spec, err := Resolve(context.Background(), "neo4j-graph-examples/movies", "4.4.0")
	require.NoError(t, err)
	assert.Equal(t, "master", spec.Branch)
	assert.Equal(t, "data/movies-43.dump", spec.DumpPath)
}

func TestResolve_NewestCompatibleOnOverlap(t *testing.T) {
	overlap := `{"dbms": [
		{"dumpFile": "data/a.dump", "targetNeo4jVersion": ">=4.0.0"},
		{"dumpFile": "data/b.dump", "targetNeo4jVersion": ">=5.0.0"}
	]}`
	f := &fakeServer{
		manifests: map[string]string{"main": overlap},
		dumps:     map[string]bool{"main/data/b.dump": true},
	}
	defer f.start(t)()

	spec, err := Resolve(context.Background(), "owner/repo", "5.5.0")
	require.NoError(t, err)
	assert.Equal(t, "data/b.dump", spec.DumpPath, "newest compatible lower bound wins")
}

func TestResolve_NoCompatibleEntry(t *testing.T) {
	f := &fakeServer{manifests: map[string]string{"main": moviesManifest}}
	defer f.start(t)()

	_, err := Resolve(context.Background(), "neo4j-graph-examples/movies", "6.1.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no dump entry compatible")
}

func TestResolve_MissingDumpPath(t *testing.T) {
	f := &fakeServer{
		manifests: map[string]string{"main": moviesManifest},
		dumps:     map[string]bool{}, // HEAD 404 for everything
	}
	defer f.start(t)()

	_, err := Resolve(context.Background(), "neo4j-graph-examples/movies", "5.13.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestResolve_ManifestNotFoundOnEitherBranch(t *testing.T) {
	f := &fakeServer{manifests: map[string]string{}}
	defer f.start(t)()

	_, err := Resolve(context.Background(), "owner/repo", "5.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no relate.project-install.json found")
}

func TestResolve_RejectsBadSlug(t *testing.T) {
	_, err := Resolve(context.Background(), "noslash", "5.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "owner/repo")
}

func TestResolve_RejectsBadVersion(t *testing.T) {
	_, err := Resolve(context.Background(), "owner/repo", "not-a-version")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid neo4j version")
}
