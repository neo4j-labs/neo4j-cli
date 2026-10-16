// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationrun

import (
	"fmt"

	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		agentID string
		limit   int
	)

	const (
		agentIDFlag = "agent-id"
		limitFlag   = "limit"
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of evaluation runs",
		Long: `This subcommand returns a list of Aura Agent evaluation runs in the specified project, newest first. Each run evaluates one agent against one evaluation dataset; pass --agent-id to return only that agent's runs.

The API returns results a page at a time; this subcommand follows every page so the output is the complete list. Use --limit to stop early, in which case a note is written to stderr saying more results exist.

Use --organization-id and --project-id to specify which project's evaluation runs to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.`,
		Example: `# List every evaluation run in the default project
neo4j-cli aura agent evaluation-run list

# List the evaluation runs of a specific agent
neo4j-cli aura agent evaluation-run list --agent-id 00000000-0000-0000-0000-000000000000

# Return at most 10 and emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura agent evaluation-run list --limit 10 --format json`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true
			page, err := auraclient.New(cfg).EvaluationRuns().List(cmd.Context(), auraclient.Scope{OrgID: organizationId, ProjectID: projectId}, agentID, limit)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, len(page.Items))
			for i, run := range page.Items {
				rows[i] = run.Record
			}
			output.PrintRecords(cmd, cfg, rows, []string{"id", "agent_id", "dataset_id", "status", "created_at"})

			if page.LimitReached {
				fmt.Fprintf(cmd.ErrOrStderr(), "Showing the first %d evaluation runs; more are available. Raise or omit --limit to see them all.\n", limit) //nolint:errcheck // narration to stderr; write errors are not actionable
			}
			if page.PageCapReached {
				fmt.Fprintf(cmd.ErrOrStderr(), "Stopped after %d pages, so this list may be incomplete.\n", auraclient.MaxListPages) //nolint:errcheck // narration to stderr; write errors are not actionable
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&agentID, agentIDFlag, "", "Only return evaluation runs of this agent. Omit to return the project's runs for every agent.")
	cmd.Flags().IntVar(&limit, limitFlag, 0, "The maximum number of evaluation runs to return. Omit to return all of them.")

	return cmd
}
