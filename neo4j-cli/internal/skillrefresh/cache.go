// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package skillrefresh implements the silent background "refresh installed agent
// skills when the binary version changes" probe. It writes a tiny JSON state
// file (skill-refresh.json) alongside config.json / credentials.json so the
// next invocation can compare the recorded version to the current binary version
// and decide whether a refresh is needed.
//
// cache.go owns the on-disk state file I/O. All errors are silently swallowed —
// a corrupt or absent cache is treated as "no prior record", triggering a
// refresh on the next invocation.
package skillrefresh

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/spf13/afero"
)

// cacheFileName is the basename of the state file. It lives in the same dir
// as config.json / credentials.json (clicfg.ConfigPrefix/neo4j/cli/).
const cacheFileName = "skill-refresh.json"

// cacheEntry is the on-disk shape of the state file.
type cacheEntry struct {
	LastRefreshedVersion string `json:"last_refreshed_version"`
}

// cachePath returns the absolute path to the state file. clicfg.ConfigPrefix
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
	if e.LastRefreshedVersion == "" {
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
		// codebase). The refresh goroutine must NEVER take down the host
		// process, so trap the panic here and discard.
		_ = recover()
	}()
	fileutils.WriteFile(fs, cachePath(), raw)
}
