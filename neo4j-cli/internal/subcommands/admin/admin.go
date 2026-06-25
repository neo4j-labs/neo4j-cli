// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package admin implements the `neo4j-cli admin` subcommand tree for managing
// Neo4j databases, users, roles, and privileges via the system database over Bolt.
package admin

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/database"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/privilege"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/role"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/user"
	"github.com/spf13/cobra"
)

// NewCmd returns the `admin` parent cobra command. It registers persistent
// connection flags and mounts the database, user, role, and privilege subcommand trees.
// The connection is resolved once in PersistentPreRunE and shared with all
// leaf commands via the adminConn pointer.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var adminConn *dbconn.Conn

	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Manage Neo4j databases, users, roles, and privileges",
		Long: "Manage Neo4j databases via the system database. " +
			"Connects over Bolt using the supplied connection flags or a stored dbms credential " +
			"(use '--credential <name>' for a named credential, " +
			"'--credential desktop' for a running Neo4j Desktop 2 DBMS, " +
			"or '--credential desktop-connection:<uuid>' for a saved Desktop connection). " +
			"Subcommands: `database` (list, get, create, drop, start, stop), " +
			"`user` (list, get, create, drop, rename, set-password, suspend, activate), " +
			"`role` (list, get, create, drop, grant, revoke), " +
			"`privilege` (list, grant, deny, revoke).",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			conn, err := dbconn.ResolveConn(cmd, cfg, true)
			if err != nil {
				return err
			}
			if conn.Password == "" {
				if !dbconn.StdinIsTTY() {
					return clierr.NewUsageError("--password is required or run interactively")
				}
				pw, perr := dbconn.PromptPassword(cmd)
				if perr != nil {
					return perr
				}
				conn.Password = pw
			}
			adminConn = conn
			return nil
		},
	}

	cmd.PersistentFlags().String("uri", "", "Neo4j server URI (env: NEO4J_URI)")
	cmd.PersistentFlags().StringP("username", "u", "", "Neo4j username (env: NEO4J_USERNAME)")
	cmd.PersistentFlags().StringP("password", "p", "", "Neo4j password (env: NEO4J_PASSWORD)")
	cmd.PersistentFlags().String("env", "", "Path to a .env file with NEO4J_URI / NEO4J_USERNAME / NEO4J_PASSWORD (walks up from cwd when unset)")
	cmd.PersistentFlags().StringP("credential", "c", "", "Name of a stored dbms credential, 'desktop' for a running Neo4j Desktop 2 DBMS, or 'desktop-connection:<uuid>' for a saved connection")
	cmd.PersistentFlags().Bool("debug", false, "Enable Bolt driver debug logging to stderr (env: NEO4J_DEBUG=1)")

	cmd.AddCommand(database.NewCmd(cfg, &adminConn, RunAdminStatement))
	cmd.AddCommand(user.NewCmd(cfg, &adminConn, RunAdminStatement))
	cmd.AddCommand(role.NewCmd(cfg, &adminConn, RunAdminStatement))
	cmd.AddCommand(privilege.NewCmd(cfg, &adminConn, RunAdminStatement))

	return cmd
}
