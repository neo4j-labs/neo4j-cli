// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
)

// coerceDriverValue normalises driver-native values that don't round-trip
// through encoding/json into displayable forms. The five dbtype temporal
// types and Duration carry their own ISO-8601 String() formatting; native
// time.Time is rendered via RFC3339Nano. Containers ([]any, map[string]any)
// are recursed in place. Graph entities (Node, Relationship, Path) are
// recursed into their shared Props map; for Path the Nodes and Relationships
// slices are walked. Mutating the shared Props map is safe because driver
// records aren't reused after result.Collect. All other types pass through
// unchanged.
func coerceDriverValue(v any) any {
	switch val := v.(type) {
	case dbtype.Date:
		return val.String()
	case dbtype.LocalDateTime:
		return val.String()
	case dbtype.LocalTime:
		return val.String()
	case dbtype.Time:
		return val.String()
	case dbtype.Duration:
		return val.String()
	case time.Time:
		return val.Format(time.RFC3339Nano)
	case []any:
		for i, item := range val {
			val[i] = coerceDriverValue(item)
		}
		return val
	case map[string]any:
		for k, item := range val {
			val[k] = coerceDriverValue(item)
		}
		return val
	case dbtype.Node:
		for k, item := range val.Props {
			val.Props[k] = coerceDriverValue(item)
		}
		return val
	case dbtype.Relationship:
		for k, item := range val.Props {
			val.Props[k] = coerceDriverValue(item)
		}
		return val
	case dbtype.Path:
		for i := range val.Nodes {
			for k, item := range val.Nodes[i].Props {
				val.Nodes[i].Props[k] = coerceDriverValue(item)
			}
		}
		for i := range val.Relationships {
			for k, item := range val.Relationships[i].Props {
				val.Relationships[i].Props[k] = coerceDriverValue(item)
			}
		}
		return val
	default:
		return val
	}
}
