// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
)

func newSetEmbedCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "set-embed <dbms-name> [embed-name]",
		Short: "Links (or clears) an embed credential on a dbms credential",
		Long: "Link a stored dbms credential to an existing embed credential by name. Pass only the dbms name to clear the link. " +
			"No embed-credential is required for `query` to run plain Cypher; this only links one for downstream embedding via `--param NAME:embed=...` and `query :embed`. " +
			"With a link in place, `query --credential <dbms-name>` picks up both the connection and the embed config in a single selector.",
		Example: `# Link a dbms credential to an embed credential
neo4j-cli credential dbms set-embed local openai-small --rw

# Replace the linked embed credential
neo4j-cli credential dbms set-embed local ollama-nomic --rw

# Clear the embed-credential link on a dbms credential
neo4j-cli credential dbms set-embed local --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Credentials.WarnIfEnvMode(cmd.ErrOrStderr())
			dbmsName := args[0]
			embedName := ""
			if len(args) == 2 {
				embedName = args[1]
			}

			if _, err := cfg.Credentials.Dbms.Get(dbmsName); err != nil {
				return clierr.NewUsageError("no dbms credential named %q (run `credential dbms list` to see available)", dbmsName)
			}

			if embedName != "" {
				if _, err := cfg.Credentials.Embed.Get(embedName); err != nil {
					return clierr.NewUsageError("no embed credential named %q (run `credential embed list` to see available)", embedName)
				}
			}

			return cfg.Credentials.Dbms.SetEmbed(dbmsName, embedName)
		},
	}
}
