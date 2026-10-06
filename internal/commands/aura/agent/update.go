// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"encoding/json"
	"fmt"
	"github.com/neo4j/cli/internal/auraclient"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func newUpdateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name         string
		description  string
		dbid         string
		isPrivate    bool
		toolsJSON    string
		systemPrompt string
		isMcpEnabled bool
		enabled      bool
	)

	const (
		nameFlag         = "name"
		descriptionFlag  = "description"
		dbidFlag         = "dbid"
		isPrivateFlag    = "is-private"
		toolsFlag        = "tools"
		systemPromptFlag = "system-prompt"
		isMcpEnabledFlag = "is-mcp-enabled"
		enabledFlag      = "enabled"
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

# Update an agent's tools with a text2cypher tool
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --rw

# Update an agent and emit the response as JSON
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --description "updated" --rw --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			var patch auraclient.AgentPatch
			if name != "" {
				patch.Name = &name
			}
			if description != "" {
				patch.Description = &description
			}
			if dbid != "" {
				patch.DBID = &dbid
			}
			if systemPrompt != "" {
				patch.SystemPrompt = &systemPrompt
			}
			if toolsJSON != "" {
				var tools []any
				if err := json.Unmarshal([]byte(toolsJSON), &tools); err != nil {
					return fmt.Errorf("invalid tools JSON: %w", err)
				}
				patch.Tools = tools
			}
			if cmd.Flags().Changed(isPrivateFlag) {
				patch.IsPrivate = &isPrivate
			}
			if cmd.Flags().Changed(isMcpEnabledFlag) {
				patch.IsMCPEnabled = &isMcpEnabled
			}
			if cmd.Flags().Changed(enabledFlag) {
				patch.Enabled = &enabled
			}

			cmd.SilenceUsage = true
			agent, err := auraclient.New(cfg).Agents().Update(cmd.Context(), auraclient.Scope{OrgID: organizationId, ProjectID: projectId}, args[0], patch)
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, agent.Record, []string{"id", "name", "description", "dbid", "is_private", "is_mcp_enabled", "enabled"})

			return nil
		},
	}

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
