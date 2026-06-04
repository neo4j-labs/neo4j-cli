// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dataset

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startMediaServer wires mediaBaseURL, httpDoFn, the host allowlist, and the
// https guard at the httptest server so Download can be exercised over plain
// HTTP without touching the network. It returns a teardown func.
func startMediaServer(t *testing.T, h http.HandlerFunc) func() {
	t.Helper()
	srv := httptest.NewServer(h)

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	origBase, origDo, origHTTPS := mediaBaseURL, httpDoFn, requireHTTPS
	mediaBaseURL = srv.URL
	httpDoFn = srv.Client().Do
	requireHTTPS = false
	allowedDownloadHosts[u.Hostname()] = struct{}{}

	return func() {
		mediaBaseURL, httpDoFn, requireHTTPS = origBase, origDo, origHTTPS
		delete(allowedDownloadHosts, u.Hostname())
		srv.Close()
	}
}

func testSpec() Spec {
	return Spec{Owner: "neo4j-graph-examples", Repo: "movies", Branch: "main", DumpPath: "data/movies-50.dump"}
}

func TestDownload_StreamsToTempFile(t *testing.T) {
	want := []byte("not-an-lfs-pointer: these are dump bytes")
	defer startMediaServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/media/neo4j-graph-examples/movies/main/data/movies-50.dump", r.URL.Path)
		_, _ = w.Write(want)
	})()

	path, cleanup, err := Download(context.Background(), testSpec(), 0)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestDownload_DetectsLFSPointer(t *testing.T) {
	pointer := "version https://git-lfs.github.com/spec/v1\noid sha256:abc\nsize 123\n"
	defer startMediaServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(pointer))
	})()

	path, cleanup, err := Download(context.Background(), testSpec(), 0)
	require.Error(t, err)
	assert.Nil(t, cleanup)
	assert.Empty(t, path)
	assert.Contains(t, err.Error(), "Git-LFS pointer")
}

func TestDownload_EnforcesSizeCap(t *testing.T) {
	big := strings.Repeat("x", 5000)
	defer startMediaServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(big))
	})()

	path, cleanup, err := Download(context.Background(), testSpec(), 1000)
	require.Error(t, err)
	assert.Nil(t, cleanup)
	assert.Empty(t, path)
	assert.Contains(t, err.Error(), "exceeds max size")
}

func TestDownload_AtCapSucceeds(t *testing.T) {
	body := strings.Repeat("y", 1000)
	defer startMediaServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	})()

	path, cleanup, err := Download(context.Background(), testSpec(), 1000)
	require.NoError(t, err)
	defer cleanup()

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Len(t, got, 1000)
}

func TestDownload_NotFound(t *testing.T) {
	defer startMediaServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})()

	_, cleanup, err := Download(context.Background(), testSpec(), 0)
	require.Error(t, err)
	assert.Nil(t, cleanup)
	assert.Contains(t, err.Error(), "not found")
}

func TestDownload_RejectsNonAllowlistedHost(t *testing.T) {
	orig := mediaBaseURL
	mediaBaseURL = "https://evil.example.com"
	defer func() { mediaBaseURL = orig }()

	_, cleanup, err := Download(context.Background(), testSpec(), 0)
	require.Error(t, err)
	assert.Nil(t, cleanup)
	assert.Contains(t, err.Error(), "not in download allowlist")
}

func TestDownload_RejectsNonHTTPS(t *testing.T) {
	orig := mediaBaseURL
	mediaBaseURL = "http://media.githubusercontent.com"
	defer func() { mediaBaseURL = orig }()

	_, cleanup, err := Download(context.Background(), testSpec(), 0)
	require.Error(t, err)
	assert.Nil(t, cleanup)
	assert.Contains(t, err.Error(), "non-https")
}

func TestRedactURL_StripsQuery(t *testing.T) {
	assert.Equal(t, "https://media.githubusercontent.com/x", redactURL("https://media.githubusercontent.com/x?token=secret"))
}
