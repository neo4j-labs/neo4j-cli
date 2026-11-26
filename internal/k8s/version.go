// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"strconv"
	"strings"
)

// ParseMajorMinor extracts the major and minor numbers from a git version
// such as "v1.35.2", "1.35" or "v3.16.4+g8e8f4e2". Build metadata after a
// plus sign is ignored. The third return value is false when the string
// does not start with a major.minor pair.
func ParseMajorMinor(v string) (major, minor int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "+")
	majorStr, rest, found := strings.Cut(v, ".")
	if !found {
		return 0, 0, false
	}
	minorStr, _, _ := strings.Cut(rest, ".")
	major, err := strconv.Atoi(majorStr)
	if err != nil {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(minorStr)
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// MajorMinorAtLeast reports whether version v is at least wantMajor.wantMinor.
// An unparseable version is never at least anything, so callers fail loud.
func MajorMinorAtLeast(v string, wantMajor, wantMinor int) bool {
	major, minor, ok := ParseMajorMinor(v)
	if !ok {
		return false
	}
	if major != wantMajor {
		return major > wantMajor
	}
	return minor >= wantMinor
}

// MinorSkew returns the signed minor-version distance between two git
// versions of the same major, client minus server. A positive value means
// the client is newer. The second return value is false when either version
// is unparseable or the majors differ, because a cross-major comparison has
// no skew policy.
func MinorSkew(clientV, serverV string) (skew int, ok bool) {
	cMajor, cMinor, cOK := ParseMajorMinor(clientV)
	sMajor, sMinor, sOK := ParseMajorMinor(serverV)
	if !cOK || !sOK || cMajor != sMajor {
		return 0, false
	}
	return cMinor - sMinor, true
}
