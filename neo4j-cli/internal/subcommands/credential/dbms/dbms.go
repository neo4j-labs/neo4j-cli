// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dbms",
		Short: "Manage and view dbms credential values",
		Long: "Manage stored Neo4j Bolt connection profiles (URI, username, password, database, optional embed-credential link). " +
			"`query` consumes the default profile (or one selected by `--credential <name>`) when no `--uri`/`NEO4J_URI`/.env value is set.",
	}

	cmd.AddCommand(newAddCmd(cfg))
	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newRemoveCmd(cfg))
	cmd.AddCommand(newUseCmd(cfg))
	cmd.AddCommand(newSetEmbedCmd(cfg))

	return cmd
}
