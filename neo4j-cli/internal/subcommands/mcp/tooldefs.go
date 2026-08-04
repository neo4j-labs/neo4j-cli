// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"encoding/json"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/afero"
)

var (
	// toolTreeNames is the enum for the neo4j_cli_list_commands `tree`
	// parameter, populated from the live tree at server start so new top-level
	// trees auto-surface.
	toolTreeNames []string

	// storedFlagStates captures the feature-flag state at server start so both
	// the tool-definition enum (ensureToolDefinitions) and the tool handler
	// (HandleListCommands) build trees with the same flag configuration.
	storedFlagStates map[string]bool

	toolDefsOnce sync.Once
)

// ensureToolDefinitions populates toolTreeNames from the live tree. The config's
// flag state is mirrored so flag-gated trees (mcp itself) appear in the enum.
// Called from the mcp parent's PersistentPreRunE, once per process.
func ensureToolDefinitions(srcCfg *clicfg.Config) {
	toolDefsOnce.Do(func() {
		if storedRootFactory == nil {
			return
		}
		storedFlagStates = make(map[string]bool)
		for name := range clicfg.Registry {
			enabled := srcCfg.Flags.Enabled(name)
			storedFlagStates[name] = enabled
		}
		cfg := clicfg.NewConfig(afero.NewMemMapFs(), storedVersion, clicfg.GlobalScope)
		defer cfg.Events.Flush()
		for name, enabled := range storedFlagStates {
			if enabled {
				cfg.Flags.SetForTest(name, true)
			}
		}
		root := storedRootFactory(cfg)
		for _, sub := range root.Commands() {
			if !sub.IsAvailableCommand() {
				continue
			}
			name := sub.Name()
			if name == "completion" {
				continue
			}
			toolTreeNames = append(toolTreeNames, name)
		}
	})
}

// toolDefinitions returns the tool definitions the server registers, in the
// order tools are advertised to clients. Both `mcp tools` and the server read
// them from here, so the printed surface cannot drift from the registered one.
func toolDefinitions() []*mcpsdk.Tool {
	return []*mcpsdk.Tool{
		{
			Name:        "neo4j_cli_list_commands",
			Title:       "List neo4j-cli commands",
			Description: "List the neo4j-cli command trees. Without `tree` returns the index of top-level trees. With `tree` returns that tree's commands as `use: short` lines.",
			Annotations: &mcpsdk.ToolAnnotations{
				ReadOnlyHint:   true,
				IdempotentHint: true,
			},
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tree": map[string]any{
						"type":        "string",
						"enum":        toolTreeNames,
						"description": "A top-level command tree to expand (e.g. docker, aura). When omitted returns the tree index.",
					},
				},
			},
		},
		{
			Name:        "neo4j_cli_read_docs",
			Title:       "Read neo4j-cli documentation",
			Description: "Read the documentation for a neo4j-cli command from the embedded skill bundle. The `command` parameter is a space-separated CLI path (e.g. `docker load`, `docker`, `aura`). Returns the matched section's prose, flags and examples. A tree name alone (e.g. `aura`) returns only the table of contents and tree overview, not its child commands. Use `offset` and `max_chars` for pagination through large sections (default 6000 chars, max 20000). The bundle is the same documentation the `neo4j-cli skill install` command writes onto disk, sourced from the generated reference files.",
			Annotations: &mcpsdk.ToolAnnotations{
				ReadOnlyHint:   true,
				IdempotentHint: true,
			},
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The CLI command path (e.g. 'docker load', 'docker'). The first token is a top-level tree name.",
					},
					"offset": map[string]any{
						"type":        "integer",
						"description": "Character offset from the start of the matched section. Default 0.",
					},
					"max_chars": map[string]any{
						"type":        "integer",
						"description": "Maximum characters to return. Default 6000, max 20000.",
					},
				},
				"required": []string{"command"},
			},
		},
	}
}

// toolRows projects tool definitions into snake_case output rows. The SDK's own
// JSON tags are camelCase (`readOnlyHint`) because they are wire fields, which
// the repo's OUTPUT casing rule forbids for rendered output.
type toolRows []*mcpsdk.Tool

func (r toolRows) AsArray() []map[string]any {
	out := make([]map[string]any, 0, len(r))
	for _, t := range r {
		// An absent hint is not "false": the MCP spec defaults destructiveHint
		// and openWorldHint to true, so a nil pointer must widen to the
		// pessimistic value rather than the zero value.
		row := map[string]any{
			"name":             t.Name,
			"title":            t.Title,
			"description":      t.Description,
			"read_only_hint":   false,
			"idempotent_hint":  false,
			"destructive_hint": true,
			"open_world_hint":  true,
		}
		if a := t.Annotations; a != nil {
			row["read_only_hint"] = a.ReadOnlyHint
			row["idempotent_hint"] = a.IdempotentHint
			if a.DestructiveHint != nil {
				row["destructive_hint"] = *a.DestructiveHint
			}
			if a.OpenWorldHint != nil {
				row["open_world_hint"] = *a.OpenWorldHint
			}
		}
		out = append(out, row)
	}
	return out
}

// MarshalJSON delegates to AsArray so the json and toon renderings emit the
// snake_case projection instead of the SDK struct's camelCase wire tags, and so
// an empty manifest renders as `[]` rather than `null`.
func (r toolRows) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.AsArray())
}
