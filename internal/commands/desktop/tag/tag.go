// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package tag implements the `neo4j-cli desktop tag` subtree as a spec over
// the shared catalogcmd scaffold (see desktop/internal/catalogcmd).
package tag

import (
	"context"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/desktop/internal/catalogcmd"
	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/spf13/cobra"
)

// spec declares the tag catalog to the catalogcmd scaffold: the noun and
// output fields, the verbatim per-leaf help text, and hooks closing over the
// typed desktopclient tag methods. Tags carry an optional palette color, so
// Color is set and the generated create/update leaves take --color.
var spec = catalogcmd.Spec[desktopclient.Tag, *desktopclient.TagsState]{
	Noun:       "tag",
	Plural:     "tags",
	Fields:     []string{"id", "name", "color"},
	UpdateVerb: "Update",

	ListLong: "List the tag catalog of the local Neo4j Desktop 2 install. " +
		"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
		"One row per tag with `id`, `name` and `color` (Desktop's palette index \"1\"..\"12\"; empty when the tag carries no color). " +
		"`--format json` emits a JSON array of `Tag` objects. " +
		"`--format toon` mirrors the JSON shape.",
	ListExample: `# List tags as a table
neo4j-cli desktop tag list

# List tags as JSON (full Tag payload, agent-friendly)
neo4j-cli desktop tag list --format json

# List tags against a pinned port instead of probing 44222..44232
neo4j-cli desktop tag list --port 44225`,
	CreateLong: "Create a tag in the tag catalog of the local Neo4j Desktop 2 install. " +
		"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
		"`--color` is optional and accepts only Desktop's palette indices \"1\"..\"12\" (not a hex colour); when omitted the POST body carries no color key and Desktop picks a default. " +
		"Prints the created tag (`id`, `name`, `color`) resolved from the post-create catalog state Desktop returns.",
	CreateExample: `# Create a tag and let Desktop pick the default color
neo4j-cli desktop tag create production --rw

# Create a tag with an explicit palette color
neo4j-cli desktop tag create staging --color 5 --rw

# Create a tag and emit the created tag as JSON
neo4j-cli desktop tag create staging --color 5 --format json --rw`,
	UpdateLong: "Update a tag in the tag catalog of the local Neo4j Desktop 2 install. " +
		"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
		"The positional `<tag>` accepts a tag UUID or an exact tag name (find both with `neo4j-cli desktop tag list`). " +
		"`--name` is required (Desktop's TagUpdateSchema demands it); `--color` accepts only Desktop's palette indices \"1\"..\"12\" and is omitted from the PATCH body when unset. " +
		"Prints the updated tag (`id`, `name`, `color`) resolved from the post-update catalog state Desktop returns.",
	UpdateExample: `# Rename a tag selected by name
neo4j-cli desktop tag update staging --name pre-prod --rw

# Rename and recolor a tag selected by UUID, emitting the updated tag as JSON
neo4j-cli desktop tag update f4e2f3c0-1111-2222-3333-444455556666 --name pre-prod --color 7 --format json --rw`,
	DeleteLong: "Delete a tag from the tag catalog of the local Neo4j Desktop 2 install. " +
		"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
		"The positional `<tag>` accepts a tag UUID or an exact tag name (find both with `neo4j-cli desktop tag list`). " +
		"Destructive: requires `--yes --force` (or a `y` answer at the TTY prompt) when invoked non-interactively; the confirmation fires before any Desktop contact. " +
		"Prints a minimal confirmation envelope carrying the removed id.",
	DeleteExample: `# Delete a tag by name with an interactive y/N confirmation
neo4j-cli desktop tag delete staging --rw

# Delete a tag by UUID without prompting (scripts, CI, non-TTY shells)
neo4j-cli desktop tag delete f4e2f3c0-1111-2222-3333-444455556666 --yes --force --rw

# Delete a tag and emit a machine-readable confirmation for scripting
neo4j-cli desktop tag delete staging --yes --force --format json --rw`,

	Color: &catalogcmd.ColorSpec{
		CreateUsage: `Optional tag color: one of Desktop's palette indices "1".."12" (not a hex colour). Omitted from the POST body when unset`,
		UpdateUsage: `New tag color: one of Desktop's palette indices "1".."12" (not a hex colour). Omitted from the PATCH body when unset`,
	},

	List: func(ctx context.Context, c *desktopclient.Client) ([]desktopclient.Tag, error) {
		return c.ListTags(ctx)
	},
	Create: func(ctx context.Context, c *desktopclient.Client, name, color string) (*desktopclient.TagsState, error) {
		return c.CreateTag(ctx, name, color)
	},
	Update: func(ctx context.Context, c *desktopclient.Client, id, name, color string) (*desktopclient.TagsState, error) {
		return c.UpdateTag(ctx, id, name, color)
	},
	Delete: func(ctx context.Context, c *desktopclient.Client, id string) error {
		_, err := c.DeleteTag(ctx, id)
		return err
	},
	Resolve: func(ctx context.Context, c *desktopclient.Client, nameOrID string) (string, error) {
		ids, err := c.ResolveTagIDs(ctx, []string{nameOrID})
		if err != nil {
			return "", err
		}
		return ids[0], nil
	},
	FindByName: desktopclient.FindTagByName,
	FindByID:   desktopclient.FindTagByID,

	RowJSON:   func(t desktopclient.Tag) any { return t },
	RowValues: func(t desktopclient.Tag) []string { return []string{t.ID, t.Name, t.Color} },
}

// NewCmd returns the `desktop tag` parent cobra command with all leaves
// mounted. The desktop root's persistent --port flag is inherited; tag does
// not re-register it.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manage the tag catalog of a Neo4j Desktop 2 install",
		Long: "Manage the tag catalog of the local Neo4j Desktop 2 install — list, create, update, delete. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"Tags organise DBMSes and saved remote connections in Desktop's UI; a tag's color is one of Desktop's palette indices \"1\"..\"12\" (not a hex colour). " +
			"Write commands (`create`, `update`, `delete`) require `--rw`.",
	}

	catalogcmd.Mount(cfg, cmd, spec)

	return cmd
}
