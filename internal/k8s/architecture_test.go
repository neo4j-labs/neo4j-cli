// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

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

// TestNoKubernetesOrHelmSDKImports keeps the CLI on host binaries. The rule
// covers production and test files, so a typed client cannot enter gradually.
func TestNoKubernetesOrHelmSDKImports(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	checked := 0
	forbiddenPrefixes := []string{"k8s.io/", "sigs.k8s.io/", "helm.sh/"}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "bin", "dist", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		checked++
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		require.NoError(t, err, path)
		for _, imported := range file.Imports {
			importPath, _ := strconv.Unquote(imported.Path.Value)
			for _, prefix := range forbiddenPrefixes {
				if strings.HasPrefix(importPath, prefix) {
					assert.Failf(t, "Kubernetes or Helm SDK imported",
						"%s imports %s. Drive the host kubectl and helm binaries instead",
						fset.Position(imported.Pos()), importPath)
				}
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, checked, "the walk found no Go files, so the test root is wrong")
}
