// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package catalogcmd is the shared scaffold behind the `desktop project` and
// `desktop tag` subtrees. Both catalogs are structurally identical — Desktop's
// /projects and /tags routes identify the mutated entry in the BODY (PATCH) or
// QUERYSTRING (DELETE), and every mutation responds with the full post-mutation
// state — so the four leaves (list/create/update/delete) are generated from a
// per-resource Spec rather than hand-authored per resource. This is the same
// data-driven exception to the one-file-per-leaf rule that `admin privilege`
// uses for its category leaves (see AGENTS.md "Cobra Command Layout").
//
// A Spec supplies the resource noun, the output field list, the user-facing
// help text, and typed hooks closing over the desktopclient methods; the
// scaffold core stays generic over the row (T) and post-mutation state (S)
// types only — no reflection, no any-heavy magic.
package catalogcmd

import (
	"context"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/spf13/cobra"
)

// portFlag is the desktop root's persistent --port flag (0 = probe
// 44222..44232), inherited by every generated leaf.
const portFlag = "port"

// Spec declares one Desktop catalog resource (project, tag) to the scaffold.
// Mount generates the list/create/update/delete leaves from it.
//
// Type parameters: T is the catalog row type (desktopclient.Project / Tag);
// S is the post-mutation state type every /projects or /tags mutation returns
// (*desktopclient.ProjectsState / *TagsState).
type Spec[T any, S any] struct {
	// Noun is the singular resource noun ("project"); it composes the
	// update/delete Use strings, every Short, and the delete confirmation
	// line. Plural is used by the list Short. Fields is the output column
	// order for table/toon single-entry output and the list table header.
	Noun   string
	Plural string
	Fields []string

	// UpdateVerb is the update leaf's verb ("Rename" for project, "Update"
	// for tag); the other leaf verbs are fixed. The Long and Example strings
	// are verbatim: each leaf's Example must be flush-left with >=2
	// invocations, a `# comment` each, the `neo4j-cli` prefix, `--rw` on
	// writes, and `--format json` on reads (the whole-tree example gate).
	UpdateVerb    string
	ListLong      string
	ListExample   string
	CreateLong    string
	CreateExample string
	UpdateLong    string
	UpdateExample string
	DeleteLong    string
	DeleteExample string

	// Color, when non-nil, mounts the optional palette --color flag on the
	// create/update leaves and validates it against Desktop's "1".."12"
	// palette BEFORE any Desktop contact. Nil means the resource has no color
	// and the leaves carry no --color flag.
	Color *ColorSpec

	// Client hooks. Create/Update receive the validated --color value (""
	// when the spec has no Color or the flag was unset). Delete's post-delete
	// state is discarded by the hook: the delete leaf prints a slim
	// {"id","deleted":true} envelope keyed on the resolved id instead.
	List    func(ctx context.Context, client *desktopclient.Client) ([]T, error)
	Create  func(ctx context.Context, client *desktopclient.Client, name, color string) (S, error)
	Update  func(ctx context.Context, client *desktopclient.Client, id, name, color string) (S, error)
	Delete  func(ctx context.Context, client *desktopclient.Client, id string) error
	Resolve func(ctx context.Context, client *desktopclient.Client, nameOrID string) (string, error)

	// Finders pick the mutated entry out of the post-mutation catalog state.
	// The fail-loud policy (absent or ambiguous is a FATAL error, never a
	// printed `null`) lives in the desktopclient finders these wrap: create
	// finds by name, update finds by the resolved id.
	FindByName func(state S, name string) (*T, error)
	FindByID   func(state S, id string) (*T, error)

	// Projections. RowJSON is the JSON/toon form of one entry (list array
	// element and single-entry body alike). RowValues returns the row's cell
	// values in Fields order, unescaped; the scaffold strips control bytes
	// for table cells and zips Fields x values for the AsArray map.
	RowJSON   func(item T) any
	RowValues func(item T) []string
}

// ColorSpec carries the --color flag's per-leaf usage strings for a
// color-bearing resource (tag). The palette validation itself is shared.
type ColorSpec struct {
	CreateUsage string
	UpdateUsage string
}

// Mount generates the list/create/update/delete leaves from spec and mounts
// them on parent. Registration order matches the hand-written predecessors
// (help output is alphabetical either way).
func Mount[T any, S any](cfg *clicfg.Config, parent *cobra.Command, spec Spec[T, S]) {
	parent.AddCommand(newListCmd(cfg, spec))
	parent.AddCommand(newCreateCmd(cfg, spec))
	parent.AddCommand(newUpdateCmd(cfg, spec))
	parent.AddCommand(newDeleteCmd(cfg, spec))
}
