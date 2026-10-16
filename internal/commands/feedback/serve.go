// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"context"
	"encoding/json"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

// feedbackToolName and feedbackToolDescription are fixed: changing the wording changes how often agents report.
const (
	feedbackToolName        = "feedback_submit"
	feedbackToolDescription = "Report a problem with neo4j-cli itself to the Neo4j team. Call this when neo4j-cli prevented or complicated your task: " +
		"a missing capability, an unclear or unhelpful error, behavior that differs from its help, or impractical performance, even if you found a workaround. " +
		"Describe what you expected and what happened, in your own words, each under 255 characters. " +
		"Never include queries, parameters, schema names, results, file paths, connection details, credentials, names or identifiers. " +
		"Report each distinct problem once, then continue your task."
)

// feedbackToolSchema is the tool's input schema: category, expected and
// observed (maxLength 255) are required; command is optional.
func feedbackToolSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category": map[string]any{"type": "string", "enum": validCategories},
			"expected": map[string]any{"type": "string", "maxLength": maxFieldBytes},
			"observed": map[string]any{"type": "string", "maxLength": maxFieldBytes},
			"command":  map[string]any{"type": "string"},
		},
		"required":             []string{"category", "expected", "observed"},
		"additionalProperties": false,
	}
}

// newServeCmd builds the hidden stdio MCP server with the single feedback_submit
// tool. It runs the same checks as `feedback submit` and returns the same
// outcome JSON as the tool result. Not `neo4j-cli mcp`: only this one tool is exposed.
func newServeCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "serve",
		Short:  "Serve the feedback_submit tool over MCP (stdio)",
		Long:   "Start a stdio MCP server exposing one tool, feedback_submit. It applies the same authorization, endpoint, validation and receipt rules as `neo4j-cli feedback submit`.",
		Hidden: true,
		// serve speaks MCP over the process's own stdin and stdout.
		Annotations: map[string]string{"stdin-reader": "true"},
		Example: `# Start the feedback-only MCP server (an MCP client launches this and speaks JSON-RPC on stdin/stdout)
neo4j-cli feedback serve

# Same, with JSON as the output format for any error envelope
neo4j-cli feedback serve --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return newFeedbackServer(cfg, cmd.Root()).Run(cmd.Context(), &mcpsdk.StdioTransport{})
		},
	}
	return cmd
}

// newFeedbackServer builds the MCP server (separate from the stdio wiring so tests can use an in-memory transport).
func newFeedbackServer(cfg *clicfg.Config, tree *cobra.Command) *mcpsdk.Server {
	s := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "neo4j-cli-feedback", Version: cfg.Version},
		&mcpsdk.ServerOptions{Instructions: serverInstructions(lookupEnv(instructionsEnv))})
	consent := &consentSession{}
	s.AddTool(&mcpsdk.Tool{
		Name:        feedbackToolName,
		Description: feedbackToolDescription,
		InputSchema: feedbackToolSchema(),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		var args struct {
			Category string `json:"category"`
			Expected string `json:"expected"`
			Observed string `json:"observed"`
			Command  string `json:"command"`
		}
		if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return toolText("invalid arguments: "+err.Error(), true), nil
			}
		}
		out := submitReport(ctx, cfg, tree, Input{Category: args.Category, Expected: args.Expected, Observed: args.Observed, Command: args.Command}, elicitConsent(req.Session, consent))
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return toolText(err.Error(), true), nil
		}
		return toolText(string(b), out.Outcome != outcomeSent && out.Outcome != outcomeSkipped), nil
	})
	return s
}

func toolText(text string, isErr bool) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: text}}, IsError: isErr}
}
