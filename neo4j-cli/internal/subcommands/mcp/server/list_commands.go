// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/agentcontext"
	"github.com/spf13/afero"
)

// HandleListCommands handles the neo4j_cli_list_commands tool call. No "tree"
// argument returns the top-level tree index. A "tree" argument returns every
// command in that tree as "use: short" lines. Never returns the full
// agent-context envelope (~75k tokens).
func HandleListCommands(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	tree := resolveTreeArg(req)

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), storedVersion, clicfg.GlobalScope)
	defer cfg.Events.Flush()
	for name, enabled := range storedFlagStates {
		if enabled {
			cfg.Flags.SetForTest(name, true)
		}
	}
	root := storedRootFactory(cfg)
	agCtx := agentcontext.BuildContext(root, storedVersion)
	// cobra injects completion at Execute time; the skill bundle generator runs
	// pre-Execute, so it is absent from bundle reference files. Filter it here
	// so the catalog and the docs resolver (task-009) agree.
	delete(agCtx.Commands, "completion")

	// Per REQ-F-031 every returned string passes through sanitize (RedactText
	// then StripControl). The error path echoes the client-supplied tree, and
	// the success paths are kept consistent so the invariant holds for the
	// whole handler rather than case by case.
	if tree == "" {
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: sanitize(indexText(agCtx.Commands))},
			},
		}, nil
	}

	cmd, ok := agCtx.Commands[tree]
	if !ok {
		return &mcpsdk.CallToolResult{
			IsError: true,
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: sanitize(fmt.Sprintf("Unknown tree: %q", tree))},
			},
		}, nil
	}

	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: sanitize(treeText(tree, &cmd))},
		},
	}, nil
}

// resolveTreeArg extracts the "tree" argument from a raw JSON arguments map.
func resolveTreeArg(req *mcpsdk.CallToolRequest) string {
	if req == nil || req.Params == nil || req.Params.Arguments == nil {
		return ""
	}
	var args map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return ""
	}
	tree, _ := args["tree"].(string)
	return tree
}

// indexText formats the top-level tree index as "name — short" lines, sorted
// alphabetically for deterministic output.
func indexText(commands map[string]agentcontext.Command) string {
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	var lines []string
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("%s — %s", name, commands[name].Short))
	}
	return strings.Join(lines, "\n")
}

// treeText formats every command in a tree as "full-path — short" lines.
func treeText(prefix string, cmd *agentcontext.Command) string {
	var lines []string
	collectCommands(prefix, cmd, &lines)
	return strings.Join(lines, "\n")
}

// collectCommands walks the command tree recursively, appending
// "full-path — short" lines with sorted siblings for deterministic output.
func collectCommands(prefix string, cmd *agentcontext.Command, lines *[]string) {
	*lines = append(*lines, fmt.Sprintf("%s — %s", prefix, cmd.Short))
	names := make([]string, 0, len(cmd.Subcommands))
	for name := range cmd.Subcommands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry := cmd.Subcommands[name]
		collectCommands(prefix+" "+name, &entry, lines)
	}
}
