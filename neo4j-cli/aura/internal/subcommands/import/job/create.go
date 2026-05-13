// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package job

import (
	"fmt"
	"log"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		organizationId string
		projectId      string
		importModelId  string
		auraDbId       string
		user           string
		password       string
		importType     flags.ImportType = "online"
	)

	const (
		organizationIdFlag = "organization-id"
		projectIdFlag      = "project-id"
		importModelIdFlag  = "import-model-id"
		dbIdFlag           = "db-id"
		userFlag           = "user"
		passwordFlag       = "password"
		importTypeFlag     = "import-type"
	)
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "create",
		Short:       "Allows you to create a new import job",
		Example: `# Create an online import job targeting an Aura instance
neo4j-cli aura import job create --rw --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --import-model-id 22222222-2222-2222-2222-222222222222 --db-id 33333333 --user neo4j --password mySecret

# Create a bulk import job (overwrites all existing data) with explicit project flags
neo4j-cli aura import job create --rw --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --import-model-id 22222222-2222-2222-2222-222222222222 --db-id 33333333 --user neo4j --password mySecret --import-type bulk

# Create an import job and emit JSON for scripting (capture the returned job id)
neo4j-cli aura import job create --rw --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --import-model-id 22222222-2222-2222-2222-222222222222 --db-id 33333333 --user neo4j --password mySecret --format json`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return utils.SetProjectFlagsAsRequired(cfg, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.SetProjetDefaults(cfg, organizationId, projectId)
			if err != nil {
				return err
			}

			path := fmt.Sprintf("/organizations/%s/projects/%s/import/jobs", organizationId, projectId)

			responseBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:  http.MethodPost,
				Version: api.AuraApiVersion2,
				PostBody: map[string]any{
					"importModelId": importModelId,
					"auraCredentials": map[string]any{
						"dbId":     auraDbId,
						"user":     user,
						"password": password,
					},
					"importConfig": map[string]any{
						"importType": importType,
					},
				},
			})
			if err != nil || statusCode != 201 {
				return err
			}
			output.PrintBody(cmd, cfg, responseBody, []string{"id"})
			return nil
		},
	}

	cmd.Flags().StringVar(&organizationId, organizationIdFlag, "", "(required) Sets the organization ID the job belongs to")
	cmd.Flags().StringVar(&projectId, projectIdFlag, "", "(required) Project/Tenant ID")
	cmd.Flags().StringVar(&importModelId, importModelIdFlag, "", "(required) The model ID can be found in the URL as such console-preview.neo4j.io/tools/import/model/<model ID>.")
	cmd.Flags().StringVar(&auraDbId, dbIdFlag, "", "(required) Aura database ID to import data into. Currently, it's the same as Aura instance ID. In the future, instance ID and database ID are different")
	cmd.Flags().StringVar(&user, userFlag, "", "Username to use for authentication")
	cmd.Flags().StringVar(&password, passwordFlag, "", "Password to use for authentication")
	cmd.Flags().Var(&importType, importTypeFlag, "Type of import to perform. Warning: Bulk imports overwrite all existing data in the database.")

	err := cmd.MarkFlagRequired(importModelIdFlag)
	if err != nil {
		log.Fatal(err)
	}
	err = cmd.MarkFlagRequired(dbIdFlag)
	if err != nil {
		log.Fatal(err)
	}

	return cmd
}
