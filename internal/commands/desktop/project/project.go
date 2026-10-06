// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package project implements the `neo4j-cli desktop project` subtree as a spec
// over the shared catalogcmd scaffold (see desktop/internal/catalogcmd).
package project

import (
	"context"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/desktop/internal/catalogcmd"
	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/spf13/cobra"
)

// spec declares the project catalog to the catalogcmd scaffold: the noun and
// output fields, the verbatim per-leaf help text, and hooks closing over the
// typed desktopclient project methods. Projects carry no color, so Color is
// nil and the generated leaves have no --color flag.
var spec = catalogcmd.Spec[desktopclient.Project, *desktopclient.ProjectsState]{
	Noun:       "project",
	Plural:     "projects",
	Fields:     []string{"id", "name", "created_at"},
	UpdateVerb: "Rename",

	ListLong: "List the project catalog of the local Neo4j Desktop 2 install. " +
		"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
		"Each row carries `id`, `name` and `created_at`; `created_at` is Desktop's unix-milliseconds timestamp rendered as RFC3339 UTC (empty when Desktop omits it for legacy entries). " +
		"`--format json` emits a JSON array of projects; `--format toon` mirrors the JSON shape.",
	ListExample: `# List Desktop projects as a table
neo4j-cli desktop project list

# List Desktop projects as JSON (agent-friendly)
neo4j-cli desktop project list --format json

# List Desktop projects against a pinned port instead of probing 44222..44232
neo4j-cli desktop project list --port 44225`,
	CreateLong: "Create a project in the local Neo4j Desktop 2 install. " +
		"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
		"Prints the created project (`id`, `name`, `created_at` as RFC3339 UTC) resolved from the post-create catalog state Desktop returns.",
	CreateExample: `# Create a Desktop project
neo4j-cli desktop project create my-project --rw

# Create a Desktop project and emit the created entry as JSON
neo4j-cli desktop project create my-project --format json --rw`,
	UpdateLong: "Rename a project in the local Neo4j Desktop 2 install. " +
		"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
		"`<project>` accepts a project UUID or an exact project name from the Desktop catalog (see `neo4j-cli desktop project list`). " +
		"`--name` is required: Desktop's ProjectUpdateSchema demands a name on every PATCH. " +
		"Prints the updated project (`id`, `name`, `created_at` as RFC3339 UTC) resolved from the post-update catalog state Desktop returns.",
	UpdateExample: `# Rename a Desktop project addressed by its exact name
neo4j-cli desktop project update my-project --name my-renamed-project --rw

# Rename a Desktop project addressed by UUID, emitting the updated entry as JSON
neo4j-cli desktop project update f4e2f3c0-1111-2222-3333-444455556666 --name my-renamed-project --format json --rw`,
	DeleteLong: "Delete a project from the local Neo4j Desktop 2 install. " +
		"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
		"`<project>` accepts a project UUID or an exact project name from the Desktop catalog (see `neo4j-cli desktop project list`). " +
		"Destructive: requires `--yes --force` (or a `y` answer at the TTY prompt) when invoked non-interactively. " +
		"Prints a minimal confirmation envelope carrying the removed id.",
	DeleteExample: `# Delete a Desktop project by exact name with an interactive y/N confirmation
neo4j-cli desktop project delete my-project --rw

# Delete a Desktop project by UUID without prompting (scripts, CI, non-TTY shells)
neo4j-cli desktop project delete f4e2f3c0-1111-2222-3333-444455556666 --yes --force --rw

# Delete a Desktop project and emit a machine-readable confirmation for scripting
neo4j-cli desktop project delete my-project --yes --force --format json --rw`,

	List: func(ctx context.Context, c *desktopclient.Client) ([]desktopclient.Project, error) {
		return c.ListProjects(ctx)
	},
	Create: func(ctx context.Context, c *desktopclient.Client, name, _ string) (*desktopclient.ProjectsState, error) {
		return c.CreateProject(ctx, name)
	},
	Update: func(ctx context.Context, c *desktopclient.Client, id, name, _ string) (*desktopclient.ProjectsState, error) {
		return c.UpdateProject(ctx, id, name)
	},
	Delete: func(ctx context.Context, c *desktopclient.Client, id string) error {
		_, err := c.DeleteProject(ctx, id)
		return err
	},
	Resolve: func(ctx context.Context, c *desktopclient.Client, nameOrID string) (string, error) {
		ids, err := c.ResolveProjectIDs(ctx, []string{nameOrID})
		if err != nil {
			return "", err
		}
		return ids[0], nil
	},
	FindByName: desktopclient.FindProjectByName,
	FindByID:   desktopclient.FindProjectByID,

	RowJSON:   func(p desktopclient.Project) any { return toProjectOutput(p) },
	RowValues: func(p desktopclient.Project) []string { return []string{p.ID, p.Name, formatCreatedAt(p.CreatedAt)} },
}

// NewCmd returns the `desktop project` parent cobra command.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects in the local Neo4j Desktop 2 install",
		Long: "Manage the project catalog of the local Neo4j Desktop 2 install via its local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"Projects are Desktop's grouping mechanism for local DBMSes and saved remote connections. " +
			"Every read and mutation round-trips the full project catalog; `list` shows it, and `create`/`update`/`delete` print the affected entry. " +
			"Write commands (`create`, `update`, `delete`) require `--rw`.",
	}

	catalogcmd.Mount(cfg, cmd, spec)

	return cmd
}
