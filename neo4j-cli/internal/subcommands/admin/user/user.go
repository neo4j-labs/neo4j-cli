// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

// NewCmd returns the `admin user` parent cobra command. execFn is the Cypher
// execution function injected by the parent (admin.RunAdminStatement in
// production); passing it here avoids an import cycle between the user and
// admin packages. conn is a pointer to the connection resolved by admin's
// PersistentPreRunE and shared with all leaf commands.
func NewCmd(cfg *clicfg.Config, conn **dbconn.Conn, execFn adminutil.ExecFn) *cobra.Command {
	userExecFn = execFn

	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage Neo4j users via the system database",
		Long: "Manage Neo4j users. Read commands (list, get) do not require --rw. " +
			"Write commands (create, drop, rename, set-password, suspend, activate) require --rw.",
		Example: `# Show help for the user subcommands
neo4j-cli admin user --help

# List all users (read-only)
neo4j-cli admin user list --credential local --format json`,
	}

	cmd.AddCommand(newListCmd(cfg, conn))
	cmd.AddCommand(newGetCmd(cfg, conn))
	cmd.AddCommand(newCreateCmd(cfg, conn))
	cmd.AddCommand(newDropCmd(cfg, conn))
	cmd.AddCommand(newRenameCmd(cfg, conn))
	cmd.AddCommand(newSetPasswordCmd(cfg, conn))
	cmd.AddCommand(newSuspendCmd(cfg, conn))
	cmd.AddCommand(newActivateCmd(cfg, conn))

	return cmd
}
