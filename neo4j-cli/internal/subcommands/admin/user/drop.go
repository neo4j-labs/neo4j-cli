// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"errors"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
)

func newDropCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "drop <name>",
		Short:       "Drop a user",
		Annotations: map[string]string{"write": "true"},
		Long: "Drop a user via DROP USER <name> against the system database. " +
			"Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.",
		Example: `# Drop a user, prompting for confirmation on a TTY
neo4j-cli admin user drop alice --credential local --rw

# Drop a user without prompting (required for scripts and non-TTY callers)
neo4j-cli admin user drop alice --credential local --rw --yes --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			if err := confirm.Require(cmd, name); err != nil {
				return err
			}
			if _, err := userExecFn(cmd.Context(), cfg, *conn, "DROP USER $name", map[string]any{"name": name}); err != nil {
				var ne *neo4j.Neo4jError
				if errors.As(err, &ne) && ne.Code == "Neo.ClientError.Statement.ArgumentError" && strings.Contains(ne.Msg, "does not exist") {
					return clierr.NewNotFoundError("user %q not found", name)
				}
				return err
			}
			dropFields := []string{"user", "status"}
			row := map[string]any{"user": name, "status": "dropped"}
			commonoutput.PrintBodyMap(cmd, cfg, adminutil.NewRow(row, dropFields), dropFields)
			return nil
		},
	}

	confirm.Register(cmd)

	return cmd
}
