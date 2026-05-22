// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package connection implements the `neo4j-cli desktop connection` subtree.
package connection

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewCmd returns the `desktop connection` parent cobra command.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connection",
		Short: "Manage saved remote DB connections registered with Neo4j Desktop 2",
		Long: "Manage the saved remote DB connection profiles Neo4j Desktop 2 stores via its local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"Connections are remote Neo4j endpoints (Aura, self-hosted, …) the user has registered with Desktop; they appear under `Remote connections` in `neo4j-cli desktop list`. " +
			"Use `neo4j-cli desktop connection list` for a connection-only view, or `neo4j-cli desktop list` for the composed view alongside local DBMSes. " +
			"Use `neo4j-cli query --credential desktop-connection:<uuid>` to run Cypher against a saved connection without restating the URI / username / password. " +
			"Write commands (`create`, `update`, `delete`) require `--rw`.",
	}

	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newCreateCmd(cfg))
	cmd.AddCommand(newUpdateCmd(cfg))
	cmd.AddCommand(newDeleteCmd(cfg))

	return cmd
}
