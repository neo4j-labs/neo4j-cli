// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

func newSetPasswordCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var passwordChangeRequired bool

	cmd := &cobra.Command{
		Use:         "set-password <name>",
		Short:       "Set the password for a user",
		Annotations: map[string]string{"write": "true"},
		Long: "Set the password for an existing user via ALTER USER $name SET PASSWORD against the " +
			"system database. Use --password-change-required to control whether the user must " +
			"change their password on next login (defaults to false).",
		Example: `# Set a password explicitly
neo4j-cli admin user set-password alice --new-password s3cr3t --credential local --rw

# Set a password and require the user to change it on next login
neo4j-cli admin user set-password alice --new-password s3cr3t --password-change-required --credential local --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]

			pw, err := promptUserPassword(cmd, "new-password")
			if err != nil {
				return err
			}

			cypher := "ALTER USER $name SET PASSWORD $password " + passwordChangeClause(passwordChangeRequired)
			if _, err := userExecFn(cmd.Context(), cfg, *conn, cypher, map[string]any{
				"name":     name,
				"password": pw,
			}); err != nil {
				return err
			}

			return outputUser(cmd, cfg, *conn, name)
		},
	}

	cmd.Flags().String("new-password", "", "New password for the user (prompted if absent in interactive mode)")
	cmd.Flags().BoolVar(&passwordChangeRequired, "password-change-required", false, "Require the user to change their password on next login")

	return cmd
}
