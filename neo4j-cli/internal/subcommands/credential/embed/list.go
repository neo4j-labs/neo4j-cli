// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

var embedCredentialFields = []string{"name", "provider", "model", "base-url", "dimensions", "default"}

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lists embed credentials",
		Long:  "List stored embedding-provider credentials. The `api-key` column is never shown — keys are persisted on disk but redacted in every printable form.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			output.PrintBodyMap(cmd, cfg, cfg.Credentials.Embed.Printable(), embedCredentialFields)
			return nil
		},
	}
}
