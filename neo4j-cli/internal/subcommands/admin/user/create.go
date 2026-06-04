// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

func newCreateCmd(cfg *clicfg.Config, credential *string) *cobra.Command {
	var password string
	var passwordChangeRequired bool
	var homeDatabase string

	cmd := &cobra.Command{
		Use:         "create <username>",
		Short:       "Create a Neo4j user",
		Annotations: map[string]string{"write": "true"},
		Long: "Create a new user in the system database. " +
			"If --password is not supplied, prompts on a TTY or returns a usage error on non-TTY. " +
			"--password-change-required (default true) controls whether the user must change their password on first login. " +
			"--home-database sets the user's default database (Enterprise edition only). " +
			"Uses the dbms credential named by --credential on the parent `admin` command.",
		Example: `# Create a user interactively (password will be prompted)
neo4j-cli admin user create alice --credential local --rw

# Create a user with a password and no change required
neo4j-cli admin user create bob --password secret --password-change-required=false --credential local --rw

# Create a user with a home database (Enterprise)
neo4j-cli admin user create carol --password secret --home-database mydb --credential local --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]

			pw, err := promptPassword(cmd)
			if err != nil {
				return err
			}

			cred, err := adminutil.ResolveCredential(cfg, credential)
			if err != nil {
				return err
			}

			changeClause := "CHANGE REQUIRED"
			if !passwordChangeRequired {
				changeClause = "CHANGE NOT REQUIRED"
			}

			cypher := fmt.Sprintf(
				"CREATE USER $name IF NOT EXISTS SET PASSWORD $password SET PASSWORD %s",
				changeClause,
			)
			params := map[string]any{
				"name":     name,
				"password": pw,
			}

			if homeDatabase != "" {
				cypher += " SET HOME DATABASE $homeDatabase"
				params["homeDatabase"] = homeDatabase
			}

			_, err = userExecFn(cmd.Context(), cfg, cred, cypher, params)
			return err
		},
	}

	cmd.Flags().StringVar(&password, "password", "", "Password for the new user (prompted if not supplied on a TTY)")
	cmd.Flags().BoolVar(&passwordChangeRequired, "password-change-required", true, "Require the user to change their password on first login")
	cmd.Flags().StringVar(&homeDatabase, "home-database", "", "Set the user's home database (Enterprise edition only)")

	return cmd
}
