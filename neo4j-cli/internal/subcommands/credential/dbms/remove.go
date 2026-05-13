// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func newRemoveCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Removes a dbms credential",
		Long:  "Remove a stored Bolt connection profile by name. Linked embed-credential references on other profiles are not modified.",
		Example: `# Remove a dbms credential by name
neo4j-cli credential dbms remove local --rw

# Remove a staging dbms credential
neo4j-cli credential dbms remove staging --rw

# Remove a prod dbms credential
neo4j-cli credential dbms remove prod --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Credentials.Dbms.Remove(args[0])
		},
	}
}
