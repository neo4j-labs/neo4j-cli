// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

func newCreateCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var passwordChangeRequired bool

	cmd := &cobra.Command{
		Use:         "create <name>",
		Short:       "Create a user",
		Annotations: map[string]string{"write": "true"},
		Long: "Create a user via CREATE USER $name SET PASSWORD $password SET PASSWORD CHANGE [NOT] REQUIRED " +
			"against the system database. " +
			"Pass --set-password to supply the password non-interactively; omit it on a TTY to be prompted. " +
			"Pass --password-change-required=false to allow the new user to log in without changing password.",
		Example: `# Create a user (prompted for password on a TTY)
neo4j-cli admin user create alice --credential local --rw

# Create a user with an explicit password and no change-on-login requirement
neo4j-cli admin user create alice --set-password s3cr3t --password-change-required=false --credential local --rw --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]

			password, err := promptUserPassword(cmd, "set-password")
			if err != nil {
				return err
			}

			var changeClause string
			if passwordChangeRequired {
				changeClause = "SET PASSWORD CHANGE REQUIRED"
			} else {
				changeClause = "SET PASSWORD CHANGE NOT REQUIRED"
			}

			cypher := fmt.Sprintf("CREATE USER $name SET PASSWORD $password %s", changeClause)
			if _, err := userExecFn(cmd.Context(), cfg, *conn, cypher, map[string]any{
				"name":     name,
				"password": password,
			}); err != nil {
				return err
			}

			return outputUser(cmd, cfg, *conn, name)
		},
	}

	cmd.Flags().String("set-password", "", "Password for the new user (prompted on TTY if omitted)")
	cmd.Flags().BoolVar(&passwordChangeRequired, "password-change-required", true, "Require the user to change password on first login")

	return cmd
}
