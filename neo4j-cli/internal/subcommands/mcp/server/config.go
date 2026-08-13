// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// RootFactory builds a fresh neo4j-cli root command for the given config. It is
// injected rather than imported: package app imports the mcp group to mount the
// command, so this package must not import app.
type RootFactory func(*clicfg.Config) *cobra.Command

// storedRootFactory is the injected factory, kept for tool-definition
// initialization and tool handlers that need to project the live tree.
var storedRootFactory RootFactory
var storedVersion string

// Configure sets the server's global root factory and version. Called once from
// the mcp parent command's NewCmd during CLI startup.
func Configure(newRoot RootFactory, version string) {
	storedRootFactory = newRoot
	storedVersion = version
}

// StoredRootFactory returns the root factory set via Configure. Used by staying
// mcp-package files (serve.go) to build the executor.
func StoredRootFactory() RootFactory {
	return storedRootFactory
}

// StoredVersion returns the version string set via Configure.
func StoredVersion() string {
	return storedVersion
}
