// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/confirm"
	"github.com/neo4j/cli/internal/relate"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/stretchr/testify/require"
)

// harness wires a relate parent command over an in-memory config fs with
// captured stdout/stderr, mirroring the query tree's runHarness.
type harness struct {
	cfg            *clicfg.Config
	stdout, stderr *bytes.Buffer
}

func newHarness(t *testing.T, output string) *harness {
	t.Helper()

	cfgJSON := `{"format":"` + output + `"}`
	fs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)
	return &harness{
		cfg:    clicfg.NewConfig(fs, "test"),
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
	}
}

func (h *harness) execute(t *testing.T, args ...string) error {
	t.Helper()
	cmd := NewCmd(h.cfg)
	cmd.SetOut(h.stdout)
	cmd.SetErr(h.stderr)
	cmd.SetContext(context.Background())
	cmd.SetArgs(args)
	return cmd.Execute()
}

// installFakeOpen swaps the engine's write opener for one returning the fake
// store, restoring the previous hook on cleanup.
func installFakeOpen(t *testing.T, f *fakeStore) {
	t.Helper()
	orig := relate.OpenStore
	relate.OpenStore = func(path, namespace string) (relate.Store, error) {
		f.openedPath = path
		f.openedNamespace = namespace
		return f, nil
	}
	t.Cleanup(func() { relate.OpenStore = orig })
}

// installFakeOpenReadOnly is installFakeOpen for the read-only hook.
func installFakeOpenReadOnly(t *testing.T, f *fakeStore) {
	t.Helper()
	orig := relate.OpenStoreReadOnly
	relate.OpenStoreReadOnly = func(path, namespace string) (relate.Store, error) {
		f.openedPath = path
		return f, nil
	}
	t.Cleanup(func() { relate.OpenStoreReadOnly = orig })
}

// fakeStore records the open/run/checkpoint/close traffic of one command
// invocation and answers statements through the responder.
type fakeStore struct {
	openedPath      string
	openedNamespace string
	statements      []string
	checkpoints     int
	closed          bool
	created         int
	respond         func(stmt string) (relate.StatementResult, error)
}

func (f *fakeStore) Run(statements []string, params map[string]any, atomic bool) ([]relate.StatementResult, error) {
	f.statements = append(f.statements, statements...)
	results := make([]relate.StatementResult, 0, len(statements))
	for _, stmt := range statements {
		if f.respond == nil {
			// Default responder: identity-capturing statements (the pull
			// materializer's CREATE ... RETURN elementId ...) get synthetic
			// local ids extracted from the inlined src literals; everything
			// else answers empty.
			res := relate.StatementResult{Rows: [][]any{}}
			if strings.Contains(stmt, "RETURN elementId") {
				res.Columns = []string{"eid", "src"}
				for _, m := range srcLiteralPattern.FindAllStringSubmatch(stmt, -1) {
					f.created++
					res.Rows = append(res.Rows, []any{"local-" + strconv.Itoa(f.created), m[1]})
				}
			}
			results = append(results, res)
			continue
		}
		res, err := f.respond(stmt)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}

// srcLiteralPattern extracts the inlined `src: '...'` identities from a
// materializer statement.
var srcLiteralPattern = regexp.MustCompile(`src: '([^']*)'`)

func (f *fakeStore) Checkpoint() error {
	f.checkpoints++
	return nil
}

func (f *fakeStore) Close() { f.closed = true }

// seedStore materializes a store-looking directory (manifest.json marker)
// under the managed store directory of the harness fs.
func (h *harness) seedStore(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(relate.StoreDir(), name)
	require.NoError(t, h.cfg.Fs().MkdirAll(path, 0o700))
	require.NoError(t, afero.WriteFile(h.cfg.Fs(), filepath.Join(path, "manifest.json"), []byte("{}"), 0o600))
	return path
}

// nonInteractive makes the confirm gate demand --yes --force (no TTY prompt).
func nonInteractive(t *testing.T) {
	t.Helper()
	confirm.SetStdinIsTerminal(func() bool { return false })
}
