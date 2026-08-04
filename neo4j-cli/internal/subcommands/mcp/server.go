// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"context"
	"encoding/json"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clicfg"
)

// serverState holds the server's dependencies, assembled once when the server
// is created and used for every tool call. It is unexported so transport wiring
// is the only access path.
type serverState struct {
	exec           *Executor
	gates          Gates
	newRoot        RootFactory
	defaultFormat  string
	maxOutputChars int
}

// NewServer creates an MCP server with the five neo4j_cli_* tools registered,
// plus the instructions field. It is testable without a transport: after creating
// the server, connect it to an SDK transport (e.g. NewInMemoryTransports) and
// call Server.Run or Server.Connect.
func NewServer(cfg *clicfg.Config, exec *Executor, gates Gates, defaultFormat string, maxOutputChars int) (*mcpsdk.Server, error) {
	if err := validateWiring(cfg, storedRootFactory); err != nil {
		return nil, err
	}

	state := &serverState{
		exec:           exec,
		gates:          gates,
		newRoot:        storedRootFactory,
		defaultFormat:  defaultFormat,
		maxOutputChars: maxOutputChars,
	}

	s := mcpsdk.NewServer(
		&mcpsdk.Implementation{
			Name:    "neo4j-cli",
			Version: storedVersion,
		},
		&mcpsdk.ServerOptions{
			Instructions: instructions(),
		},
	)

	for _, tool := range toolDefinitions() {
		t := tool // copy the pointer so each handler closure captures its tool
		s.AddTool(t, state.handlerFor(t.Name))
	}

	return s, nil
}

// instructions returns the MCP instructions field, sent once at initialize.
// It carries the 12-tree top-level index plus orientation rules. Content that
// would otherwise inflate per-request tool descriptions lives here instead.
func instructions() string {
	return `Neo4j CLI — Manage Neo4j databases through your terminal.

Available command trees:
admin        — Manage Neo4j administration
agent-context — Display live CLI context for AI agents
aura         — Manage Neo4j Aura instances
config       — Manage configuration
credential   — Manage stored credentials
dataset      — Manage example datasets
desktop      — Manage Neo4j Desktop
docker       — Manage Docker containers
history      — View and clear command history
query        — Run Cypher queries
skill        — Manage agent skill bundles
update       — Update neo4j-cli

Rules:
- Before writing Cypher, run 'query :schema' to see the graph schema.
- Use neo4j_cli_read_docs to learn a command's flags before calling a run tool.
- Never call neo4j_cli_run_write without user confirmation.
- Start with neo4j_cli_list_targets to discover available databases.
- Use neo4j_cli_list_commands to explore available commands.`
}

// handlerFor returns the tool handler for the named tool, wrapping read-run
// and write-run handlers to inject the default --format when args do not
// already carry one.
func (s *serverState) handlerFor(name string) func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	switch name {
	case "neo4j_cli_list_commands":
		return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return HandleListCommands(ctx, req)
		}
	case "neo4j_cli_read_docs":
		return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return HandleReadDocs(ctx, req)
		}
	case "neo4j_cli_run":
		return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			modifiedReq := injectFormat(req, s.defaultFormat)
			return HandleRun(ctx, modifiedReq, s.exec, s.gates, s.newRoot)
		}
	case "neo4j_cli_run_write":
		return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			modifiedReq := injectFormat(req, s.defaultFormat)
			modifiedReq = injectNoPrintPassword(modifiedReq)
			return HandleRunWrite(ctx, modifiedReq, s.exec, s.gates, s.newRoot)
		}
	case "neo4j_cli_list_targets":
		return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return HandleListTargets(ctx, s.exec)
		}
	default:
		return nil
	}
}

// injectFormat injects --format <defaultFormat> into the args array when the
// request does not already carry a --format flag. format is deliberately not
// a tool parameter (REQ-F-021): tool parameters are what the model chooses,
// and output format is a server configuration that does not vary per call.
func injectFormat(req *mcpsdk.CallToolRequest, format string) *mcpsdk.CallToolRequest {
	if req == nil || req.Params == nil || req.Params.Arguments == nil || format == "" {
		return req
	}
	var args map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return req
	}
	existing, _ := args["args"].([]any)
	if hasFormatFlag(existing) {
		return req
	}
	// Build --format <val> args preserving the model-supplied args after.
	var newArgs []any
	if len(existing) == 0 {
		// "args" may be absent entirely (nil) or present with zero items.
		// In either case, create the array rather than mutating nil so the
		// remarshalled JSON carries the injected flag regardless.
		newArgs = make([]any, 0, 4)
	}
	newArgs = append(newArgs, "--format", format)
	newArgs = append(newArgs, existing...)
	args["args"] = newArgs

	raw, err := json.Marshal(args)
	if err != nil {
		return req
	}
	return &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: raw,
		},
	}
}

// hasFormatFlag checks whether args already contains a --format flag, in any
// supported spelling (--format toon, --format=json).
func hasFormatFlag(args []any) bool {
	for _, a := range args {
		s, ok := a.(string)
		if !ok {
			continue
		}
		if strings.HasPrefix(s, "--format") {
			return true
		}
	}
	return false
}

// maxOutputChars stores the --max-output-chars value from serve for future
// wiring into MapCommandResult's ResultOptions. Currently unused: handlers
// use DefaultMaxOutputChars (8000) which matches the flag's default.

// injectNoPrintPassword injects --no-print-password into the args array when
// the command is "docker create" and the flag is not already present. Under MCP
// a generated password must not enter the model context.
func injectNoPrintPassword(req *mcpsdk.CallToolRequest) *mcpsdk.CallToolRequest {
	if req == nil || req.Params == nil || req.Params.Arguments == nil {
		return req
	}
	var args map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return req
	}
	command, _ := args["command"].(string)
	if command != "docker create" {
		return req
	}
	existing, _ := args["args"].([]any)
	if hasNoPrintPasswordFlag(existing) {
		return req
	}
	// Prepend --no-print-password so docker create omits the password from its
	// output without the model having to know about the flag.
	newArgs := make([]any, 0, len(existing)+2)
	newArgs = append(newArgs, "--no-print-password")
	newArgs = append(newArgs, existing...)
	args["args"] = newArgs

	raw, err := json.Marshal(args)
	if err != nil {
		return req
	}
	return &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: raw,
		},
	}
}

// hasNoPrintPasswordFlag reports whether args already contains
// --no-print-password in any supported spelling.
func hasNoPrintPasswordFlag(args []any) bool {
	for _, a := range args {
		s, ok := a.(string)
		if !ok {
			continue
		}
		if s == "--no-print-password" || strings.HasPrefix(s, "--no-print-password=") {
			return true
		}
	}
	return false
}
