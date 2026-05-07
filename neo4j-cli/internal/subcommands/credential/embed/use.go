// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func newUseCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:         "use <name>",
		Short:       "Sets the default embed credential to be used",
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Credentials.Embed.SetDefault(args[0])
		},
	}
}
