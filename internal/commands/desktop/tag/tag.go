// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package tag implements the `neo4j-cli desktop tag` subtree as a spec over
// the shared catalogcmd scaffold (see desktop/internal/catalogcmd).
package tag

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/desktop/internal/catalogcmd"
	"github.com/spf13/cobra"
)

// NewCmd returns the `desktop tag` parent cobra command with all leaves
// mounted. The desktop root's persistent --port flag is inherited; tag does
// not re-register it.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manage the tag catalog of a Neo4j Desktop 2 install",
		Long: "Manage the tag catalog of the local Neo4j Desktop 2 install — list, create, update, delete. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"Tags organise DBMSes and saved remote connections in Desktop's UI; a tag's color is one of Desktop's palette indices \"1\"..\"12\" (not a hex colour). " +
			"Write commands (`create`, `update`, `delete`) require `--rw`.",
	}

	catalogcmd.Mount(cfg, cmd, spec)

	return cmd
}
