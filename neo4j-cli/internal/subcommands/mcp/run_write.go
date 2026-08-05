// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
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
// Command resolution, flag validation, and stdin-leaf checks are delegated to
// resolveCommand, which is shared with HandleRun.
func HandleRunWrite(ctx context.Context, req *mcpsdk.CallToolRequest, exec *Executor, gates Gates, newRoot RootFactory) (*mcpsdk.CallToolResult, error) {
	// Layer 1: Process gate — the server was started without --rw.
	if !gates.WriteAllowed {
		return runError("Write operations are not allowed; restart the server with `neo4j-cli mcp serve --rw` to enable them"), nil
	}

	r := resolveCommand(ctx, req, exec, gates, newRoot)
	if r.err != nil {
		return r.err, nil
	}

	// Refuse non-write-classified commands — read intent must route through
	// neo4j_cli_run. Accept PolicyWrite and gated policies (write intersection
	// gated).
	policy, _ := Classify(r.cmd, r.args)
	if policy == PolicyAllow || policy == PolicyDeny {
		return runError(fmt.Sprintf("%q is a read-only command; use neo4j_cli_run instead", r.command)), nil
	}

	if len(r.args) == 0 && isStdinLeaf(r.cmd) {
		return runError(fmt.Sprintf("%q requires a positional argument; pass the query as the first item in args", r.command)), nil
	}

	// Layer 3: flags.EnforceWriteGate fires inside Execute() off
	// cmd.Annotations["write"]. We do NOT bypass or reimplement it — it is the
	// authoritative gate that stops an unflagged write even if layers 1 and 2
	// were bypassed.
	result := exec.Execute(ctx, r.execArgs)
	return MapCommandResult(result, ResultOptions{Args: r.execArgs}), nil
}
