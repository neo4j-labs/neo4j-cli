// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/spf13/afero"
)

// SkillEntry is a single marketplace skill: a name (used as the on-disk
// install dir and the addressable id) plus the upstream path from
// `plugin.json.skills[]` (e.g. `./neo4j-cypher-skill`). The name is
// derived from Path's base segment.
type SkillEntry struct {
	Name string
	Path string
}

// Catalog is the in-memory view of the cached marketplace metadata. It
// carries the cacheRoot so callers can resolve per-skill content paths
// via Lookup without re-deriving the cache location.
type Catalog struct {
	Version   string
	Skills    []SkillEntry
	cacheRoot string
}

// pluginJSON mirrors the upstream marketplace metadata shape. Only the
// fields this package consumes are decoded — extras (description, author,
// homepage, license, userConfig, etc.) are ignored.
type pluginJSON struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Skills  []string `json:"skills"`
}

// Load reads the cached `plugin.json` from `cacheRoot` and returns a
// populated Catalog. Does not touch the network. Returns an error
// wrapping fs.ErrNotExist when the cache is cold so callers can
// distinguish "no cache" from a malformed cache.
func Load(filesystem afero.Fs, cacheRoot string) (*Catalog, error) {
	if cacheRoot == "" {
		return nil, errors.New("catalog: empty cache root")
	}

	data, err := afero.ReadFile(filesystem, pluginJSONPath(cacheRoot))
	if err != nil {
		return nil, fmt.Errorf("catalog: read plugin.json: %w", err)
	}

	var pj pluginJSON
	if jerr := json.Unmarshal(data, &pj); jerr != nil {
		return nil, fmt.Errorf("catalog: parse plugin.json: %w", jerr)
	}

	skills := make([]SkillEntry, 0, len(pj.Skills))
	for _, p := range pj.Skills {
		name := skillNameFromPath(p)
		if name == "" {
			continue
		}
		skills = append(skills, SkillEntry{Name: name, Path: p})
	}

	return &Catalog{
		Version:   pj.Version,
		Skills:    skills,
		cacheRoot: cacheRoot,
	}, nil
}

// Stale reports whether the cached `plugin.json` is missing or older
// than `ttl`. A missing cache or missing/unparseable `fetched-at`
// timestamp is treated as stale.
func Stale(filesystem afero.Fs, cacheRoot string, ttl time.Duration) bool {
	if cacheRoot == "" {
		return true
	}
	data, err := afero.ReadFile(filesystem, fetchedAtPath(cacheRoot))
	if err != nil {
		return true
	}
	ts, perr := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if perr != nil {
		return true
	}
	return nowFn().Sub(ts) >= ttl
}

// skillNameFromPath derives the skill id from an upstream `plugin.json`
// path entry. Entries look like `./neo4j-cypher-skill`; the id is the
// final path segment with any leading `./` stripped. Forward-slash only
// (upstream JSON convention); never touches OS separators.
func skillNameFromPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.TrimPrefix(p, "./")
	return path.Base(p)
}
