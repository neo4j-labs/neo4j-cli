// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"fmt"
	"strings"
)

// defaultGraphQLName returns the lowest GraphQLNN name that does not
// case-insensitively match any name in existingNames. Names in the range
// 1–99 are zero-padded to two digits (GraphQL01 … GraphQL99); names at
// 100 and above use their full decimal representation (GraphQL100, …).
func defaultGraphQLName(existingNames []string) string {
	taken := make(map[string]bool, len(existingNames))
	for _, n := range existingNames {
		taken[strings.ToLower(n)] = true
	}
	for i := 1; ; i++ {
		var candidate string
		if i < 100 {
			candidate = fmt.Sprintf("GraphQL%02d", i)
		} else {
			candidate = fmt.Sprintf("GraphQL%d", i)
		}
		if !taken[strings.ToLower(candidate)] {
			return candidate
		}
	}
}
