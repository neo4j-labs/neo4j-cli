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
	// Calver months are zero-padded (2026.05.0), but semver rejects leading
	// zeros in numeric components — strip them so calver targets parse.
	v = stripLeadingZeros(v)
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return semver.Canonical(v)
}

// stripLeadingZeros removes leading zeros from each dot-separated all-digit
// component (2026.05.0 -> 2026.5.0) so calver strings satisfy semver, which
// forbids leading zeros. Non-numeric components are left untouched.
func stripLeadingZeros(v string) string {
	prefix := ""
	if strings.HasPrefix(v, "v") {
		prefix, v = "v", v[1:]
	}
	parts := strings.Split(v, ".")
	for i, p := range parts {
		if !allDigits(p) {
			continue
		}
		trimmed := strings.TrimLeft(p, "0")
		if trimmed == "" {
			trimmed = "0"
		}
		parts[i] = trimmed
	}
	return prefix + strings.Join(parts, ".")
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isCalver reports whether a canonical target version is calendar-versioned
// neo4j (year >= 2025). Calver is the continuation of the 5.x line, so its
// semver MAJOR component sorts at/above 2025.
func isCalver(target string) bool {
	return semver.Compare(target, "v2025.0.0") >= 0
}

// rangeLowerBound returns the effective lower bound of a manifest range
// expression (the highest >=/>/= comparator version), independent of any
// target. Used to classify an entry as a 5.x-or-newer dump for calver/latest
// matching. Defaults to v0.0.0 when the range has only upper bounds.
func rangeLowerBound(expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", fmt.Errorf("empty version range")
	}
	lower := "v0.0.0"
	for _, tok := range strings.Fields(expr) {
		op, ver := splitComparator(tok)
		cv := canonicalVersion(ver)
		if !semver.IsValid(cv) {
			return "", fmt.Errorf("invalid version %q in range %q", ver, expr)
		}
		switch op {
		case ">=", ">", "=":
			if semver.Compare(cv, lower) > 0 {
				lower = cv
			}
		}
	}
	return lower, nil
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
