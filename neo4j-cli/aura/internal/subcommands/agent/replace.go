// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewReplaceCmd(cfg *clicfg.Config) *cobra.Command {
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
		Use:         "replace <id>",
		Short:       "Fully replaces an existing agent",
		Long:        "Fully replaces an existing agent's configuration. All fields are required (PUT semantics).",
		Example: `# Replace an agent's full definition with a text2cypher tool
neo4j-cli aura agent replace 00000000-0000-0000-0000-000000000000 --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --rw

# Replace an agent with a system prompt
neo4j-cli aura agent replace 00000000-0000-0000-0000-000000000000 --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --system-prompt "you are helpful" --rw

# Replace an agent and emit the response as JSON
neo4j-cli aura agent replace 00000000-0000-0000-0000-000000000000 --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --rw --format json`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return utils.SetProjectFlagsAsRequired(cfg, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.SetProjetDefaults(cfg, organizationId, projectId)
			if err != nil {
				return err
			}

			var tools []any
			if err := json.Unmarshal([]byte(toolsJSON), &tools); err != nil {
				return fmt.Errorf("invalid tools JSON: %w", err)
			}

			agentId := args[0]
			path := fmt.Sprintf("/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId)

			body := map[string]any{
				"name":           name,
				"description":    description,
				"dbid":           dbid,
				"is_private":     isPrivate,
				"tools":          tools,
				"system_prompt":  systemPrompt,
				"is_mcp_enabled": isMcpEnabled,
				"enabled":        enabled,
			}

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:   http.MethodPut,
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

	cmd.Flags().StringVar(&organizationId, organizationIdFlag, "", "Organization ID")
	cmd.Flags().StringVar(&projectId, projectIdFlag, "", "Project/tenant ID")
	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Agent name")
	cmd.Flags().StringVar(&description, descriptionFlag, "", "(required) Agent description")
	cmd.Flags().StringVar(&dbid, dbidFlag, "", "(required) Aura database instance ID the agent connects to")
	cmd.Flags().BoolVar(&isPrivate, isPrivateFlag, false, "Whether the agent is private")
	cmd.Flags().StringVar(&toolsJSON, toolsFlag, "", "(required) Tools configuration as a JSON array")
	cmd.Flags().StringVar(&systemPrompt, systemPromptFlag, "", "System prompt for the agent")
	cmd.Flags().BoolVar(&isMcpEnabled, isMcpEnabledFlag, false, "Whether MCP is enabled for the agent")
	cmd.Flags().BoolVar(&enabled, enabledFlag, true, "Whether the agent is enabled")

	for _, f := range []string{nameFlag, descriptionFlag, dbidFlag, toolsFlag} {
		if err := cmd.MarkFlagRequired(f); err != nil {
			log.Fatal(err)
		}
	}

	return cmd
}
