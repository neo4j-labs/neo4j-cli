// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package update implements the `neo4j-cli update` self-update command.
//
// release.go owns the GitHub release discovery side of the package: fetching
// releases from the GitHub API, filtering them by --pre-releases via
// golang.org/x/mod/semver, and constructing GoReleaser-style archive +
// checksum URLs. Production callers use Latest, BuildAssetURLs, and
// ValidateVersionTag; the package-level test seams (apiBaseURL, dlBaseURL,
// goosFn, goarchFn, httpDoFn) let tests drive the path against an httptest
// server without touching real GitHub.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// repoSlug is the upstream repository the CLI self-updates from. Kept as a
// constant rather than a flag — REQ "no support for forks" in the PRD.
const repoSlug = "neo4j-labs/neo4j-cli"

// httpTimeout caps any single GitHub HTTP call. Network failures surface as
// errors rather than hanging indefinitely, but the timeout is generous enough
// that a slow connection on the release-list call (which paginates 30 entries
// of metadata) still completes.
const httpTimeout = 30 * time.Second

// Test seams. Production fills with the real values; tests swap via the
// withApiBaseURL / withDlBaseURL / withGoos / withGoarch helpers below.
var (
	// apiBaseURL is the GitHub REST API root used for release discovery.
	apiBaseURL = "https://api.github.com"
	// dlBaseURL is the host for release download URLs (archives + checksums).
	dlBaseURL = "https://github.com"
	// goosFn / goarchFn shadow runtime.GOOS / runtime.GOARCH so the asset URL
	// builder is testable across platforms from a single host.
	goosFn   = func() string { return runtime.GOOS }
	goarchFn = func() string { return runtime.GOARCH }
	// httpDoFn allows tests to inject a custom transport without depending on
	// the global default client. Production uses a single client with a
	// modest timeout.
	httpDoFn = func(req *http.Request) (*http.Response, error) {
		client := &http.Client{Timeout: httpTimeout}
		return client.Do(req)
	}
)

// ErrNoStableRelease is returned by Latest when the stable filter excludes
// every release. The RunE flow surfaces this as a friendly "pass --pre-releases"
// hint per REQ-F-006.
var ErrNoStableRelease = errors.New("no stable release published yet")

// ErrTagNotFound is returned by GetByTag when the named tag does not match a
// non-draft release on the API. The RunE flow surfaces this as a clear "tag
// not found" error per the task-006 acceptance criteria.
var ErrTagNotFound = errors.New("release tag not found")

// Release is the slim subset of the GitHub release JSON the package needs.
// Field names match the REST API; extra fields are ignored.
type Release struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// AssetURLs is the pair of URLs needed to fetch + verify a release asset.
type AssetURLs struct {
	Archive  string
	Checksum string
}

// Latest returns the head release matching the stable / pre-release filter.
// preReleases=false keeps tags where semver.Prerelease(tag) == ""; true accepts
// any non-draft release. Returns ErrNoStableRelease when the stable filter
// yields zero matches.
func Latest(ctx context.Context, preReleases bool) (*Release, error) {
	releases, err := fetchReleases(ctx)
	if err != nil {
		return nil, err
	}

	for _, r := range releases {
		if r.Draft {
			continue
		}
		if !semver.IsValid(r.TagName) {
			// Skip non-semver tags defensively. The repo convention is vX.Y.Z
			// but a malformed tag should not break discovery.
			continue
		}
		if !preReleases && semver.Prerelease(r.TagName) != "" {
			continue
		}
		rr := r
		return &rr, nil
	}

	if !preReleases {
		return nil, ErrNoStableRelease
	}
	return nil, fmt.Errorf("no releases found")
}

// GetByTag returns the named release, or ErrTagNotFound when the tag does not
// match any non-draft release. The caller is expected to have validated the
// tag via ValidateVersionTag before calling this.
func GetByTag(ctx context.Context, tag string) (*Release, error) {
	releases, err := fetchReleases(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range releases {
		if r.Draft {
			continue
		}
		if r.TagName == tag {
			rr := r
			return &rr, nil
		}
	}
	return nil, ErrTagNotFound
}

// fetchReleases hits the GitHub REST API and decodes the response. Honors
// GH_TOKEN / GITHUB_TOKEN for ratelimit relief; the token value is never
// included in any returned error string.
func fetchReleases(ctx context.Context) ([]Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases?per_page=30", apiBaseURL, repoSlug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build releases request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "neo4j-cli-update")
	if tok := releaseAuthToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := httpDoFn(req)
	if err != nil {
		// req.URL is the public GitHub API endpoint; no token leakage risk
		// because Authorization is in headers, not the URL. Net/http's
		// *url.Error wrapping does not echo request headers either.
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// REQ-S-001 / defense-in-depth against a hijacked GitHub API: validate the
	// FINAL URL host after net/http transparently followed any redirects. The
	// stdlib already strips the Authorization header on cross-host redirects,
	// but pinning the response host means we never read or decode a JSON
	// payload from an unexpected origin. Reuses the swap.go allowedDownloadHosts
	// allowlist (which already includes api.github.com); tests that point
	// apiBaseURL at httptest.NewServer add the loopback host via
	// withAllowedHost as they do for the swap path.
	if resp.Request != nil && resp.Request.URL != nil {
		if err := assertAllowedHostURL(resp.Request.URL); err != nil {
			return nil, fmt.Errorf("fetch releases: %w", err)
		}
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		// Drain a small amount of body for diagnostic context but do NOT
		// echo headers (the Authorization header would have a token).
		_, _ = io.CopyN(io.Discard, resp.Body, 1<<10)
		return nil, fmt.Errorf("github api rate-limited (status %d); set GH_TOKEN or GITHUB_TOKEN to raise the limit", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	// Cap the body size to a sane upper bound. 30 releases of metadata is
	// well under 1 MiB; an oversized body indicates either a misconfigured
	// proxy or a malicious server impersonating GitHub.
	const maxBody = 4 << 20 // 4 MiB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("read releases response: %w", err)
	}

	var releases []Release
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("decode releases response: %w", err)
	}
	return releases, nil
}

// releaseAuthToken returns the GitHub auth token from GH_TOKEN or GITHUB_TOKEN,
// preferring GH_TOKEN. The value is returned verbatim — callers that surface
// errors must NEVER include this string in error messages.
func releaseAuthToken() string {
	if v := os.Getenv("GH_TOKEN"); v != "" {
		return v
	}
	return os.Getenv("GITHUB_TOKEN")
}

// BuildAssetURLs constructs the archive + checksum download URLs for a given
// release tag, mirroring the layout produced by GoReleaser and the
// install-neo4j-cli.sh script. Returns an error for unsupported OS/arch
// combinations rather than silently building a URL the server will 404 on.
func BuildAssetURLs(tag string) (AssetURLs, error) {
	if !semver.IsValid(tag) {
		return AssetURLs{}, fmt.Errorf("invalid release tag: %q", tag)
	}
	verNoV := strings.TrimPrefix(tag, "v")

	osTitle, err := mapGOOS(goosFn())
	if err != nil {
		return AssetURLs{}, err
	}
	archUname, err := mapGOARCH(goarchFn())
	if err != nil {
		return AssetURLs{}, err
	}

	ext := ".tar.gz"
	if goosFn() == "windows" {
		ext = ".zip"
	}

	archive := fmt.Sprintf(
		"%s/%s/releases/download/%s/neo4j-cli_%s_%s_%s%s",
		dlBaseURL, repoSlug, tag, verNoV, osTitle, archUname, ext,
	)
	checksum := fmt.Sprintf(
		"%s/%s/releases/download/%s/neo4j-cli_%s_checksums.txt",
		dlBaseURL, repoSlug, tag, verNoV,
	)
	return AssetURLs{Archive: archive, Checksum: checksum}, nil
}

// mapGOOS converts a runtime.GOOS value to the GoReleaser title-case form
// used in archive filenames.
func mapGOOS(goos string) (string, error) {
	switch goos {
	case "linux":
		return "Linux", nil
	case "darwin":
		return "Darwin", nil
	case "windows":
		return "Windows", nil
	default:
		return "", fmt.Errorf("unsupported OS for self-update: %q", goos)
	}
}

// mapGOARCH converts a runtime.GOARCH value to the uname -m form used in
// archive filenames.
func mapGOARCH(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "x86_64", nil
	case "386":
		return "i386", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported architecture for self-update: %q", goarch)
	}
}

// ValidateVersionTag enforces a strict allowlist on user-supplied --version
// values BEFORE the tag flows into URL construction. semver.IsValid covers
// the structural shape; the explicit byte rejections defend against shell
// metacharacters and traversal sequences that a buggy/malicious server-side
// rewrite could otherwise turn into a path injection.
func ValidateVersionTag(tag string) error {
	if tag == "" {
		return fmt.Errorf("version tag is empty")
	}
	if !semver.IsValid(tag) {
		return fmt.Errorf("invalid semver tag: %q (expected form vMAJOR.MINOR.PATCH[-prerelease])", tag)
	}
	// semver.IsValid accepts metadata after `+`; reject for our purposes
	// because GoReleaser tags do not embed `+` and any such char on the URL
	// is a smell.
	disallowed := []string{"..", "/", "\\", "\x00", " ", "\t", "\n", "\r", ";", "&", "|", "$", "`", "*", "?", "<", ">", "(", ")", "{", "}", "[", "]", "\"", "'", "+"}
	for _, bad := range disallowed {
		if strings.Contains(tag, bad) {
			return fmt.Errorf("tag contains disallowed character sequence %q", bad)
		}
	}
	return nil
}
