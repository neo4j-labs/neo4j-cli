// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dataset

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

// canonicalVersion normalises a Neo4j version string into the leading-"v" form
// golang.org/x/mod/semver requires. Aura/calver versions like "2025.01.0" are
// passed through unchanged structurally — semver treats the leading component
// as MAJOR, which preserves ordering for comparison purposes here.
func canonicalVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return semver.Canonical(v)
}

// rangeMatches reports whether target satisfies the manifest's
// targetNeo4jVersion expression and returns the range's effective lower bound
// (for newest-compatible tie-breaking). The expression is npm-style: one or
// more whitespace-separated comparators, all of which must hold (logical AND).
// Each comparator is a >=, <=, >, <, or = operator followed by a version, or a
// bare version (treated as =). target must already be canonical (leading "v").
func rangeMatches(expr, target string) (bool, string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false, "", fmt.Errorf("empty version range")
	}

	lower := "v0.0.0"
	for _, tok := range strings.Fields(expr) {
		op, ver := splitComparator(tok)
		cv := canonicalVersion(ver)
		if !semver.IsValid(cv) {
			return false, "", fmt.Errorf("invalid version %q in range %q", ver, expr)
		}
		cmp := semver.Compare(target, cv)
		switch op {
		case ">=":
			if cmp < 0 {
				return false, "", nil
			}
			if semver.Compare(cv, lower) > 0 {
				lower = cv
			}
		case ">":
			if cmp <= 0 {
				return false, "", nil
			}
			if semver.Compare(cv, lower) > 0 {
				lower = cv
			}
		case "<=":
			if cmp > 0 {
				return false, "", nil
			}
		case "<":
			if cmp >= 0 {
				return false, "", nil
			}
		case "=":
			if cmp != 0 {
				return false, "", nil
			}
			if semver.Compare(cv, lower) > 0 {
				lower = cv
			}
		default:
			return false, "", fmt.Errorf("unsupported operator %q in range %q", op, expr)
		}
	}
	return true, lower, nil
}

// splitComparator separates a comparator token into its operator and version.
// A bare version (no operator prefix) yields the "=" operator.
func splitComparator(tok string) (op, ver string) {
	for _, o := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(tok, o) {
			return o, strings.TrimSpace(strings.TrimPrefix(tok, o))
		}
	}
	return "=", tok
}
