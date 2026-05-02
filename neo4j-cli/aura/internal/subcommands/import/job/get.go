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

func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		organizationId string
		projectId      string
		jobId          string
		showProgress   bool
	)

	const (
		organizationIdFlag = "organization-id"
		projectIdFlag      = "project-id"
		showProgressFlag   = "progress"
	)

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a job by id",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return utils.SetProjectFlagsAsRequired(cfg, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.SetProjetDefaults(cfg, organizationId, projectId)
			if err != nil {
				return err
			}

			jobId = args[0]
			path := fmt.Sprintf("/organizations/%s/projects/%s/import/jobs/%s", organizationId, projectId, jobId)

			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:  http.MethodGet,
				Version: api.AuraApiVersion2,
				QueryParams: map[string]string{
					"progress": fmt.Sprintf("%v", showProgress),
				},
			})
			if err != nil {
				return err
			}
			outputType := cfg.Global.Output()

			if statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "import_type", "info:state", "info:exit_status:state", "info:percentage_complete", "data_source:name", "aura_target:db_id"})
				if outputType != "json" {
					output.PrintBody(cmd, cfg, resBody, []string{"info:exit_status:message"})
				}
			}

			if showProgress && outputType != "json" {
				printJobProgressTable(cmd, cfg, resBody)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&organizationId, organizationIdFlag, "", "(required) Organization ID targeting for import job")
	cmd.Flags().StringVar(&projectId, projectIdFlag, "", "(required) Project/tenant ID")
	cmd.Flags().BoolVar(&showProgress, showProgressFlag, false, "Show progress details")

	return cmd
}

func printJobProgressTable(cmd *cobra.Command, cfg *clicfg.Config, resBody []byte) {
	cmd.Println("# Progress details:")
	parsedBody := api.ParseBody(resBody)
	data, err := parsedBody.GetSingleOrError()
	if err != nil {
		panic(err)
	}
	info := data["info"].(map[string]interface{})
	progress := info["progress"].(map[string]interface{})
	nodes := progress["nodes"].([]interface{})
	wrappedNodes := make([]map[string]any, 0)
	for _, node := range nodes {
		wrappedNodes = append(wrappedNodes, node.(map[string]interface{}))
	}
	nodesResponseData := api.NewListResponseData(wrappedNodes)
	cmd.Println("# Nodes progress:")
	output.PrintBodyMap(cmd, cfg, nodesResponseData, []string{"id", "labels", "processed_rows", "total_rows", "created_nodes", "created_constraints", "created_indexes"})

	relationships := progress["relationships"].([]interface{})
	wrappedRelationships := make([]map[string]any, 0)
	for _, relationship := range relationships {
		wrappedRelationships = append(wrappedRelationships, relationship.(map[string]interface{}))
	}
	relationshipsResponseData := api.NewListResponseData(wrappedRelationships)
	cmd.Println("# Relationships progress:")
	output.PrintBodyMap(cmd, cfg, relationshipsResponseData, []string{"id", "type", "processed_rows", "total_rows", "created_relationships", "created_constraints", "created_indexes"})
}
