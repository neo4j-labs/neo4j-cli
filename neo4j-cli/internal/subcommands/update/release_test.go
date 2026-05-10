// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withApiBaseURL swaps the package-level apiBaseURL seam for the test's
// duration, restoring it via t.Cleanup.
func withApiBaseURL(t *testing.T, v string) {
	t.Helper()
	prev := apiBaseURL
	apiBaseURL = v
	t.Cleanup(func() { apiBaseURL = prev })
}

// withDlBaseURL swaps the package-level dlBaseURL seam. Used by asset-URL
// tests so the assertions stay readable (don't have to embed github.com).
func withDlBaseURL(t *testing.T, v string) {
	t.Helper()
	prev := dlBaseURL
	dlBaseURL = v
	t.Cleanup(func() { dlBaseURL = prev })
}

// withGoos swaps the goosFn seam.
func withGoos(t *testing.T, v string) {
	t.Helper()
	prev := goosFn
	goosFn = func() string { return v }
	t.Cleanup(func() { goosFn = prev })
}

// withGoarch swaps the goarchFn seam.
func withGoarch(t *testing.T, v string) {
	t.Helper()
	prev := goarchFn
	goarchFn = func() string { return v }
	t.Cleanup(func() { goarchFn = prev })
}

// releaseListServer spins up an httptest.NewServer that serves the canned
// release list at /repos/<slug>/releases. Happy-path tests respond
// immediately; the cancellation test uses a separate handler with the AGENTS
// "2s safety timeout + 5s test-side fallback" pattern.
func releaseListServer(t *testing.T, releases []Release, capture *http.Header) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			*capture = r.Header.Clone()
		}
		expectPath := "/repos/" + repoSlug + "/releases"
		if r.URL.Path != expectPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(releases)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLatest_StableOnly_FiltersPrereleases(t *testing.T) {
	releases := []Release{
		{TagName: "v0.1.0-alpha.9", Draft: false, Prerelease: true},
		{TagName: "v0.0.5", Draft: false, Prerelease: false},
		{TagName: "v0.0.6-beta.1", Draft: false, Prerelease: true},
		{TagName: "v0.0.4", Draft: false, Prerelease: false},
	}
	srv := releaseListServer(t, releases, nil)
	withApiBaseURL(t, srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := Latest(ctx, false)
	require.NoError(t, err)
	assert.Equal(t, "v0.0.5", got.TagName)
}

func TestLatest_Prereleases_AcceptsAll(t *testing.T) {
	releases := []Release{
		{TagName: "v0.1.0-alpha.9", Draft: false, Prerelease: true},
		{TagName: "v0.0.5", Draft: false, Prerelease: false},
	}
	srv := releaseListServer(t, releases, nil)
	withApiBaseURL(t, srv.URL)

	got, err := Latest(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, "v0.1.0-alpha.9", got.TagName)
}

func TestLatest_StableOnly_AllPrereleases_ReturnsErrNoStableRelease(t *testing.T) {
	releases := []Release{
		{TagName: "v0.1.0-alpha.9", Draft: false, Prerelease: true},
		{TagName: "v0.1.0-beta.1", Draft: false, Prerelease: true},
	}
	srv := releaseListServer(t, releases, nil)
	withApiBaseURL(t, srv.URL)

	_, err := Latest(context.Background(), false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoStableRelease))
}

func TestLatest_SkipsDrafts(t *testing.T) {
	releases := []Release{
		{TagName: "v0.0.7", Draft: true, Prerelease: false},
		{TagName: "v0.0.5", Draft: false, Prerelease: false},
	}
	srv := releaseListServer(t, releases, nil)
	withApiBaseURL(t, srv.URL)

	got, err := Latest(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, "v0.0.5", got.TagName)
}

func TestLatest_SkipsNonSemverTags(t *testing.T) {
	releases := []Release{
		{TagName: "nightly", Draft: false, Prerelease: true},
		{TagName: "v0.0.5", Draft: false, Prerelease: false},
	}
	srv := releaseListServer(t, releases, nil)
	withApiBaseURL(t, srv.URL)

	got, err := Latest(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, "v0.0.5", got.TagName)
}

func TestFetchReleases_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer srv.Close()
	withApiBaseURL(t, srv.URL)

	_, err := Latest(context.Background(), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate-limited")
	assert.Contains(t, err.Error(), "GH_TOKEN")
}

func TestFetchReleases_AuthHeaderSet_FromGHToken(t *testing.T) {
	var captured http.Header
	srv := releaseListServer(t, []Release{{TagName: "v0.0.5"}}, &captured)
	withApiBaseURL(t, srv.URL)

	t.Setenv("GH_TOKEN", "ghs_test_token_value")
	t.Setenv("GITHUB_TOKEN", "")

	_, err := Latest(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, "Bearer ghs_test_token_value", captured.Get("Authorization"))
}

func TestFetchReleases_AuthHeaderSet_FromGitHubToken(t *testing.T) {
	var captured http.Header
	srv := releaseListServer(t, []Release{{TagName: "v0.0.5"}}, &captured)
	withApiBaseURL(t, srv.URL)

	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "github_token_fallback")

	_, err := Latest(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, "Bearer github_token_fallback", captured.Get("Authorization"))
}

func TestFetchReleases_PrefersGHTokenOverGitHubToken(t *testing.T) {
	var captured http.Header
	srv := releaseListServer(t, []Release{{TagName: "v0.0.5"}}, &captured)
	withApiBaseURL(t, srv.URL)

	t.Setenv("GH_TOKEN", "preferred_value")
	t.Setenv("GITHUB_TOKEN", "fallback_value")

	_, err := Latest(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, "Bearer preferred_value", captured.Get("Authorization"))
}

func TestFetchReleases_NoTokenWhenEnvUnset(t *testing.T) {
	var captured http.Header
	srv := releaseListServer(t, []Release{{TagName: "v0.0.5"}}, &captured)
	withApiBaseURL(t, srv.URL)

	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	_, err := Latest(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, "", captured.Get("Authorization"))
}

// TestFetchReleases_TokenNeverInError asserts that even when the request
// fails with a token configured, the secret value never leaks into the
// returned error string. The token is set to a recognizable sentinel and the
// test fails if the sentinel appears anywhere in the error chain.
func TestFetchReleases_TokenNeverInError(t *testing.T) {
	const sentinel = "ghp_VERY_SECRET_TOKEN_VALUE_42"

	cases := []struct {
		name   string
		status int
	}{
		{"forbidden", http.StatusForbidden},
		{"too-many-requests", http.StatusTooManyRequests},
		{"server-error", http.StatusInternalServerError},
		{"bad-request", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			withApiBaseURL(t, srv.URL)
			t.Setenv("GH_TOKEN", sentinel)

			_, err := Latest(context.Background(), true)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), sentinel)
		})
	}
}

func TestFetchReleases_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()
	withApiBaseURL(t, srv.URL)

	_, err := Latest(context.Background(), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestGetByTag_Hit(t *testing.T) {
	releases := []Release{
		{TagName: "v0.1.0-alpha.9", Draft: false, Prerelease: true},
		{TagName: "v0.0.5", Draft: false, Prerelease: false},
	}
	srv := releaseListServer(t, releases, nil)
	withApiBaseURL(t, srv.URL)

	got, err := GetByTag(context.Background(), "v0.0.5")
	require.NoError(t, err)
	assert.Equal(t, "v0.0.5", got.TagName)
}

func TestGetByTag_NotFound(t *testing.T) {
	releases := []Release{
		{TagName: "v0.0.5", Draft: false},
	}
	srv := releaseListServer(t, releases, nil)
	withApiBaseURL(t, srv.URL)

	_, err := GetByTag(context.Background(), "v9.9.9")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTagNotFound))
}

func TestGetByTag_SkipsDrafts(t *testing.T) {
	releases := []Release{
		{TagName: "v0.0.5", Draft: true},
	}
	srv := releaseListServer(t, releases, nil)
	withApiBaseURL(t, srv.URL)

	_, err := GetByTag(context.Background(), "v0.0.5")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTagNotFound))
}

func TestBuildAssetURLs_TableDriven(t *testing.T) {
	withDlBaseURL(t, "https://example.test")

	cases := []struct {
		name         string
		goos         string
		goarch       string
		tag          string
		wantArchive  string
		wantChecksum string
	}{
		{
			name:         "linux/amd64",
			goos:         "linux",
			goarch:       "amd64",
			tag:          "v0.1.0",
			wantArchive:  "https://example.test/neo4j-labs/neo4j-cli/releases/download/v0.1.0/neo4j-cli_0.1.0_Linux_x86_64.tar.gz",
			wantChecksum: "https://example.test/neo4j-labs/neo4j-cli/releases/download/v0.1.0/neo4j-cli_0.1.0_checksums.txt",
		},
		{
			name:         "darwin/arm64",
			goos:         "darwin",
			goarch:       "arm64",
			tag:          "v0.2.3",
			wantArchive:  "https://example.test/neo4j-labs/neo4j-cli/releases/download/v0.2.3/neo4j-cli_0.2.3_Darwin_arm64.tar.gz",
			wantChecksum: "https://example.test/neo4j-labs/neo4j-cli/releases/download/v0.2.3/neo4j-cli_0.2.3_checksums.txt",
		},
		{
			name:         "windows/amd64",
			goos:         "windows",
			goarch:       "amd64",
			tag:          "v1.0.0-rc.1",
			wantArchive:  "https://example.test/neo4j-labs/neo4j-cli/releases/download/v1.0.0-rc.1/neo4j-cli_1.0.0-rc.1_Windows_x86_64.zip",
			wantChecksum: "https://example.test/neo4j-labs/neo4j-cli/releases/download/v1.0.0-rc.1/neo4j-cli_1.0.0-rc.1_checksums.txt",
		},
		{
			name:         "linux/386",
			goos:         "linux",
			goarch:       "386",
			tag:          "v0.0.1",
			wantArchive:  "https://example.test/neo4j-labs/neo4j-cli/releases/download/v0.0.1/neo4j-cli_0.0.1_Linux_i386.tar.gz",
			wantChecksum: "https://example.test/neo4j-labs/neo4j-cli/releases/download/v0.0.1/neo4j-cli_0.0.1_checksums.txt",
		},
		{
			name:         "linux/arm64",
			goos:         "linux",
			goarch:       "arm64",
			tag:          "v2.0.0",
			wantArchive:  "https://example.test/neo4j-labs/neo4j-cli/releases/download/v2.0.0/neo4j-cli_2.0.0_Linux_arm64.tar.gz",
			wantChecksum: "https://example.test/neo4j-labs/neo4j-cli/releases/download/v2.0.0/neo4j-cli_2.0.0_checksums.txt",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withGoos(t, tc.goos)
			withGoarch(t, tc.goarch)

			got, err := BuildAssetURLs(tc.tag)
			require.NoError(t, err)
			assert.Equal(t, tc.wantArchive, got.Archive)
			assert.Equal(t, tc.wantChecksum, got.Checksum)
		})
	}
}

func TestBuildAssetURLs_UnsupportedOS(t *testing.T) {
	withGoos(t, "plan9")
	withGoarch(t, "amd64")

	_, err := BuildAssetURLs("v0.1.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan9")
}

func TestBuildAssetURLs_UnsupportedArch(t *testing.T) {
	withGoos(t, "linux")
	withGoarch(t, "riscv64")

	_, err := BuildAssetURLs("v0.1.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "riscv64")
}

func TestBuildAssetURLs_InvalidTag(t *testing.T) {
	withGoos(t, "linux")
	withGoarch(t, "amd64")

	_, err := BuildAssetURLs("not-a-version")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestValidateVersionTag(t *testing.T) {
	cases := []struct {
		name    string
		tag     string
		wantErr bool
	}{
		{name: "valid stable", tag: "v0.1.0", wantErr: false},
		{name: "valid prerelease", tag: "v0.1.0-alpha.9", wantErr: false},
		{name: "valid rc", tag: "v1.0.0-rc.1", wantErr: false},

		{name: "empty", tag: "", wantErr: true},
		{name: "no leading v", tag: "0.1.0", wantErr: true},
		{name: "garbage", tag: "definitely-not-semver", wantErr: true},

		{name: "traversal dot-dot", tag: "v0.1.0/..", wantErr: true},
		{name: "forward slash", tag: "v0.1.0/etc", wantErr: true},
		{name: "back slash", tag: "v0.1.0\\foo", wantErr: true},
		{name: "nul byte", tag: "v0.1.0\x00", wantErr: true},
		{name: "shell semicolon", tag: "v0.1.0;rm", wantErr: true},
		{name: "shell ampersand", tag: "v0.1.0&", wantErr: true},
		{name: "shell pipe", tag: "v0.1.0|", wantErr: true},
		{name: "shell dollar", tag: "v0.1.0$x", wantErr: true},
		{name: "shell backtick", tag: "v0.1.0`x`", wantErr: true},
		{name: "shell glob star", tag: "v0.1.0*", wantErr: true},
		{name: "shell glob ?", tag: "v0.1.0?", wantErr: true},
		{name: "shell redirect <", tag: "v0.1.0<x", wantErr: true},
		{name: "shell redirect >", tag: "v0.1.0>x", wantErr: true},
		{name: "shell paren (", tag: "v0.1.0(", wantErr: true},
		{name: "shell paren )", tag: "v0.1.0)", wantErr: true},
		{name: "space", tag: "v0.1 .0", wantErr: true},
		{name: "tab", tag: "v0.1\t0", wantErr: true},
		{name: "newline", tag: "v0.1.0\n", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateVersionTag(tc.tag)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestLatest_Cancellation makes sure context cancellation propagates and the
// call returns rather than hanging on a slow server. The handler blocks on
// ctx.Done() with a fallback so a propagation miss is loud.
func TestLatest_Cancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2 * time.Second):
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	withApiBaseURL(t, srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already-cancelled

	errCh := make(chan error, 1)
	go func() {
		_, err := Latest(ctx, true)
		errCh <- err
	}()

	select {
	case err := <-errCh:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Latest did not return after context cancellation")
	}
}

// TestLatest_HostInURLNotInError ensures the API URL host (where the token
// is sent as a Bearer header) does not accidentally appear with the token in
// any error message — it's belt-and-suspenders alongside the explicit
// sentinel-not-leaked check above.
func TestLatest_ErrorDoesNotContainAuthSubstring(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	withApiBaseURL(t, srv.URL)
	t.Setenv("GH_TOKEN", "ghs_x_secret")

	_, err := Latest(context.Background(), true)
	require.Error(t, err)
	assert.False(t, strings.Contains(err.Error(), "Authorization:"), "error should not include Authorization header literal: %v", err)
	assert.False(t, strings.Contains(err.Error(), "Bearer "), "error should not echo Bearer prefix: %v", err)
}

// Sanity: ensure the package-level seams are wired up correctly (the symbols
// are usable and callable). Exists primarily so the package compiles cleanly
// even if someone deletes a test that touches a seam.
func TestSeams_Smoke(t *testing.T) {
	require.NotEmpty(t, apiBaseURL)
	require.NotEmpty(t, dlBaseURL)
	require.NotEmpty(t, goosFn())
	require.NotEmpty(t, goarchFn())
	require.NotNil(t, httpDoFn)
	// Basic invocation: build a request and confirm httpDoFn returns
	// without panicking. Use a closed httptest server URL to force an
	// error and avoid hitting real GitHub.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	_, _ = httpDoFn(req)
	_ = fmt.Sprintf("%T", apiBaseURL) // touch all seam vars
}
