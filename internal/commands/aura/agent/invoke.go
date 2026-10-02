// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"fmt"
	"github.com/neo4j/cli/internal/aura"
	"log"
	"strings"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

func newInvokeCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		input string
	)

	const (
		inputFlag = "input"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "invoke <id>",
		Short:       "Invoke an agent with an input prompt",
		Long:        "Invokes an agent with the provided input string. Use --format json for the full response including content blocks.",
		Example: `# Invoke an agent with a prompt
neo4j-cli aura agent invoke 00000000-0000-0000-0000-000000000000 --input "hello" --rw

# Invoke an agent in a specific organization and project
neo4j-cli aura agent invoke 00000000-0000-0000-0000-000000000000 --input "hello" --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000 --rw

# Invoke an agent and emit the response as JSON
neo4j-cli aura agent invoke 00000000-0000-0000-0000-000000000000 --input "hello" --rw --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true
			result, err := aura.New(cfg).Agents().Invoke(cmd.Context(), aura.Scope{OrgID: organizationId, ProjectID: projectId}, args[0], input)
			if err != nil {
				return err
			}
			printInvokeResult(cmd, cfg, result)

			return nil
		},
	}

	cmd.Flags().StringVar(&input, inputFlag, "", "(required) Input message to send to the agent")

	if err := cmd.MarkFlagRequired(inputFlag); err != nil {
		log.Fatal(err)
	}

	return cmd
}

// withInvocationID appends the agent invocation id to err for support/tracing.
// It is a no-op when err is nil or id is empty.
func printInvokeResult(cmd *cobra.Command, cfg *clicfg.Config, result *aura.AgentInvocation) {
	invocationID := result.InvocationID
	if commonoutput.ResolveOutput(cmd, cfg) == "json" {
		output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(result.Record), nil)
		if invocationID != "" {
			cmd.PrintErrln("Invocation ID: " + invocationID)
		}
		return
	}

	var texts []string
	toolCalls := 0
	for _, block := range result.Content {
		switch {
		case block.Type == "text":
			texts = append(texts, block.Text)
		case strings.HasSuffix(block.Type, "tool_use"):
			toolCalls++
		}
	}

	if len(texts) > 0 {
		cmd.Println(strings.Join(texts, "\n"))
	}

	statsLine := fmt.Sprintf("\nStatus: %s | End reason: %s | Tool calls: %d | Tokens: %d req / %d res / %d total",
		strings.ToUpper(result.Status),
		strings.ToUpper(strings.ReplaceAll(result.EndReason, "_", " ")),
		toolCalls,
		result.Usage.RequestTokens,
		result.Usage.ResponseTokens,
		result.Usage.TotalTokens,
	)
	if invocationID != "" {
		statsLine += " | Invocation ID: " + invocationID
	}
	cmd.Println(statsLine)
}
