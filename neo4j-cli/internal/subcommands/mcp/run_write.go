// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"context"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/afero"
)

// HandleRunWrite handles the neo4j_cli_run_write MCP tool call. It accepts only
// write-classified commands (PolicyWrite or gated policies) and gates them
// through three independent layers:
//
//  1. Process gate (WriteAllowed in Gates) — rejects every call when the server
//     started without --rw. Task-013 wires this from the serve --rw flag.
//  2. Per-call policy gate — mcp.Check rejects deny-classified paths and
//     gated policies whose --allow-* flag is not set.
//  3. CLI's own flags.EnforceWriteGate — fires inside Execute() off
//     cmd.Annotations["write"]. Authoritative: a bug in the MCP layers cannot
//     produce an unflagged write.
//
// This function reuses the same helpers as HandleRun (parseRunArgs,
// validateRunFlags, isStdinLeaf, runError) rather than duplicating them.
func HandleRunWrite(ctx context.Context, req *mcpsdk.CallToolRequest, exec *Executor, gates Gates, newRoot RootFactory) (*mcpsdk.CallToolResult, error) {
	// Layer 1: Process gate — the server was started without --rw.
	if !gates.WriteAllowed {
		return runError("Write operations are not allowed; restart the server with `neo4j-cli mcp serve --rw` to enable them"), nil
	}

	command, args := parseRunArgs(req)
	if command == "" {
		return runError("Missing required argument: command"), nil
	}

	if len(args) > MaxRunArgs {
		return runError(fmt.Sprintf("args exceeds maximum of %d items", MaxRunArgs)), nil
	}

	// Build tree with stored flag states so flag-gated commands are resolvable.
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
		return runError(fmt.Sprintf("Unknown command: %q", command)), nil
	}

	resolvedPath := strings.Join(commandPath(cmd), " ")
	if resolvedPath != command {
		return runError(fmt.Sprintf("Unknown command: %q", command)), nil
	}

	// Layer 2: Policy check — refuse deny-classified paths and closed gates.
	if err := Check(cmd, args, gates); err != nil {
		return runError(err.Error()), nil
	}

	// Refuse non-write-classified commands — read intent must route through
	// neo4j_cli_run. Accept PolicyWrite and gated policies (write intersection
	// gated).
	policy, _ := Classify(cmd, args)
	if policy == PolicyAllow || policy == PolicyDeny {
		return runError(fmt.Sprintf("%q is a read-only command; use neo4j_cli_run instead", command)), nil
	}

	// Reject --debug in args (same concern as neo4j_cli_run).
	if containsFlag(args, "debug") {
		return runError("--debug is not accepted by neo4j_cli_run_write"), nil
	}

	// Validate flag long-names against the resolved leaf before execution.
	if err := validateRunFlags(cmd, args); err != nil {
		return runError(err.Error()), nil
	}

	// Catch stdin-reading leaves with no positional argument before exec.
	if len(args) == 0 && isStdinLeaf(cmd) {
		return runError(fmt.Sprintf("%q requires a positional argument; pass the query as the first item in args", command)), nil
	}

	// Layer 3: flags.EnforceWriteGate fires inside Execute() off
	// cmd.Annotations["write"]. We do NOT bypass or reimplement it — it is the
	// authoritative gate that stops an unflagged write even if layers 1 and 2
	// were bypassed.
	execArgs := append(tokens, args...)
	result := exec.Execute(ctx, execArgs)

	return MapCommandResult(result, ResultOptions{Args: execArgs}), nil
}
