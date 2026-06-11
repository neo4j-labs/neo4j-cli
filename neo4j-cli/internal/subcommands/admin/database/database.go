// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package database implements the `neo4j-cli admin database` subcommand tree.
package database

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

// NewCmd returns the `admin database` parent cobra command. execFn is the
// Cypher execution function injected by the parent (admin.RunAdminStatement in
// production); passing it here avoids an import cycle between the database and
// admin packages. conn is a pointer to the connection resolved by admin's
// PersistentPreRunE and shared with all leaf commands.
func NewCmd(cfg *clicfg.Config, conn **dbconn.Conn, execFn adminutil.ExecFn) *cobra.Command {
	dbExecFn = execFn

	cmd := &cobra.Command{
		Use:   "database",
		Short: "Manage Neo4j databases via the system database",
		Long: "Manage Neo4j databases. Read commands (list, get) do not require --rw. " +
			"Write commands (create, drop, start, stop) require --rw.",
		Example: `# Show help for the database subcommands
neo4j-cli admin database --help

# List all databases (read-only)
neo4j-cli admin database list --credential local --format json`,
	}

	cmd.AddCommand(newListCmd(cfg, conn))
	cmd.AddCommand(newGetCmd(cfg, conn))
	cmd.AddCommand(newCreateCmd(cfg, conn))
	cmd.AddCommand(newDropCmd(cfg, conn))
	cmd.AddCommand(newStartCmd(cfg, conn))
	cmd.AddCommand(newStopCmd(cfg, conn))

	return cmd
}
