// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "embed",
		Short: "Manage and view embed credential values",
		Long: "Manage stored embedding-provider credentials (provider, model, base URL, dimensions, optional API key). " +
			"`query --param NAME:embed=<text>` and `query :embed [text]` consume the resolved embed credential when no `--embed-*` flag or `NEO4J_EMBED_*` env var overrides it. " +
			"Supported providers: openai, ollama, huggingface, gemini, vertex.",
	}

	cmd.AddCommand(newAddCmd(cfg))
	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newRemoveCmd(cfg))
	cmd.AddCommand(newUseCmd(cfg))

	return cmd
}
