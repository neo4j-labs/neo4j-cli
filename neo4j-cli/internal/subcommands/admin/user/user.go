// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package user implements the `neo4j-cli admin user` subcommand tree.
package user

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewCmd returns the `admin user` parent cobra command. execFn is the Cypher
// execution function injected by the parent (admin.RunAdminStatement in
// production); passing it here avoids an import cycle between the user and
// admin packages.
func NewCmd(cfg *clicfg.Config, credential *string, execFn ExecFnType) *cobra.Command {
	userExecFn = execFn

	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage Neo4j users via the system database",
		Long: "Manage Neo4j users. Read commands (list, get) do not require --rw. " +
			"Write commands (create, drop, rename, set-password, suspend, activate) require --rw and use the " +
			"dbms credential named by --credential on the parent `admin` command.",
		Example: `# Show help for the user subcommands
neo4j-cli admin user --help

# List all users (read-only)
neo4j-cli admin user list --credential local --format json`,
	}

	cmd.AddCommand(newListCmd(cfg, credential))
	cmd.AddCommand(newGetCmd(cfg, credential))
	cmd.AddCommand(newCreateCmd(cfg, credential))
	cmd.AddCommand(newDropCmd(cfg, credential))
	cmd.AddCommand(newRenameCmd(cfg, credential))

	return cmd
}
