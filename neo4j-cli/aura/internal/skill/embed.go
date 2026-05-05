// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package skill embeds the generated aura-cli agent-skill bundle so the
// `aura-cli skill install` command can write it onto the host filesystem.
//
// Regenerate the bundle with `go generate ./neo4j-cli/aura/internal/skill/...`
// (or `make generate`). The committed bundle/ tree is the source of truth
// shipped in the binary; CI runs `make generate-check` to fail on drift.
//
// Edit description.txt and additions.md (sibling files) to change the
// frontmatter description and the SKILL.md "Gotchas" section. The
// remainder of SKILL.md and every references/<sub>.md is derived from
// the cobra tree built by aura.NewStandaloneCmd.
package skill

import (
	"embed"
	"io/fs"
)

//go:generate go run ./gen

// rawBundle is the raw embed.FS rooted ABOVE the bundle/ directory. Do not
// expose this directly to the installer — `fs.WalkDir(rawBundle, ".")` would
// yield `bundle/SKILL.md`, not `SKILL.md`, and the installer assumes the
// flat layout produced by render.Bundle.
//
//go:embed bundle
var rawBundle embed.FS

// Bundle is the agent-skill bundle rooted at the bundle/ contents, so
// `fs.WalkDir(Bundle, ".")` yields `SKILL.md` and `references/<sub>.md`
// at the root — the flat layout the installer expects.
var Bundle fs.FS = mustSub(rawBundle, "bundle")

// mustSub wraps fs.Sub and panics on error. The only failure modes are
// programmer errors (invalid path), so a panic at init time is acceptable.
func mustSub(parent fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(parent, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
