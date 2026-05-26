// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/spf13/cobra"
)

func newRemoveCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Removes an embed credential",
		Long: "Remove a stored embedding-provider credential by name. " +
			"Removal is non-cascading: dbms credentials linked to the removed embed credential keep their `embed-credential` field; " +
			"the stale link is reported lazily at query time. Run `credential dbms list` to find linked profiles or `credential dbms set-embed <dbms-name>` to clear them.\n\n" +
			"Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.",
		Example: `# Remove an embed credential by name
neo4j-cli credential embed remove openai-small --rw --yes --force

# Remove the local Ollama embed credential
neo4j-cli credential embed remove ollama-nomic --rw --yes --force

# Remove a HuggingFace embed credential
neo4j-cli credential embed remove hf-bge --rw --yes --force`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := confirm.Require(cmd, args[0]); err != nil {
				return err
			}
			return cfg.Credentials.RemoveEmbed(args[0], cmd.ErrOrStderr())
		},
	}

	confirm.Register(cmd)

	return cmd
}
