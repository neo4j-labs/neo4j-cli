// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"time"

	"github.com/spf13/afero"
)

// refreshTimeout caps the total wall-clock time for plugin.json + tarball
// fetch via context.WithTimeout when Refresh's caller doesn't supply a
// shorter deadline.
const refreshTimeout = 30 * time.Second

// maxPluginJSONBytes caps the plugin.json body read so a malicious upstream
// or successful MITM cannot force unbounded in-memory allocation. Real
// plugin.json payloads are in the kilobyte range; 1 MiB is generous.
const maxPluginJSONBytes int64 = 1 << 20

// Refresh fetches the upstream plugin.json (and, when its version differs
// from cached state, the repo tarball) and updates the on-disk cache. The
// receiver's in-memory Version/Skills fields are replaced with the
// upstream view on success.
//
// HTTP plumbing is read from the receiver's constructor-time deps: a nil
// Doer falls back to http.DefaultClient; an empty BinaryVersion renders
// the User-Agent as `neo4j-cli/dev`. The total wall-clock budget is
// capped at 30 s.
//
// On a successful plugin.json fetch the cache root is created if missing,
// plugin.json + the fetched-at marker are rewritten, and — only when the
// upstream version differs from the cached one — the tarball is fetched
// and extracted into `<cacheRoot>/content/<version>/`. When the upstream
// version equals the cached version, the tarball download is skipped
// even if the on-disk content directory is missing (callers that need to
// force a re-extract should clear the content dir first).
//
// On any network or extraction failure Refresh returns an error;
// callers decide whether to warn and continue with the previously-cached
// catalog or hard-fail.
func (c *Catalog) Refresh(ctx context.Context, filesystem afero.Fs) error {
	if c == nil {
		return errors.New("catalog: nil receiver")
	}
	if c.cacheRoot == "" {
		return errors.New("catalog: empty cache root")
	}
	if filesystem == nil {
		return errors.New("catalog: nil filesystem")
	}

	doer := c.doer
	if doer == nil {
		doer = http.DefaultClient
	}
	userAgent := userAgentFor(c.binaryVersion)

	ctx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()

	body, err := fetch(ctx, doer, userAgent, PluginJSONURL)
	if err != nil {
		return fmt.Errorf("catalog: fetch plugin.json: %w", err)
	}
	defer func() { _ = body.Close() }()

	data, err := io.ReadAll(io.LimitReader(body, maxPluginJSONBytes+1))
	if err != nil {
		return fmt.Errorf("catalog: read plugin.json body: %w", err)
	}
	if int64(len(data)) > maxPluginJSONBytes {
		return fmt.Errorf("catalog: plugin.json exceeds %d byte cap", maxPluginJSONBytes)
	}

	var pj pluginJSON
	if jerr := json.Unmarshal(data, &pj); jerr != nil {
		return fmt.Errorf("catalog: parse upstream plugin.json: %w", jerr)
	}
	if pj.Version == "" {
		return errors.New("catalog: upstream plugin.json has empty version")
	}
	if !ValidSkillName(pj.Version) {
		return fmt.Errorf("catalog: upstream plugin.json has unsafe version %q", pj.Version)
	}

	cachedVersion := c.Version
	upstreamSkills := skillEntriesFromPluginJSON(pj.Skills)

	// Tarball fetch + extract happens BEFORE writing plugin.json so a mid-
	// refresh network or extraction failure leaves the prior cache intact.
	//
	// security: tarball integrity rests on HTTPS + GitHub's hosting of the
	// neo4j-contrib/neo4j-skills repo. Upstream plugin.json does not publish a
	// cryptographic digest (sha256/sha512) — the bytes-on-the-wire trust model
	// is "whatever codeload.github.com serves for refs/heads/main". The
	// extractor (Extract + classifyEntry + the inline Zip Slip prefix check)
	// protects the local filesystem from a malicious tarball shape; this leaves
	// only the SKILL.md CONTENT itself as the attack surface, which is
	// equivalent to the supply-chain risk of trusting any GitHub-hosted asset.
	// A future upstream PR adding a `sha256` field to plugin.json would let us
	// verify here via io.TeeReader + sha256.New — tracked separately.
	if cachedVersion != pj.Version {
		tar, terr := fetch(ctx, doer, userAgent, TarballURL)
		if terr != nil {
			return fmt.Errorf("catalog: fetch tarball: %w", terr)
		}
		defer func() { _ = tar.Close() }()

		destRoot := contentPath(c.cacheRoot, pj.Version)
		if mkerr := filesystem.MkdirAll(destRoot, 0755); mkerr != nil {
			return fmt.Errorf("catalog: mkdir content root: %w", mkerr)
		}
		allowed := make([]string, 0, len(upstreamSkills))
		for _, s := range upstreamSkills {
			allowed = append(allowed, s.Name)
		}
		if eerr := Extract(tar, filesystem, destRoot, allowed); eerr != nil {
			return fmt.Errorf("catalog: extract tarball: %w", eerr)
		}
	}

	if mkerr := filesystem.MkdirAll(c.cacheRoot, 0755); mkerr != nil {
		return fmt.Errorf("catalog: mkdir cache root: %w", mkerr)
	}
	if werr := afero.WriteFile(filesystem, pluginJSONPath(c.cacheRoot), data, 0600); werr != nil {
		return fmt.Errorf("catalog: write plugin.json: %w", werr)
	}
	stamp := nowFn().UTC().Format(time.RFC3339)
	if werr := afero.WriteFile(filesystem, fetchedAtPath(c.cacheRoot), []byte(stamp), 0600); werr != nil {
		return fmt.Errorf("catalog: write fetched-at: %w", werr)
	}

	// Best-effort prune of content/<version>/ subtrees from prior catalog
	// versions; a failure here doesn't undo the successful refresh.
	_ = pruneStaleContent(filesystem, c.cacheRoot, pj.Version)

	c.Version = pj.Version
	c.Skills = upstreamSkills
	return nil
}

// pruneStaleContent removes content/<version>/ subtrees whose version
// doesn't match currentVersion. Stops at the first RemoveAll error and
// returns it; the missing-content-dir case is not an error. Callers
// typically ignore the return value — the prune is best-effort.
func pruneStaleContent(filesystem afero.Fs, cacheRoot, currentVersion string) error {
	contentRoot := filepath.Join(cacheRoot, "content")
	entries, err := afero.ReadDir(filesystem, contentRoot)
	if err != nil {
		// Missing content dir on first refresh is normal — not an error.
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == currentVersion {
			continue
		}
		if rerr := filesystem.RemoveAll(filepath.Join(contentRoot, e.Name())); rerr != nil {
			return rerr
		}
	}
	return nil
}

// Lookup resolves a catalog skill name to its on-disk content. Returns
// the SkillEntry and an `fs.FS` rooted at
// `<cacheRoot>/content/<version>/<name>/`.
//
// Rejects (fail-closed) any name that collides with the embedded self-
// skill identity — the canonical `self` id and the running binary name.
// This is the single source of truth for reserved-name enforcement; the
// parent `common/skill` package imports IsReserved to keep its alias
// map in sync.
func (c *Catalog) Lookup(filesystem afero.Fs, name, binaryName string) (*SkillEntry, fs.FS, error) {
	if c == nil {
		return nil, nil, errors.New("catalog: nil receiver")
	}
	if name == "" {
		return nil, nil, errors.New("catalog: empty skill name")
	}
	if IsReserved(name, binaryName) {
		return nil, nil, fmt.Errorf("catalog: skill name %q is reserved for the embedded self-skill", name)
	}
	if c.Version == "" {
		return nil, nil, errors.New("catalog: empty catalog version (run skill refresh)")
	}

	var found *SkillEntry
	for i := range c.Skills {
		if c.Skills[i].Name == name {
			found = &c.Skills[i]
			break
		}
	}
	if found == nil {
		return nil, nil, fmt.Errorf("catalog: skill %q not in catalog", name)
	}

	dir := filepath.Join(contentPath(c.cacheRoot, c.Version), name)
	exists, _ := afero.DirExists(filesystem, dir)
	if !exists {
		return nil, nil, fmt.Errorf("catalog: content for %q missing at %s (run skill refresh)", name, dir)
	}
	return found, afero.NewIOFS(afero.NewBasePathFs(filesystem, dir)), nil
}

// userAgentFor renders the User-Agent header value. Falls back to `dev`
// when the binary version is empty so requests are always identifiable
// in upstream logs.
func userAgentFor(version string) string {
	if version == "" {
		version = "dev"
	}
	return "neo4j-cli/" + version
}

// fetch performs a context-bound HTTP GET against url with the
// `User-Agent` header set and returns the response body on success
// (status 2xx). Non-2xx responses error and close the body.
func fetch(ctx context.Context, doer HTTPDoer, userAgent, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := doer.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}
