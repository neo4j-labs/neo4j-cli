// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package project implements the `neo4j-cli desktop project` subtree as a spec
// over the shared catalogcmd scaffold (see desktop/internal/catalogcmd).
package project

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/desktop/internal/catalogcmd"
	"github.com/spf13/cobra"
)

// NewCmd returns the `desktop project` parent cobra command.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects in the local Neo4j Desktop 2 install",
		Long: "Manage the project catalog of the local Neo4j Desktop 2 install via its local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"Projects are Desktop's grouping mechanism for local DBMSes and saved remote connections. " +
			"Every read and mutation round-trips the full project catalog; `list` shows it, and `create`/`update`/`delete` print the affected entry. " +
			"Write commands (`create`, `update`, `delete`) require `--rw`.",
	}

	catalogcmd.Mount(cfg, cmd, spec)

	return cmd
}
