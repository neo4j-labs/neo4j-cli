// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph

import (
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	var limit int

	const limitFlag = "limit"

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of virtual graphs",
		Long: `This subcommand returns a list containing a summary of each virtual graph in the specified project, newest first. To find out more about a specific virtual graph, retrieve the details using the get subcommand.

The API returns results a page at a time; this subcommand follows every page so the output is the complete list. Use --limit to stop early, in which case a note is written to stderr saying more results exist.

Use --organization-id and --project-id to specify which project's virtual graphs to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.`,
		Example: `# List every virtual graph in a project (using flags)
neo4j-cli aura virtual-graph list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List virtual graphs using a configured default workspace
neo4j-cli aura virtual-graph list

# Return at most 10 and emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura virtual-graph list --limit 10 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			if limit < 0 {
				return clierr.NewUsageError("--limit must be zero or greater; zero returns every virtual graph")
			}

			path := api.ScopedVirtualGraphsPath(orgID, projectID)
			result, err := api.ListAllPages(cfg, path, api.AuraApiVersion2, limit)
			if err != nil {
				return err
			}

			output.PrintBodyMap(cmd, cfg, api.NewListResponseData(result.Items), summaryFields)

			// Both notices go to stderr so stdout stays a clean, pipeable envelope.
			// Staying silent here would make a partial list read as a complete one.
			if result.LimitReached {
				fmt.Fprintf(cmd.ErrOrStderr(), "Showing the first %d virtual graphs; more are available. Raise or omit --limit to see them all.\n", limit) //nolint:errcheck // narration to stderr; write errors are not actionable
			}
			if result.PageCapReached {
				fmt.Fprintf(cmd.ErrOrStderr(), "Stopped after %d pages, so this list may be incomplete.\n", api.MaxListPages) //nolint:errcheck // narration to stderr; write errors are not actionable
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&limit, limitFlag, 0, "The maximum number of virtual graphs to return. Omit to return all of them.")

	return cmd
}
