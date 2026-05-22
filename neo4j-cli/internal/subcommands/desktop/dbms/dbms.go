// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package dbms implements the `neo4j-cli desktop dbms` subtree: lifecycle
// CRUD over the DBMSes a local Neo4j Desktop 2 install manages.
package dbms

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop/dbms/plugin"
	"github.com/spf13/cobra"
)

// portFlag default `0` means "scan the documented 44222..44232 range"
// (see desktopclient.ProbePort). Re-registered on the dbms subtree so it
// works without depending on cobra's inherited-persistent-flag walk.
const portFlag = "port"

// NewCmd returns the `desktop dbms` parent cobra command with all lifecycle leaves mounted.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dbms",
		Short: "Manage local DBMSes under a Neo4j Desktop 2 install",
		Long: "Manage local Neo4j DBMSes running under a Neo4j Desktop 2 install — list, create, delete, start, stop. " +
			"Write commands (`create`, `delete`, `start`, `stop`) require `--rw`. " +
			"For a composed view of DBMSes plus saved remote connections see `neo4j-cli desktop list`.",
	}

	cmd.PersistentFlags().Int(portFlag, 0, "Pin the Desktop relate API to a specific port instead of probing 44222..44232")

	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newCreateCmd(cfg))
	cmd.AddCommand(newDeleteCmd(cfg))
	cmd.AddCommand(newStartCmd(cfg))
	cmd.AddCommand(newStopCmd(cfg))
	cmd.AddCommand(plugin.NewCmd(cfg))

	return cmd
}
