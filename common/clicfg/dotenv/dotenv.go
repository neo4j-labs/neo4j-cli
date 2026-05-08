// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package dotenv holds the shared `.env` discovery walk used by query and
// embed. Centralising the walk keeps the rules (where we look, where we
// stop) in one place: a future change to the boundary policy is a one-file
// edit instead of a hunt across packages.
package dotenv

import (
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

// homeDirFn is overridable for hermetic tests; production wires it to
// os.UserHomeDir. We never call os.UserHomeDir directly so callers do not
// have to t.Setenv("HOME", ...) for unit tests of unrelated code paths.
var homeDirFn = os.UserHomeDir

// SetHomeDirFnForTest overrides the internal home-directory resolver for the
// duration of a test and returns a restore function. Cross-package tests
// (query/connect_test.go, query/embed/embed_test.go) use it to inject a
// memFS-friendly $HOME without relying on t.Setenv("HOME"), which is not
// cross-platform (Windows reads USERPROFILE).
func SetHomeDirFnForTest(fn func() (string, error)) func() {
	prev := homeDirFn
	homeDirFn = fn
	return func() { homeDirFn = prev }
}

// Find walks up from startDir looking for a `.env` file. It stops the walk
// at the first ancestor containing a `.git` entry (file or dir) and at the
// $HOME directory boundary (never crossing above $HOME). The walk also
// stops at the filesystem root.
//
// Returns:
//
//   - path:            absolute path to the discovered `.env`, or "" if none
//   - found:           true when a `.env` was discovered before a stop
//   - walkedAboveCWD:  true when the discovered `.env` is in a directory
//     strictly above startDir; false when found in startDir itself or when
//     no `.env` was found. Callers use this to surface a stderr "info:
//     loading .env from <path>" line so the overlay isn't silent.
//
// The .env in the start directory wins over any ancestor; .git boundary is
// inclusive of the directory containing .git (we DO check that directory
// for .env, then stop).
func Find(fs afero.Fs, startDir string) (path string, found bool, walkedAboveCWD bool) {
	if startDir == "" {
		return "", false, false
	}

	home, _ := homeDirFn()
	// home may be "" if the env is unset; in that case we just don't apply
	// the home boundary (the .git / fs-root stops still apply).

	dir := startDir
	for {
		candidate := filepath.Join(dir, ".env")
		if exists, _ := afero.Exists(fs, candidate); exists {
			return candidate, true, dir != startDir
		}

		// Stop at .git ancestor — but only AFTER having checked this dir's
		// own .env (already done above). This means a repo-root .env is
		// found, and we don't walk above the repo root.
		if hasGit(fs, dir) {
			return "", false, false
		}

		// Stop at $HOME boundary — we check $HOME's own .env (handled by
		// the candidate-check above when dir == home), but never go above.
		if home != "" && dir == home {
			return "", false, false
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// filesystem root reached
			return "", false, false
		}
		dir = parent
	}
}

// hasGit returns true when dir contains a `.git` entry (file or directory).
// `.git` files (used by submodules and worktrees) count as a boundary just
// like a `.git` directory does.
func hasGit(fs afero.Fs, dir string) bool {
	exists, _ := afero.Exists(fs, filepath.Join(dir, ".git"))
	return exists
}
