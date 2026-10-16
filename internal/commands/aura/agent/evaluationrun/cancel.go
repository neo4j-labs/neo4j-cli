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

func newCancelCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "cancel <id>",
		Short:       "Cancels an evaluation run",
		Long: `Cancels a queued or running evaluation run by its ID.

The run's status becomes cancelled once the cancellation is processed.`,
		Example: `# Cancel a running evaluation run
neo4j-cli aura agent evaluation-run cancel 00000000-0000-0000-0000-000000000000 --rw

# Cancel an evaluation run in a specific organization and project
neo4j-cli aura agent evaluation-run cancel 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Cancel an evaluation run and emit the response as JSON for scripting
neo4j-cli aura agent evaluation-run cancel 00000000-0000-0000-0000-000000000000 --rw --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true
			run, err := auraclient.New(cfg).EvaluationRuns().Cancel(cmd.Context(), auraclient.Scope{OrgID: organizationId, ProjectID: projectId}, args[0])
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, run.Record, []string{"id", "agent_id", "dataset_id", "status", "created_at"})

			return nil
		},
	}

	return cmd
}
