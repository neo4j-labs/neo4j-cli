// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package update implements the `neo4j-cli update` self-update command.
//
// This file currently only seeds the dependency on golang.org/x/mod/semver
// so it appears in the direct require block of go.mod. Subsequent tasks
// (release lookup, install-method detection, atomic swap, RunE wiring)
// will flesh out the package.
package update

import (
	"golang.org/x/mod/semver"
)

// isValidSemver wraps semver.IsValid. Exported via the package-level var
// below so the linter does not flag the symbol as unused while the rest of
// the update package is still being scaffolded.
func isValidSemver(v string) bool {
	return semver.IsValid(v)
}

// _ retains a reference to isValidSemver so `unused` linter is satisfied
// before the real call sites land in subsequent tasks.
var _ = isValidSemver
