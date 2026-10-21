// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationdataset

import (
	"github.com/neo4j/cli/internal/auraclient"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func newUpdateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name          string
		description   string
		questionsFile string
	)

	const (
		nameFlag        = "name"
		descriptionFlag = "description"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "update <id>",
		Short:       "Partially updates an existing evaluation dataset",
		Long:        "Partially updates an existing evaluation dataset. Only provided fields are updated (PATCH semantics).",
		Example: `# Rename an evaluation dataset
neo4j-cli aura agent evaluation-dataset update 00000000-0000-0000-0000-000000000000 --name "Renamed dataset" --rw

# Replace an evaluation dataset's questions from a file
neo4j-cli aura agent evaluation-dataset update 00000000-0000-0000-0000-000000000000 --questions-file ./questions.json --rw

# Update an evaluation dataset and emit the response as JSON
neo4j-cli aura agent evaluation-dataset update 00000000-0000-0000-0000-000000000000 --description "Updated" --rw --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			nameSet := cmd.Flags().Changed(nameFlag)
			descriptionSet := cmd.Flags().Changed(descriptionFlag)
			questionsSet := cmd.Flags().Changed(questionsFileFlag)
			if !nameSet && !descriptionSet && !questionsSet {
				return clierr.NewUsageError("at least one of --%s, --%s or --%s must be set", nameFlag, descriptionFlag, questionsFileFlag)
			}

			organizationId, projectId, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			var patch auraclient.EvaluationDatasetPatch
			if nameSet {
				patch.Name = &name
			}
			if descriptionSet {
				patch.Description = &description
			}
			if questionsSet {
				questions, err := readQuestionsFile(cfg.Fs(), questionsFile)
				if err != nil {
					return err
				}
				patch.Questions = questions
			}

			dataset, err := auraclient.New(cfg).EvaluationDatasets().Update(cmd.Context(), auraclient.Scope{OrgID: organizationId, ProjectID: projectId}, args[0], patch)
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, dataset.Record, []string{"id", "dataset_name", "description", "question_count", "created_at", "updated_at"})

			return nil
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "Evaluation dataset name")
	cmd.Flags().StringVar(&description, descriptionFlag, "", "Evaluation dataset description")
	cmd.Flags().StringVar(&questionsFile, questionsFileFlag, "", "Path to a JSON file with 1-50 questions, each with query_number, query_text and expected_answer")

	return cmd
}
