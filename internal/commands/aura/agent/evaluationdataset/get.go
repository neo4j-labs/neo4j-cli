// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationdataset

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
		Short: "Returns evaluation dataset details",
		Long:  "Returns the details of a specific evaluation dataset, including its questions.",
		Example: `# Get details for an evaluation dataset
neo4j-cli aura agent evaluation-dataset get 00000000-0000-0000-0000-000000000000

# Get an evaluation dataset in a specific organization and project
neo4j-cli aura agent evaluation-dataset get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000

# Get evaluation dataset details as JSON for scripting
neo4j-cli aura agent evaluation-dataset get 00000000-0000-0000-0000-000000000000 --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			organizationId, projectId, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			dataset, err := auraclient.New(cfg).EvaluationDatasets().Get(cmd.Context(), auraclient.Scope{OrgID: organizationId, ProjectID: projectId}, args[0])
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, dataset.Record, []string{"id", "dataset_name", "description", "question_count", "created_at", "updated_at"})

			return nil
		},
	}

	return cmd
}
