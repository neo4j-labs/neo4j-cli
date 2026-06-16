// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package role implements the `neo4j-cli admin role` subcommand tree for
// managing Neo4j roles and role membership via the system database over Bolt.
package role

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

// NewCmd returns the `admin role` parent cobra command. execFn is the Cypher
// execution function injected by the parent (admin.RunAdminStatement in
// production); passing it here avoids an import cycle. conn is a pointer to the
// connection resolved by admin's PersistentPreRunE and shared with all leaves.
func NewCmd(cfg *clicfg.Config, conn **dbconn.Conn, execFn adminutil.ExecFn) *cobra.Command {
	roleExecFn = execFn

	cmd := &cobra.Command{
		Use:   "role",
		Short: "Manage Neo4j roles and role membership",
		Long: "Manage Neo4j roles and role membership via the system database. " +
			"Read commands (list, get) do not require --rw. " +
			"Write commands (create, drop, grant, revoke) require --rw.",
		Example: `# Show help for the role subcommands
neo4j-cli admin role --help

# List all roles (read-only)
neo4j-cli admin role list --credential local --format json`,
	}

	cmd.AddCommand(newListCmd(cfg, conn))

	return cmd
}
