// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Command gen regenerates the aura-cli agent-skill bundle.
//
// It builds the aura-cli standalone cobra tree via aura.NewStandaloneCmd,
// walks it with common/skill/render.Bundle, and writes SKILL.md +
// references/<sub>.md into <package-dir>/bundle/. The bundle is committed
// to the repo and embedded into the binary via embed.go.
//
// NewStandaloneCmd (NOT NewCmd) is used so the generated bundle reflects
// the standalone binary's full surface — including `credential` and (after
// task-009 mounts it) `skill`. The super-CLI's nested `aura` subtree
// generates a separate bundle from neo4j-cli/internal/skill/.
//
// Inputs (sibling files under neo4j-cli/aura/internal/skill/):
//   - description.txt: frontmatter `description` (third-person, ≤1024 chars).
//   - additions.md:    gotchas inlined under SKILL.md's "Gotchas" heading.
//
// Invocation:
//   - `go generate ./neo4j-cli/aura/internal/skill/...` (preferred — runs
//     embed.go's //go:generate directive).
//   - `go run ./neo4j-cli/aura/internal/skill/gen` (direct invocation).
//
// CI runs `make generate-check` to assert the committed bundle matches
// what the generator produces from the current cobra tree.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/afero"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/skill/render"
	"github.com/neo4j/cli/neo4j-cli/aura"
)

const skillName = "aura-cli"

func main() {
	pkgDir, err := packageDir()
	if err != nil {
		fail(err)
	}
	if err := generate(pkgDir); err != nil {
		fail(err)
	}
}

// packageDir returns the directory containing this gen/main.go file's
// parent package (neo4j-cli/aura/internal/skill). Resolved via runtime.Caller
// so the generator works regardless of the caller's CWD.
func packageDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("gen: cannot resolve runtime.Caller")
	}
	// thisFile = .../neo4j-cli/aura/internal/skill/gen/main.go
	// parent of gen/ is the skill package directory.
	return filepath.Dir(filepath.Dir(thisFile)), nil
}

// generate writes the bundle into <pkgDir>/bundle/. Removes any existing
// bundle/ first so stale references files don't linger.
func generate(pkgDir string) error {
	descPath := filepath.Join(pkgDir, "description.txt")
	additionsPath := filepath.Join(pkgDir, "additions.md")

	desc, err := os.ReadFile(descPath)
	if err != nil {
		return fmt.Errorf("read description.txt: %w", err)
	}
	additions, err := os.ReadFile(additionsPath)
	if err != nil {
		return fmt.Errorf("read additions.md: %w", err)
	}

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "dev", clicfg.SkillsScope)
	root := aura.NewStandaloneCmd(cfg)

	files, err := render.Bundle(root, render.Options{
		Name:        skillName,
		Description: string(desc),
		Additions:   string(additions),
	})
	if err != nil {
		return fmt.Errorf("render bundle: %w", err)
	}

	bundleDir := filepath.Join(pkgDir, "bundle")
	if err := os.RemoveAll(bundleDir); err != nil {
		return fmt.Errorf("clean bundle dir: %w", err)
	}
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return fmt.Errorf("create bundle dir: %w", err)
	}

	for relPath, data := range files {
		dest := filepath.Join(bundleDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(dest), err)
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
	}
	return nil
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "aura-cli skill gen: %v\n", err)
	os.Exit(1)
}
