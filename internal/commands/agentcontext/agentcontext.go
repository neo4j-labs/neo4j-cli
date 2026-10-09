// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agentcontext

import (
	"encoding/json"
	"strings"

	agentctx "github.com/neo4j/cli/internal/agentcontext"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

// NewCmd returns the `agent-context` leaf command. `version` is passed in
// rather than imported from the app package to keep this package free of an
// import cycle. The command honours the root --format flag and dispatches
// json (default when piped), toon, and table renderings of the same envelope.
func NewCmd(cfg *clicfg.Config, version string) *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:   "agent-context [command-path...]",
		Short: "Emit a command index (or one command's details) as JSON for AI-agent discovery",
		Long: `Emit a stable envelope describing the neo4j-cli command surface, intended for AI agents discovering it.

With no arguments the envelope (schema_version 2) is compact: schema_version, cli_version, binary, commands (a flat index of every visible command as path + short), exit_codes, error_codes, output_formats and async_flag. Pass a command path (for example "aura instance list") to get that command's long description, example, aliases, flags and subcommand index. Use --full for the complete recursive tree with every command's details (several hundred KB).

The index and details are reflected from the live cobra tree at every invocation — adding a new subcommand, flag, or alias auto-surfaces with no regen step. JSON is the canonical machine view; --format toon carries the same data. On a TTY, --format defaults to a flat command-list table. See AGENTS.md "Agent Context Notes" for the schema-versioning rules and the hand-coded constants that live in build.go.`,
		Example: `# List every command as a compact index (default when piped)
neo4j-cli agent-context --format json

# Get the long description, example and flags of one command
neo4j-cli agent-context aura instance list --format json

# Dump the complete recursive tree (large)
neo4j-cli agent-context --full --format json | jq '.commands | keys'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format := commonoutput.ResolveOutput(cmd, cfg)
			if full && len(args) > 0 {
				return clierr.NewUsageError("--full cannot be combined with a command path")
			}
			if full {
				return render(cmd, format, agentctx.BuildContext(cmd.Root(), version))
			}
			if len(args) > 0 {
				detail, ok := agentctx.BuildDetail(cmd.Root(), version, args)
				if !ok {
					return clierr.NewNotFoundError("no command %q", strings.Join(args, " ")).
						WithSuggestion("Run 'neo4j-cli agent-context' for the command index.")
				}
				return render(cmd, format, detail)
			}
			if format == "table" {
				return renderTable(cmd, agentctx.BuildContext(cmd.Root(), version))
			}
			return render(cmd, format, agentctx.BuildIndex(cmd.Root(), version))
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Emit the complete recursive command tree with all details (large)")
	return cmd
}

// render writes v as TOON or JSON; table callers handle their own rendering.
func render(cmd *cobra.Command, format string, v any) error {
	if format == "toon" {
		return renderToon(cmd, v)
	}
	if format == "table" {
		if c, ok := v.(agentctx.Context); ok {
			return renderTable(cmd, c)
		}
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(v)
}
