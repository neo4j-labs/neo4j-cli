// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package mcp provides the `neo4j-cli mcp` command tree, which exposes the CLI
// to Model Context Protocol clients such as Claude Desktop. The whole group is
// experimental: app.go registers it only when `flag.mcp-server` is enabled, so
// with the flag off the command tree is byte-identical to before.
package mcp

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// RootFactory builds a fresh neo4j-cli root command for the given config. It is
// injected rather than imported: package app imports this package to mount the
// group, so this package must not import app.
type RootFactory func(*clicfg.Config) *cobra.Command

// NewCmd returns the `mcp` parent command. newRoot is the injected root factory
// (see RootFactory) that the leaves use to build a tree per tool call.
func NewCmd(cfg *clicfg.Config, newRoot RootFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Expose neo4j-cli to MCP clients such as Claude Desktop",
		Long: "Expose neo4j-cli to Model Context Protocol (MCP) clients such as Claude Desktop, " +
			"so an assistant can discover your Neo4j targets, read the CLI's own documentation, " +
			"and run commands on your behalf. This group is experimental and only appears when " +
			"the `flag.mcp-server` feature flag is enabled " +
			"(`neo4j-cli config set flag.mcp-server true`, or NEO4J_CLI_FLAG_MCP_SERVER=1).",
	}

	cmd.AddCommand(newToolsCmd(cfg))

	return cmd
}
