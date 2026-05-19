// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewUpdateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		organizationId string
		projectId      string
		name           string
		description    string
		dbid           string
		isPrivate      bool
		toolsJSON      string
		systemPrompt   string
		isMcpEnabled   bool
		enabled        bool
	)

	const (
		organizationIdFlag = "organization-id"
		projectIdFlag      = "project-id"
		nameFlag           = "name"
		descriptionFlag    = "description"
		dbidFlag           = "dbid"
		isPrivateFlag      = "is-private"
		toolsFlag          = "tools"
		systemPromptFlag   = "system-prompt"
		isMcpEnabledFlag   = "is-mcp-enabled"
		enabledFlag        = "enabled"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "update <id>",
		Short:       "Partially updates an existing agent",
		Long:        "Partially updates an existing agent's configuration. Only provided fields are updated (PATCH semantics).",
		Example: `# Rename an agent
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --name my-renamed-agent --rw

# Disable an agent
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --enabled=false --rw

# Update an agent and emit the response as JSON
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --description "updated" --rw --format json`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return utils.SetProjectFlagsAsRequired(cfg, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.SetProjetDefaults(cfg, organizationId, projectId)
			if err != nil {
				return err
			}

			body := map[string]any{}

			if name != "" {
				body["name"] = name
			}
			if description != "" {
				body["description"] = description
			}
			if dbid != "" {
				body["dbid"] = dbid
			}
			if systemPrompt != "" {
				body["system_prompt"] = systemPrompt
			}
			if toolsJSON != "" {
				var tools []any
				if err := json.Unmarshal([]byte(toolsJSON), &tools); err != nil {
					return fmt.Errorf("invalid tools JSON: %w", err)
				}
				body["tools"] = tools
			}
			if cmd.Flags().Changed(isPrivateFlag) {
				body["is_private"] = isPrivate
			}
			if cmd.Flags().Changed(isMcpEnabledFlag) {
				body["is_mcp_enabled"] = isMcpEnabled
			}
			if cmd.Flags().Changed(enabledFlag) {
				body["enabled"] = enabled
			}

			agentId := args[0]
			path := fmt.Sprintf("/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId)

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:   http.MethodPatch,
				PostBody: body,
				Version:  api.AuraApiVersion2,
			})
			if err != nil {
				return err
			}

			if api.IsSuccessful(statusCode) {
				output.PrintRawBody(cmd, cfg, resBody, []string{"id", "name", "description", "dbid", "is_private", "is_mcp_enabled", "enabled"})
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&organizationId, organizationIdFlag, "", "(required) Organization ID")
	cmd.Flags().StringVar(&projectId, projectIdFlag, "", "(required) Project/tenant ID")
	cmd.Flags().StringVar(&name, nameFlag, "", "Agent name")
	cmd.Flags().StringVar(&description, descriptionFlag, "", "Agent description")
	cmd.Flags().StringVar(&dbid, dbidFlag, "", "Aura database instance ID the agent connects to")
	cmd.Flags().BoolVar(&isPrivate, isPrivateFlag, false, "Whether the agent is private")
	cmd.Flags().StringVar(&toolsJSON, toolsFlag, "", "Tools configuration as a JSON array")
	cmd.Flags().StringVar(&systemPrompt, systemPromptFlag, "", "System prompt for the agent")
	cmd.Flags().BoolVar(&isMcpEnabled, isMcpEnabledFlag, false, "Whether MCP is enabled for the agent")
	cmd.Flags().BoolVar(&enabled, enabledFlag, true, "Whether the agent is enabled")

	cmd.MarkFlagsOneRequired(nameFlag, descriptionFlag, dbidFlag, toolsFlag, systemPromptFlag, isPrivateFlag, isMcpEnabledFlag, enabledFlag)

	return cmd
}
