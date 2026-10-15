// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withMetadataBaseURL swaps the package-level metadataBaseURL seam for the
// test's duration, restoring it via t.Cleanup. It also flips requireHTTPS off
// and adds the URL's host to allowedDownloadHosts so the REQ-S-001 host pins
// in downloadCapped (pre-flight AND final-URL, both now on the fetchReleases
// path) let the loopback request through. The dedicated
// TestFetchReleases_RejectsRedirectAwayFromAllowlist test asserts the gate
// still fires when a redirect goes off-allowlist (it sets requireHTTPS back
// to true via withRequireHTTPS), and TestFetchReleases_PreFlightHostCheck
// asserts the pre-flight gate rejects a bad metadataBaseURL before any HTTP.
func withMetadataBaseURL(t *testing.T, v string) {
	t.Helper()
	prev := metadataBaseURL
	metadataBaseURL = v
	t.Cleanup(func() { metadataBaseURL = prev })
	withRequireHTTPS(t, false)
	if u, err := url.Parse(v); err == nil && u.Hostname() != "" {
		withAllowedHost(t, u.Hostname())
	}
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

// releaseMetadataServer spins up an httptest.NewServer that serves the given
// releases wrapped in the releases.json envelope (schema_version 1) at
// /releases.json. Happy-path tests respond immediately; the cancellation test
// uses a separate handler with the AGENTS "2s safety timeout + 5s test-side
// fallback" pattern. Callers must follow up with withMetadataBaseURL(t,
// srv.URL) which also wires the requireHTTPS / allowedDownloadHosts seams for
// the REQ-S-001 final-URL host pin.
func releaseMetadataServer(t *testing.T, releases []Release, capture *http.Header) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			*capture = r.Header.Clone()
		}
		if r.URL.Path != "/releases.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		meta := releasesMetadata{
			SchemaVersion: metadataSchemaVersion,
			GeneratedAt:   "2026-10-15T12:00:00Z",
			Repository:    repoSlug,
			Releases:      &releases,
		}
		_ = json.NewEncoder(w).Encode(meta)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// releaseMetadataRawServer serves a raw status + body at /releases.json for
// the malformed-JSON / schema-version / missing-key / body-cap error tests.
func releaseMetadataRawServer(t *testing.T, status int, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLatest_StableOnly_FiltersPrereleases(t *testing.T) {
	releases := []Release{
		{TagName: "v0.1.0-alpha.9"},
		{TagName: "v0.0.5"},
		{TagName: "v0.0.6-beta.1"},
		{TagName: "v0.0.4"},
	}
	srv := releaseMetadataServer(t, releases, nil)
	withMetadataBaseURL(t, srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := Latest(ctx, false)
	require.NoError(t, err)
	assert.Equal(t, "v0.0.5", got.TagName)
}

func TestLatest_Prereleases_AcceptsAll(t *testing.T) {
	releases := []Release{
		{TagName: "v0.0.5"},
		{TagName: "v0.1.0-alpha.9"},
	}
	srv := releaseMetadataServer(t, releases, nil)
	withMetadataBaseURL(t, srv.URL)

	got, err := Latest(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, "v0.1.0-alpha.9", got.TagName)
}

func TestLatest_StableOnly_AllPrereleases_ReturnsErrNoStableRelease(t *testing.T) {
	releases := []Release{
		{TagName: "v0.1.0-alpha.9"},
		{TagName: "v0.1.0-beta.1"},
	}
	srv := releaseMetadataServer(t, releases, nil)
	withMetadataBaseURL(t, srv.URL)

	_, err := Latest(context.Background(), false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoStableRelease))
}

func TestLatest_SkipsNonSemverTags(t *testing.T) {
	releases := []Release{
		{TagName: "nightly"},
		{TagName: "v0.0.5"},
	}
	srv := releaseMetadataServer(t, releases, nil)
	withMetadataBaseURL(t, srv.URL)

	got, err := Latest(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, "v0.0.5", got.TagName)
}

// TestFetchReleases_SortsSemverDescending pins the contract rule that file
// order is NEVER trusted: a shuffled releases.json must come back sorted by
// semver descending, with non-semver tags (which the generator includes and
// the CLI filters) sunk below every valid tag.
func TestFetchReleases_SortsSemverDescending(t *testing.T) {
	releases := []Release{
		{TagName: "v0.0.5"},
		{TagName: "nightly"},
		{TagName: "v0.2.0"},
		{TagName: "v0.10.0"},
		{TagName: "v0.2.0-alpha.1"},
		{TagName: "v0.1.0"},
	}
	srv := releaseMetadataServer(t, releases, nil)
	withMetadataBaseURL(t, srv.URL)

	got, err := fetchReleases(context.Background())
	require.NoError(t, err)
	tags := make([]string, 0, len(got))
	for _, r := range got {
		tags = append(tags, r.TagName)
	}
	assert.Equal(t, []string{"v0.10.0", "v0.2.0", "v0.2.0-alpha.1", "v0.1.0", "v0.0.5", "nightly"}, tags)
}

// TestFetchReleases_NonOKStatus pins the post-CLI-244 failure mode: ANY
// non-200 (including the old 403/429 rate-limit pair and a 404 for a missing
// releases.json) errors clearly with the status and NO credential hint —
// there is no token to configure any more. The error text is downloadCapped's
// unified "status <n> for <redacted url>" convention (the metadata endpoint
// URL is static and public, so echoing it is safe); the exact-match assertion
// proves nothing (hint, header, body) is appended beyond that.
func TestFetchReleases_NonOKStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{"not-found", http.StatusNotFound},
		{"forbidden", http.StatusForbidden},
		{"too-many-requests", http.StatusTooManyRequests},
		{"server-error", http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := releaseMetadataRawServer(t, tc.status, []byte(`{"error":"boom"}`))
			withMetadataBaseURL(t, srv.URL)

			_, err := Latest(context.Background(), true)
			require.Error(t, err)
			assert.Equal(t, fmt.Sprintf("status %d for %s/releases.json", tc.status, srv.URL), err.Error(),
				"non-200 errors must carry the status + redacted URL only — no credential hint, no echoed body")
			// A fetch failure must NEVER surface as ErrNoStableRelease.
			assert.False(t, errors.Is(err, ErrNoStableRelease))
		})
	}
}

// TestFetchReleases_PreFlightHostCheck proves the metadata fetch goes through
// the SAME REQ-S-001 pre-flight gate as the asset downloads: a metadataBaseURL
// whose scheme is non-HTTPS or whose host is not on the download allowlist is
// rejected BEFORE any HTTP request is dispatched (httpDoFn must not fire).
// Uses the raw metadataBaseURL seam rather than withMetadataBaseURL, which
// would add the host to the allowlist and disable requireHTTPS.
func TestFetchReleases_PreFlightHostCheck(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		wantSub string
	}{
		{"foreign host", "https://evil.example.com", "allowlist"},
		{"non-https", "http://neo4j.sh", "non-https"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prev := metadataBaseURL
			metadataBaseURL = tc.baseURL
			t.Cleanup(func() { metadataBaseURL = prev })

			withHttpDo(t, func(req *http.Request) (*http.Response, error) {
				t.Fatal("httpDoFn must not fire when the pre-flight host check rejects the URL")
				return nil, nil
			})

			_, err := fetchReleases(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantSub)
			// A pre-flight rejection must NEVER surface as ErrNoStableRelease.
			assert.False(t, errors.Is(err, ErrNoStableRelease))
		})
	}
}

// TestFetchReleases_NoAuthorizationHeader proves release discovery is fully
// anonymous: the outgoing request carries no Authorization header at all,
// and a failure-path error string never echoes an auth-shaped value.
func TestFetchReleases_NoAuthorizationHeader(t *testing.T) {
	t.Run("happy path sends no Authorization header", func(t *testing.T) {
		var captured http.Header
		srv := releaseMetadataServer(t, []Release{{TagName: "v0.0.5"}}, &captured)
		withMetadataBaseURL(t, srv.URL)

		_, err := Latest(context.Background(), false)
		require.NoError(t, err)
		assert.Equal(t, "", captured.Get("Authorization"),
			"discovery must be anonymous — no Authorization header, ever")
		assert.Equal(t, "neo4j-cli-update", captured.Get("User-Agent"))
		assert.Equal(t, "application/json", captured.Get("Accept"),
			"the metadata request keeps its Accept header (set unconditionally inside downloadCapped)")
	})

	t.Run("error path echoes no auth material", func(t *testing.T) {
		srv := releaseMetadataRawServer(t, http.StatusForbidden, nil)
		withMetadataBaseURL(t, srv.URL)

		_, err := Latest(context.Background(), true)
		require.Error(t, err)
		assert.False(t, strings.Contains(err.Error(), "Authorization:"), "error must not include an Authorization header literal: %v", err)
		assert.False(t, strings.Contains(err.Error(), "Bearer "), "error must not echo a Bearer prefix: %v", err)
	})
}

func TestFetchReleases_MalformedJSON(t *testing.T) {
	srv := releaseMetadataRawServer(t, http.StatusOK, []byte(`{not valid json`))
	withMetadataBaseURL(t, srv.URL)

	_, err := Latest(context.Background(), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
	assert.False(t, errors.Is(err, ErrNoStableRelease))
}

// TestFetchReleases_SchemaVersionMismatch pins the hard-fail on any
// schema_version other than 1.
func TestFetchReleases_SchemaVersionMismatch(t *testing.T) {
	for _, body := range []string{
		`{"schema_version":2,"releases":[]}`,
		`{"releases":[]}`, // missing schema_version decodes as 0
	} {
		srv := releaseMetadataRawServer(t, http.StatusOK, []byte(body))
		withMetadataBaseURL(t, srv.URL)

		_, err := fetchReleases(context.Background())
		require.Error(t, err, "body %s", body)
		assert.Contains(t, err.Error(), "schema_version")
	}
}

// TestFetchReleases_MissingReleasesKey pins the hard-fail on a null or
// absent releases key (distinct from a valid empty array).
func TestFetchReleases_MissingReleasesKey(t *testing.T) {
	for _, body := range []string{
		`{"schema_version":1}`,
		`{"schema_version":1,"releases":null}`,
	} {
		srv := releaseMetadataRawServer(t, http.StatusOK, []byte(body))
		withMetadataBaseURL(t, srv.URL)

		_, err := fetchReleases(context.Background())
		require.Error(t, err, "body %s", body)
		assert.Contains(t, err.Error(), "releases")
	}

	// Control: an empty array is valid and yields no releases (Latest then
	// surfaces ErrNoStableRelease).
	srv := releaseMetadataRawServer(t, http.StatusOK, []byte(`{"schema_version":1,"releases":[]}`))
	withMetadataBaseURL(t, srv.URL)
	got, err := fetchReleases(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestFetchReleases_BodyExceedsCap pins the 4 MiB body cap: an oversized
// document fails the whole fetch rather than being truncated into a partial
// decode.
func TestFetchReleases_BodyExceedsCap(t *testing.T) {
	body := bytes.Repeat([]byte("x"), maxMetadataBytes+1)
	srv := releaseMetadataRawServer(t, http.StatusOK, body)
	withMetadataBaseURL(t, srv.URL)

	_, err := fetchReleases(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds cap")
}

// TestFetchReleases_GoldenFixture decodes the committed releases.json
// fixture (testdata/releases.json — shuffled file order, extra schema fields,
// a non-semver tag) into the exact expected Release list: semver descending,
// body verbatim markdown with NO HTML conversion and NO trimming.
func TestFetchReleases_GoldenFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "releases.json"))
	require.NoError(t, err)
	srv := releaseMetadataRawServer(t, http.StatusOK, fixture)
	withMetadataBaseURL(t, srv.URL)

	got, err := fetchReleases(context.Background())
	require.NoError(t, err)

	want := []Release{
		{TagName: "v1.3.0-alpha.1", Body: "## Release notes\n\n### Patch\n\n- Alpha preview entry"},
		{TagName: "v1.2.0", Body: "## Release notes\n\n### Minor\n\n- Add release discovery via releases.json\n\n---\n\n## Versions\n- v1.2.0"},
		{TagName: "v1.1.0", Body: "## Release notes\n\n### Minor\n\n- First fixture entry"},
		{TagName: "nightly", Body: "Continuous build; not a semver tag."},
	}
	assert.Equal(t, want, got)
}

// TestChangelogForRange_GoldenFixture feeds the decoded releases.json
// fixture through changelogForRange to cover changelog rendering of the
// native (verbatim markdown) body: trimReleaseBody stays the single trimming
// point, cutting at "---" and stripping the "## Release notes" heading.
func TestChangelogForRange_GoldenFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "releases.json"))
	require.NoError(t, err)
	srv := releaseMetadataRawServer(t, http.StatusOK, fixture)
	withMetadataBaseURL(t, srv.URL)

	releases, err := fetchReleases(context.Background())
	require.NoError(t, err)

	entries, elided := changelogForRange(releases, "v1.1.0", "v1.2.0")
	assert.False(t, elided)
	require.Len(t, entries, 1)
	assert.Equal(t, "v1.2.0", entries[0].tag)
	assert.Equal(t, "### Minor\n\n- Add release discovery via releases.json", entries[0].body)
}

func TestGetByTag_Hit(t *testing.T) {
	releases := []Release{
		{TagName: "v0.1.0-alpha.9"},
		{TagName: "v0.0.5"},
	}
	srv := releaseMetadataServer(t, releases, nil)
	withMetadataBaseURL(t, srv.URL)

	got, err := GetByTag(context.Background(), "v0.0.5")
	require.NoError(t, err)
	assert.Equal(t, "v0.0.5", got.TagName)
}

func TestGetByTag_NotFound(t *testing.T) {
	releases := []Release{
		{TagName: "v0.0.5"},
	}
	srv := releaseMetadataServer(t, releases, nil)
	withMetadataBaseURL(t, srv.URL)

	_, err := GetByTag(context.Background(), "v9.9.9")
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
	withMetadataBaseURL(t, srv.URL)

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

func TestListReleases_HappyPath(t *testing.T) {
	releases := []Release{
		{TagName: "v0.1.0", Body: "## v0.1.0\n\nfirst release"},
		{TagName: "v0.1.0-alpha.1"},
		{TagName: "v0.2.0", Body: "## v0.2.0\n\nsecond release"},
	}
	srv := releaseMetadataServer(t, releases, nil)
	withMetadataBaseURL(t, srv.URL)

	got, err := ListReleases(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "v0.2.0", got[0].TagName)
	assert.Equal(t, "v0.1.0", got[1].TagName)
	assert.Equal(t, "## v0.2.0\n\nsecond release", got[0].Body)
	assert.Equal(t, "## v0.1.0\n\nfirst release", got[1].Body)
}

func TestListReleases_PrereleasesFiltered(t *testing.T) {
	releases := []Release{
		{TagName: "v0.2.0-alpha.1"},
		{TagName: "v0.1.0"},
	}
	srv := releaseMetadataServer(t, releases, nil)
	withMetadataBaseURL(t, srv.URL)

	got, err := ListReleases(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "v0.1.0", got[0].TagName)
}

func TestListReleases_AcceptsAllWithPreReleases(t *testing.T) {
	releases := []Release{
		{TagName: "v0.1.0"},
		{TagName: "v0.2.0-alpha.1"},
	}
	srv := releaseMetadataServer(t, releases, nil)
	withMetadataBaseURL(t, srv.URL)

	got, err := ListReleases(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "v0.2.0-alpha.1", got[0].TagName)
	assert.Equal(t, "v0.1.0", got[1].TagName)
}

func TestListReleases_SkipsNonSemverTags(t *testing.T) {
	releases := []Release{
		{TagName: "nightly"},
		{TagName: "v0.0.5"},
	}
	srv := releaseMetadataServer(t, releases, nil)
	withMetadataBaseURL(t, srv.URL)

	got, err := ListReleases(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "v0.0.5", got[0].TagName)
}

func TestFilterReleases_InputNotAliased(t *testing.T) {
	input := []Release{
		{TagName: "not-semver"},
		{TagName: "v2.0.0"},
		{TagName: "v1.0.0"},
	}
	inputCopy := make([]Release, len(input))
	copy(inputCopy, input)

	got := filterReleases(input, false)
	require.Len(t, got, 2)
	assert.Equal(t, "v2.0.0", got[0].TagName)
	assert.Equal(t, "v1.0.0", got[1].TagName)
	assert.Equal(t, inputCopy, input, "filterReleases must not alias its input backing array")
}

// TestFetchReleases_RejectsRedirectAwayFromAllowlist asserts the REQ-S-001
// host-pin in fetchReleases fires when neo4j.sh (or its test seam) is
// redirected to a host that is NOT on the download allowlist. Without this
// guard, a hijacked metadata host or DNS-poisoned route could feed the CLI a
// JSON payload from an unexpected origin.
//
// We use the httpDoFn test seam to inject a synthetic response whose
// resp.Request.URL.Host points at a host NOT on the allowlist — that is
// exactly the post-redirect state the production code must catch. A real
// httptest server would also work but adds dial / timeout flake risk on
// hosts where 127.0.0.x aliases aren't routable.
func TestFetchReleases_RejectsRedirectAwayFromAllowlist(t *testing.T) {
	// Set metadataBaseURL to a synthetic https URL so the request build
	// succeeds; we override what httpDoFn returns anyway.
	withMetadataBaseURL(t, "https://neo4j.sh")
	withHttpDo(t, func(req *http.Request) (*http.Response, error) {
		// Synthesize a final URL that is foreign to the allowlist (the
		// post-redirect state). The body is intentionally non-empty so we
		// can prove fetchReleases rejected it BEFORE decoding.
		finalURL, _ := url.Parse("https://evil.example.com/releases.json")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"schema_version":1,"releases":[{"tag":"v9.9.9-evil"}]}`))),
			Header:     make(http.Header),
			Request:    &http.Request{URL: finalURL},
		}, nil
	})

	_, err := Latest(context.Background(), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allowlist")
}

// TestFetchReleases_RejectsNonHTTPSFinalURL asserts the requireHTTPS gate
// in assertAllowedHostURL fires on the FINAL URL when a redirect
// downgrades the scheme. Same httpDoFn-seam pattern as the test above,
// with requireHTTPS forced on AFTER withMetadataBaseURL turned it off.
func TestFetchReleases_RejectsNonHTTPSFinalURL(t *testing.T) {
	withMetadataBaseURL(t, "https://neo4j.sh")
	withHttpDo(t, func(req *http.Request) (*http.Response, error) {
		// Synthesize a final URL on an allowlisted host but with a plain
		// http:// scheme — the requireHTTPS gate must reject this.
		finalURL, _ := url.Parse("http://neo4j.sh/releases.json")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"schema_version":1,"releases":[]}`))),
			Header:     make(http.Header),
			Request:    &http.Request{URL: finalURL},
		}, nil
	})
	// Force the production HTTPS guard on AFTER withMetadataBaseURL turned it
	// off — the final URL check should now reject the http:// scheme.
	withRequireHTTPS(t, true)

	_, err := Latest(context.Background(), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-https")
}

// Sanity: ensure the package-level seams are wired up correctly (the symbols
// are usable and callable). Exists primarily so the package compiles cleanly
// even if someone deletes a test that touches a seam.
func TestSeams_Smoke(t *testing.T) {
	require.NotEmpty(t, metadataBaseURL)
	require.NotEmpty(t, dlBaseURL)
	require.NotEmpty(t, goosFn())
	require.NotEmpty(t, goarchFn())
	require.NotNil(t, httpDoFn)
	// Basic invocation: build a request and confirm httpDoFn returns
	// without panicking. Use a closed httptest server URL to force an
	// error and avoid hitting the real metadata host.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	_, _ = httpDoFn(req)
	_ = fmt.Sprintf("%T", metadataBaseURL) // touch all seam vars
}
