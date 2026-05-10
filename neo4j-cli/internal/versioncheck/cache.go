// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package versioncheck implements the silent background "is there a newer
// neo4j-cli release?" probe and the resulting one-line stderr nag.
//
// cache.go owns the on-disk cache (JSON file in the OS config dir alongside
// config.json / credentials.json). The cache is intentionally tiny and
// stable-only: power users on prerelease tags can still run
// `neo4j-cli update --check --pre-releases` themselves.
package versioncheck

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/spf13/afero"
)

// cacheFileName is the basename of the cache file. It lives in the same dir
// as config.json / credentials.json (clicfg.ConfigPrefix/neo4j/cli/).
const cacheFileName = "version-check.json"

// CacheTTL controls how long a cached value is considered fresh. After this
// window the goroutine will refetch on its next dice-roll hit.
const CacheTTL = 24 * time.Hour

// cacheEntry is the on-disk shape of the cache file. The exported field
// names match the documented JSON keys.
type cacheEntry struct {
	CheckedAt    time.Time `json:"checked_at"`
	LatestStable string    `json:"latest_stable"`
}

// cachePath returns the absolute path to the cache file. clicfg.ConfigPrefix
// is set per-OS in clicfg/{darwin,linux,windows}.go.
func cachePath() string {
	return filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", cacheFileName)
}

// readCache returns the cached entry, or nil when the file is absent,
// unreadable, or malformed. Every error path returns nil — a corrupt cache
// must never break the foreground command.
func readCache(fs afero.Fs) *cacheEntry {
	if fs == nil {
		return nil
	}
	raw, err := afero.ReadFile(fs, cachePath())
	if err != nil {
		// errors.Is(err, os.ErrNotExist) is the common case; everything
		// else (perm denied, IO error, etc.) is also silently treated as
		// "no cache".
		_ = errors.Is(err, os.ErrNotExist)
		return nil
	}
	var e cacheEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil
	}
	if e.LatestStable == "" {
		return nil
	}
	return &e
}

// writeCache persists the entry. Errors are swallowed — a write failure must
// never break the foreground command. The file is written atomically via
// fileutils.WriteFile (mode 0600); fileutils panics on failure so we trap.
func writeCache(fs afero.Fs, e cacheEntry) {
	if fs == nil {
		return
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return
	}
	defer func() {
		// fileutils.WriteFile panics on failure (matches the rest of the
		// codebase). The check goroutine must NEVER take down the host
		// process, so trap the panic here and discard.
		_ = recover()
	}()
	fileutils.WriteFile(fs, cachePath(), raw)
}

// fresh reports whether the cache entry is younger than CacheTTL.
func (e *cacheEntry) fresh(now time.Time) bool {
	if e == nil {
		return false
	}
	return now.Sub(e.CheckedAt) < CacheTTL
}
