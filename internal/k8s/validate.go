// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"os"
	"path/filepath"

	"github.com/neo4j/cli/internal/clierr"
)

// validateBinaryFn is the seam behind binary validation. Tests replace it so
// the fake paths the lookPath seams return do not hit the real filesystem.
var validateBinaryFn = validateBinary

// validateBinary makes sure that a kubectl or helm path found on PATH is the
// intended binary and not a substitute planted through PATH manipulation
// (CWE-426, untrusted search path). The path must be absolute. It must
// resolve through any symlinks to a regular executable file. It must sit in
// a location a local attacker cannot write to (see validateLocation).
//
// Hash or signature pinning is not feasible here: kubectl and helm ship many
// versions from many distributors, so no known-good digest exists to compare
// against. The location and ownership rules are the practical control.
func validateBinary(name, path string) error {
	if !filepath.IsAbs(path) {
		return untrustedBinaryErr(name, path, "the path is not absolute, so PATH contains a relative entry")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return untrustedBinaryErr(name, path, "the path cannot be resolved")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return untrustedBinaryErr(name, path, "the resolved file cannot be read")
	}
	if !info.Mode().IsRegular() {
		return untrustedBinaryErr(name, path, "the resolved path is not a regular file")
	}
	if info.Mode().Perm()&0o111 == 0 {
		return untrustedBinaryErr(name, path, "the resolved file is not executable")
	}
	return validateLocation(name, resolved, info)
}

// untrustedBinaryErr builds the shared refusal for a binary that fails
// validation.
func untrustedBinaryErr(name, path, reason string) error {
	return clierr.NewUsageError(
		"refusing to run %s at %s: %s — reinstall %s into a trusted directory such as /usr/local/bin or fix your PATH",
		name, path, reason, name,
	)
}
