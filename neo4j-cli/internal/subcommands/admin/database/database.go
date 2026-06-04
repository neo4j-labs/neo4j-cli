// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package database implements the `neo4j-cli admin database` subcommand tree.
package database

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config, credential *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "database",
		Short: "Manage Neo4j databases via the system database",
		Long: "Manage Neo4j databases. Read commands (list, get) do not require --rw. " +
			"Write commands (create, drop, start, stop) require --rw and use the " +
			"dbms credential named by --credential on the parent `admin` command.",
		Example: `# Show help for the database subcommands
neo4j-cli admin database --help

# List all databases (read-only)
neo4j-cli admin database list --credential local --format json`,
	}

	_ = cfg
	_ = credential

	return cmd
}
