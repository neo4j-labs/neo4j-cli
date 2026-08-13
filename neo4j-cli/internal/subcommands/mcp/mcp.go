// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package mcp provides the `neo4j-cli mcp` command tree, which exposes the CLI
// to Model Context Protocol clients such as Claude Desktop. The whole group is
// experimental: app.go registers it only when `flag.mcp-server` is enabled, so
// with the flag off the command tree is byte-identical to before.
package mcp

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp/server"
	"github.com/spf13/cobra"
)

// RootFactory is a type alias for server.RootFactory, re-exported so
// app.go imports only the mcp package.
type RootFactory = server.RootFactory

// NewCmd returns the `mcp` parent command. newRoot is the injected root factory
// (see server.RootFactory) that the leaves use to build a tree per tool call.
func NewCmd(cfg *clicfg.Config, newRoot RootFactory) *cobra.Command {
	server.Configure(newRoot, cfg.Version)

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Expose neo4j-cli to MCP clients such as Claude Desktop",
		Long: "Expose neo4j-cli to Model Context Protocol (MCP) clients such as Claude Desktop, " +
			"so an assistant can discover your Neo4j targets, read the CLI's own documentation, " +
			"and run commands on your behalf. This group is experimental and only appears when " +
			"the `flag.mcp-server` feature flag is enabled " +
			"(`neo4j-cli config set flag.mcp-server true`, or NEO4J_CLI_FLAG_MCP_SERVER=1).",
	}

	// newRoot is consumed by the leaves that dispatch CLI commands; validating
	// it for the whole group means a wiring mistake in app.go surfaces as a
	// reportable error on the first `mcp` invocation rather than as a nil
	// dereference in the middle of a tool call.
	cmd.PersistentPreRunE = func(*cobra.Command, []string) error {
		if err := server.ValidateWiring(cfg, newRoot); err != nil {
			return err
		}
		server.EnsureToolDefinitions(cfg)
		return nil
	}

	cmd.AddCommand(newToolCmd(cfg))
	cmd.AddCommand(newServeCmd(cfg))
	cmd.AddCommand(newBundleCmd(cfg))
	cmd.AddCommand(newInstallCmd(cfg))
	cmd.AddCommand(newRemoveCmd(cfg))
	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newCheckCmd(cfg))

	return cmd
}
