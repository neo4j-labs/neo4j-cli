// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

var dbmsCredentialFields = []string{"name", "username", "database-name", "uri", "insecure", "default"}

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lists dbms credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			output.PrintBodyMap(cmd, cfg, cfg.Credentials.Dbms.Printable(), dbmsCredentialFields)
			return nil
		},
	}
}
