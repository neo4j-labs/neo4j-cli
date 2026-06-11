// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package privilege implements the `neo4j-cli admin privilege` subcommand tree.
package privilege

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

// NewCmd returns the `admin privilege` parent cobra command. execFn is the Cypher
// execution function injected by the parent (admin.RunAdminStatement in
// production); passing it here avoids an import cycle between the privilege and
// admin packages. conn is a pointer to the connection resolved by admin's
// PersistentPreRunE and shared with all leaf commands.
func NewCmd(cfg *clicfg.Config, conn **dbconn.Conn, execFn adminutil.ExecFn) *cobra.Command {
	privilegeExecFn = execFn

	cmd := &cobra.Command{
		Use:   "privilege",
		Short: "Show Neo4j privileges (Enterprise only)",
		Long: "Show Neo4j privileges (Enterprise edition only). " +
			"Read commands (list) do not require --rw.",
		Example: `# Show help for the privilege subcommands
neo4j-cli admin privilege --help

# List all privileges as JSON for scripting
neo4j-cli admin privilege list --credential local --format json`,
	}

	cmd.AddCommand(newListCmd(cfg, conn))

	return cmd
}
