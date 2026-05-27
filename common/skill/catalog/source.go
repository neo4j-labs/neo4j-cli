// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package catalog provides discovery, cache, and per-skill content access
// for the curated `neo4j-contrib/neo4j-skills` marketplace. The upstream
// publishes a single `plugin.json` (the catalog) plus a tarball of the
// repo; this package fetches both, caches them under the user cache dir,
// and exposes per-skill `fs.FS` handles to the rest of `common/skill/`.
package catalog

import "net/http"

// PluginJSONURL is the upstream URL for the marketplace metadata file.
// Hardcoded per PRD (MVP — no pluggable source flag).
const PluginJSONURL = "https://raw.githubusercontent.com/neo4j-contrib/neo4j-skills/main/.claude-plugin/plugin.json"

// TarballURL is the upstream URL for the marketplace content tarball.
// Hardcoded per PRD (MVP — no pluggable source flag).
const TarballURL = "https://codeload.github.com/neo4j-contrib/neo4j-skills/tar.gz/refs/heads/main"

// SelfCanonicalName is the reserved skill id for the binary-embedded
// self-skill. Catalog Lookup rejects any upstream skill whose name
// equals this constant.
const SelfCanonicalName = "self"

// httpDoer is the subset of net/http.Client used by Refresh. The default
// is http.DefaultClient; tests inject an httptest server via the Doer
// field on Catalog.
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// ReservedNames lists the catalog skill names that conflict with the
// embedded self-skill: the canonical `self` id and the running binary
// name. Both are forbidden from appearing in upstream `plugin.json`.
// Shared with the parent skill package's alias resolver.
func ReservedNames(binaryName string) []string {
	names := []string{SelfCanonicalName}
	if binaryName != "" && binaryName != SelfCanonicalName {
		names = append(names, binaryName)
	}
	return names
}

// IsReserved reports whether name collides with the embedded self-skill
// identity (case-sensitive — upstream skill names are lowercase forward-
// slash ids).
func IsReserved(name, binaryName string) bool {
	for _, r := range ReservedNames(binaryName) {
		if r == name {
			return true
		}
	}
	return false
}
