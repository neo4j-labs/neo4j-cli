// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"encoding/json"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

var dbmsUpdateFields = []string{"id", "name", "version", "status", "connection_uri", "projects", "tags"}

// dbmsUpdateResult adapts the updated `*DbmsInfo` to output.ResponseData.
type dbmsUpdateResult struct {
	Item *desktopclient.DbmsInfo
}

func (r dbmsUpdateResult) AsArray() []map[string]any {
	if r.Item == nil {
		return nil
	}
	return []map[string]any{
		{
			"id":             r.Item.ID,
			"name":           r.Item.Name,
			"version":        r.Item.Version,
			"status":         r.Item.Status,
			"connection_uri": r.Item.ConnectionURI,
			"projects":       r.Item.Projects,
			"tags":           r.Item.Tags,
		},
	}
}

// MarshalJSON emits the snake_case DbmsInfo projection so `--format json` matches `desktop dbms list`.
func (r dbmsUpdateResult) MarshalJSON() ([]byte, error) {
	if r.Item == nil {
		return []byte("null"), nil
	}
	return json.Marshal(r.Item.ToOutput())
}

// newUpdateCmd builds the `desktop dbms update <id>` leaf. At least one
// mutating flag must be supplied (an empty PATCH would surface as a confusing
// no-op). `--project`/`--tags` have REPLACE semantics — the supplied values
// become the entire set — and each value is resolved against Desktop's
// project/tag catalogs (a UUID passes through verbatim) BEFORE the PATCH.
func newUpdateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name        string
		description string
		projects    []string
		tags        []string
	)

	const (
		nameFlag        = "name"
		descriptionFlag = "description"
		projectFlag     = "project"
		tagsFlag        = "tags"
	)

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a DBMS managed by the local Neo4j Desktop 2 install",
		Long: "Update a DBMS managed by the local Neo4j Desktop 2 install. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"At least one of `--name --description --project --tags` must be supplied; the PATCH body contains ONLY the keys you set, so empty-string is a legitimate update for `--description`. " +
			"`--project` and `--tags` have REPLACE semantics: the values you pass become the entire set (re-run with the full list when adding one); pass an empty value (`--tags \"\"`) to clear the set. " +
			"Each value is a Desktop catalog name or ID — names are resolved against Desktop's project/tag catalogs before the PATCH, a UUID passes through verbatim. " +
			"Find DBMS ids with `neo4j-cli desktop list`.",
		Example: `# Rename a DBMS
neo4j-cli desktop dbms update my-dbms-id --name my-renamed-dbms --rw

# Replace the tag set (names resolve against Desktop's tag catalog)
neo4j-cli desktop dbms update my-dbms-id --tags prod,eu --rw

# Move a DBMS into a project and emit the updated DbmsInfo as JSON
neo4j-cli desktop dbms update my-dbms-id --project "Customer 360" --format json --rw

# Clear all tags from a DBMS
neo4j-cli desktop dbms update my-dbms-id --tags "" --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			id := args[0]
			nameSet := cmd.Flag(nameFlag).Changed
			descriptionSet := cmd.Flag(descriptionFlag).Changed
			projectSet := cmd.Flag(projectFlag).Changed
			tagsSet := cmd.Flag(tagsFlag).Changed
			if !nameSet && !descriptionSet && !projectSet && !tagsSet {
				return clierr.NewUsageError(
					"at least one of --name, --description, --project, --tags must be supplied")
			}

			ctx := cmd.Context()
			fs := cfg.Fs()
			port, _ := cmd.Flags().GetInt(portFlag)

			client, err := desktopclient.Connect(ctx, fs, port)
			if err != nil {
				return err
			}

			updateArgs := desktopclient.DbmsUpdateArgs{}
			if nameSet {
				updateArgs.Name = &name
			}
			if descriptionSet {
				updateArgs.Description = &description
			}
			if projectSet {
				ids, err := client.ResolveProjectIDs(ctx, desktopclient.NonEmptyStrings(projects))
				if err != nil {
					return err
				}
				updateArgs.Projects = &ids
			}
			if tagsSet {
				ids, err := client.ResolveTagIDs(ctx, desktopclient.NonEmptyStrings(tags))
				if err != nil {
					return err
				}
				updateArgs.Tags = &ids
			}

			updated, err := client.UpdateDbms(ctx, id, updateArgs)
			if err != nil {
				return err
			}

			output.PrintBodyMap(cmd, cfg, dbmsUpdateResult{Item: updated}, dbmsUpdateFields)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "New human-readable name for the DBMS")
	cmd.Flags().StringVar(&description, descriptionFlag, "", "New description for the DBMS. Pass an empty string to clear the existing description")
	cmd.Flags().StringSliceVar(&projects, projectFlag, nil, "Replace the DBMS's project set with the given Desktop project names or IDs (comma-separated or repeated). Pass an empty value to clear.")
	cmd.Flags().StringSliceVar(&tags, tagsFlag, nil, "Replace the DBMS's tag set with the given Desktop tag names or IDs (comma-separated or repeated). Pass an empty value to clear.")

	return cmd
}
