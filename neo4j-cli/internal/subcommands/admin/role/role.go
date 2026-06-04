// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package role implements the `neo4j-cli admin role` subcommand tree.
package role

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

// NewCmd returns the `admin role` parent cobra command. execFn is the Cypher
// execution function injected by the parent (admin.RunAdminStatement in
// production); passing it here avoids an import cycle between the role and
// admin packages.
func NewCmd(cfg *clicfg.Config, credential *string, execFn adminutil.ExecFn) *cobra.Command {
	roleExecFn = execFn

	cmd := &cobra.Command{
		Use:   "role",
		Short: "Manage Neo4j roles via the system database",
		Long: "Manage Neo4j roles (Enterprise edition only). Read commands (list, get) do not require --rw. " +
			"Write commands (create, drop, grant, revoke) require --rw and use the " +
			"dbms credential named by --credential on the parent `admin` command.",
		Example: `# Show help for the role subcommands
neo4j-cli admin role --help

# List all roles (read-only)
neo4j-cli admin role list --credential local --format json`,
	}

	cmd.AddCommand(newListCmd(cfg, credential))
	cmd.AddCommand(newGetCmd(cfg, credential))
	cmd.AddCommand(newCreateCmd(cfg, credential))
	cmd.AddCommand(newDropCmd(cfg, credential))
	cmd.AddCommand(newGrantCmd(cfg, credential))
	cmd.AddCommand(newRevokeCmd(cfg, credential))

	return cmd
}
