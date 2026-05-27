// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package catalog provides discovery, cache, and per-skill content access
// for the curated `neo4j-contrib/neo4j-skills` marketplace. The upstream
// publishes a single `plugin.json` (the catalog) plus a tarball of the
// repo; this package fetches both, caches them under the user cache dir,
// and exposes per-skill `fs.FS` handles to the rest of `common/skill/`.
package catalog

// PluginJSONURL is the upstream URL for the marketplace metadata file.
// Hardcoded per PRD (MVP — no pluggable source flag).
const PluginJSONURL = "https://raw.githubusercontent.com/neo4j-contrib/neo4j-skills/main/.claude-plugin/plugin.json"

// TarballURL is the upstream URL for the marketplace content tarball.
// Hardcoded per PRD (MVP — no pluggable source flag).
const TarballURL = "https://codeload.github.com/neo4j-contrib/neo4j-skills/tar.gz/refs/heads/main"
