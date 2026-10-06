// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package dataset provides the non-cobra support layer for loading example
// Neo4j datasets: a curated suggestion catalog (catalog.go), manifest
// resolution against a repo's relate.project-install.json (resolve.go), and a
// secure dump downloader (download.go).
//
// catalog.go holds an embedded, curated list of suggested datasets sourced from
// the README of neo4j-graph-examples/demo.neo4jlabs.com. The list is only a
// suggestion set surfaced by `neo4j-cli dataset list`; the load verbs accept any
// <owner/repo> carrying the manifest, so loading is not constrained to it.
package dataset

// Suggestion is one curated example dataset. OwnerRepo is the GitHub
// "<owner>/<repo>" slug passed to the per-target load verbs.
type Suggestion struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	OwnerRepo   string `json:"ownerRepo"`
}

// suggestions is the curated set. Each entry maps to a neo4j-graph-examples repo
// that carries a relate.project-install.json manifest. Verified against the
// demo.neo4jlabs.com README at implementation time.
var suggestions = []Suggestion{
	{
		Slug:        "movies",
		Title:       "Movies",
		Description: "The classic movies graph of actors, directors, and the films they made.",
		OwnerRepo:   "neo4j-graph-examples/movies",
	},
	{
		Slug:        "recommendations",
		Title:       "Movie recommendations",
		Description: "Movie ratings and metadata for building recommendation queries.",
		OwnerRepo:   "neo4j-graph-examples/recommendations",
	},
	{
		Slug:        "northwind",
		Title:       "Northwind retail",
		Description: "The Northwind product/order/customer dataset modeled as a graph.",
		OwnerRepo:   "neo4j-graph-examples/northwind",
	},
	{
		Slug:        "fincen",
		Title:       "FinCEN files",
		Description: "Suspicious-activity reports linking banks, entities, and transactions.",
		OwnerRepo:   "neo4j-graph-examples/fincen",
	},
	{
		Slug:        "twitter",
		Title:       "Twitter",
		Description: "Users, tweets, hashtags, and the interactions between them.",
		OwnerRepo:   "neo4j-graph-examples/twitter-v2",
	},
	{
		Slug:        "stackoverflow",
		Title:       "Stack Overflow",
		Description: "Questions, answers, tags, and users from Stack Overflow.",
		OwnerRepo:   "neo4j-graph-examples/stackoverflow",
	},
	{
		Slug:        "twitch",
		Title:       "Twitch",
		Description: "Twitch streamers, their teams, and viewer relationships.",
		OwnerRepo:   "neo4j-graph-examples/twitch",
	},
	{
		Slug:        "offshoreleaks",
		Title:       "ICIJ Offshore Leaks",
		Description: "The ICIJ offshore leaks investigation of entities and intermediaries.",
		OwnerRepo:   "neo4j-graph-examples/icij-offshoreleaks",
	},
	{
		Slug:        "network-management",
		Title:       "Network management",
		Description: "A telco network topology of devices, interfaces, and dependencies.",
		OwnerRepo:   "neo4j-graph-examples/network-management",
	},
	{
		Slug:        "openstreetmap",
		Title:       "OpenStreetMap",
		Description: "Geospatial OpenStreetMap data modeled as a routable graph.",
		OwnerRepo:   "neo4j-graph-examples/openstreetmap",
	},
}

// List returns the curated suggestion set. The data is embedded in the binary;
// List performs no network access.
func List() []Suggestion {
	out := make([]Suggestion, len(suggestions))
	copy(out, suggestions)
	return out
}
