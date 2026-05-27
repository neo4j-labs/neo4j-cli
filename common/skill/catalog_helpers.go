// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/spf13/afero"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/skill/catalog"
)

// CatalogTTL is the staleness threshold for the cached plugin.json
// (REQ-F-015). list/install/check auto-refresh when the cache is older
// than this.
const CatalogTTL = 24 * time.Hour

// catalogCacheRootFn is the test seam for catalog.CacheRoot. Production
// callers leave it at the default; tests override to point at a temp dir
// inside the in-memory afero.Fs.
var catalogCacheRootFn = catalog.CacheRoot

// catalogHTTPDoer is the test seam for the http client used by
// catalog.Refresh. nil means "use http.DefaultClient" (the catalog
// package's own default).
var catalogHTTPDoer func() catalog.HTTPDoer

// catalogOpts threads the auto-refresh policy into helpers without
// turning every call into a 6-arg signature. Used by install/list/check
// /refresh leaves.
type catalogOpts struct {
	// forceRefresh forces a network refresh even when the cache is warm.
	// Bound to the --refresh flag.
	forceRefresh bool
	// requireUsableCache controls the cold-cache + network-failure
	// branch: when true (install of a catalog skill) the helper returns
	// a fatal error pointing at `skill refresh`; when false (list/check)
	// it returns the empty catalog so the caller can render a cold-cache
	// fallback.
	requireUsableCache bool
}

// catalogLoad is the in-memory + on-disk view returned by
// loadOrRefreshCatalog. Cat may be nil when the cache is cold AND the
// refresh failed AND requireUsableCache is false.
type catalogLoad struct {
	Cat *catalog.Catalog
	// Warn is non-nil when the helper fell back to the cached catalog
	// after a network failure (REQ-F-019). The leaf should surface it to
	// stderr and continue.
	Warn error
}

// catalogRefreshWarnPrefix is the leading clause shared by every
// network-failure-with-cache warning. Centralised so wording lives in
// one place across install / list / check / refresh.
const catalogRefreshWarnPrefix = "warning: skill catalog refresh failed, using cached content"

// PrintWarn writes the standard "refresh failed, using cached content"
// warning to w when the load fell back to cached content (l.Warn != nil
// && l.Cat != nil). No-op otherwise so leaves can call it unconditionally.
func (l catalogLoad) PrintWarn(w io.Writer) {
	if l.Warn == nil || l.Cat == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "%s: %v\n", catalogRefreshWarnPrefix, l.Warn)
}

// PrintColdCacheHint writes the cold-cache hint to w when the catalog
// cache is empty (l.Cat == nil). No-op when the catalog is populated.
// Used by list (cold-cache fallback emits self-only rows + this hint).
func (l catalogLoad) PrintColdCacheHint(w io.Writer, binaryName string) {
	if l.Cat != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "%s\n", coldCacheHint(binaryName))
}

// coldCacheHint returns the canonical "cache empty, run refresh" wording
// shared by list (stderr) and print (error message). Wording lives in
// one place; callers wrap it in whatever surrounding clause they need.
func coldCacheHint(binaryName string) string {
	return fmt.Sprintf("skill catalog cache is empty — run '%s skill refresh' to populate the catalog", binaryName)
}

// loadOrRefreshCatalog implements the auto-refresh policy from REQ-F-015,
// REQ-F-018, and REQ-F-019:
//
//   - cache missing and (refresh requested OR network available)  → refresh
//   - cache present and (refresh requested OR stale > CatalogTTL) → refresh
//   - refresh succeeds → return the fresh catalog
//   - refresh fails AND cache populated → return cached + Warn
//   - refresh fails AND cache cold AND requireUsableCache → fatal
//   - refresh fails AND cache cold AND !requireUsableCache    → return (nil, nil) so caller can render a cold-cache fallback
//
// The fatal error wraps a UsageError pointing the user at
// `neo4j-cli skill refresh` per the PRD.
func loadOrRefreshCatalog(ctx context.Context, cfg *clicfg.Config, opts catalogOpts) (catalogLoad, error) {
	filesystem := cfg.Aura.Fs()

	cacheRoot, err := catalogCacheRootFn()
	if err != nil {
		return catalogLoad{}, fmt.Errorf("skill: resolve cache root: %w", err)
	}

	newOpts := catalog.Options{CacheRoot: cacheRoot, BinaryVersion: cfg.Version}
	if catalogHTTPDoer != nil {
		newOpts.Doer = catalogHTTPDoer()
	}
	cat := catalog.New(newOpts)
	cacheCold := cat.Load(filesystem) != nil

	needRefresh := opts.forceRefresh || cacheCold || catalog.Stale(filesystem, cacheRoot, CatalogTTL)
	if !needRefresh {
		return catalogLoad{Cat: cat}, nil
	}

	if rerr := cat.Refresh(ctx, filesystem); rerr != nil {
		if cacheCold {
			if opts.requireUsableCache {
				return catalogLoad{}, clierr.NewUsageError(
					"skill catalog unavailable and no local cache: %v\nrun 'neo4j-cli skill refresh' once you have network connectivity",
					rerr,
				)
			}
			return catalogLoad{Warn: rerr}, nil
		}
		return catalogLoad{Cat: cat, Warn: rerr}, nil
	}
	return catalogLoad{Cat: cat}, nil
}

// resolveSkillSource maps a positional skill-name to its Source.
// Consults the self-skill resolver first (REQ-F-021 alias map), then
// falls back to catalog.Lookup when `cat` is non-nil. Returns a
// did-you-mean-agent usage error when the name matches an agent
// (REQ-F-012) and an unknown-skill error otherwise.
//
// A nil `cat` means the caller has not loaded the catalog (print's
// offline-only mode, or a cold-cache remove). Catalog-name positionals
// then fall straight to the unknown-skill branch — the caller decides
// whether to dress that error with a refresh hint.
//
// Single resolver shared by install / print / remove so all three honour
// the same self-alias map, agent-collision hard-break, and unknown-skill
// shape.
func resolveSkillSource(bundle fs.FS, version string, cat *catalog.Catalog, filesystem afero.Fs, binaryName, skillArg string) (Source, *catalog.SkillEntry, error) {
	if skillArg == "" {
		return Source{FS: bundle, Version: version}, nil, nil
	}

	src, err := ResolveSelf(bundle, version, binaryName, skillArg)
	if err == nil {
		return src, nil, nil
	}
	if !errors.Is(err, ErrNotSelfSkill) {
		return Source{}, nil, err
	}

	if cat != nil {
		entry, sub, lookupErr := cat.Lookup(filesystem, skillArg, binaryName)
		if lookupErr == nil {
			return Source{FS: sub, Version: cat.Version}, entry, nil
		}
	}

	if isAgentName(skillArg) {
		return Source{}, nil, didYouMeanAgentErr(skillArg)
	}
	return Source{}, nil, unknownSkillErr(skillArg)
}
