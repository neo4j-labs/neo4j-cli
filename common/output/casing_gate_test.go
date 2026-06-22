// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snakeCase matches a lower-case snake identifier: one or more lower-case
// alphanumeric segments joined by single underscores (e.g. `id`, `bolt_port`,
// `connection_uri`). It rejects kebab (`-`) and any upper-case rune, which is
// exactly the CLI-127 contract for rendered OUTPUT field names.
var snakeCase = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)

// printFuncs are the rendering entry points whose final `fields` argument lists
// the rendered output column / key names.
var printFuncs = map[string]bool{
	"PrintBodyMap":  true,
	"PrintBodyMaps": true,
	"PrintBody":     true,
	"PrintRawBody":  true,
}

// outputStructAllowlist names the OUTPUT structs whose json tags are the rendered
// field names (CLI-127 scope). The gate checks json tags only on these enumerated
// types so that wire/parse structs (which legitimately keep camelCase/kebab tags)
// are not false-positives. Key is the repo-relative file, value the type names in it.
var outputStructAllowlist = map[string][]string{
	"neo4j-cli/internal/subcommands/docker/labels.go":       {"Container"},
	"neo4j-cli/internal/desktopclient/output.go":            {"DbmsInfoOutput", "ConnectionOutput", "DbmsPluginOutput"},
	"neo4j-cli/aura/internal/subcommands/workspace/list.go": {"workspaceEntry"},
	"common/clierr/render.go":                               {"Envelope", "EnvelopeBody"},
	"neo4j-cli/internal/subcommands/agentcontext/build.go":  {"Context", "Command", "Flag"},
	"neo4j-cli/query/lint.go":                               {"lintDiagnostic"},
}

// fieldsSliceFileSkip lists output-rendering files whose Print* `fields` argument
// is a column list mirroring Neo4j's own wire column names (SHOW INDEXES /
// db.schema.* YIELD aliases), not CLI-authored field names. The row maps are
// keyed by those Neo4j column names, so renaming them would mean re-aliasing the
// Cypher — out of CLI-127 scope and treated as wire data.
var fieldsSliceFileSkip = map[string]bool{
	"neo4j-cli/query/schema.go": true,
}

// TestOutputFields_AreSnakeCase walks the whole module and asserts that rendered
// output field names are snake_case. It covers two surfaces:
//
//  1. The `fields` argument passed to PrintBodyMap/PrintBodyMaps/PrintBody/
//     PrintRawBody — extracted from an inline []string / [][]string literal or
//     resolved from an identifier whose []string{...} declarations / appends live
//     in the same file. Computed args (function calls, data-driven column slices)
//     are skipped: they carry no static literal to check.
//  2. The json tags on the enumerated OUTPUT structs (outputStructAllowlist).
//
// Failure names the file and the offending token. The gate passes on the
// converted tree and fails the moment a kebab/camel output field is reintroduced.
func TestOutputFields_AreSnakeCase(t *testing.T) {
	root := moduleRoot(t)

	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == "vendor" || base == ".git" || base == "testdata" || base == ".claude" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	require.NoError(t, err)

	fset := token.NewFileSet()
	for _, path := range files {
		rel, rerr := filepath.Rel(root, path)
		require.NoError(t, rerr)
		rel = filepath.ToSlash(rel)

		f, perr := parser.ParseFile(fset, path, nil, 0)
		require.NoErrorf(t, perr, "parse %s", rel)

		checkFieldsSlices(t, rel, f)
		checkEnumeratedStructs(t, rel, f)
	}
}

// checkFieldsSlices finds Print* call sites in the file and validates each
// element of the resolved `fields` argument.
func checkFieldsSlices(t *testing.T, rel string, f *ast.File) {
	if fieldsSliceFileSkip[rel] {
		return
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := calleeName(call.Fun)
		if !printFuncs[name] || len(call.Args) == 0 {
			return true
		}
		arg := call.Args[len(call.Args)-1]
		for _, field := range resolveFieldStrings(f, arg) {
			// Nested-field syntax `a:b` addresses a sub-key; each segment is an
			// output field name in its own right.
			for _, seg := range strings.Split(field, ":") {
				assert.Regexp(t, snakeCase, seg,
					"%s: output field %q passed to %s must be snake_case (^[a-z0-9]+(_[a-z0-9]+)*$)", rel, seg, name)
			}
		}
		return true
	})
}

// resolveFieldStrings returns the string literals making up a Print* fields
// argument. It handles an inline []string / [][]string composite literal and an
// identifier resolved to same-file []string{...} declarations and append(...)
// calls. Unresolvable args (function results, data-driven slices) yield nothing.
func resolveFieldStrings(f *ast.File, arg ast.Expr) []string {
	switch a := arg.(type) {
	case *ast.CompositeLit:
		return stringsFromComposite(a)
	case *ast.Ident:
		var out []string
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.AssignStmt:
				out = append(out, stringsFromAssignTo(a.Name, x.Lhs, x.Rhs)...)
			case *ast.ValueSpec:
				for i, name := range x.Names {
					if name.Name == a.Name && i < len(x.Values) {
						if cl, ok := x.Values[i].(*ast.CompositeLit); ok {
							out = append(out, stringsFromComposite(cl)...)
						}
					}
				}
			case *ast.CallExpr:
				// append(ident, "lit", ...) extends the slice with more fields.
				if calleeName(x.Fun) == "append" && len(x.Args) > 0 {
					if id, ok := x.Args[0].(*ast.Ident); ok && id.Name == a.Name {
						for _, e := range x.Args[1:] {
							if s, ok := stringLit(e); ok {
								out = append(out, s)
							}
						}
					}
				}
			}
			return true
		})
		return out
	}
	return nil
}

func stringsFromAssignTo(name string, lhs, rhs []ast.Expr) []string {
	var out []string
	for i, l := range lhs {
		id, ok := l.(*ast.Ident)
		if !ok || id.Name != name || i >= len(rhs) {
			continue
		}
		if cl, ok := rhs[i].(*ast.CompositeLit); ok {
			out = append(out, stringsFromComposite(cl)...)
		}
	}
	return out
}

// stringsFromComposite extracts string literals from a []string{...} or a
// [][]string{...} literal (recursing one level for the latter).
func stringsFromComposite(cl *ast.CompositeLit) []string {
	var out []string
	for _, elt := range cl.Elts {
		if s, ok := stringLit(elt); ok {
			out = append(out, s)
			continue
		}
		if inner, ok := elt.(*ast.CompositeLit); ok {
			out = append(out, stringsFromComposite(inner)...)
		}
	}
	return out
}

func stringLit(e ast.Expr) (string, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// checkEnumeratedStructs validates the json tags on the OUTPUT structs enumerated
// for this file.
func checkEnumeratedStructs(t *testing.T, rel string, f *ast.File) {
	want := outputStructAllowlist[rel]
	if len(want) == 0 {
		return
	}
	wantSet := make(map[string]bool, len(want))
	for _, n := range want {
		wantSet[n] = true
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || !wantSet[ts.Name.Name] {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, fld := range st.Fields.List {
				if fld.Tag == nil {
					continue
				}
				tag, err := strconv.Unquote(fld.Tag.Value)
				if err != nil {
					continue
				}
				key := jsonKey(tag)
				if key == "" || key == "-" {
					continue
				}
				assert.Regexp(t, snakeCase, key,
					"%s: struct %s json tag %q must be snake_case (^[a-z0-9]+(_[a-z0-9]+)*$)", rel, ts.Name.Name, key)
			}
		}
	}
}

// jsonKey returns the json field name from a struct tag (the part before any
// comma-separated options like `,omitempty`).
func jsonKey(tag string) string {
	v := reflect.StructTag(tag).Get("json")
	if v == "" {
		return ""
	}
	if idx := strings.IndexByte(v, ','); idx >= 0 {
		return v[:idx]
	}
	return v
}

func calleeName(fun ast.Expr) string {
	switch x := fun.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}

// moduleRoot walks up from the package directory to the directory containing
// go.mod so the gate works regardless of the test's working directory.
func moduleRoot(t *testing.T) string {
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqualf(t, parent, dir, "reached filesystem root without finding go.mod")
		dir = parent
	}
}
