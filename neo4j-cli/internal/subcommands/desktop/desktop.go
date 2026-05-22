// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package desktop implements the `neo4j-cli desktop` subcommand tree.
package desktop

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop/connection"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop/dbms"
	"github.com/spf13/cobra"
)

// portFlag default `0` means scan 44222..44232; non-zero pins the probe.
const portFlag = "port"

// NewCmd returns the `desktop` parent cobra command with all subtrees mounted.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "desktop",
		Short: "Manage DBMSes under a local Neo4j Desktop 2 install",
		Long: "Manage Neo4j Desktop 2 — local DBMSes (`dbms`), saved remote connections (`connection`), and install the Desktop app itself (`install`). " +
			"`desktop list` shows DBMSes and saved connections together; use `desktop dbms list` or `desktop connection list` for single-resource views. " +
			"Write commands (`dbms create/delete/start/stop`, `connection create/update/delete`, `install`) require `--rw`.",
	}

	cmd.PersistentFlags().Int(portFlag, 0, "Pin the Desktop relate API to a specific port instead of probing 44222..44232")

	cmd.AddCommand(dbms.NewCmd(cfg))
	cmd.AddCommand(connection.NewCmd(cfg))
	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newDoctorCmd(cfg))
	cmd.AddCommand(newInstallCmd(cfg))

	return cmd
}
