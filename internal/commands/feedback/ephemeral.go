// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"os"
	"path/filepath"
	"strings"
)

// tempDir is a seam for tests.
var tempDir = os.TempDir

// ephemeralKind says whether path is in a place that a package manager or the
// operating system cleans up on its own, and names it for the warning ("" means
// no: a normal install). Registering such a path with an agent works at first and
// then breaks when the place is cleaned: for example `uv cache clean` removes the
// environment that `uvx neo4j-cli ...` runs from.
//
// It matches on path segments, so it works for Windows paths on any OS the tests
// run on. It is a heuristic for the common cases, not a list of every tool.
func ephemeralKind(path string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	segs := strings.FieldsFunc(strings.ToLower(clean), func(r rune) bool { return r == '/' || r == '\\' })

	seenUV, seenPipx, seenPnpm, seenBun := false, false, false, false
	for _, s := range segs {
		switch {
		case s == "_npx":
			return "the npx cache"
		case s == "pnpm":
			seenPnpm = true
		case seenPnpm && s == "dlx":
			return "the pnpm cache (what `pnpm dlx` runs from)"
		case s == ".bun":
			seenBun = true
		case seenBun && s == "cache":
			return "the bun cache (what `bunx` runs from)"
		case s == "uv":
			seenUV = true
		case seenUV && (strings.HasPrefix(s, "archive-v") || strings.HasPrefix(s, "environments-v")):
			// ~/.cache/uv/archive-v0/<hash>/..., %LOCALAPPDATA%\uv\cache\archive-v0\..., uv's cache of ephemeral environments.
			// Not ~/.local/share/uv/tools/..., which is `uv tool install` and is permanent.
			return "the uv cache (what `uvx` runs from)"
		case s == "pipx":
			seenPipx = true
		case seenPipx && s == ".cache":
			return "the pipx cache (what `pipx run` runs from)"
		}
	}
	if underDir(clean, tempDir()) {
		return "a temporary folder"
	}
	return ""
}

// underDir reports whether path is inside dir, comparing real paths when they can be resolved
// (macOS temp folders are reached through /var -> /private/var).
func underDir(path, dir string) bool {
	if dir == "" {
		return false
	}
	candidates := []string{filepath.Clean(dir)}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		candidates = append(candidates, real)
	}
	for _, d := range candidates {
		p := path
		if goos == "windows" {
			// Windows paths are case-insensitive.
			p, d = strings.ToLower(p), strings.ToLower(d)
		}
		if p == d || strings.HasPrefix(p, d+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// ephemeralWarning is the note shown by `register` and `enable` when the registered path looks temporary.
// It must not contain the phrase "registration failed" (the install script looks for it) or an agent name in quotes.
func ephemeralWarning(path, kind string) string {
	return "Warning: this neo4j-cli runs from " + kind + " (" + path + "). The feedback tool is registered at that path, " +
		"so it will stop working when the location is cleaned up (for example by `uv cache clean`). " +
		"Install neo4j-cli properly (`uv tool install neo4j-cli`, `pipx install neo4j-cli`, `npm i -g @neo4j-labs/cli`, " +
		"`brew install neo4j-labs/tap/neo4j-cli` or the install script), then run `neo4j-cli feedback register` again from that copy."
}
