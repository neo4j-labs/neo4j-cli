// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agentcontext

import (
	"encoding/json"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

// NewCmd returns the `agent-context` leaf command. `version` is passed in
// rather than imported from the app package to keep this package free of an
// import cycle. The command honours the root --format flag and dispatches
// json (default when piped), toon, and table renderings of the same envelope.
func NewCmd(cfg *clicfg.Config, version string) *cobra.Command {
	return &cobra.Command{
		Use:   "agent-context",
		Short: "Emit the full CLI shape as JSON for AI-agent discovery",
		Long: `Emit a stable JSON envelope describing the neo4j-cli command tree, exit codes, error categories, supported output formats, and the canonical async flag — intended for AI agents discovering the CLI's surface.

The envelope (schema_version 1) carries: schema_version, cli_version, binary, commands (recursive tree of every visible subcommand with use/short/long/example/aliases/deprecated/flags/subcommands), exit_codes, error_codes, output_formats, and async_flag. The commands tree is reflected from the live cobra tree at every invocation — adding a new subcommand, flag, or alias auto-surfaces with no regen step.

JSON is the canonical machine view. On a TTY, --format defaults to a degraded flat command-list table. The same envelope is also available via --format toon. See AGENTS.md "Agent Context Notes" for the schema-versioning rules and the hand-coded constants that live in build.go.`,
		Example: `neo4j-cli agent-context
neo4j-cli agent-context --format json | jq '.commands | keys'
neo4j-cli agent-context --format json | jq -e '.commands.aura.subcommands.instance.subcommands.list.flags'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := BuildContext(cmd.Root(), version)
			switch commonoutput.ResolveOutput(cmd, cfg) {
			case "toon":
				return renderToon(cmd, ctx)
			case "table":
				return renderTable(cmd, ctx)
			default:
				return json.NewEncoder(cmd.OutOrStdout()).Encode(ctx)
			}
		},
	}
}
