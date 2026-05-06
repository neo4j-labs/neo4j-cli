// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package deployment

import (
	"fmt"
	"log"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		organizationId string
		projectId      string
		name           string
		connectionUrl  string
	)

	const (
		organizationIdFlag = "organization-id"
		projectIdFlag      = "project-id"
		nameFlag           = "name"
		connectionUrlFlag  = "connection-url"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "create",
		Short:       "Create a new deployment",
		Long:        "Creates a new unmonitored Fleet Manager deployment.",
		Args:        cobra.ExactArgs(0),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return utils.SetProjectFlagsAsRequired(cfg, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.SetProjetDefaults(cfg, organizationId, projectId)
			if err != nil {
				return err
			}

			path := fmt.Sprintf("/organizations/%s/projects/%s/fleet-manager/deployments", organizationId, projectId)

			body := map[string]any{
				"name":           name,
				"connection_url": connectionUrl,
			}

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:   http.MethodPost,
				PostBody: body,
				Version:  api.AuraApiVersion2,
			})
			if err != nil {
				return err
			}

			if api.IsSuccessful(statusCode) {
				output.PrintBody(cmd, cfg, resBody, []string{"id"})
			}

			return nil
		},
	}
	cmd.Flags().StringVar(&organizationId, organizationIdFlag, "", "(required) Organization ID")
	cmd.Flags().StringVar(&projectId, projectIdFlag, "", "(required) Project/tenant ID")
	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Deployment name")
	cmd.Flags().StringVar(&connectionUrl, connectionUrlFlag, "", "An optional connection URL for the deployment")

	err := cmd.MarkFlagRequired(nameFlag)
	if err != nil {
		log.Fatal(err)
	}

	return cmd
}
