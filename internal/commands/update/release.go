// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package update implements the `neo4j-cli update` self-update command.
//
// release.go owns the release discovery side of the package: fetching the
// static releases.json metadata document served from neo4j.sh, filtering
// releases by --pre-releases via golang.org/x/mod/semver, and constructing
// GoReleaser-style archive + checksum URLs. Production callers use Latest,
// BuildAssetURLs, and ValidateVersionTag; the package-level test seams
// (metadataBaseURL, dlBaseURL, goosFn, goarchFn, httpDoFn) let tests drive
// the path against an httptest server without touching real network hosts.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// repoSlug is the upstream repository the CLI self-updates from. Kept as a
// constant rather than a flag — REQ "no support for forks" in the PRD.
const repoSlug = "neo4j-labs/neo4j-cli"

// httpTimeout caps any single metadata/download HTTP call. Network failures
// surface as errors rather than hanging indefinitely, but the timeout is
// generous enough that a slow connection on the release-metadata call still
// completes.
const httpTimeout = 30 * time.Second

// maxMetadataBytes caps the releases.json body size. All published releases
// of metadata are well under 1 MiB; an oversized body indicates either a
// misconfigured proxy or a malicious server impersonating neo4j.sh, and must
// fail the whole fetch rather than be truncated into a partial decode.
const maxMetadataBytes = 4 << 20 // 4 MiB

// metadataSchemaVersion is the only releases.json schema version this CLI
// understands. Any other value hard-fails the fetch — a silently
// mis-decoded document is worse than a loud error.
const metadataSchemaVersion = 1

// Test seams. Production fills with the real values; tests swap via the
// withMetadataBaseURL / withDlBaseURL / withGoos / withGoarch helpers below.
var (
	// metadataBaseURL is the static host serving the releases.json release
	// metadata document used for release discovery.
	metadataBaseURL = "https://neo4j.sh"
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

// ErrTagNotFound is returned by GetByTag when the named tag does not match
// any release in the metadata document. The RunE flow surfaces this as a
// clear "tag not found" error per the task-006 acceptance criteria.
var ErrTagNotFound = errors.New("release tag not found")

// Release is the slim subset of a releases.json entry the package needs.
// Field names match the releases.json schema (schema_version 1); extra
// fields (name, prerelease, published_at, …) are ignored. The body is the
// GitHub release markdown verbatim — trimReleaseBody in changelog.go remains
// the single trimming point. Drafts never appear: the generator excludes
// them, so the schema carries no draft key.
type Release struct {
	TagName string `json:"tag"`
	Body    string `json:"body"`
}

// releasesMetadata mirrors the releases.json document (schema_version 1).
// generated_at / repository are informational only — the CLI never acts on
// them. Releases is a pointer so a missing or null "releases" key is
// distinguishable from an empty array and fails the whole fetch.
type releasesMetadata struct {
	SchemaVersion int        `json:"schema_version"`
	GeneratedAt   string     `json:"generated_at"`
	Repository    string     `json:"repository"`
	Releases      *[]Release `json:"releases"`
}

// AssetURLs is the pair of URLs needed to fetch + verify a release asset.
type AssetURLs struct {
	Archive  string
	Checksum string
}

// Latest returns the head release matching the stable / pre-release filter.
// preReleases=false keeps tags where semver.Prerelease(tag) == ""; true
// accepts any release. Returns ErrNoStableRelease when the stable filter
// yields zero matches.
func Latest(ctx context.Context, preReleases bool) (*Release, error) {
	releases, err := fetchReleases(ctx)
	if err != nil {
		return nil, err
	}

	filtered := filterReleases(releases, preReleases)
	if len(filtered) == 0 {
		if !preReleases {
			return nil, ErrNoStableRelease
		}
		return nil, fmt.Errorf("no releases found")
	}
	r := filtered[0]
	return &r, nil
}

// GetByTag returns the named release, or ErrTagNotFound when the tag does
// not match any release in the metadata document. The caller is expected to
// have validated the tag via ValidateVersionTag before calling this.
func GetByTag(ctx context.Context, tag string) (*Release, error) {
	releases, err := fetchReleases(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range releases {
		if r.TagName == tag {
			rr := r
			return &rr, nil
		}
	}
	return nil, ErrTagNotFound
}

// ListReleases returns all releases matching the pre-release filter, newest
// first (semver descending — file order is never trusted). Applies the same
// two filters Latest uses: skip non-semver tags, skip prereleases unless
// preReleases is true.
func ListReleases(ctx context.Context, preReleases bool) ([]Release, error) {
	releases, err := fetchReleases(ctx)
	if err != nil {
		return nil, err
	}
	return filterReleases(releases, preReleases), nil
}

// filterReleases applies the two-check release filter (non-semver,
// prereleases) without aliasing the input slice's backing array.
func filterReleases(releases []Release, preReleases bool) []Release {
	filtered := make([]Release, 0, len(releases))
	for _, r := range releases {
		if !semver.IsValid(r.TagName) {
			continue
		}
		if !preReleases && semver.Prerelease(r.TagName) != "" {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// fetchReleases downloads the static releases.json metadata document and
// decodes it. No authentication is sent or read from the environment — the
// document is public and anonymous by design (CLI-244).
//
// The download goes through downloadCapped (swap.go) so the REQ-S-001
// pipeline — pre-flight AND post-redirect host pin, HTTPS-only, drain on
// non-200, cap+1 oversize reader, httpDoFn timeout — is single-sourced with
// the archive and checksum fetches: the metadata path is never weaker than
// the asset path. A non-200 (including a 404 for a missing releases.json)
// surfaces as downloadCapped's plain status error — no credential hint
// (there is no token to configure) and never ErrNoStableRelease /
// ErrTagNotFound, so the runUpdate caller routes it to the look-up-release
// upstream error (exit 8).
func fetchReleases(ctx context.Context) ([]Release, error) {
	body, err := downloadCapped(ctx, metadataBaseURL+"/releases.json", maxMetadataBytes)
	if err != nil {
		return nil, err
	}

	var meta releasesMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("decode releases response: %w", err)
	}
	if meta.SchemaVersion != metadataSchemaVersion {
		return nil, fmt.Errorf("unsupported releases metadata schema_version %d (expected %d)", meta.SchemaVersion, metadataSchemaVersion)
	}
	if meta.Releases == nil {
		return nil, fmt.Errorf("releases metadata is missing the releases key")
	}

	releases := *meta.Releases
	// NEVER trust file order: sort semver descending so Latest/ListReleases
	// see newest first regardless of how the generator ordered the document.
	// Non-semver tags (included by the generator, filtered out later by
	// filterReleases) compare below every valid tag and sink to the bottom.
	sort.SliceStable(releases, func(i, j int) bool {
		return semver.Compare(releases[i].TagName, releases[j].TagName) > 0
	})
	return releases, nil
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
