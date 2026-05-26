// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package plugin implements the `neo4j-cli desktop dbms plugin` subtree.
package plugin

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewCmd returns the `desktop dbms plugin` parent cobra command.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage Neo4j plugins on a local Desktop-managed DBMS",
		Long: "Manage Neo4j plugins on a local Neo4j Desktop 2-managed DBMS — list installed plugins, browse the installable catalog, install, and uninstall. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"Plugin changes take effect after the DBMS is restarted; `install` and `uninstall` auto-restart a running DBMS (Stop → Start) unless `--no-restart` is passed. " +
			"Write commands (`install`, `uninstall`) require `--rw`.",
	}

	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newAvailableCmd(cfg))
	cmd.AddCommand(newInstallCmd(cfg))
	cmd.AddCommand(newUninstallCmd(cfg))

	return cmd
}
