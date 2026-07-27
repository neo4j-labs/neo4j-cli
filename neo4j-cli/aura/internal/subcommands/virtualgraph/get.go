// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph

import (
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func newGetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Returns virtual graph details",
		Long: `This subcommand returns details about a specific virtual graph in the specified project.

Use --organization-id and --project-id to specify which project the virtual graph belongs to, or configure a default with 'aura workspace use <org-id>/<project-id>'.`,
		Example: `# Get details of a virtual graph by ID
neo4j-cli aura virtual-graph get ge82059a --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get details using a configured default workspace
neo4j-cli aura virtual-graph get ge82059a

# Emit JSON for scripting and extract the Bolt URL with jq
neo4j-cli aura virtual-graph get ge82059a --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json | jq -r '.data.bolt_url'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			virtualGraphID := strings.TrimSpace(args[0])

			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			resBody, err := utils.FetchScopedVirtualGraph(cfg, orgID, projectID, virtualGraphID)
			if err != nil {
				return err
			}

			if resBody == nil {
				return nil
			}

			return printVirtualGraph(cmd, cfg, resBody)
		},
	}
}
