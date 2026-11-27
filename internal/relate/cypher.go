// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import "strings"

// QuoteIdent renders a label or relationship type as a Cypher identifier,
// backticked with internal backticks doubled. Always backticking (not just
// when the name needs it) keeps the rendering uniform and safe for any
// spelling the engine can store.
func QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
