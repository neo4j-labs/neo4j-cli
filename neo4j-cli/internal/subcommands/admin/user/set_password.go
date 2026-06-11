// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newSetPasswordCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var password string
	var passwordChangeRequired bool

	cmd := &cobra.Command{
		Use:         "set-password <username>",
		Short:       "Set the password for a Neo4j user",
		Annotations: map[string]string{"write": "true"},
		Long: "Set the password for an existing user in the system database. " +
			"If --password is not supplied, prompts on a TTY or returns a usage error on non-TTY. " +
			"--password-change-required (default false) controls whether the user must change their password on next login.",
		Example: `# Set a user's password interactively (password will be prompted)
neo4j-cli admin user set-password alice --credential local --rw

# Set a user's password with a flag and require change on next login
neo4j-cli admin user set-password bob --password newsecret --password-change-required --credential local --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]

			pw, err := promptPassword(cmd)
			if err != nil {
				return err
			}

			changeClause := "CHANGE NOT REQUIRED"
			if passwordChangeRequired {
				changeClause = "CHANGE REQUIRED"
			}

			cypher := fmt.Sprintf(
				"ALTER USER $name SET PASSWORD $password SET PASSWORD %s",
				changeClause,
			)
			params := map[string]any{
				"name":     name,
				"password": pw,
			}

			_, err = userExecFn(cmd.Context(), cfg, *conn, cypher, params)
			return err
		},
	}

	cmd.Flags().StringVar(&password, "password", "", "New password (prompted if not supplied on a TTY)")
	cmd.Flags().BoolVar(&passwordChangeRequired, "password-change-required", false, "Require the user to change their password on next login")

	return cmd
}
