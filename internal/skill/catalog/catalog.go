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

// Options carries the constructor-time dependencies for a Catalog: the
// on-disk cache root, the optional HTTP client (nil = http.DefaultClient
// at Refresh time), and the running binary version (used for the
// outgoing User-Agent header).
type Options struct {
	CacheRoot     string
	Doer          HTTPDoer
	BinaryVersion string
}

// Catalog is the cached marketplace view plus the deps Refresh needs.
// Version + Skills are the in-memory data; cacheRoot / doer /
// binaryVersion are dependencies wired at construction via New.
type Catalog struct {
	Version       string
	Skills        []SkillEntry
	cacheRoot     string
	doer          HTTPDoer
	binaryVersion string
}

// New constructs a Catalog with the given dependencies. Version + Skills
// start empty; call Load to read the cached `plugin.json`, or Refresh to
// fetch upstream and re-extract.
func New(opts Options) *Catalog {
	return &Catalog{
		cacheRoot:     opts.CacheRoot,
		doer:          opts.Doer,
		binaryVersion: opts.BinaryVersion,
	}
}

// pluginJSON mirrors the upstream marketplace metadata shape. Only the
// fields this package consumes are decoded — extras (description, author,
// homepage, license, userConfig, etc.) are ignored.
type pluginJSON struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Skills  []string `json:"skills"`
}

// Load reads the cached `plugin.json` from the receiver's cache root and
// populates Version + Skills. Does not touch the network. Returns an
// error wrapping fs.ErrNotExist when the cache is cold so callers can
// distinguish "no cache" from a malformed cache.
func (c *Catalog) Load(filesystem afero.Fs) error {
	if c == nil {
		return errors.New("catalog: nil receiver")
	}
	if c.cacheRoot == "" {
		return errors.New("catalog: empty cache root")
	}

	data, err := afero.ReadFile(filesystem, pluginJSONPath(c.cacheRoot))
	if err != nil {
		return fmt.Errorf("catalog: read plugin.json: %w", err)
	}

	var pj pluginJSON
	if jerr := json.Unmarshal(data, &pj); jerr != nil {
		return fmt.Errorf("catalog: parse plugin.json: %w", jerr)
	}
	if !ValidSkillName(pj.Version) {
		return fmt.Errorf("catalog: cached plugin.json has unsafe version %q", pj.Version)
	}

	c.Version = pj.Version
	c.Skills = skillEntriesFromPluginJSON(pj.Skills)
	return nil
}

// Stale reports whether the cached `plugin.json` is missing or older
// than `ttl`. A missing cache or missing/unparseable `fetched-at`
// timestamp is treated as stale. Free-standing because it has no need
// for the deps a Catalog carries — it's a pure filesystem probe.
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

// skillEntriesFromPluginJSON converts the raw upstream string list into
// the in-memory SkillEntry slice. The single conversion path used by
// both Load and Refresh.
//
// Entries whose derived name fails ValidSkillName are SILENTLY DROPPED.
// This is defense against a compromised upstream catalog publishing
// path-traversal names (e.g. ".."): such a name would otherwise reach
// Install where filepath.Join(skillsRoot, name) escapes skillsRoot and
// the cleanup RemoveAll deletes the parent directory.
func skillEntriesFromPluginJSON(paths []string) []SkillEntry {
	out := make([]SkillEntry, 0, len(paths))
	for _, p := range paths {
		name := skillNameFromPath(p)
		if !ValidSkillName(name) {
			continue
		}
		out = append(out, SkillEntry{Name: name, Path: p})
	}
	return out
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

// ValidSkillName reports whether name is safe to use as an on-disk skill
// directory id. Rejects empty (after TrimSpace) / "." / ".." / names
// containing '/' or '\\' / names containing any whitespace (surrounding
// or embedded space, tab, newline, carriage return) / names starting
// with '.' / names containing NUL bytes. The catalog package applies it
// inside skillEntriesFromPluginJSON (so upstream-supplied names are
// filtered at the conversion boundary); the parent skill package
// re-checks it inside Install as defense-in-depth against a bypass.
func ValidSkillName(name string) bool {
	if strings.TrimSpace(name) != name || strings.ContainsAny(name, " \t\n\r") {
		return false
	}
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return false
	}
	if name[0] == '.' {
		return false
	}
	return true
}
