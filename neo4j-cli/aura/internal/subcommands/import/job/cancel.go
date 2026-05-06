// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package job

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewCancelCommand(cfg *clicfg.Config) *cobra.Command {
	var (
		organizationId string
		projectId      string
		jobId          string
	)

	const (
		organizationIdFlag = "organization-id"
		projectIdFlag      = "project-id"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "cancel <id>",
		Short:       "Cancel a job by id",
		Args:        cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return utils.SetProjectFlagsAsRequired(cfg, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.SetProjetDefaults(cfg, organizationId, projectId)
			if err != nil {
				return err
			}

			jobId = args[0]
			path := fmt.Sprintf("/organizations/%s/projects/%s/import/jobs/%s/cancellation", organizationId, projectId, jobId)
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:  http.MethodPost,
				Version: api.AuraApiVersion2,
			})
			if err != nil || statusCode != http.StatusOK {
				return err
			}
			output.PrintBody(cmd, cfg, resBody, []string{"id"})

			return nil
		},
	}
	cmd.Flags().StringVar(&organizationId, organizationIdFlag, "", "(required) Organization ID")
	cmd.Flags().StringVar(&projectId, projectIdFlag, "", "(required) Project/tenant ID")

	return cmd
}
