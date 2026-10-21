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

func newCreateCmd(cfg *clicfg.Config) *cobra.Command {
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
		Use:         "create",
		Short:       "Creates a new evaluation dataset",
		Long:        "Creates a new evaluation dataset for the specified project from a JSON questions file holding 1-50 questions.",
		Example: `# Create an evaluation dataset from a questions file
neo4j-cli aura agent evaluation-dataset create --name "Product Q&A" --questions-file ./questions.json --rw

# Create an evaluation dataset with a description in a specific organization and project
neo4j-cli aura agent evaluation-dataset create --name "Product Q&A" --description "Smoke tests" --questions-file ./questions.json --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000 --rw

# Create an evaluation dataset and emit the response as JSON
neo4j-cli aura agent evaluation-dataset create --name "Product Q&A" --questions-file ./questions.json --rw --format json`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			organizationId, projectId, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			questions, err := readQuestionsFile(cfg.Fs(), questionsFile)
			if err != nil {
				return err
			}

			dataset, err := auraclient.New(cfg).EvaluationDatasets().Create(cmd.Context(), auraclient.Scope{OrgID: organizationId, ProjectID: projectId}, auraclient.EvaluationDatasetSpec{
				Name:        name,
				Description: description,
				Questions:   questions,
			})
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, dataset.Record, []string{"id", "dataset_name", "description", "question_count", "created_at", "updated_at"})

			return nil
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Evaluation dataset name")
	cmd.Flags().StringVar(&description, descriptionFlag, "", "Optional evaluation dataset description")
	cmd.Flags().StringVar(&questionsFile, questionsFileFlag, "", "(required) Path to a JSON file with 1-50 questions, each with query_number, query_text and expected_answer")

	for _, f := range []string{nameFlag, questionsFileFlag} {
		cmd.MarkFlagRequired(f) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
	}

	return cmd
}
