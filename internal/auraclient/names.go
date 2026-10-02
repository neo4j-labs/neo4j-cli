// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import (
	"fmt"
	"strings"
)

// DefaultName returns the lowest <prefix>NN name that does not
// case-insensitively match any name in existing. Numbers 1–99 are zero-padded
// to two digits (Instance01 … Instance99); 100 and above use their full decimal
// representation (Instance100, …).
func DefaultName(prefix string, existing []string) string {
	taken := make(map[string]bool, len(existing))
	for _, n := range existing {
		taken[strings.ToLower(n)] = true
	}
	for i := 1; ; i++ {
		var candidate string
		if i < 100 {
			candidate = fmt.Sprintf("%s%02d", prefix, i)
		} else {
			candidate = fmt.Sprintf("%s%d", prefix, i)
		}
		if !taken[strings.ToLower(candidate)] {
			return candidate
		}
	}
}
