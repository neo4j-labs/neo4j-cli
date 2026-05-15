// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package context

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage the active organization and project context",
	}

	return cmd
}
