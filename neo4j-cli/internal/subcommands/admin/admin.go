// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package admin implements the `neo4j-cli admin` subcommand tree for managing
// Neo4j databases, users, and roles via the system database over Bolt.
package admin

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/database"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/role"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/user"
	"github.com/spf13/cobra"
)

// NewCmd returns the `admin` parent cobra command. It registers a persistent
// --credential flag (the name of a stored dbms credential) and mounts the
// database, user, and role subcommand trees.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var credential string

	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Manage Neo4j databases, users, and roles",
		Long: "Manage Neo4j databases, users, and roles via the system database. " +
			"All subcommands connect over Bolt using the dbms credential named by " +
			"--credential (see `neo4j-cli credential dbms list`). " +
			"Subcommands: `database` (list, get, create, drop, start, stop), " +
			"`user` (list, get, create, drop, rename, set-password, suspend, activate), " +
			"`role` (list, get, create, drop, grant, revoke — Enterprise only).",
	}

	cmd.PersistentFlags().StringVar(&credential, "credential", "", "Name of the stored dbms credential to use (see `neo4j-cli credential dbms list`)")

	cmd.AddCommand(database.NewCmd(cfg, &credential, RunAdminStatement))
	cmd.AddCommand(user.NewCmd(cfg, &credential, RunAdminStatement))
	cmd.AddCommand(role.NewCmd(cfg, &credential, RunAdminStatement))

	return cmd
}
