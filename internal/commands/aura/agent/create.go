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

func newCreateCmd(cfg *clicfg.Config) *cobra.Command {
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
		Use:         "create",
		Short:       "Creates a new agent",
		Long:        "Creates a new agent for the specified project.",
		Example: `# Create an agent with a text2cypher tool
neo4j-cli aura agent create --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --rw

# Create an agent with a system prompt
neo4j-cli aura agent create --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --system-prompt "you are helpful" --rw

# Create an agent and emit the response as JSON
neo4j-cli aura agent create --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools '[{"name":"query-tool","type":"text2cypher","description":"Converts natural language to Cypher queries","enabled":true}]' --rw --format json`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			var tools []any
			if err := json.Unmarshal([]byte(toolsJSON), &tools); err != nil {
				return fmt.Errorf("invalid tools JSON: %w", err)
			}

			cmd.SilenceUsage = true
			agent, err := auraclient.New(cfg).Agents().Create(cmd.Context(), auraclient.Scope{OrgID: organizationId, ProjectID: projectId}, auraclient.AgentSpec{
				Name:         name,
				Description:  description,
				DBID:         dbid,
				IsPrivate:    isPrivate,
				Tools:        tools,
				SystemPrompt: systemPrompt,
				IsMCPEnabled: isMcpEnabled,
				Enabled:      enabled,
			})
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, agent.Record, []string{"id", "name", "description", "dbid", "is_private", "is_mcp_enabled", "enabled"})

			return nil
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Agent name")
	cmd.Flags().StringVar(&description, descriptionFlag, "", "(required) Agent description")
	cmd.Flags().StringVar(&dbid, dbidFlag, "", "(required) Aura database instance ID the agent connects to")
	cmd.Flags().BoolVar(&isPrivate, isPrivateFlag, false, "Whether the agent is private")
	cmd.Flags().StringVar(&toolsJSON, toolsFlag, "", "(required) Tools configuration as a JSON array")
	cmd.Flags().StringVar(&systemPrompt, systemPromptFlag, "", "Optional system prompt for the agent")
	cmd.Flags().BoolVar(&isMcpEnabled, isMcpEnabledFlag, false, "Whether MCP is enabled for the agent")
	cmd.Flags().BoolVar(&enabled, enabledFlag, true, "Whether the agent is enabled")

	for _, f := range []string{nameFlag, descriptionFlag, dbidFlag, toolsFlag} {
		cmd.MarkFlagRequired(f) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
	}

	return cmd
}
