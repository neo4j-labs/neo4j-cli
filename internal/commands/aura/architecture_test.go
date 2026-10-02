// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"go/ast"
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

const apiImport = "github.com/neo4j/cli/internal/aura/api"

// transportSymbols are the parts of internal/aura/api that talk to the Aura API
// or know its URL layout. Commands reach Aura only through internal/aura, so
// that the transport can be replaced (by the Aura SDK) in one place.
//
// Deliberately NOT banned: the response-data types and constructors commands
// render with, the status/type constants, and MakeRawRequest, which backs the
// `aura api` passthrough that by design targets endpoints the CLI has no model
// of.
var transportSymbols = map[string]bool{
	"MakeRequest":                    true,
	"ListAllPages":                   true,
	"Poll":                           true,
	"PollWithVersion":                true,
	"PollInstance":                   true,
	"PollSnapshot":                   true,
	"PollCMK":                        true,
	"PollGraphQLDataApi":             true,
	"PollGraphAnalyticsSessionReady": true,
	"PollVirtualGraph":               true,
}

func TestCommandsDoNotCallTheAuraTransportDirectly(t *testing.T) {
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
		file, err := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, err, path)

		local := ""
		for _, imp := range file.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == apiImport {
				local = "api"
				if imp.Name != nil {
					local = imp.Name.Name
				}
			}
		}
		if local == "" {
			return nil
		}
		checked++

		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == local {
				name := sel.Sel.Name
				if transportSymbols[name] || strings.HasPrefix(name, "Scoped") {
					assert.Failf(t, "transport used from a command",
						"%s: %s.%s — call the internal/aura service instead (see AGENTS.md, \"Aura service layer\")",
						fset.Position(sel.Pos()), local, name)
				}
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, checked, "the walk found no command files importing internal/aura/api; the test root is wrong")
}
