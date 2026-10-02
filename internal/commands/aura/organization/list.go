// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package organization

import (
	"github.com/neo4j/cli/internal/aura"

	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Returns a list of organizations",
		Long:  "This subcommand returns a list of Aura organizations accessible to the current user.",
		Example: `# List all organizations the current user has access to
neo4j-cli aura organization list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura organization list --format json

# Pipe organization ids through jq for a follow-up command
neo4j-cli aura organization list --format json | jq -r '.data[].id'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgs, err := aura.New(cfg).Organizations().List(cmd.Context())
			if err != nil {
				return err
			}

			rows := make([]map[string]any, len(orgs))
			for i, o := range orgs {
				rows[i] = o.Record
			}
			output.PrintRecords(cmd, cfg, rows, []string{"id", "name"})

			return nil
		},
	}
}
