// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "database",
		Short: "Relates to the databases of a deployment server",
	}

	cmd.AddCommand(newListCmd(cfg))

	cmd.PersistentFlags().String("server-id", "", "(required) Fleet Manager deployment server ID")
	cmd.MarkPersistentFlagRequired("server-id") //nolint:errcheck // MarkPersistentFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
