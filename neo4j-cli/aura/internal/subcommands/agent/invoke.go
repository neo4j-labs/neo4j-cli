// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

type invokeResponse struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Content   []invokeContent `json:"content"`
	Status    string          `json:"status"`
	EndReason string          `json:"end_reason"`
	Usage     invokeUsage     `json:"usage"`
	Error     *invokeError    `json:"error,omitempty"`
}

type invokeContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type invokeUsage struct {
	RequestTokens  int `json:"request_tokens"`
	ResponseTokens int `json:"response_tokens"`
	TotalTokens    int `json:"total_tokens"`
}

type invokeError struct {
	Message    string `json:"message"`
	Type       string `json:"type"`
	StatusCode int    `json:"status_code"`
}

func newInvokeCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		organizationId string
		projectId      string
		input          string
	)

	const (
		organizationIdFlag = "organization-id"
		projectIdFlag      = "project-id"
		inputFlag          = "input"
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
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return utils.SetProjectFlagsAsRequired(cfg, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.SetProjetDefaults(cfg, organizationId, projectId)
			if err != nil {
				return err
			}

			agentId := args[0]
			path := fmt.Sprintf("/organizations/%s/projects/%s/agents/%s/invoke", organizationId, projectId, agentId)

			body := map[string]any{"input": input}

			cmd.SilenceUsage = true
			var respHeader http.Header
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:         http.MethodPost,
				PostBody:       body,
				Version:        api.AuraApiVersion2,
				ResponseHeader: &respHeader,
			})
			invocationID := respHeader.Get("X-Agent-Invocation-Id")
			if err != nil {
				if statusCode == http.StatusForbidden {
					return withInvocationID(fmt.Errorf("agent invocation forbidden: agent may be disabled or private"), invocationID)
				}
				return withInvocationID(err, invocationID)
			}

			if api.IsSuccessful(statusCode) {
				var result invokeResponse
				if err := json.Unmarshal(resBody, &result); err != nil {
					return fmt.Errorf("unexpected invoke response: %w", err)
				}

				if err := invokeApplicationError(result); err != nil {
					return withInvocationID(err, invocationID)
				}

				printInvokeResult(cmd, cfg, resBody, result, invocationID)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&organizationId, organizationIdFlag, "", "Organization ID")
	cmd.Flags().StringVar(&projectId, projectIdFlag, "", "Project ID")
	cmd.Flags().StringVar(&input, inputFlag, "", "(required) Input message to send to the agent")

	if err := cmd.MarkFlagRequired(inputFlag); err != nil {
		log.Fatal(err)
	}

	return cmd
}

// withInvocationID wraps err with the agent invocation id for support/tracing.
// It is a no-op when err is nil or id is empty.
func withInvocationID(err error, id string) error {
	if err == nil || id == "" {
		return err
	}
	return fmt.Errorf("%w (invocation id: %s)", err, id)
}

// invokeApplicationError returns an error for application-level failures (type "error" with HTTP 200).
func invokeApplicationError(r invokeResponse) error {
	if r.Type != "error" {
		return nil
	}
	if r.Error != nil && r.Error.Message != "" {
		return fmt.Errorf("agent invocation failed: %s", r.Error.Message)
	}
	return fmt.Errorf("agent invocation failed")
}

// printInvokeResult prints the invoke response: raw JSON or text answer + stats line.
func printInvokeResult(cmd *cobra.Command, cfg *clicfg.Config, resBody []byte, result invokeResponse, invocationID string) {
	if commonoutput.ResolveOutput(cmd, cfg) == "json" {
		output.PrintRawBody(cmd, cfg, resBody, nil)
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
