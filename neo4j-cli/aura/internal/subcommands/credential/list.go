// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

var credentialFields = []string{"name", "client-id"}

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			output.PrintBodyMap(cmd, cfg, cfg.Credentials.Aura.Printable(), credentialFields)
			return nil
		},
	}
}
