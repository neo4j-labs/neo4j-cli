// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// transportImport is the Aura HTTP transport. Commands reach Aura only through
// the service layer (internal/auraclient), so the transport can be replaced — by the
// Aura SDK — in one place.
const transportImport = "github.com/neo4j/cli/internal/auraclient/transport"

// passthroughDir is the one command allowed to use the transport directly: `aura
// api` makes arbitrary requests to endpoints the CLI has no model of, so it
// cannot go through typed services.
var passthroughDir = filepath.Join("commands", "aura", "api")

// TestCommandsImportTheServiceLayerNotTheTransport fails if any command package
// imports the Aura transport. An import-level rule is deliberately stricter than
// banning individual calls: it is visible in the import block of every file.
func TestCommandsImportTheServiceLayerNotTheTransport(t *testing.T) {
	root := filepath.Join("..", "..", "commands")
	fset := token.NewFileSet()
	checked := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		checked++

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		require.NoError(t, err, path)

		allowed := strings.Contains(filepath.ToSlash(path), filepath.ToSlash(passthroughDir)+"/")
		for _, imp := range file.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == transportImport && !allowed {
				assert.Failf(t, "transport imported by a command",
					"%s imports %s — use the internal/auraclient service layer instead (see AGENTS.md, \"Aura service layer\")",
					fset.Position(imp.Pos()), transportImport)
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, checked, "the walk found no command files; the test root is wrong")
}
