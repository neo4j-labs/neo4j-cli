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

// startHosts wires BOTH the raw and the media hosts at a single httptest server
// (raw requests under /<owner>/..., media under /media/<owner>/...) so Download
// can be exercised over plain HTTP without touching the network. The handler
// receives the full request and routes on r.URL.Path. It returns a teardown
// func. rawBaseURL and mediaBaseURL both point at the server; the host is added
// to the allowlist and the https guard is disabled for the duration.
func startHosts(t *testing.T, h http.HandlerFunc) func() {
	t.Helper()
	srv := httptest.NewServer(h)

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	origRaw, origMedia, origDo, origHTTPS := rawBaseURL, mediaBaseURL, httpDoFn, requireHTTPS
	rawBaseURL = srv.URL
	mediaBaseURL = srv.URL
	httpDoFn = srv.Client().Do
	requireHTTPS = false
	allowedDownloadHosts[u.Hostname()] = struct{}{}

	return func() {
		rawBaseURL, mediaBaseURL, httpDoFn, requireHTTPS = origRaw, origMedia, origDo, origHTTPS
		delete(allowedDownloadHosts, u.Hostname())
		srv.Close()
	}
}

func testSpec() Spec {
	return Spec{Owner: "neo4j-graph-examples", Repo: "movies", Branch: "main", DumpPath: "data/movies-50.dump"}
}

const (
	rawPath   = "/neo4j-graph-examples/movies/main/data/movies-50.dump"
	mediaPath = "/media/neo4j-graph-examples/movies/main/data/movies-50.dump"
)

func lfsPointer() string {
	return "version https://git-lfs.github.com/spec/v1\noid sha256:abc\nsize 123\n"
}

// TestDownload_RawBlobStreamedDirectly covers the common case: raw serves the
// real dump bytes (regular blob), so media is never hit.
func TestDownload_RawBlobStreamedDirectly(t *testing.T) {
	want := []byte("not-an-lfs-pointer: these are dump bytes")
	mediaHit := false
	defer startHosts(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case rawPath:
			_, _ = w.Write(want)
		case mediaPath:
			mediaHit = true
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})()

	path, cleanup, err := Download(context.Background(), testSpec(), 0)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	assert.False(t, mediaHit, "media host must not be hit for a regular blob")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// TestDownload_PointerFallsBackToMedia covers the LFS case: raw serves a
// pointer, so the real bytes are streamed from the media host.
func TestDownload_PointerFallsBackToMedia(t *testing.T) {
	want := []byte("real dump bytes from the media host")
	defer startHosts(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case rawPath:
			_, _ = w.Write([]byte(lfsPointer()))
		case mediaPath:
			_, _ = w.Write(want)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})()

	path, cleanup, err := Download(context.Background(), testSpec(), 0)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestDownload_PointerThenMediaMissing covers raw pointer + media 404: clear
// error, cleanup nil.
func TestDownload_PointerThenMediaMissing(t *testing.T) {
	defer startHosts(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case rawPath:
			_, _ = w.Write([]byte(lfsPointer()))
		case mediaPath:
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})()

	path, cleanup, err := Download(context.Background(), testSpec(), 0)
	require.Error(t, err)
	assert.Nil(t, cleanup)
	assert.Empty(t, path)
	assert.Contains(t, err.Error(), "not found")
}

// TestDownload_PointerThenMediaPointer covers raw pointer + media also a
// pointer: clear error, cleanup nil.
func TestDownload_PointerThenMediaPointer(t *testing.T) {
	defer startHosts(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(lfsPointer()))
	})()

	path, cleanup, err := Download(context.Background(), testSpec(), 0)
	require.Error(t, err)
	assert.Nil(t, cleanup)
	assert.Empty(t, path)
	assert.Contains(t, err.Error(), "Git-LFS pointer")
}

func TestDownload_EnforcesSizeCap(t *testing.T) {
	big := strings.Repeat("x", 5000)
	defer startHosts(t, func(w http.ResponseWriter, r *http.Request) {
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
	defer startHosts(t, func(w http.ResponseWriter, r *http.Request) {
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
	defer startHosts(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})()

	_, cleanup, err := Download(context.Background(), testSpec(), 0)
	require.Error(t, err)
	assert.Nil(t, cleanup)
	assert.Contains(t, err.Error(), "not found")
}

func TestDownload_RejectsNonAllowlistedHost(t *testing.T) {
	orig := rawBaseURL
	rawBaseURL = "https://evil.example.com"
	defer func() { rawBaseURL = orig }()

	_, cleanup, err := Download(context.Background(), testSpec(), 0)
	require.Error(t, err)
	assert.Nil(t, cleanup)
	assert.Contains(t, err.Error(), "not in download allowlist")
}

func TestDownload_RejectsNonHTTPS(t *testing.T) {
	orig := rawBaseURL
	rawBaseURL = "http://raw.githubusercontent.com"
	defer func() { rawBaseURL = orig }()

	_, cleanup, err := Download(context.Background(), testSpec(), 0)
	require.Error(t, err)
	assert.Nil(t, cleanup)
	assert.Contains(t, err.Error(), "non-https")
}

func TestRedactURL_StripsQuery(t *testing.T) {
	assert.Equal(t, "https://media.githubusercontent.com/x", redactURL("https://media.githubusercontent.com/x?token=secret"))
}
