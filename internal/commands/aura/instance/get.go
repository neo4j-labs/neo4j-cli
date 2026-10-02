// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"github.com/neo4j/cli/internal/aura"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
)

func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Returns instance details",
		Long:  "This endpoint returns details about a specific Aura Instance.",
		Example: `# Get details of an instance by ID
neo4j-cli aura instance get 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get details and emit JSON for scripting
neo4j-cli aura instance get 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json

# Pipe details through jq to extract the connection URL
neo4j-cli aura instance get 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json | jq -r '.data.connection_url'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceId := strings.TrimSpace(args[0])

			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}
			inst, err := aura.New(cfg).Instances().Get(cmd.Context(), aura.Scope{OrgID: orgID, ProjectID: projectID}, instanceId)
			if err != nil {
				return err
			}

			output.PrintRecord(cmd, cfg, inst.Record, getFields(inst.Record))

			return nil
		},
	}
}

func getFields(instance map[string]any) []string {
	fields := []string{"id", "name", "project_id", "status", "connection_url", "cloud_provider", "region", "type", "memory", "storage", "customer_managed_key_id"}
	if HasMetricsIntegrationEndpointUrl(instance) {
		fields = append(fields, "metrics_integration_url")
	}
	return fields
}

func HasMetricsIntegrationEndpointUrl(instance map[string]any) bool {
	cmiEndpointUrl := instance["metrics_integration_url"]
	switch cmiEndpointUrl := cmiEndpointUrl.(type) {
	case string:
		return len(cmiEndpointUrl) > 0
	}
	return false
}
