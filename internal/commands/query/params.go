// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"github.com/neo4j/cli/internal/params"
)

// EmbedJob describes a single `--param NAME:embed=TEXT` entry. The parsing
// lives in the shared internal/params package (also used by the relate
// tree); this alias keeps the query package's call sites unchanged.
type EmbedJob = params.EmbedJob

// parseParams converts a slice of `key=value` entries (from `--param`) into
// a map of Cypher query parameters plus a slice of pending embed jobs. See
// params.ParseParams for the parsing contract.
func parseParams(raw []string) (map[string]any, []EmbedJob, error) {
	return params.ParseParams(raw)
}
