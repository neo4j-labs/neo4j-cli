// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"github.com/neo4j/cli/common/skill"
	"github.com/spf13/cobra"
)

// ResolveInstallGates exposes resolveInstallGates for external-package tests.
func ResolveInstallGates(cmd *cobra.Command) skill.MCPGates {
	return resolveInstallGates(cmd)
}

// SetOpenFileFn sets the openFileFn test seam for external-package tests.
func SetOpenFileFn(fn func(string) error) {
	openFileFn = fn
}
