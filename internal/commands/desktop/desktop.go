// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package desktop implements the `neo4j-cli desktop` subcommand tree.
package desktop

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/desktop/connection"
	"github.com/neo4j/cli/internal/commands/desktop/dbms"
	"github.com/neo4j/cli/internal/commands/desktop/project"
	"github.com/neo4j/cli/internal/commands/desktop/tag"
	"github.com/neo4j/cli/internal/debug"
	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/spf13/cobra"
)

// portFlag default `0` means scan 44222..44232; non-zero pins the probe.
const portFlag = "port"

// NewCmd returns the `desktop` parent cobra command with all subtrees mounted.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "desktop",
		Short: "Manage DBMSes under a local Neo4j Desktop 2 install",
		Long: "Manage Neo4j Desktop 2 — local DBMSes (`dbms`), saved remote connections (`connection`), the project catalog (`project`), the tag catalog (`tag`), and install the Desktop app itself (`install`). " +
			"`desktop list` shows DBMSes and saved connections together; use `desktop dbms list` or `desktop connection list` for single-resource views. " +
			"Write commands (`dbms create/update/delete/start/stop`, `connection create/update/delete`, `project create/update/delete`, `tag create/update/delete`, `install`) require `--rw`.",
	}

	cmd.PersistentFlags().Int(portFlag, 0, "Pin the Desktop relate API to a specific port instead of probing 44222..44232")
	cmd.PersistentFlags().Bool("debug", false, "Route Neo4j Desktop relate API activity (discovery probes, mDNS, the local HTTP request/response wire) to stderr; stdout is unaffected [env: NEO4J_DEBUG (set to 1 to enable)]")

	// Resolve --debug once at startup and toggle the desktopclient package seam
	// (its emit helpers take no *cobra.Command). cobra.EnableTraverseRunHooks
	// (set on the neo4j-cli root) runs every PersistentPreRunE up the ancestry,
	// so this fires for every nested leaf.
	prev := cmd.PersistentPreRunE
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(cmd, args); err != nil {
				return err
			}
		}
		desktopclient.SetDebug(debug.Resolve(cmd))
		return nil
	}

	cmd.AddCommand(dbms.NewCmd(cfg))
	cmd.AddCommand(connection.NewCmd(cfg))
	cmd.AddCommand(project.NewCmd(cfg))
	cmd.AddCommand(tag.NewCmd(cfg))
	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newDoctorCmd(cfg))
	cmd.AddCommand(newInstallCmd(cfg))

	return cmd
}
