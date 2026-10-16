// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationrun

import (
	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/neo4j/cli/internal/confirm"
	"github.com/spf13/cobra"
)

func newDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "delete <id>",
		Short:       "Deletes an evaluation run",
		Long: `Deletes an evaluation run by its ID.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Delete an evaluation run by ID
neo4j-cli aura agent evaluation-run delete 00000000-0000-0000-0000-000000000000 --rw --yes --force

# Delete an evaluation run in a specific organization and project
neo4j-cli aura agent evaluation-run delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Delete an evaluation run and emit the response as JSON
neo4j-cli aura agent evaluation-run delete 00000000-0000-0000-0000-000000000000 --rw --yes --force --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			organizationId, projectId, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			evaluationRunId := args[0]

			if err := confirm.Require(cmd, evaluationRunId); err != nil {
				return err
			}

			if err := auraclient.New(cfg).EvaluationRuns().Delete(cmd.Context(), auraclient.Scope{OrgID: organizationId, ProjectID: projectId}, evaluationRunId); err != nil {
				return err
			}
			cmd.Println("Evaluation run deleted successfully", evaluationRunId)

			return nil
		},
	}

	confirm.Register(cmd)

	return cmd
}
