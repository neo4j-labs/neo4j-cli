// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalogcmd_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/desktop/internal/catalogcmd"
	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/neo4j/cli/internal/flags"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// fakeEntry is the scaffold test's catalog row type; fakeState is its
// post-mutation state type. They stand in for desktopclient.Project/Tag and
// ProjectsState/TagsState so the scaffold core is exercised without any HTTP.
type fakeEntry struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

type fakeState struct {
	entries []fakeEntry
}

const validFakeID = "f4e2f3c0-1111-2222-3333-444455556666"

// Flush-left example blocks satisfying the whole-tree example gate (>=2
// invocations, `# comment` each, neo4j-cli prefix, --rw on writes, --format
// json on reads).
const (
	fakeListExample = `# List widgets as a table
neo4j-cli desktop widget list

# List widgets as JSON
neo4j-cli desktop widget list --format json`
	fakeCreateExample = `# Create a widget
neo4j-cli desktop widget create my-widget --rw

# Create a widget and emit the created entry as JSON
neo4j-cli desktop widget create my-widget --format json --rw`
	fakeUpdateExample = `# Rename a widget addressed by its exact name
neo4j-cli desktop widget update my-widget --name renamed --rw

# Rename a widget addressed by UUID, emitting the updated entry as JSON
neo4j-cli desktop widget update f4e2f3c0-1111-2222-3333-444455556666 --name renamed --format json --rw`
	fakeDeleteExample = `# Delete a widget by exact name with an interactive y/N confirmation
neo4j-cli desktop widget delete my-widget --rw

# Delete a widget by UUID without prompting
neo4j-cli desktop widget delete f4e2f3c0-1111-2222-3333-444455556666 --yes --force --rw

# Delete a widget and emit a machine-readable confirmation
neo4j-cli desktop widget delete my-widget --yes --force --format json --rw`
)

// baseSpec returns a colorless widget spec with hooks that fail the test if
// called — tests override the hooks they exercise. RowJSON is the identity
// projection (like tag's raw Tag payload).
func baseSpec(t *testing.T) catalogcmd.Spec[fakeEntry, *fakeState] {
	t.Helper()
	called := func(name string) {
		t.Helper()
		t.Fatalf("spec hook %s called unexpectedly", name)
	}
	return catalogcmd.Spec[fakeEntry, *fakeState]{
		Noun:       "widget",
		Plural:     "widgets",
		Fields:     []string{"id", "name"},
		UpdateVerb: "Update",

		ListLong:      "List the widget catalog.",
		ListExample:   fakeListExample,
		CreateLong:    "Create a widget.",
		CreateExample: fakeCreateExample,
		UpdateLong:    "Update a widget.",
		UpdateExample: fakeUpdateExample,
		DeleteLong:    "Delete a widget.",
		DeleteExample: fakeDeleteExample,

		List: func(context.Context, *desktopclient.Client) ([]fakeEntry, error) {
			called("List")
			return nil, nil
		},
		Create: func(context.Context, *desktopclient.Client, string, string) (*fakeState, error) {
			called("Create")
			return nil, nil
		},
		Update: func(context.Context, *desktopclient.Client, string, string, string) (*fakeState, error) {
			called("Update")
			return nil, nil
		},
		Delete: func(context.Context, *desktopclient.Client, string) error {
			called("Delete")
			return nil
		},
		Resolve: func(context.Context, *desktopclient.Client, string) (string, error) {
			called("Resolve")
			return "", nil
		},
		FindByName: func(*fakeState, string) (*fakeEntry, error) {
			called("FindByName")
			return nil, nil
		},
		FindByID: func(*fakeState, string) (*fakeEntry, error) {
			called("FindByID")
			return nil, nil
		},

		RowJSON:   func(e fakeEntry) any { return e },
		RowValues: func(e fakeEntry) []string { return []string{e.ID, e.Name} },
	}
}

// withColor turns a base spec into a color-bearing one (the tag shape): the
// color field joins the output columns and --color mounts on create/update.
func withColor(spec catalogcmd.Spec[fakeEntry, *fakeState]) catalogcmd.Spec[fakeEntry, *fakeState] {
	spec.Fields = []string{"id", "name", "color"}
	spec.Color = &catalogcmd.ColorSpec{
		CreateUsage: `Optional widget color: one of Desktop's palette indices "1".."12"`,
		UpdateUsage: `New widget color: one of Desktop's palette indices "1".."12"`,
	}
	spec.RowValues = func(e fakeEntry) []string { return []string{e.ID, e.Name, e.Color} }
	return spec
}

// findByName is a find-one fake mirroring the desktopclient finder policy.
func findByName(state *fakeState, name string) (*fakeEntry, error) {
	if state != nil {
		for i := range state.entries {
			if state.entries[i].Name == name {
				return &state.entries[i], nil
			}
		}
	}
	return nil, errors.New(`fake: widget "` + name + `" is missing from the catalog state`)
}

// findByID is a find-one fake mirroring the desktopclient finder policy.
func findByID(state *fakeState, id string) (*fakeEntry, error) {
	if state != nil {
		for i := range state.entries {
			if state.entries[i].ID == id {
				return &state.entries[i], nil
			}
		}
	}
	return nil, errors.New(`fake: widget "` + id + `" is missing from the catalog state`)
}

// harness wires a fake `desktop` root (persistent --port + the output flag)
// against an in-memory FS, mirroring how the real desktop root mounts the
// generated leaves.
type harness struct {
	t   *testing.T
	in  *bytes.Buffer
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	return &harness{
		t:   t,
		in:  &bytes.Buffer{},
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

// stubConnect pins the desktopclient.Connect seam to a no-op constructor; the
// fake spec hooks ignore the client. Returns a pointer to the last port the
// constructor saw.
func (h *harness) stubConnect() *int {
	h.t.Helper()
	seen := new(int)
	h.t.Cleanup(desktopclient.SetConnectFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		*seen = port
		return nil, nil //nolint:nilnil // the fake hooks never dereference the client
	}))
	return seen
}

func (h *harness) run(spec catalogcmd.Spec[fakeEntry, *fakeState], command string) error {
	h.t.Helper()
	args, err := shlex.Split(command)
	if err != nil {
		h.t.Fatalf("shlex: %v", err)
	}
	cfg := clicfg.NewConfig(h.fs, "test")
	defer cfg.Events.Flush()
	root := &cobra.Command{Use: "desktop"}
	root.PersistentFlags().Int("port", 0, "Pin the Desktop relate API to a specific port")
	flags.RegisterOutputFlag(root, cfg)
	parent := &cobra.Command{Use: spec.Noun}
	root.AddCommand(parent)
	catalogcmd.Mount(cfg, parent, spec)
	root.SetArgs(args)
	root.SetIn(h.in)
	root.SetOut(h.out)
	root.SetErr(h.err)
	return root.Execute()
}

// findLeaf locates one generated leaf under a freshly mounted parent.
func findLeaf(t *testing.T, spec catalogcmd.Spec[fakeEntry, *fakeState], leafName string) *cobra.Command {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	cfg := clicfg.NewConfig(fs, "test")
	defer cfg.Events.Flush()
	parent := &cobra.Command{Use: spec.Noun}
	catalogcmd.Mount(cfg, parent, spec)
	for _, c := range parent.Commands() {
		if c.Name() == leafName {
			return c
		}
	}
	t.Fatalf("leaf %q not generated", leafName)
	return nil
}
