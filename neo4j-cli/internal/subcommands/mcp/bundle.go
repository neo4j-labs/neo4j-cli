// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp/server"
	"github.com/spf13/cobra"
)

// bundleResult implements output.ResponseData for the bundle command output.
type bundleResult struct {
	Path string `json:"path"`
}

func (r bundleResult) AsArray() []map[string]any {
	return []map[string]any{{"path": r.Path}}
}

func newBundleCmd(cfg *clicfg.Config) *cobra.Command {
	var outPath string

	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Generate an .mcpb bundle file",
		Long: "Generate an .mcpb bundle file for installing the neo4j-cli MCP connector " +
			"into Claude Desktop or sharing with other users. The bundle contains a " +
			"manifest.json and README.md — no binary is included. The manifest embeds " +
			"the absolute path of this binary so the connector points at the generating CLI.\n\n" +
			"Capability gate flags seed the initial toggle state in the extension's " +
			"settings screen. The settings screen still controls the gates at runtime, " +
			"but setting defaults here means --rw produces a bundle that arrives with " +
			"all toggles on.",
		Example: `# Generate an .mcpb bundle for this machine's neo4j-cli
neo4j-cli mcp bundle --out ~/Desktop/neo4j-cli.mcpb --rw

# Generate the bundle with only write capability, not aura or credentials
neo4j-cli mcp bundle --out ~/Desktop/neo4j-cli.mcpb --rw --allow-writes

# Generate the bundle and inspect the output path as JSON
neo4j-cli mcp bundle --out /tmp/neo4j-cli.mcpb --format json --rw`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			gates := resolveInstallGates(cmd)
			if err := GenerateBundle(outPath, gates); err != nil {
				return clierr.NewFatalError("cannot generate MCP bundle: %s", err.Error())
			}
			commonoutput.PrintBodyMap(cmd, cfg, bundleResult{Path: outPath}, []string{"path"})
			return nil
		},
		Annotations: map[string]string{"write": "true"},
	}

	cmd.Flags().StringVar(&outPath, "out", "", "Path to write the .mcpb bundle file")
	_ = cmd.MarkFlagRequired("out")
	cmd.Flags().Bool("allow-writes", false,
		"Allow write operations through MCP server (seeds the initial toggle state in the settings screen)")
	cmd.Flags().Bool(server.AllowAuraFlag, false,
		"Allow Aura resource operations through MCP server (seeds the initial toggle state in the settings screen)")
	cmd.Flags().Bool(server.AllowCredentialWriteFlag, false,
		"Allow credential writes through MCP server (seeds the initial toggle state in the settings screen)")

	return cmd
}
