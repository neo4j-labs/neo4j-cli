// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dataset

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/mod/semver"
)

// rawBaseURL is the base for repo source files (including the manifest). The
// actual dump bytes live on the LFS media host instead; see download.go. It is
// a var so tests can point it at an httptest server; assertRawHost validates
// every fetch host against this base.
var rawBaseURL = "https://raw.githubusercontent.com"

// branches lists the default-branch candidates tried in order when fetching
// the manifest. neo4j-graph-examples repos use either main or master.
var branches = []string{"main", "master"}

// maxManifestBytes caps the manifest download. relate.project-install.json is
// a few hundred bytes in practice; 256 KiB is generous headroom while still
// rejecting an adversarial multi-megabyte body.
const maxManifestBytes = 256 << 10

// httpDoFn lets tests inject a transport without touching the network. Mirrors
// the seam used by the update package's release.go.
var httpDoFn = func(req *http.Request) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

// manifest is the parsed shape of a repo's relate.project-install.json. Only
// the dbms[] array is consumed here; other fields (name/title/icon) are
// ignored.
type manifest struct {
	DBMS []manifestEntry `json:"dbms"`
}

// manifestEntry is one entry of the dbms[] array. Entries carrying a
// scriptFile (Cypher seed) instead of a dumpFile are out of scope and skipped.
type manifestEntry struct {
	DumpFile           string   `json:"dumpFile"`
	ScriptFile         string   `json:"scriptFile"`
	TargetNeo4jVersion string   `json:"targetNeo4jVersion"`
	Plugins            []string `json:"plugins"`
}

// Spec is the resolved load plan for one dataset/target-version pair. Owner,
// Repo, and Branch identify where the dump lives so download.go can build the
// LFS media URL; DumpPath is the in-repo path of the selected dump file.
type Spec struct {
	Owner               string
	Repo                string
	Branch              string
	DumpPath            string
	Plugins             []string
	MatchedVersionRange string
}

// Resolve fetches the relate.project-install.json manifest for ownerRepo
// (trying branch main then master), parses dbms[], and selects the dump entry
// whose targetNeo4jVersion range is compatible with neo4jVersion. When several
// ranges match, the newest compatible (highest lower bound) wins. The selected
// dump path is verified to exist in the repo. Performs no full in-memory read
// of any large payload — only the small manifest is fetched here.
func Resolve(ctx context.Context, ownerRepo, neo4jVersion string) (Spec, error) {
	owner, repo, ok := strings.Cut(ownerRepo, "/")
	if !ok || owner == "" || repo == "" {
		return Spec{}, fmt.Errorf("invalid dataset %q: want owner/repo", ownerRepo)
	}

	// "latest" means newest neo4j, which is calver — the 5.x line continues as
	// 2025.x/2026.x, so a 5.x-or-newer dump applies. Treat it as calver below.
	calver := strings.EqualFold(strings.TrimSpace(neo4jVersion), "latest")
	target := canonicalVersion(neo4jVersion)
	if !calver {
		if !semver.IsValid(target) {
			return Spec{}, fmt.Errorf("invalid neo4j version %q", neo4jVersion)
		}
		calver = isCalver(target)
	}

	m, branch, err := fetchManifest(ctx, owner, repo)
	if err != nil {
		return Spec{}, err
	}

	entry, err := selectEntry(m, target, calver)
	if err != nil {
		return Spec{}, fmt.Errorf("%s/%s: %w", owner, repo, err)
	}

	if err := assertPathExists(ctx, owner, repo, branch, entry.DumpFile); err != nil {
		return Spec{}, err
	}

	return Spec{
		Owner:               owner,
		Repo:                repo,
		Branch:              branch,
		DumpPath:            entry.DumpFile,
		Plugins:             entry.Plugins,
		MatchedVersionRange: entry.TargetNeo4jVersion,
	}, nil
}

// fetchManifest downloads and parses the manifest, trying each candidate
// branch in turn. Returns the parsed manifest and the branch it was found on.
func fetchManifest(ctx context.Context, owner, repo string) (manifest, string, error) {
	var lastErr error
	for _, branch := range branches {
		u := fmt.Sprintf("%s/%s/%s/%s/relate.project-install.json", rawBaseURL, owner, repo, branch)
		body, status, err := getCapped(ctx, u, maxManifestBytes)
		if err != nil {
			lastErr = err
			continue
		}
		if status == http.StatusNotFound {
			lastErr = fmt.Errorf("manifest not found on branch %q", branch)
			continue
		}
		if status != http.StatusOK {
			lastErr = fmt.Errorf("fetch manifest (branch %q): status %d", branch, status)
			continue
		}
		var m manifest
		if err := json.Unmarshal(body, &m); err != nil {
			return manifest{}, "", fmt.Errorf("parse manifest for %s/%s: %w", owner, repo, err)
		}
		return m, branch, nil
	}
	return manifest{}, "", fmt.Errorf("no relate.project-install.json found for %s/%s on branches %v: %w", owner, repo, branches, lastErr)
}

// selectEntry picks the dbms[] dump entry whose range matches target, choosing
// the newest compatible (highest lower bound) when several match. scriptFile
// entries and entries without a dumpFile are skipped. When calver is true the
// target is a calver/latest neo4j (the 5.x line's continuation), so any entry
// whose range lower bound is >= 5.0.0 applies rather than requiring the calver
// string to fall inside the (5.x-style) range.
func selectEntry(m manifest, target string, calver bool) (manifestEntry, error) {
	var (
		best    manifestEntry
		bestLow string
		found   bool
	)
	for _, e := range m.DBMS {
		if e.DumpFile == "" {
			continue
		}
		ok, low, err := entryMatches(e.TargetNeo4jVersion, target, calver)
		if err != nil {
			return manifestEntry{}, err
		}
		if !ok {
			continue
		}
		if !found || semver.Compare(low, bestLow) > 0 {
			best, bestLow, found = e, low, true
		}
	}
	if !found {
		return manifestEntry{}, fmt.Errorf("no dump entry compatible with neo4j %s", strings.TrimPrefix(target, "v"))
	}
	return best, nil
}

// entryMatches reports whether a manifest entry's range applies to target and
// returns the range lower bound for tie-breaking. For concrete (non-calver)
// targets it is exact semver range matching. For calver/latest targets it
// matches any 5.x-or-newer dump (lower bound >= 5.0.0).
func entryMatches(expr, target string, calver bool) (bool, string, error) {
	low, err := rangeLowerBound(expr)
	if err != nil {
		return false, "", err
	}
	if calver {
		return semver.Compare(low, "v5.0.0") >= 0, low, nil
	}
	return rangeMatches(expr, target)
}

// assertPathExists verifies the selected dump file actually exists in the repo
// (the manifest can drift from data/). A HEAD against the raw host is enough —
// raw returns the LFS pointer rather than the dump bytes, but its presence
// confirms the path. download.go fetches the real bytes from the media host.
func assertPathExists(ctx context.Context, owner, repo, branch, dumpPath string) error {
	u := fmt.Sprintf("%s/%s/%s/%s/%s", rawBaseURL, owner, repo, branch, dumpPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := httpDoFn(req)
	if err != nil {
		return fmt.Errorf("verify dump path: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("dump file %q referenced by the manifest does not exist in %s/%s@%s", dumpPath, owner, repo, branch)
	}
	return fmt.Errorf("verify dump path %q: status %d", dumpPath, resp.StatusCode)
}

// getCapped issues a GET and returns the body (read up to cap bytes) and the
// HTTP status. Errors if the body exceeds the cap.
func getCapped(ctx context.Context, rawURL string, cap int64) ([]byte, int, error) {
	if err := assertRawHost(rawURL); err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	resp, err := httpDoFn(req)
	if err != nil {
		return nil, 0, fmt.Errorf("http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.CopyN(io.Discard, resp.Body, 1<<10)
		return nil, resp.StatusCode, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, cap+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > cap {
		return nil, resp.StatusCode, fmt.Errorf("manifest exceeds cap of %d bytes", cap)
	}
	return body, resp.StatusCode, nil
}

// assertRawHost rejects any URL whose scheme/host differs from rawBaseURL.
// Production rawBaseURL is https://raw.githubusercontent.com; tests override it
// to an httptest server and the check follows. The dump-bytes media host is
// allowlisted separately in download.go.
func assertRawHost(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	base, err := url.Parse(rawBaseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	if u.Scheme != base.Scheme {
		return fmt.Errorf("scheme %q not allowed for manifest fetch", u.Scheme)
	}
	if u.Host != base.Host {
		return fmt.Errorf("host %q not allowed for manifest fetch", u.Hostname())
	}
	return nil
}
