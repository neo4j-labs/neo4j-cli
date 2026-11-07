// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"regexp"
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

// releaseVersionPattern matches an operator release version: a semver
// core with an optional pre-release suffix, and no build metadata.
var releaseVersionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z.-]+)?$`)

// NormaliseReleaseVersion returns an operator release version without a
// leading "v", so "v1.0.0" and "1.0.0" both name the asset tag v1.0.0. The
// second return value is false when v is not a release version at all.
func NormaliseReleaseVersion(v string) (string, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	return v, releaseVersionPattern.MatchString(v)
}

// CompareReleaseVersions compares two operator release versions and returns
// -1, 0 or 1. A pre-release sorts before its release; two pre-releases of
// the same core compare as text. The second return value is false when
// either version is unparseable, so a caller can skip the comparison.
func CompareReleaseVersions(a, b string) (int, bool) {
	aParts := releaseVersionPattern.FindStringSubmatch(strings.TrimPrefix(a, "v"))
	bParts := releaseVersionPattern.FindStringSubmatch(strings.TrimPrefix(b, "v"))
	if aParts == nil || bParts == nil {
		return 0, false
	}
	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(aParts[i])
		y, _ := strconv.Atoi(bParts[i])
		if x != y {
			if x < y {
				return -1, true
			}
			return 1, true
		}
	}
	aPre, bPre := aParts[4], bParts[4]
	switch {
	case aPre == bPre:
		return 0, true
	case aPre == "":
		return 1, true
	case bPre == "":
		return -1, true
	default:
		return strings.Compare(aPre, bPre), true
	}
}
