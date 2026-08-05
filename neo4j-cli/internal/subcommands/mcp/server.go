// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"context"
	"encoding/json"
	"strconv"
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
- Use neo4j_cli_list_commands to explore available commands.

Aura scoping:
- Pair --organization-id and --project-id from the same org. A project ID is only valid within its own organization.
- A project/tenant not-found is a scoping error, not an auth error. Retrying or re-authenticating will not help.`
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
			modifiedReq = injectFlag(modifiedReq, shouldInjectNoPrintPassword, "--no-print-password")
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
	if format == "" {
		return req
	}
	return injectFlag(req, shouldInjectFormat, "--format", format)
}

func shouldInjectFormat(args map[string]any) bool {
	existing, _ := args["args"].([]any)
	for _, a := range existing {
		s, ok := a.(string)
		if !ok {
			continue
		}
		if s == "--format" || strings.HasPrefix(s, "--format=") {
			return false
		}
	}
	return true
}

// injectFlag injects flag items into the request's args when the predicate
// returns true. The predicate receives the unmarshalled Arguments map and
// should return true when injection should proceed.
func injectFlag(req *mcpsdk.CallToolRequest, predicate func(map[string]any) bool, flagItems ...string) *mcpsdk.CallToolRequest {
	if req == nil || req.Params == nil || req.Params.Arguments == nil {
		return req
	}
	var args map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return req
	}
	if !predicate(args) {
		return req
	}
	existing, _ := args["args"].([]any)
	// No capacity hint: `existing` is client-supplied and reaches this
	// point before the handlers' MaxRunArgs cap in resolveCommand runs, so
	// sizing an allocation from its length is an overflow risk. Truncating
	// it here is not an option either — that would bring an oversized
	// request back under the handlers' cap and silently execute it with the
	// tail dropped. Let append grow the slice instead; it is capped at
	// MaxRunArgs downstream and the growth cost is irrelevant at that size.
	var newArgs []any
	newArgs = append(newArgs, existing...)
	// Injected flags go LAST: pflag resolves repeated flags to the final
	// occurrence, so prepending would let a caller-supplied
	// `--no-print-password=false` override our injected suppression.
	for _, f := range flagItems {
		newArgs = append(newArgs, f)
	}
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

// maxOutputChars stores the --max-output-chars value from serve for future
// wiring into MapCommandResult's ResultOptions. Currently unused: handlers
// use DefaultMaxOutputChars (8000) which matches the flag's default.

func shouldInjectNoPrintPassword(args map[string]any) bool {
	command, _ := args["command"].(string)
	// Normalise whitespace the same way resolveCommand's strings.Fields does.
	// Exact equality here would let "docker  create" skip password
	// suppression while still resolving and executing as `docker create`,
	// leaking the generated password into the tool result.
	if strings.Join(strings.Fields(command), " ") != "docker create" {
		return false
	}
	existing, _ := args["args"].([]any)
	for _, a := range existing {
		s, ok := a.(string)
		if !ok {
			continue
		}
		// Only skip injection when the caller already ENABLES suppression.
		// An explicit --no-print-password=false must still inject, because
		// injectFlag appends last and pflag resolves to the final
		// occurrence — otherwise the caller could disable suppression and
		// surface the generated password, which docker create does not
		// pass to clievents.RegisterSecretValue.
		if s == "--no-print-password" {
			return false
		}
		if v, found := strings.CutPrefix(s, "--no-print-password="); found {
			if enabled, err := strconv.ParseBool(v); err == nil && enabled {
				return false
			}
			return true
		}
	}
	return true
}
