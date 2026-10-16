// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationrun

import (
	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func newGetCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Returns evaluation run details",
		Long: `Returns the details of a specific evaluation run.

Once the run has finished, JSON output also carries the per-question results in the questions field.`,
		Example: `# Get details for an evaluation run
neo4j-cli aura agent evaluation-run get 00000000-0000-0000-0000-000000000000

# Get an evaluation run in a specific organization and project
neo4j-cli aura agent evaluation-run get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get the full record, including per-question results, as JSON for scripting
neo4j-cli aura agent evaluation-run get 00000000-0000-0000-0000-000000000000 --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true
			run, err := auraclient.New(cfg).EvaluationRuns().Get(cmd.Context(), auraclient.Scope{OrgID: organizationId, ProjectID: projectId}, args[0])
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, run.Record, []string{"id", "status", "metrics", "avg_accuracy", "avg_tool_trajectory", "avg_groundedness", "avg_relevance", "created_at", "started_at", "finished_at"})

			return nil
		},
	}

	return cmd
}
