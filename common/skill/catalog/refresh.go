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

// Refresh fetches the upstream plugin.json (and, when its version differs
// from cached state, the repo tarball) and updates the on-disk cache. The
// receiver's in-memory Version/Skills fields are replaced with the
// upstream view on success.
//
// HTTP plumbing is read from the receiver: a nil Doer uses
// http.DefaultClient; an empty BinaryVersion renders the User-Agent as
// `neo4j-cli/dev`. The total wall-clock budget is capped at 30 s.
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

	doer := c.Doer
	if doer == nil {
		doer = http.DefaultClient
	}
	userAgent := userAgentFor(c.BinaryVersion)

	ctx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()

	body, err := fetch(ctx, doer, userAgent, PluginJSONURL)
	if err != nil {
		return fmt.Errorf("catalog: fetch plugin.json: %w", err)
	}
	defer func() { _ = body.Close() }()

	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("catalog: read plugin.json body: %w", err)
	}

	var pj pluginJSON
	if jerr := json.Unmarshal(data, &pj); jerr != nil {
		return fmt.Errorf("catalog: parse upstream plugin.json: %w", jerr)
	}
	if pj.Version == "" {
		return errors.New("catalog: upstream plugin.json has empty version")
	}

	cachedVersion := c.Version
	upstreamSkills := skillEntriesFromPluginJSON(pj.Skills)

	// Tarball fetch + extract happens BEFORE writing plugin.json so a mid-
	// refresh network or extraction failure leaves the prior cache intact.
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

	c.Version = pj.Version
	c.Skills = upstreamSkills
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

// skillEntriesFromPluginJSON converts the raw upstream string list into
// the in-memory SkillEntry slice. Mirrors Load's logic so refresh and
// load produce identical catalog shapes.
func skillEntriesFromPluginJSON(paths []string) []SkillEntry {
	out := make([]SkillEntry, 0, len(paths))
	for _, p := range paths {
		name := skillNameFromPath(p)
		if name == "" {
			continue
		}
		out = append(out, SkillEntry{Name: name, Path: p})
	}
	return out
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
func fetch(ctx context.Context, doer httpDoer, userAgent, url string) (io.ReadCloser, error) {
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
