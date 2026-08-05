// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// MaxRunArgs is the maximum number of args the neo4j_cli_run tool accepts.
const MaxRunArgs = 64

// resolvedCommand is the result of resolving a tool call request into a cobra
// command and execution args. err is non-nil when a pre-exec check (unknown
// command, args cap, policy gate, flag validation, stdin leaf) fails.
type resolvedCommand struct {
	cmd      *cobra.Command
	args     []string // user-supplied args (for policy checks)
	execArgs []string // full arg list for the executor: tokens + args
	command  string   // original command string (for error messages)
	err      *mcpsdk.CallToolResult
}

// resolveCommand extracts command and args from a CallToolRequest, builds a
// fresh config and cobra tree, resolves and validates the command path, runs
// the policy gate, rejects --debug, validates flags, and assembles execArgs.
// The caller is responsible for the policy-specific classification check,
// --rw rejection (HandleRun only), the stdin-leaf check (must run after
// classify), execution, and result mapping.
func resolveCommand(ctx context.Context, req *mcpsdk.CallToolRequest, exec *Executor, gates Gates, newRoot RootFactory) *resolvedCommand {
	command, args := parseRunArgs(req)
	if command == "" {
		return &resolvedCommand{err: runError("Missing required argument: command")}
	}

	if len(args) > MaxRunArgs {
		return &resolvedCommand{err: runError(fmt.Sprintf("args exceeds maximum of %d items", MaxRunArgs))}
	}

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), storedVersion, clicfg.GlobalScope)
	defer cfg.Events.Flush()
	for name, enabled := range storedFlagStates {
		if enabled {
			cfg.Flags.SetForTest(name, true)
		}
	}
	root := newRoot(cfg)

	tokens := strings.Fields(command)
	cmd, _, err := root.Find(tokens)
	if err != nil || cmd == nil || !cmd.IsAvailableCommand() {
		return &resolvedCommand{err: runError(fmt.Sprintf("Unknown command: %q", command))}
	}

	resolvedPath := strings.Join(commandPath(cmd), " ")
	if resolvedPath != command {
		return &resolvedCommand{err: runError(fmt.Sprintf("Unknown command: %q", command))}
	}

	if err := Check(cmd, args, gates); err != nil {
		return &resolvedCommand{err: runError(err.Error())}
	}

	// Reject --debug in args: its traces go to package-global debugW seams
	// whose setters are test-only, making them unreliable under MCP.
	if containsFlag(args, "debug") {
		return &resolvedCommand{err: runError("--debug is not accepted")}
	}

	if err := validateRunFlags(cmd, args); err != nil {
		return &resolvedCommand{err: runError(err.Error())}
	}

	execArgs := append(tokens, args...)
	return &resolvedCommand{
		cmd:      cmd,
		args:     args,
		execArgs: execArgs,
		command:  command,
	}
}

// HandleRun handles the neo4j_cli_run MCP tool call. It resolves the command
// through the cobra tree built by newRoot, checks write classification and
// rejects --rw/--debug, then dispatches through the executor and maps the
// result through MapCommandResult.
//
// newRoot must build a tree with the same flag configuration the server started
// with so flag-gated commands can be resolved (callers should mirror
// storedRootFactory and storedFlagStates).
func HandleRun(ctx context.Context, req *mcpsdk.CallToolRequest, exec *Executor, gates Gates, newRoot RootFactory) (*mcpsdk.CallToolResult, error) {
	r := resolveCommand(ctx, req, exec, gates, newRoot)
	if r.err != nil {
		return r.err, nil
	}

	// Refuse write-classified commands -- write intent must route through
	// neo4j_cli_run_write so annotations stay honest and the write gate in
	// flags.EnforceWriteGate is the authoritative arbiter.
	policy, _ := Classify(r.cmd, r.args)
	if policy == PolicyWrite {
		return runError(fmt.Sprintf("%q is a write command; use neo4j_cli_run_write instead", r.command)), nil
	}

	// Reject --rw in args: write intent must route through neo4j_cli_run_write
	// so annotations stay honest.
	if containsFlag(r.args, "rw") {
		return runError("--rw is not accepted by neo4j_cli_run; use neo4j_cli_run_write instead"), nil
	}

	if len(r.args) == 0 && isStdinLeaf(r.cmd) {
		return runError(fmt.Sprintf("%q requires a positional argument; pass the query as the first item in args", r.command)), nil
	}

	result := exec.Execute(ctx, r.execArgs)
	return MapCommandResult(result, ResultOptions{Args: r.execArgs}), nil
}

// parseRunArgs extracts command and args from a raw CallToolRequest arguments
// map. Both are snake_case per the tool's InputSchema.
func parseRunArgs(req *mcpsdk.CallToolRequest) (command string, args []string) {
	if req == nil || req.Params == nil || req.Params.Arguments == nil {
		return "", nil
	}
	var m map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &m); err != nil {
		return "", nil
	}
	command, _ = m["command"].(string)

	if raw, ok := m["args"].([]any); ok {
		for _, v := range raw {
			s, ok := v.(string)
			if ok {
				args = append(args, s)
			}
		}
	}
	return command, args
}

// runError builds an isError CallToolResult with a plain text message.
func runError(msg string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: msg},
		},
	}
}

// alwaysKnownFlags are flag names that are universally registered on the
// production root (--rw, --format) and so are always valid in any args.
// Listed here rather than expecting every stub tree to register them.
var alwaysKnownFlags = map[string]bool{
	"rw":     true,
	"format": true,
}

// validateRunFlags checks every --flag in args against the command's known
// flag names (local + inherited, excluding hidden). Unknown flags return a
// usage-style error with a did-you-mean suggestion.
func validateRunFlags(cmd *cobra.Command, args []string) error {
	known := knownFlagNames(cmd)
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		flagName, _, _ := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if flagName == "" {
			continue
		}
		if known[flagName] || alwaysKnownFlags[flagName] {
			continue
		}
		if suggestion := closestFlag(flagName, known); suggestion != "" {
			return fmt.Errorf("unknown flag --%s, did you mean --%s?", flagName, suggestion)
		}
		return fmt.Errorf("unknown flag --%s", flagName)
	}
	return nil
}

// knownFlagNames collects every non-hidden flag name from a command, both
// local and inherited, into a lookup set.
func knownFlagNames(cmd *cobra.Command) map[string]bool {
	names := map[string]bool{}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			names[f.Name] = true
		}
	})
	cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			names[f.Name] = true
		}
	})
	return names
}

// closestFlag finds the flag name with the smallest edit distance to query,
// returning it only when the distance is <= 3 and strictly better than any
// other candidate. Returns "" when no candidate is close enough.
func closestFlag(query string, known map[string]bool) string {
	best := ""
	bestDist := 999
	for name := range known {
		d := editDistance(query, name)
		if d > 0 && d < bestDist {
			bestDist = d
			best = name
		}
	}
	if bestDist <= 3 {
		return best
	}
	return ""
}

// editDistance computes the Levenshtein distance between two strings.
func editDistance(s, t string) int {
	if len(s) < len(t) {
		s, t = t, s
	}
	if len(t) == 0 {
		return len(s)
	}
	prev := make([]int, len(t)+1)
	curr := make([]int, len(t)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(s); i++ {
		curr[0] = i
		for j := 1; j <= len(t); j++ {
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}
			del := curr[j-1] + 1
			ins := prev[j] + 1
			sub := prev[j-1] + cost
			if ins < del {
				del = ins
			}
			if sub < del {
				curr[j] = sub
			} else {
				curr[j] = del
			}
		}
		prev, curr = curr, prev
	}
	return prev[len(t)]
}

// isStdinLeaf reports whether the command reads from os.Stdin when no
// positional argument is given — detected via the "stdin-reader" cobra
// annotation. Such commands would hang under an MCP stdio transport because
// os.Stdin carries the protocol frames. Only leaves annotated with
// "stdin-reader"="true" are protected pre-exec; the executor's per-call
// SetIn(bytes.NewReader(nil)) is the general backstop for commands that read
// cmd.InOrStdin().
func isStdinLeaf(cmd *cobra.Command) bool {
	return cmd.Annotations["stdin-reader"] == "true"
}
