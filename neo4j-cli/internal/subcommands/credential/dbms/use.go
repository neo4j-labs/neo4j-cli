// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func newUseCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Sets the default dbms credential to be used",
		Long:  "Set the named dbms credential as the default consumed by `query` when no `--credential <name>` flag and no connection flags / env vars / .env values are present.",
		Example: `# Make 'local' the default dbms credential
neo4j-cli credential dbms use local --rw

# Switch the default to 'staging'
neo4j-cli credential dbms use staging --rw

# Switch the default to 'prod'
neo4j-cli credential dbms use prod --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Credentials.Dbms.SetDefault(args[0])
		},
	}
}
