// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package privilege implements the `neo4j-cli admin privilege` subcommand tree
// for managing Neo4j Enterprise privileges via the system database over Bolt.
package privilege

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

// NewCmd returns the `admin privilege` parent cobra command. execFn is the
// Cypher execution function injected by the parent (admin.RunAdminStatement in
// production); passing it here avoids an import cycle. conn is a pointer to the
// connection resolved by admin's PersistentPreRunE and shared with all leaves.
func NewCmd(cfg *clicfg.Config, conn **dbconn.Conn, execFn adminutil.ExecFn) *cobra.Command {
	privilegeExecFn = execFn

	cmd := &cobra.Command{
		Use:   "privilege",
		Short: "Manage Neo4j privileges",
		Long: "Manage Neo4j Enterprise privileges via the system database. " +
			"Read commands (list) do not require --rw. " +
			"Write commands (grant, deny, revoke) require --rw.",
		Example: `# Show help for the privilege subcommands
neo4j-cli admin privilege --help

# List all privileges (read-only)
neo4j-cli admin privilege list --credential local --format json`,
	}

	cmd.AddCommand(newListCmd(cfg, conn))
	cmd.AddCommand(newGrantCmd(cfg, conn))

	return cmd
}
