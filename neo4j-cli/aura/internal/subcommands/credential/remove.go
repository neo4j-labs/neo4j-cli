// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential

import (
	"errors"
	"fmt"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/spf13/cobra"
)

func NewRemoveCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "remove <name>",
		Short:       "Removes a credential",
		Annotations: map[string]string{"write": "true"},
		Long: `Removes a stored Aura API credential by name.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Remove a stored credential by name
neo4j-cli aura credential remove my-creds --rw --yes --force

# Remove a staging credential
neo4j-cli aura credential remove staging --rw --yes --force

# Remove and confirm by listing remaining credentials as JSON
neo4j-cli aura credential remove my-creds --rw --yes --force && neo4j-cli aura credential list --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			credName := strings.TrimSpace(args[0])
			if err := confirm.Require(cmd, credName); err != nil {
				if errors.Is(err, confirm.ErrCancelled) {
					fmt.Fprintln(cmd.ErrOrStderr(), "cancelled.") //nolint:errcheck // narration to stderr; write errors are not actionable
					return nil
				}
				return err
			}
			return cfg.Credentials.Aura.Remove(credName)
		},
	}

	confirm.Register(cmd)

	return cmd
}
