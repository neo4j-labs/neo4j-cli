// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"
	"strings"
)

// defaultInstanceName returns the lowest InstanceNN name that does not
// case-insensitively match any name in existingNames. Names in the range
// 1–99 are zero-padded to two digits (Instance01 … Instance99); names at
// 100 and above use their full decimal representation (Instance100, …).
func defaultInstanceName(existingNames []string) string {
	taken := make(map[string]bool, len(existingNames))
	for _, n := range existingNames {
		taken[strings.ToLower(n)] = true
	}
	for i := 1; ; i++ {
		var candidate string
		if i < 100 {
			candidate = fmt.Sprintf("Instance%02d", i)
		} else {
			candidate = fmt.Sprintf("Instance%d", i)
		}
		if !taken[strings.ToLower(candidate)] {
			return candidate
		}
	}
}
