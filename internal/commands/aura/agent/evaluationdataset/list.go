// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationdataset

import (
	"fmt"

	"github.com/neo4j/cli/internal/auraclient"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	var limit int

	const limitFlag = "limit"

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of evaluation datasets",
		Long: `Returns a list containing a summary of each evaluation dataset in the specified project.

The API returns results a page at a time; this subcommand follows every page so the output is the complete list. Use --limit to stop early, in which case a note is written to stderr saying more results exist.

Use --organization-id and --project-id to specify which project's evaluation datasets to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.`,
		Example: `# List every evaluation dataset in the default project
neo4j-cli aura agent evaluation-dataset list

# List at most 10 evaluation datasets in a specific organization and project
neo4j-cli aura agent evaluation-dataset list --limit 10 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000

# List evaluation datasets as JSON for scripting
neo4j-cli aura agent evaluation-dataset list --format json`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			organizationId, projectId, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			page, err := auraclient.New(cfg).EvaluationDatasets().List(cmd.Context(), auraclient.Scope{OrgID: organizationId, ProjectID: projectId}, limit)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, len(page.Items))
			for i, dataset := range page.Items {
				rows[i] = dataset.Record
			}
			output.PrintRecords(cmd, cfg, rows, []string{"id", "dataset_name", "description", "question_count", "created_at", "updated_at"})

			if page.LimitReached {
				fmt.Fprintf(cmd.ErrOrStderr(), "Showing the first %d evaluation datasets; more are available. Raise or omit --limit to see them all.\n", limit) //nolint:errcheck // narration to stderr; write errors are not actionable
			}
			if page.PageCapReached {
				fmt.Fprintf(cmd.ErrOrStderr(), "Stopped after %d pages, so this list may be incomplete.\n", auraclient.MaxListPages) //nolint:errcheck // narration to stderr; write errors are not actionable
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&limit, limitFlag, 0, "The maximum number of evaluation datasets to return. Omit to return all of them.")

	return cmd
}
