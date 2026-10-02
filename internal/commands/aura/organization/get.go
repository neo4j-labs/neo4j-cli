// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package organization

import (
	"github.com/neo4j/cli/internal/auraclient"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/spf13/cobra"
)

func newGetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Returns organization details",
		Long:  "This subcommand returns details about a specific Aura organization.",
		Example: `# Get details of an organization by ID
neo4j-cli aura organization get 00000000-0000-0000-0000-000000000000

# Emit JSON for scripting
neo4j-cli aura organization get 00000000-0000-0000-0000-000000000000 --format json

# Pipe details through jq to extract the organization name
neo4j-cli aura organization get 00000000-0000-0000-0000-000000000000 --format json | jq -r '.data.name'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			orgID := strings.TrimSpace(args[0])

			cmd.SilenceUsage = true
			org, err := auraclient.New(cfg).Organizations().Get(cmd.Context(), orgID)
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, org.Record, []string{"id", "name"})

			return nil
		},
	}
}
