// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func newRemoveCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Removes an embed credential",
		Long: "Remove a stored embedding-provider credential by name. " +
			"Removal is non-cascading: dbms credentials linked to the removed embed credential keep their `embed-credential` field; " +
			"the stale link is reported lazily at query time. Run `credential dbms list` to find linked profiles or `credential dbms set-embed <dbms-name>` to clear them.",
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Credentials.Embed.Remove(args[0])
		},
	}
}
