// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"errors"
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/spf13/cobra"
)

func newRemoveCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Removes a dbms credential",
		Long: `Remove a stored Bolt connection profile by name. Linked embed-credential references on other profiles are not modified.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Remove a dbms credential by name
neo4j-cli credential dbms remove local --rw --yes --force

# Remove a staging dbms credential
neo4j-cli credential dbms remove staging --rw --yes --force

# Remove a prod dbms credential
neo4j-cli credential dbms remove prod --rw --yes --force`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := confirm.Require(cmd, args[0]); err != nil {
				if errors.Is(err, confirm.ErrCancelled) {
					fmt.Fprintln(cmd.ErrOrStderr(), "cancelled.") //nolint:errcheck // narration to stderr; write errors are not actionable
					return nil
				}
				return err
			}
			return cfg.Credentials.Dbms.Remove(args[0])
		},
	}

	confirm.Register(cmd)

	return cmd
}
